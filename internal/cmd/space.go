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
	"strconv"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
	"golang.org/x/mod/modfile"
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
		Commands: []*cli.Command{
			{
				Name:    "list",
				Aliases: []string{"ls"},
				Usage:   "list all spaces",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return RunSpaceList(os.Stdout)
				},
			},
			{
				Name:          "create",
				Usage:         "create a new workspace",
				ArgsUsage:     "<name> [<repo>...]",
				ShellComplete: completeRepos,
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
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() == 0 {
						return fmt.Errorf("missing required argument: <name>")
					}
					cfg, err := config.Load(cmd.Root().String("config"))
					if err != nil {
						return err
					}
					return RunSpaceCreate(cfg, runner, SpaceCreateArgs{
						Name:   cmd.Args().First(),
						Branch: cmd.String("branch"),
						Path:   cmd.String("path"),
						Base:   cmd.String("base"),
						Repos:  cmd.Args().Tail(),
					}, os.Stdout)
				},
			},
			{
				Name:          "delete",
				Aliases:       []string{"rm"},
				Usage:         "remove a workspace's worktrees and optionally delete its branches",
				ArgsUsage:     "<name>",
				ShellComplete: completeSpaces,
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
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() == 0 {
						return fmt.Errorf("missing required argument: <name>")
					}
					return RunSpaceDelete(runner, SpaceDeleteArgs{
						Name:         cmd.Args().First(),
						DeleteBranch: cmd.Bool("d"),
						ForceBranch:  cmd.Bool("D"),
					}, os.Stdin, os.Stdout)
				},
			},
			{
				Name:          "path",
				Usage:         "print the root path of a space",
				ArgsUsage:     "<name>",
				ShellComplete: completeSpaces,
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() == 0 {
						return fmt.Errorf("missing required argument: <name>")
					}
					sp, err := state.Load(cmd.Args().First())
					if err != nil {
						return err
					}
					fmt.Println(sp.Path)
					return nil
				},
			},
			{
				Name:          "exec",
				Usage:         "run a command in each worktree of a space",
				ArgsUsage:     "<name> <cmd> [<args>...]",
				ShellComplete: completeSpaces,
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() < 2 {
						return fmt.Errorf("usage: wtg space exec <name> <cmd> [<args>...]")
					}
					return RunSpaceExec(cmd.Args().First(), cmd.Args().Tail(), os.Stdout)
				},
			},
			{
				Name:          "push",
				Usage:         "push all branches in a space to origin",
				ArgsUsage:     "<name>",
				ShellComplete: completeSpaces,
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() == 0 {
						return fmt.Errorf("missing required argument: <name>")
					}
					return RunSpacePush(runner, cmd.Args().First(), os.Stdout)
				},
			},
			{
				Name:          "rebase",
				Usage:         "rebase all worktrees in a space onto origin's default branch",
				ArgsUsage:     "<name>",
				ShellComplete: completeSpaces,
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() == 0 {
						return fmt.Errorf("missing required argument: <name>")
					}
					return RunSpaceRebase(runner, cmd.Args().First(), os.Stdout)
				},
			},
			{
				Name:          "status",
				Aliases:       []string{"st"},
				Usage:         "show status of workspaces (alias for wtg status)",
				ArgsUsage:     "[<space>]",
				ShellComplete: completeSpaces,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "detailed",
						Usage: "show individual modified files per repo",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return RunStatus(runner, cmd.Args().Slice(), cmd.Bool("detailed"), os.Stdout)
				},
			},
			{
				Name:          "add",
				Usage:         "add repos to an existing workspace",
				ArgsUsage:     "<name> <repo>...",
				ShellComplete: completeRepos,
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() < 2 {
						return fmt.Errorf("usage: wtg space add <name> <repo>")
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

	results := make([]opResult, len(sp.Repos))

	var g errgroup.Group
	for i, r := range sp.Repos {
		g.Go(func() error {
			if err := runner.Push(r.WorktreePath, sp.Branch); err != nil {
				results[i] = opResult{r.Name, ui.SymFail, err.Error()}
			} else {
				results[i] = opResult{r.Name, ui.SymOK, "pushed " + sp.Branch}
			}
			return nil
		})
	}
	_ = g.Wait() // goroutines always return nil; outcomes are written to results[i]

	tbl := ui.NewTableWriter(out)
	var failed []string
	for _, r := range results {
		tbl.Row(r.name, r.sym+" "+r.msg)
		if r.sym == ui.SymFail {
			failed = append(failed, r.name)
		}
	}
	tbl.Flush()
	if len(failed) > 0 {
		return fmt.Errorf("push failed in: %s", strings.Join(failed, ", "))
	}
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

	results := make([]opResult, len(sp.Repos))

	var g errgroup.Group
	for i, r := range sp.Repos {
		g.Go(func() error {
			defaultBranch, err := runner.DefaultBranch(r.RepoPath)
			if err != nil {
				results[i] = opResult{r.Name, ui.SymFail, fmt.Sprintf("default branch: %v", err)}
				return nil
			}
			onto := "origin/" + defaultBranch
			if err := runner.Rebase(r.WorktreePath, onto); err != nil {
				results[i] = opResult{r.Name, ui.SymFail, err.Error()}
			} else {
				results[i] = opResult{r.Name, ui.SymOK, "rebased onto " + onto}
			}
			return nil
		})
	}
	_ = g.Wait() // goroutines always return nil; outcomes are written to results[i]

	tbl := ui.NewTableWriter(out)
	var failed []string
	for _, r := range results {
		tbl.Row(r.name, r.sym+" "+r.msg)
		if r.sym == ui.SymFail {
			failed = append(failed, r.name)
		}
	}
	tbl.Flush()
	if len(failed) > 0 {
		return fmt.Errorf("rebase failed in: %s", strings.Join(failed, ", "))
	}
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

	if err := classifyBranchTargets(runner, targets, branch); err != nil {
		return err
	}

	hasGoMod, anyGoMod := detectGoMods(targets)

	var steps []saga.Step
	for i := range targets {
		steps = append(steps, worktreeStep(runner, targets[i], branch, args.Base))
	}
	goWorkPath := filepath.Join(spacePath, "go.work")
	if anyGoMod {
		goVersion := detectGoVersion(targets, hasGoMod)
		steps = append(steps, goWorkStep(goWorkPath, spacePath, targets, hasGoMod, goVersion))
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

	needsForce := len(warnings) > 0
	if needsForce {
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
		sym, msg := deleteOne(runner, r, sp.Branch, args.DeleteBranch, args.ForceBranch, needsForce)
		tbl.Row(r.Name, sym+" "+msg)
		if sym == ui.SymFail {
			hadError = true
		}
	}
	tbl.Flush()

	if hadError {
		return fmt.Errorf("some worktrees could not be removed; space %q not deleted from state", args.Name)
	}

	// Warn if the user's working directory is inside the space root; the shell
	// will be left in a broken state after the directory is removed.
	if cwd, err := os.Getwd(); err == nil {
		if cwd == sp.Path || strings.HasPrefix(cwd, sp.Path+string(filepath.Separator)) {
			fmt.Fprintf(out, "  %s your working directory is inside the space; cd elsewhere after deletion\n", ui.SymWarn)
		}
	}

	// Remove the go.work file that was written into the space root. Failure here
	// is reported but does not abort the delete — the state and worktrees are
	// already gone.
	if sp.GoWorkspace {
		goWorkPath := filepath.Join(sp.Path, "go.work")
		if err := os.Remove(goWorkPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(out, "  %s could not remove go.work: %v\n", ui.SymWarn, err)
		}
	}

	// Remove the space root directory. Only succeeds when empty; if the user
	// placed other files there, report a warning so they can clean up manually.
	if err := os.Remove(sp.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(out, "  %s could not remove space directory %s: %v\n", ui.SymWarn, sp.Path, err)
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

// classifyBranchTargets inspects the branch in each target repo and sets
// t.createBranch accordingly: true if the branch does not yet exist, false if
// it exists but is not checked out elsewhere. Returns an error if the branch is
// already checked out in another worktree, which would cause a git conflict.
func classifyBranchTargets(runner git.Runner, targets []*repoTarget, branch string) error {
	for _, t := range targets {
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
			t.createBranch = false
		} else {
			t.createBranch = true
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

// goWorkFallbackVersion is used when no go.mod declares a go directive.
// go.work files were introduced in Go 1.18; 1.21 is a safe modern baseline.
const goWorkFallbackVersion = "1.21"

// detectGoVersion returns the maximum go directive version found across all
// targets that have a go.mod, falling back to goWorkFallbackVersion.
func detectGoVersion(targets []*repoTarget, hasGoMod []bool) string {
	best := ""
	for i, t := range targets {
		if !hasGoMod[i] {
			continue
		}
		data, err := os.ReadFile(filepath.Join(t.repoPath, "go.mod"))
		if err != nil {
			continue
		}
		f, err := modfile.Parse("go.mod", data, nil)
		if err != nil || f.Go == nil {
			continue
		}
		if best == "" || cmpGoVersion(f.Go.Version, best) > 0 {
			best = f.Go.Version
		}
	}
	if best == "" {
		return goWorkFallbackVersion
	}
	return best
}

// cmpGoVersion compares two go directive version strings (e.g. "1.21", "1.22.1").
// Returns a positive value if a > b, negative if a < b, zero if equal.
func cmpGoVersion(a, b string) int {
	aParts := strings.SplitN(a, ".", 3)
	bParts := strings.SplitN(b, ".", 3)
	for len(aParts) < 3 {
		aParts = append(aParts, "0")
	}
	for len(bParts) < 3 {
		bParts = append(bParts, "0")
	}
	for i := range 3 {
		an, _ := strconv.Atoi(aParts[i])
		bn, _ := strconv.Atoi(bParts[i])
		if an != bn {
			return an - bn
		}
	}
	return 0
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
func goWorkStep(goWorkPath, spacePath string, targets []*repoTarget, hasGoMod []bool, goVersion string) saga.Step {
	// Capture existing content before the saga runs so undo can restore it.
	oldContent, _ := os.ReadFile(goWorkPath)
	return saga.Step{
		Name: "write go.work",
		Do: func(ctx context.Context) error {
			return writeGoWork(goWorkPath, spacePath, targets, hasGoMod, goVersion)
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
// goVersion is written as the go directive (e.g. "1.24").
func writeGoWork(goWorkPath, spacePath string, targets []*repoTarget, hasGoMod []bool, goVersion string) error {
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
	b.WriteString("go " + goVersion + "\n\nuse (\n")
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
