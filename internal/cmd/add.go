package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/urfave/cli/v3"

	"github.com/geoffamey/wtg/internal/config"
	"github.com/geoffamey/wtg/internal/git"
	"github.com/geoffamey/wtg/internal/saga"
	"github.com/geoffamey/wtg/internal/state"
	"github.com/geoffamey/wtg/internal/ui"
)

// AddCommand returns the `wtg add` command.
func AddCommand(runner git.Runner) *cli.Command {
	return &cli.Command{
		Name:      "add",
		Usage:     "add repos to a workspace",
		ArgsUsage: "<workspace> <repo>...",
		Description: `Creates a new worktree for each specified repo inside an existing workspace,
checking out the workspace's branch. Updates go.work automatically if the
workspace already has one.

If the branch already exists in a repo (locally or on the remote) it is
checked out as-is — no reset or rebase is performed.

Paths in always.secrets are copied from each source repo into its new
worktree when present.`,
		ShellComplete: completeSpaceThenRepos,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() < 2 {
				return fmt.Errorf("usage: wtg add <workspace> <repo>")
			}
			cfg, err := config.Load(cmd.Root().String("config"))
			if err != nil {
				return err
			}
			return RunSpaceAdd(cfg, runner, SpaceAddArgs{
				Name:  cmd.Args().First(),
				Repos: cmd.Args().Tail(),
			}, os.Stdout)
		},
	}
}

// SpaceAddArgs holds the parsed arguments for RunSpaceAdd.
type SpaceAddArgs struct {
	Name  string
	Repos []string // short names of repos to add (required)
}

// RunSpaceAdd adds one or more repos to a workspace by creating worktrees on
// the space's branch and updating the go.work file and state. If a requested
// repo is currently a symlink in the space (added via always.repos), the
// symlink is replaced with a proper worktree on the space's branch.
func RunSpaceAdd(cfg *config.Config, runner git.Runner, args SpaceAddArgs, out io.Writer) error {
	if len(args.Repos) == 0 {
		return fmt.Errorf("no repos specified")
	}

	sp, err := state.Load(args.Name)
	if err != nil {
		return fmt.Errorf("load space %q: %w", args.Name, err)
	}

	// Partition requested repos into: upgrade from symlink, or add fresh.
	existingByName := make(map[string]state.RepoEntry, len(sp.Repos))
	for _, r := range sp.Repos {
		existingByName[r.Name] = r
	}
	var toUpgrade []state.RepoEntry // currently symlinks; will become worktrees
	var toAdd []string              // repos not yet in the space
	for _, name := range args.Repos {
		r, ok := existingByName[name]
		if !ok {
			toAdd = append(toAdd, name)
		} else if r.Symlink {
			toUpgrade = append(toUpgrade, r)
		} else {
			return fmt.Errorf("repo %q is already in space %q", name, args.Name)
		}
	}

	allPaths, err := discoverRepoPaths(cfg.Discovery.RootDir, cfg.Discovery.MaxDepth)
	if err != nil {
		return fmt.Errorf("scan %s: %w", cfg.Discovery.RootDir, err)
	}
	sort.Strings(allPaths)

	// Build repoTargets for brand-new repos (not upgrades).
	var newTargets []*repoTarget
	if len(toAdd) > 0 {
		newTargets, err = buildTargets(cfg.Discovery.RootDir, sp.Path, allPaths, toAdd)
		if err != nil {
			return err
		}
	}

	// Build repoTargets for symlink upgrades (same path, now a worktree).
	upgradeTargets := make([]*repoTarget, len(toUpgrade))
	for i, r := range toUpgrade {
		upgradeTargets[i] = &repoTarget{
			name:         r.Name,
			repoPath:     r.RepoPath,
			worktreePath: r.WorktreePath,
		}
	}

	allNew := append(upgradeTargets, newTargets...)
	if err := classifyBranchTargets(runner, allNew, sp.Branch); err != nil {
		return err
	}

	// Build the full target list for go.work: existing non-symlink repos +
	// upgraded repos (now worktrees) + brand-new repos.
	upgradedNames := make(map[string]bool, len(upgradeTargets))
	for _, t := range upgradeTargets {
		upgradedNames[t.name] = true
	}
	var keepFromState []*repoTarget
	for _, t := range targetsFromState(sp) {
		if !upgradedNames[t.name] {
			keepFromState = append(keepFromState, t)
		}
	}
	allTargets := append(append(keepFromState, upgradeTargets...), newTargets...)
	hasGoMod, anyGoMod := detectGoMods(allTargets)

	var steps []saga.Step
	// For each symlink being upgraded: remove the symlink, then add the worktree.
	for i := range upgradeTargets {
		t := upgradeTargets[i]
		steps = append(steps, saga.Step{
			Name: fmt.Sprintf("remove symlink %s", t.name),
			Do:   func(ctx context.Context) error { return os.Remove(t.worktreePath) },
			Undo: func(ctx context.Context) error { return os.Symlink(t.repoPath, t.worktreePath) },
		})
		steps = append(steps, worktreeStep(runner, t, sp.Branch, ""))
		sec, err := secretCopySteps(cfg, t)
		if err != nil {
			return err
		}
		steps = append(steps, sec...)
	}
	for i := range newTargets {
		steps = append(steps, worktreeStep(runner, newTargets[i], sp.Branch, ""))
		sec, err := secretCopySteps(cfg, newTargets[i])
		if err != nil {
			return err
		}
		steps = append(steps, sec...)
	}

	goWorkPath := filepath.Join(sp.Path, "go.work")
	if anyGoMod {
		goVersion := detectGoVersion(allTargets, hasGoMod)
		steps = append(steps, goWorkStep(goWorkPath, sp.Path, allTargets, hasGoMod, goVersion))
	}

	oldState := *sp
	steps = append(steps, saga.Step{
		Name: "save state",
		Do: func(ctx context.Context) error {
			updated := oldState
			updated.GoWorkspace = anyGoMod
			// Rebuild repo list: keep non-upgraded existing entries, then add
			// upgraded (now worktrees) and new repos.
			var repos []state.RepoEntry
			for _, r := range updated.Repos {
				if !upgradedNames[r.Name] {
					repos = append(repos, r)
				}
			}
			for _, t := range allNew {
				repos = append(repos, state.RepoEntry{
					Name:         t.name,
					RepoPath:     t.repoPath,
					WorktreePath: t.worktreePath,
				})
			}
			updated.Repos = repos
			return state.Save(&updated)
		},
		Undo: func(ctx context.Context) error {
			return state.Save(&oldState)
		},
	})

	if err := saga.Run(context.Background(), steps); err != nil {
		return err
	}

	fmt.Fprintf(out, "%s added to space %q\n", ui.SymOK, args.Name)
	tbl := ui.NewTableWriter(out)
	addedNames := make([]string, len(allNew))
	for i, t := range allNew {
		tbl.Row("  "+t.name, t.worktreePath)
		addedNames[i] = t.name
	}
	tbl.Flush()

	if sp, err := state.Load(args.Name); err == nil {
		runSpaceScript(cfg, "add", sp, addedNames, out)
	}
	return nil
}
