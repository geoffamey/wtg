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
		Name:          "add",
		Usage:         "add repos to a workspace",
		ArgsUsage:     "<workspace> <repo>...",
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
// the space's branch and updating the go.work file and state.
func RunSpaceAdd(cfg *config.Config, runner git.Runner, args SpaceAddArgs, out io.Writer) error {
	if cfg.Discovery.RootDir == "" {
		return fmt.Errorf("discovery.root_dir is not set; run `wtg init` to configure")
	}
	if len(args.Repos) == 0 {
		return fmt.Errorf("no repos specified")
	}

	sp, err := state.Load(args.Name)
	if err != nil {
		return fmt.Errorf("load space %q: %w", args.Name, err)
	}

	// Reject repos already in the space.
	existing := make(map[string]bool, len(sp.Repos))
	for _, r := range sp.Repos {
		existing[r.Name] = true
	}
	for _, name := range args.Repos {
		if existing[name] {
			return fmt.Errorf("repo %q is already in space %q", name, args.Name)
		}
	}

	allPaths, err := discoverRepoPaths(cfg.Discovery.RootDir, cfg.Discovery.MaxDepth)
	if err != nil {
		return fmt.Errorf("scan %s: %w", cfg.Discovery.RootDir, err)
	}
	sort.Strings(allPaths)

	newTargets, err := buildTargets(cfg.Discovery.RootDir, sp.Path, allPaths, args.Repos)
	if err != nil {
		return err
	}

	if err := classifyBranchTargets(runner, newTargets, sp.Branch); err != nil {
		return err
	}

	// Build the full target list (existing + new) for go.work.
	allTargets := append(targetsFromState(sp), newTargets...)
	hasGoMod, anyGoMod := detectGoMods(allTargets)

	var steps []saga.Step
	for i := range newTargets {
		steps = append(steps, worktreeStep(runner, newTargets[i], sp.Branch, ""))
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
			for _, t := range newTargets {
				updated.Repos = append(updated.Repos, state.RepoEntry{
					Name:         t.name,
					RepoPath:     t.repoPath,
					WorktreePath: t.worktreePath,
				})
			}
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
	for _, t := range newTargets {
		tbl.Row("  "+t.name, t.worktreePath)
	}
	tbl.Flush()
	return nil
}
