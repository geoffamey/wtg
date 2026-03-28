package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"

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
					&cli.StringFlag{
						Name:  "base",
						Usage: "commit-ish to branch from (default: HEAD)",
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
						Base:   c.String("base"),
						Repos:  c.Args().Tail(),
					}, os.Stdout)
				},
			},
			{
				Name:      "delete",
				Aliases:   []string{"rm"},
				Usage:     "remove a workspace's worktrees and optionally delete its branches",
				ArgsUsage: "<name>",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "d",
						Usage: "delete branch if fully merged into upstream",
					},
					&cli.BoolFlag{
						Name:  "D",
						Usage: "force-delete branch regardless of merge state",
					},
				},
				Action: func(c *cli.Context) error {
					if c.NArg() == 0 {
						return fmt.Errorf("missing required argument: <name>")
					}
					return RunSpaceDelete(runner, SpaceDeleteArgs{
						Name:         c.Args().First(),
						DeleteBranch: c.Bool("d"),
						ForceBranch:  c.Bool("D"),
					}, os.Stdin, os.Stdout)
				},
			},
			{
				Name:      "path",
				Usage:     "print the root path of a space",
				ArgsUsage: "<name>",
				Action: func(c *cli.Context) error {
					if c.NArg() == 0 {
						return fmt.Errorf("missing required argument: <name>")
					}
					sp, err := state.Load(c.Args().First())
					if err != nil {
						return err
					}
					fmt.Println(sp.Path)
					return nil
				},
			},
			{
				Name:      "exec",
				Usage:     "run a command in each worktree of a space",
				ArgsUsage: "<name> <cmd> [<args>...]",
				Action: func(c *cli.Context) error {
					if c.NArg() < 2 {
						return fmt.Errorf("usage: wtg space exec <name> <cmd> [<args>...]")
					}
					return RunSpaceExec(c.Args().First(), c.Args().Tail(), os.Stdout)
				},
			},
			{
				Name:      "push",
				Usage:     "push all branches in a space to origin",
				ArgsUsage: "<name>",
				Action: func(c *cli.Context) error {
					if c.NArg() == 0 {
						return fmt.Errorf("missing required argument: <name>")
					}
					return RunSpacePush(runner, c.Args().First(), os.Stdout)
				},
			},
			{
				Name:      "rebase",
				Usage:     "rebase all worktrees in a space onto origin's default branch",
				ArgsUsage: "<name>",
				Action: func(c *cli.Context) error {
					if c.NArg() == 0 {
						return fmt.Errorf("missing required argument: <name>")
					}
					return RunSpaceRebase(runner, c.Args().First(), os.Stdout)
				},
			},
			{
				Name:      "add",
				Usage:     "add repos to an existing workspace",
				ArgsUsage: "<name> <repo>...",
				Action: func(c *cli.Context) error {
					if c.NArg() < 2 {
						return fmt.Errorf("usage: wtg space add <name> <repo>")
					}
					cfg, err := config.Load(c.String("config"))
					if err != nil {
						return err
					}
					return RunSpaceAdd(cfg, runner, SpaceAddArgs{
						Name:  c.Args().First(),
						Repos: c.Args().Tail(),
					}, os.Stdout)
				},
			},
		},
	}
}

// =============================================================================
// space list
// =============================================================================

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
		names := make([]string, len(sp.Repos))
		for i, r := range sp.Repos {
			names[i] = r.Name
		}
		tbl.Row(sp.Name, sp.Branch, sp.Path, fmt.Sprintf("%d repos", len(sp.Repos)), strings.Join(names, ", "))
	}
	tbl.Flush()
	return nil
}

// =============================================================================
// space exec
// =============================================================================

// RunSpaceExec runs a command in each worktree of the named space sequentially,
// streaming output as it goes. A header line identifies each repo. Execution
// continues even if a command fails; failed repos are reported at the end.
func RunSpaceExec(spaceName string, args []string, out io.Writer) error {
	sp, err := state.Load(spaceName)
	if err != nil {
		return fmt.Errorf("load space %q: %w", spaceName, err)
	}

	var failed []string
	for _, r := range sp.Repos {
		fmt.Fprintf(out, "=== %s ===\n", r.Name)
		cmd := exec.Command(args[0], args[1:]...) //nolint:gosec
		cmd.Dir = r.WorktreePath
		cmd.Stdout = out
		cmd.Stderr = out
		if err := cmd.Run(); err != nil {
			failed = append(failed, r.Name)
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("command failed in: %s", strings.Join(failed, ", "))
	}
	return nil
}

// =============================================================================
// space push
// =============================================================================

// RunSpacePush pushes each repo's branch to origin in parallel.
func RunSpacePush(runner git.Runner, spaceName string, out io.Writer) error {
	sp, err := state.Load(spaceName)
	if err != nil {
		return fmt.Errorf("load space %q: %w", spaceName, err)
	}

	type pushResult struct {
		name string
		sym  string
		msg  string
	}
	results := make([]pushResult, len(sp.Repos))

	var g errgroup.Group
	for i, r := range sp.Repos {
		g.Go(func() error {
			if err := runner.Push(r.WorktreePath, sp.Branch); err != nil {
				results[i] = pushResult{r.Name, ui.SymFail, err.Error()}
			} else {
				results[i] = pushResult{r.Name, ui.SymOK, "pushed " + sp.Branch}
			}
			return nil
		})
	}
	_ = g.Wait()

	tbl := ui.NewTableWriter(out)
	for _, r := range results {
		tbl.Row(r.name, r.sym+" "+r.msg)
	}
	tbl.Flush()
	return nil
}

// =============================================================================
// space rebase
// =============================================================================

// RunSpaceRebase fetches origin and rebases each worktree in the named space
// onto origin's default branch, in parallel. Repos where rebase fails are
// reported individually; the command itself returns nil so the caller can see
// the full picture before acting.
func RunSpaceRebase(runner git.Runner, spaceName string, out io.Writer) error {
	sp, err := state.Load(spaceName)
	if err != nil {
		return fmt.Errorf("load space %q: %w", spaceName, err)
	}

	type rebaseResult struct {
		name string
		sym  string
		msg  string
	}
	results := make([]rebaseResult, len(sp.Repos))

	var g errgroup.Group
	for i, r := range sp.Repos {
		g.Go(func() error {
			defaultBranch, err := runner.DefaultBranch(r.RepoPath)
			if err != nil {
				results[i] = rebaseResult{r.Name, ui.SymFail, fmt.Sprintf("default branch: %v", err)}
				return nil
			}
			onto := "origin/" + defaultBranch
			if err := runner.Rebase(r.WorktreePath, onto); err != nil {
				results[i] = rebaseResult{r.Name, ui.SymFail, err.Error()}
			} else {
				results[i] = rebaseResult{r.Name, ui.SymOK, "rebased onto " + onto}
			}
			return nil
		})
	}
	_ = g.Wait()

	tbl := ui.NewTableWriter(out)
	for _, r := range results {
		tbl.Row(r.name, r.sym+" "+r.msg)
	}
	tbl.Flush()
	return nil
}

// =============================================================================
// space create
// =============================================================================

// SpaceCreateArgs holds the parsed arguments for RunSpaceCreate.
type SpaceCreateArgs struct {
	Name   string
	Branch string   // overrides cfg.Git.BranchPrefix+Name when set
	Path   string   // overrides cfg.Spaces.RootDir/Name when set
	Base   string   // commit-ish to branch from (default: HEAD)
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

	if err := checkBranchConflicts(runner, targets, branch); err != nil {
		return err
	}

	hasGoMod, anyGoMod := detectGoMods(targets)

	var steps []saga.Step
	for i := range targets {
		steps = append(steps, worktreeStep(runner, targets[i], branch, args.Base))
	}
	goWorkPath := filepath.Join(spacePath, "go.work")
	if anyGoMod {
		steps = append(steps, goWorkStep(goWorkPath, spacePath, targets, hasGoMod))
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

// =============================================================================
// space add
// =============================================================================

// SpaceAddArgs holds the parsed arguments for RunSpaceAdd.
type SpaceAddArgs struct {
	Name  string
	Repos []string // short names of repos to add (required)
}

// RunSpaceAdd adds one or more repos to an existing workspace by creating
// worktrees on the space's branch and updating the go.work file and state.
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

	if err := checkBranchConflicts(runner, newTargets, sp.Branch); err != nil {
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
		steps = append(steps, goWorkStep(goWorkPath, sp.Path, allTargets, hasGoMod))
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

// =============================================================================
// space delete
// =============================================================================

// SpaceDeleteArgs holds the parsed arguments for RunSpaceDelete.
type SpaceDeleteArgs struct {
	Name         string
	DeleteBranch bool // -d: delete branch if fully merged
	ForceBranch  bool // -D: force-delete branch regardless of merge state
}

// RunSpaceDelete removes all worktrees belonging to a space. If -d or -D is
// set, branches are also deleted. Prompts the user when uncommitted changes or
// unpushed commits are detected; only deletes state once all removals succeed.
func RunSpaceDelete(runner git.Runner, args SpaceDeleteArgs, in io.Reader, out io.Writer) error {
	sp, err := state.Load(args.Name)
	if err != nil {
		return fmt.Errorf("load space %q: %w", args.Name, err)
	}

	// Pre-flight: gather warnings about uncommitted or unpushed work.
	var warnings []string
	for _, r := range sp.Repos {
		st, err := runner.Status(r.WorktreePath)
		if err != nil {
			continue // worktree may have been externally deleted; skip
		}
		if len(st.Files) > 0 {
			warnings = append(warnings, fmt.Sprintf("%s: has uncommitted changes", r.Name))
		}
		if st.Ahead > 0 {
			warnings = append(warnings, fmt.Sprintf("%s: has %d unpushed commit(s)", r.Name, st.Ahead))
		}
	}

	forceRemove := len(warnings) > 0
	if forceRemove {
		for _, w := range warnings {
			fmt.Fprintf(out, "  %s %s\n", ui.SymWarn, w)
		}
		ok, err := confirm(bufio.NewReader(in), out, "Delete anyway?")
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
	}

	// SymFail means the worktree could not be removed — state must be preserved
	// because the worktree still exists. SymWarn means the worktree was removed
	// but branch deletion failed — state cleanup still proceeds since the
	// worktree is gone and the user can delete the branch manually.
	hadError := false
	tbl := ui.NewTableWriter(out)
	for _, r := range sp.Repos {
		sym, msg := deleteOne(runner, r, sp.Branch, args.DeleteBranch, args.ForceBranch, forceRemove)
		tbl.Row(r.Name, sym+" "+msg)
		if sym == ui.SymFail {
			hadError = true
		}
	}
	tbl.Flush()

	if hadError {
		return fmt.Errorf("some worktrees could not be removed; space %q not deleted from state", args.Name)
	}
	return state.Delete(args.Name)
}

// deleteOne removes the worktree for one repo and optionally deletes its branch.
// force causes WorktreeRemove to bypass git's dirty-check (used when the user
// has already confirmed they want to proceed despite uncommitted changes).
func deleteOne(runner git.Runner, r state.RepoEntry, branch string, deleteBranch, forceBranch, force bool) (sym, msg string) {
	if err := runner.WorktreeRemove(r.RepoPath, r.WorktreePath, force); err != nil {
		return ui.SymFail, fmt.Sprintf("remove worktree: %v", err)
	}
	if !deleteBranch && !forceBranch {
		return ui.SymOK, "worktree removed"
	}
	if err := runner.BranchDelete(r.RepoPath, branch, forceBranch); err != nil {
		return ui.SymWarn, fmt.Sprintf("worktree removed, branch not deleted: %v", err)
	}
	return ui.SymOK, "worktree removed, branch deleted"
}

// =============================================================================
// shared helpers
// =============================================================================

// repoTarget holds resolved paths for one repo's participation in a space.
type repoTarget struct {
	name         string // short name relative to discovery.root_dir
	repoPath     string // absolute path to the main clone
	worktreePath string // absolute path for the new worktree
	createBranch bool   // whether to create a new branch (set during pre-flight)
}

// targetsFromState converts existing state repo entries back into repoTargets.
func targetsFromState(sp *state.Space) []*repoTarget {
	targets := make([]*repoTarget, len(sp.Repos))
	for i, r := range sp.Repos {
		targets[i] = &repoTarget{
			name:         r.Name,
			repoPath:     r.RepoPath,
			worktreePath: r.WorktreePath,
		}
	}
	return targets
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

// checkBranchConflicts runs pre-flight branch checks for each target, setting
// createBranch on each target based on whether the branch already exists.
func checkBranchConflicts(runner git.Runner, targets []*repoTarget, branch string) error {
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
	return nil
}

// detectGoMods reports which targets have a go.mod in their main clone.
func detectGoMods(targets []*repoTarget) (hasGoMod []bool, anyGoMod bool) {
	hasGoMod = make([]bool, len(targets))
	for i, t := range targets {
		if _, err := os.Stat(filepath.Join(t.repoPath, "go.mod")); err == nil {
			hasGoMod[i] = true
			anyGoMod = true
		}
	}
	return
}

// worktreeStep returns a saga.Step that creates a linked worktree and undoes it
// on rollback (also deleting the branch if it was newly created).
func worktreeStep(runner git.Runner, t *repoTarget, branch, base string) saga.Step {
	return saga.Step{
		Name: fmt.Sprintf("create worktree %s", t.name),
		Do: func(ctx context.Context) error {
			if err := os.MkdirAll(filepath.Dir(t.worktreePath), 0o755); err != nil {
				return fmt.Errorf("create parent dir: %w", err)
			}
			return runner.WorktreeAdd(t.repoPath, t.worktreePath, branch, base, t.createBranch)
		},
		Undo: func(ctx context.Context) error {
			if err := runner.WorktreeRemove(t.repoPath, t.worktreePath, true); err != nil {
				return err
			}
			if t.createBranch {
				return runner.BranchDelete(t.repoPath, branch, true)
			}
			return nil
		},
	}
}

// goWorkStep returns a saga.Step that writes a go.work and restores the prior
// content (or removes the file) on rollback.
func goWorkStep(goWorkPath, spacePath string, targets []*repoTarget, hasGoMod []bool) saga.Step {
	// Capture existing content before the saga runs so undo can restore it.
	oldContent, _ := os.ReadFile(goWorkPath)
	return saga.Step{
		Name: "write go.work",
		Do: func(ctx context.Context) error {
			return writeGoWork(goWorkPath, spacePath, targets, hasGoMod)
		},
		Undo: func(ctx context.Context) error {
			if oldContent != nil {
				return os.WriteFile(goWorkPath, oldContent, 0o644)
			}
			if err := os.Remove(goWorkPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			return nil
		},
	}
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
