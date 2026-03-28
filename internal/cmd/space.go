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
	"strings"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/geoffamey/wtg/internal/config"
	"github.com/geoffamey/wtg/internal/git"
	"github.com/geoffamey/wtg/internal/saga"
	"github.com/geoffamey/wtg/internal/state"
	"github.com/geoffamey/wtg/internal/ui"
)

// SpaceCommand returns the `wtg space` command with its subcommands.
func SpaceCommand(runner git.Runner) *cli.Command {
	return &cli.Command{
		Name:  "space",
		Usage: "manage workspaces",
		Subcommands: []*cli.Command{
			{
				Name:    "list",
				Aliases: []string{"ls"},
				Usage:   "list all spaces",
				Action: func(c *cli.Context) error {
					return RunSpaceList(os.Stdout)
				},
			},
			{
				Name:      "create",
				Usage:     "create a new workspace",
				ArgsUsage: "<name> [<repo>...]",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "branch",
						Usage: "branch name (default: <git.branch_prefix><name>)",
					},
					&cli.StringFlag{
						Name:  "path",
						Usage: "workspace root path (default: <spaces.root_dir>/<name>)",
					},
				},
				Action: func(c *cli.Context) error {
					if c.NArg() == 0 {
						return fmt.Errorf("missing required argument: <name>")
					}
					cfg, err := config.Load(c.String("config"))
					if err != nil {
						return err
					}
					return RunSpaceCreate(cfg, runner, SpaceCreateArgs{
						Name:   c.Args().First(),
						Branch: c.String("branch"),
						Path:   c.String("path"),
						Repos:  c.Args().Tail(),
					}, os.Stdout)
				},
			},
		},
	}
}

// SpaceCreateArgs holds the parsed arguments for RunSpaceCreate.
type SpaceCreateArgs struct {
	Name   string
	Branch string   // overrides cfg.Git.BranchPrefix+Name when set
	Path   string   // overrides cfg.Spaces.RootDir/Name when set
	Repos  []string // short names; empty = all discovered repos
}

// RunSpaceCreate creates a new workspace, checking out a worktree in each
// selected repo and writing a go.work file if any repos are Go modules.
func RunSpaceCreate(cfg *config.Config, runner git.Runner, args SpaceCreateArgs, out io.Writer) error {
	if cfg.Spaces.RootDir == "" && args.Path == "" {
		return fmt.Errorf("spaces.root_dir is not set; run `wtg init` or specify --path")
	}
	if cfg.Discovery.RootDir == "" {
		return fmt.Errorf("discovery.root_dir is not set; run `wtg init` to configure")
	}

	// Resolve branch and space root path.
	branch := args.Branch
	if branch == "" {
		branch = cfg.Git.BranchPrefix + args.Name
	}
	spacePath := args.Path
	if spacePath == "" {
		spacePath = filepath.Join(cfg.Spaces.RootDir, args.Name)
	}

	// Space must not already exist.
	if _, err := state.Load(args.Name); err == nil {
		return fmt.Errorf("space %q already exists", args.Name)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("check state for %q: %w", args.Name, err)
	}

	// Discover repos and resolve targets.
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

	// Pre-flight: check branch conflicts per repo.
	for i, t := range targets {
		exists, err := runner.BranchExists(t.repoPath, branch)
		if err != nil {
			return fmt.Errorf("check branch in %s: %w", t.name, err)
		}
		if exists {
			wts, err := runner.WorktreeList(t.repoPath)
			if err != nil {
				return fmt.Errorf("list worktrees in %s: %w", t.name, err)
			}
			for _, wt := range wts {
				if wt.Branch == branch {
					return fmt.Errorf("branch %q is already checked out in %s (worktree: %s)",
						branch, t.name, wt.Path)
				}
			}
			targets[i].createBranch = false
		} else {
			targets[i].createBranch = true
		}
	}

	// Determine which repos have a go.mod (checked pre-saga against the main clone).
	hasGoMod := make([]bool, len(targets))
	anyGoMod := false
	for i, t := range targets {
		if _, err := os.Stat(filepath.Join(t.repoPath, "go.mod")); err == nil {
			hasGoMod[i] = true
			anyGoMod = true
		}
	}

	// Build saga steps.
	var steps []saga.Step

	for i := range targets {
		t := targets[i]
		steps = append(steps, saga.Step{
			Name: fmt.Sprintf("create worktree %s", t.name),
			Do: func(ctx context.Context) error {
				if err := os.MkdirAll(filepath.Dir(t.worktreePath), 0o755); err != nil {
					return fmt.Errorf("create parent dir: %w", err)
				}
				return runner.WorktreeAdd(t.repoPath, t.worktreePath, branch, t.createBranch)
			},
			Undo: func(ctx context.Context) error {
				if err := runner.WorktreeRemove(t.repoPath, t.worktreePath, true); err != nil {
					return err
				}
				// Only delete the branch if we created it; leave pre-existing branches alone.
				if t.createBranch {
					return runner.BranchDelete(t.repoPath, branch, true)
				}
				return nil
			},
		})
	}

	if anyGoMod {
		goWorkPath := filepath.Join(spacePath, "go.work")
		steps = append(steps, saga.Step{
			Name: "write go.work",
			Do: func(ctx context.Context) error {
				return writeGoWork(goWorkPath, spacePath, targets, hasGoMod)
			},
			Undo: func(ctx context.Context) error {
				if err := os.Remove(goWorkPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
					return err
				}
				return nil
			},
		})
	}

	savedState := false
	steps = append(steps, saga.Step{
		Name: "save state",
		Do: func(ctx context.Context) error {
			sp := buildSpaceState(args.Name, spacePath, branch, anyGoMod, targets)
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
	})

	if err := saga.Run(context.Background(), steps); err != nil {
		return err
	}

	fmt.Fprintf(out, "%s created space %q on branch %s\n", ui.SymOK, args.Name, branch)
	tbl := ui.NewTableWriter(out)
	for _, t := range targets {
		tbl.Row("  "+t.name, t.worktreePath)
	}
	tbl.Flush()
	return nil
}

// repoTarget holds resolved paths for one repo's participation in a space.
type repoTarget struct {
	name         string // short name relative to discovery.root_dir
	repoPath     string // absolute path to the main clone
	worktreePath string // absolute path for the new worktree
	createBranch bool   // whether to create a new branch (set during pre-flight)
}

// buildTargets resolves the set of repos to include in a space.
func buildTargets(rootDir, spacePath string, allPaths, names []string) ([]*repoTarget, error) {
	if len(names) == 0 {
		targets := make([]*repoTarget, 0, len(allPaths))
		for _, p := range allPaths {
			name, _ := filepath.Rel(rootDir, p)
			name = filepath.ToSlash(name)
			targets = append(targets, &repoTarget{
				name:         name,
				repoPath:     p,
				worktreePath: filepath.Join(spacePath, filepath.FromSlash(name)),
			})
		}
		return targets, nil
	}

	byName := make(map[string]string, len(allPaths))
	for _, p := range allPaths {
		name, _ := filepath.Rel(rootDir, p)
		byName[filepath.ToSlash(name)] = p
	}
	targets := make([]*repoTarget, 0, len(names))
	for _, name := range names {
		p, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("repo %q not found under %s", name, rootDir)
		}
		targets = append(targets, &repoTarget{
			name:         name,
			repoPath:     p,
			worktreePath: filepath.Join(spacePath, filepath.FromSlash(name)),
		})
	}
	return targets, nil
}

// writeGoWork writes a go.work file at goWorkPath with a use directive for
// each repo that has a go.mod. hasGoMod[i] corresponds to targets[i].
func writeGoWork(goWorkPath, spacePath string, targets []*repoTarget, hasGoMod []bool) error {
	var usePaths []string
	for i, t := range targets {
		if !hasGoMod[i] {
			continue
		}
		rel, err := filepath.Rel(spacePath, t.worktreePath)
		if err != nil {
			return fmt.Errorf("compute relative path for %s: %w", t.name, err)
		}
		usePaths = append(usePaths, "./"+filepath.ToSlash(rel))
	}
	if len(usePaths) == 0 {
		return nil
	}

	var b strings.Builder
	b.WriteString("go 1.24\n\nuse (\n")
	for _, p := range usePaths {
		b.WriteString("\t" + p + "\n")
	}
	b.WriteString(")\n")

	return os.WriteFile(goWorkPath, []byte(b.String()), 0o644)
}

// RunSpaceList prints a table of all spaces sorted by name. Each row shows the
// space name, branch, workspace path, and number of repos.
func RunSpaceList(out io.Writer) error {
	spaces, err := state.List()
	if err != nil {
		return fmt.Errorf("list spaces: %w", err)
	}
	sort.Slice(spaces, func(i, j int) bool { return spaces[i].Name < spaces[j].Name })
	tbl := ui.NewTableWriter(out)
	for _, sp := range spaces {
		repos := fmt.Sprintf("%d repos", len(sp.Repos))
		tbl.Row(sp.Name, sp.Branch, sp.Path, repos)
	}
	tbl.Flush()
	return nil
}

// buildSpaceState constructs the state.Space value to persist.
func buildSpaceState(name, spacePath, branch string, goWorkspace bool, targets []*repoTarget) *state.Space {
	sp := &state.Space{
		Name:        name,
		Path:        spacePath,
		Branch:      branch,
		CreatedAt:   time.Now(),
		GoWorkspace: goWorkspace,
	}
	for _, t := range targets {
		sp.Repos = append(sp.Repos, state.RepoEntry{
			Name:         t.name,
			RepoPath:     t.repoPath,
			WorktreePath: t.worktreePath,
		})
	}
	return sp
}
