package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
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

// NewCommand returns the `wtg new` command.
func NewCommand(runner git.Runner) *cli.Command {
	return &cli.Command{
		Name:      "new",
		Usage:     "create a workspace",
		ArgsUsage: "<workspace> [<repo>...]",
		Description: `Creates a workspace: a directory containing one linked git worktree per
repo, all on the same branch. A go.work file is written automatically when
any of the included repos have a go.mod.

At least one repo must be specified. If the branch already exists in a repo
it is checked out as-is — no reset or rebase is performed.`,
		ShellComplete: completeReposAfterFirst,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "branch",
				Usage: "branch name (default: <git.branch_prefix><workspace>)",
			},
			&cli.StringFlag{
				Name:  "path",
				Usage: "workspace root path (default: <spaces.root_dir>/<workspace>)",
			},
			&cli.StringFlag{
				Name:  "base",
				Usage: "commit-ish to branch from (default: HEAD)",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() == 0 {
				return fmt.Errorf("missing required argument: <workspace>")
			}
			if cmd.Args().Len() < 2 {
				return fmt.Errorf("at least one repo is required; use 'wtg repo status' to see available repos")
			}
			cfg, err := config.Load(cmd.Root().String("config"))
			if err != nil {
				return err
			}
			return RunSpaceNew(cfg, runner, SpaceNewArgs{
				Name:   cmd.Args().First(),
				Branch: cmd.String("branch"),
				Path:   cmd.String("path"),
				Base:   cmd.String("base"),
				Repos:  cmd.Args().Tail(),
			}, os.Stdout)
		},
	}
}

// SpaceNewArgs holds the parsed arguments for RunSpaceNew.
type SpaceNewArgs struct {
	Name   string
	Branch string   // overrides cfg.Git.BranchPrefix+Name when set
	Path   string   // overrides cfg.Spaces.RootDir/Name when set
	Base   string   // commit-ish to branch from (default: HEAD)
	Repos  []string // short names; at least one required
}

// RunSpaceNew creates a new workspace, checking out a worktree in each
// selected repo and writing a go.work file if any repos are Go modules.
// Repos listed in cfg.Always.Repos are symlinked into the space unless they
// are also explicitly named in args.Repos (in which case they get a worktree).
// Files listed in cfg.Always.Files are copied into the space root.
func RunSpaceNew(cfg *config.Config, runner git.Runner, args SpaceNewArgs, out io.Writer) error {
	if cfg.Spaces.RootDir == "" && args.Path == "" {
		return fmt.Errorf("spaces.root_dir is not set; run `wtg init` or specify --path")
	}
	if cfg.Discovery.RootDir == "" {
		return fmt.Errorf("discovery.root_dir is not set; run `wtg init` to configure")
	}

	branch := args.Branch
	if branch == "" {
		branch = cfg.Git.BranchPrefix + args.Name
	}
	spacePath := args.Path
	if spacePath == "" {
		spacePath = filepath.Join(cfg.Spaces.RootDir, args.Name)
	}

	if _, err := state.Load(args.Name); err == nil {
		return fmt.Errorf("space %q already exists", args.Name)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("check state for %q: %w", args.Name, err)
	}

	allPaths, err := discoverRepoPaths(cfg.Discovery.RootDir, cfg.Discovery.MaxDepth)
	if err != nil {
		return fmt.Errorf("scan %s: %w", cfg.Discovery.RootDir, err)
	}
	sort.Strings(allPaths)

	targets, err := buildTargets(cfg.Discovery.RootDir, spacePath, allPaths, args.Repos)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return fmt.Errorf("no repos found")
	}

	if err := classifyBranchTargets(runner, targets, branch); err != nil {
		return err
	}

	symlinkTargets, err := resolveAlwaysRepos(cfg, spacePath, allPaths, args.Repos)
	if err != nil {
		return err
	}

	allTargets := append(targets, symlinkTargets...)
	hasGoMod, anyGoMod := detectGoMods(allTargets)

	savedState := false
	steps := []saga.Step{{
		Name: "save state",
		Do: func(ctx context.Context) error {
			sp := buildSpaceState(args.Name, spacePath, branch, anyGoMod, allTargets)
			if err := state.Save(sp); err != nil {
				return err
			}
			savedState = true
			return nil
		},
		Undo: func(ctx context.Context) error {
			if savedState {
				return state.Delete(args.Name)
			}
			return nil
		},
	}}
	for i := range targets {
		steps = append(steps, worktreeStep(runner, targets[i], branch, args.Base))
	}
	for i := range symlinkTargets {
		steps = append(steps, symlinkStep(symlinkTargets[i]))
	}
	for _, f := range cfg.Always.Files {
		steps = append(steps, copyFileStep(f, spacePath))
	}
	goWorkPath := filepath.Join(spacePath, "go.work")
	if anyGoMod {
		goVersion := detectGoVersion(allTargets, hasGoMod)
		steps = append(steps, goWorkStep(goWorkPath, spacePath, allTargets, hasGoMod, goVersion))
	}

	if err := saga.Run(context.Background(), steps); err != nil {
		return err
	}

	fmt.Fprintf(out, "%s created space %q on branch %s\n", ui.SymOK, args.Name, branch)
	tbl := ui.NewTableWriter(out)
	for _, t := range targets {
		tbl.Row("  "+t.name, t.worktreePath)
	}
	for _, t := range symlinkTargets {
		tbl.Row("  "+t.name, ui.Muted.Render(ui.SymLink+" "+t.repoPath))
	}
	tbl.Flush()
	return nil
}

// resolveAlwaysRepos builds symlink targets for cfg.Always.Repos, skipping any
// repo that appears in explicitRepos (those will get a proper worktree instead).
// Returns an error if any always-repo name is not found under the discovery root.
func resolveAlwaysRepos(cfg *config.Config, spacePath string, allPaths, explicitRepos []string) ([]*repoTarget, error) {
	if len(cfg.Always.Repos) == 0 {
		return nil, nil
	}

	explicitSet := make(map[string]bool, len(explicitRepos))
	for _, r := range explicitRepos {
		explicitSet[r] = true
	}

	byName := make(map[string]string, len(allPaths))
	for _, p := range allPaths {
		name, _ := filepath.Rel(cfg.Discovery.RootDir, p)
		byName[filepath.ToSlash(name)] = p
	}

	var out []*repoTarget
	for _, name := range cfg.Always.Repos {
		if explicitSet[name] {
			continue
		}
		p, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("always.repos entry %q not found under %s", name, cfg.Discovery.RootDir)
		}
		out = append(out, &repoTarget{
			name:         name,
			repoPath:     p,
			worktreePath: filepath.Join(spacePath, filepath.FromSlash(name)),
			symlink:      true,
		})
	}
	return out, nil
}
