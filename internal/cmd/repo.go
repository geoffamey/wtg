package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"

	"github.com/geoffamey/wtg/internal/config"
	"github.com/geoffamey/wtg/internal/git"
	"github.com/geoffamey/wtg/internal/ui"
)

// opResult holds the outcome of one parallel operation for display in a table.
type opResult struct {
	name string
	sym  string
	msg  string
}

// RepoCommand returns the `wtg repo` command with its subcommands.
func RepoCommand(runner git.Runner) *cli.Command {
	return &cli.Command{
		Name:  "repo",
		Usage: "manage repositories",
		Description: `Operates on the main repo clones (not workspace worktrees). Use these
commands to inspect and update the clones that wtg discovers under
discovery.root_dir.`,
		Commands: []*cli.Command{
			{
				Name:      "status",
				Usage:     "show status of main repo clones",
				ArgsUsage: "[<repo>...]",
				Description: `Shows branch, dirty status, and ahead/behind counts for each discovered
repo clone. Without arguments, all repos are shown. Pass repo names to
filter the output. Use --long (-l) to also show each repo's remote URL
and local path.`,
				ShellComplete: completeRepos,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "long",
						Aliases: []string{"l"},
						Usage:   "also show remote URL and local path",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					cfg, err := config.Load(cmd.String("config"))
					if err != nil {
						return err
					}
					return RunRepoStatus(cfg, runner, cmd.Args().Slice(), cmd.Bool("long"), os.Stdout)
				},
			},
			{
				Name:      "fetch",
				Usage:     "fetch from origin for all repos (no fast-forward)",
				ArgsUsage: "[<repo>...]",
				Description: `Downloads new commits from origin for each repo without updating any
local branches (equivalent to git fetch origin). Runs in parallel.`,
				ShellComplete: completeRepos,
				Action: func(ctx context.Context, cmd *cli.Command) error {
					cfg, err := config.Load(cmd.String("config"))
					if err != nil {
						return err
					}
					return RunFetch(cfg, runner, cmd.Args().Slice(), os.Stdout)
				},
			},
			{
				Name:      "sync",
				Usage:     "fetch and fast-forward repos to origin's default branch",
				ArgsUsage: "[<repo>...]",
				Description: `Fetches origin and fast-forwards each repo's default branch (main, master,
etc.) if it has no local changes. Repos with uncommitted changes or a
diverged branch are skipped with a warning. Runs in parallel.`,
				ShellComplete: completeRepos,
				Action: func(ctx context.Context, cmd *cli.Command) error {
					cfg, err := config.Load(cmd.String("config"))
					if err != nil {
						return err
					}
					return RunSync(cfg, runner, cmd.Args().Slice(), os.Stdout)
				},
			},
		},
	}
}

// discoverRepoPaths recursively scans root up to maxDepth levels deep and
// returns the absolute paths of all directories containing a .git entry.
// Directories that are themselves git repos are not recursed into.
// Hidden directories (name starting with ".") are skipped.
func discoverRepoPaths(root string, maxDepth int) ([]string, error) {
	return scanDir(root, maxDepth, 0)
}

func scanDir(dir string, maxDepth, depth int) ([]string, error) {
	if isGitRepo(dir) {
		return []string{dir}, nil
	}
	if depth >= maxDepth {
		return nil, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, e := range entries {
		if !e.IsDir() || e.Name()[0] == '.' {
			continue
		}
		sub, err := scanDir(filepath.Join(dir, e.Name()), maxDepth, depth+1)
		if err != nil {
			return nil, err
		}
		paths = append(paths, sub...)
	}
	return paths, nil
}

func isGitRepo(path string) bool {
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
}

// RunFetch fetches from origin for each repo without fast-forwarding. If args
// is empty, all discovered repos are fetched. Otherwise args are short names
// (relative to discovery.root_dir) to fetch.
func RunFetch(cfg *config.Config, runner git.Runner, args []string, out io.Writer) error {
	if cfg.Discovery.RootDir == "" {
		return fmt.Errorf("discovery.root_dir is not set; run `wtg init` to configure")
	}

	paths, err := resolveRepoPaths(cfg, args)
	if err != nil {
		return err
	}

	results := make([]opResult, len(paths))

	var g errgroup.Group
	for i, p := range paths {
		g.Go(func() error {
			name, _ := filepath.Rel(cfg.Discovery.RootDir, p)
			if err := runner.Fetch(p); err != nil {
				results[i] = opResult{name, ui.SymFail, err.Error()}
			} else {
				results[i] = opResult{name, ui.SymOK, "fetched"}
			}
			return nil
		})
	}
	_ = g.Wait() // goroutines always return nil; outcomes are written to results[i]

	tbl := ui.NewTableWriter(out)
	for _, r := range results {
		tbl.Row(r.name, r.sym+" "+r.msg)
	}
	tbl.Flush()
	return nil
}

// RunSync fetches and fast-forwards each repo's default branch. If args is
// empty, all discovered repos are synced. Otherwise, args are short names
// (relative to discovery.root_dir) to sync.
func RunSync(cfg *config.Config, runner git.Runner, args []string, out io.Writer) error {
	if cfg.Discovery.RootDir == "" {
		return fmt.Errorf("discovery.root_dir is not set; run `wtg init` to configure")
	}

	paths, err := resolveRepoPaths(cfg, args)
	if err != nil {
		return err
	}

	results := make([]opResult, len(paths))

	var g errgroup.Group
	for i, p := range paths {
		g.Go(func() error {
			name, _ := filepath.Rel(cfg.Discovery.RootDir, p)
			sym, msg := syncOne(p, runner)
			results[i] = opResult{name, sym, msg}
			return nil
		})
	}
	_ = g.Wait() // goroutines always return nil; outcomes are written to results[i]

	tbl := ui.NewTableWriter(out)
	for _, r := range results {
		tbl.Row(r.name, r.sym+" "+r.msg)
	}
	tbl.Flush()
	return nil
}

// syncOne performs the fetch + fast-forward for a single repo and returns a
// symbol and human-readable message describing the outcome.
func syncOne(repoPath string, runner git.Runner) (string, string) {
	defaultBranch, err := runner.DefaultBranch(repoPath)
	if err != nil {
		return ui.SymFail, fmt.Sprintf("error: %v", err)
	}

	st, err := runner.Status(repoPath)
	if err != nil {
		return ui.SymFail, fmt.Sprintf("error: %v", err)
	}

	if st.Branch != defaultBranch {
		return ui.SymWarn, fmt.Sprintf("skipped — on branch %s, not %s", st.Branch, defaultBranch)
	}
	if len(st.Files) > 0 {
		return ui.SymWarn, "skipped — working tree is dirty"
	}

	if err := runner.Fetch(repoPath); err != nil {
		return ui.SymFail, fmt.Sprintf("error fetching: %v", err)
	}

	st, err = runner.Status(repoPath)
	if err != nil {
		return ui.SymFail, fmt.Sprintf("error: %v", err)
	}

	if st.Behind == 0 {
		return ui.SymOK, "up to date"
	}

	if err := runner.FastForward(repoPath, defaultBranch); err != nil {
		return ui.SymFail, fmt.Sprintf("error fast-forwarding: %v", err)
	}

	n := st.Behind
	commits := "commits"
	if n == 1 {
		commits = "commit"
	}
	return ui.SymUp, fmt.Sprintf("fast-forwarded to origin/%s (%d %s)", defaultBranch, n, commits)
}

// resolveRepoPath converts a short repo name to an absolute path, checking
// that a git repo actually exists there.
func resolveRepoPath(rootDir, name string) (string, error) {
	p := filepath.Join(rootDir, filepath.FromSlash(name))
	if !isGitRepo(p) {
		return "", fmt.Errorf("repo %q not found under %s", name, rootDir)
	}
	return p, nil
}

// resolveRepoPaths returns paths for all repos to operate on. When args is
// empty it discovers all repos under cfg.Discovery.RootDir (sorted); otherwise
// it resolves each named arg to an absolute path.
func resolveRepoPaths(cfg *config.Config, args []string) ([]string, error) {
	if len(args) == 0 {
		paths, err := discoverRepoPaths(cfg.Discovery.RootDir, cfg.Discovery.MaxDepth)
		if err != nil {
			return nil, fmt.Errorf("scan %s: %w", cfg.Discovery.RootDir, err)
		}
		sort.Strings(paths)
		return paths, nil
	}
	paths := make([]string, 0, len(args))
	for _, name := range args {
		p, err := resolveRepoPath(cfg.Discovery.RootDir, name)
		if err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, nil
}

// RunRepoStatus prints a status table for all discovered repos (or the named
// subset). Each row shows the repo name, current branch, working-tree state,
// and ahead/behind counts relative to origin. When long is true, the remote
// URL and absolute path are appended in a muted style.
func RunRepoStatus(cfg *config.Config, runner git.Runner, args []string, long bool, out io.Writer) error {
	if cfg.Discovery.RootDir == "" {
		return fmt.Errorf("discovery.root_dir is not set; run `wtg init` to configure")
	}

	paths, err := resolveRepoPaths(cfg, args)
	if err != nil {
		return err
	}

	type statusResult struct {
		name string
		cols []string
	}
	results := make([]statusResult, len(paths))

	var g errgroup.Group
	for i, p := range paths {
		g.Go(func() error {
			name, _ := filepath.Rel(cfg.Discovery.RootDir, p)
			results[i] = statusResult{name, repoStatusCols(p, runner, long)}
			return nil
		})
	}
	_ = g.Wait() // goroutines always return nil; outcomes are written to results[i]

	tbl := ui.NewTableWriter(out)
	for _, r := range results {
		tbl.Row(append([]string{r.name}, r.cols...)...)
	}
	tbl.Flush()
	return nil
}

// repoStatusCols returns the branch, status, and ahead/behind columns for one
// repo. When long is true, two additional muted columns are appended: the
// remote URL (origin) and the absolute local path.
func repoStatusCols(repoPath string, runner git.Runner, long bool) []string {
	st, err := runner.Status(repoPath)
	if err != nil {
		return []string{ui.Fail.Render(ui.SymFail + " " + err.Error())}
	}

	defaultBranch, _ := runner.DefaultBranch(repoPath) // empty if no remote

	cols := []string{
		branchCol(st.Branch, defaultBranch),
		statusCol(st.Files),
		aheadBehindCol(st.Ahead, st.Behind, st.Upstream != ""),
	}

	if long {
		remote, _ := runner.RemoteURL(repoPath, "origin")
		cols = append(cols, ui.Muted.Render(remote), ui.Muted.Render(repoPath))
	}

	return cols
}

// branchCol renders the branch name, highlighted if it differs from the default.
func branchCol(branch, defaultBranch string) string {
	if branch == "" {
		return ui.Warn.Render("[(detached)]")
	}
	text := "[" + branch + "]"
	if defaultBranch == "" || branch == defaultBranch {
		return ui.Muted.Render(text)
	}
	return ui.Warn.Render(text)
}

// statusCol renders the working-tree state as a clean/dirty summary.
func statusCol(files []git.FileStatus) string {
	modified, untracked := classifyFiles(files)
	if modified == 0 && untracked == 0 {
		return ui.OK.Render(ui.SymOK + " clean")
	}
	var parts []string
	if modified > 0 {
		parts = append(parts, fmt.Sprintf("%d modified", modified))
	}
	if untracked > 0 {
		parts = append(parts, fmt.Sprintf("%d untracked", untracked))
	}
	return ui.Fail.Render(ui.SymFail + " " + strings.Join(parts, ", "))
}

// aheadBehindCol renders ahead/behind counts. Returns an empty string if
// the repo has no upstream tracking branch.
// Colours: behind > 0 → yellow; both non-zero (diverged) → red.
func aheadBehindCol(ahead, behind int, hasUpstream bool) string {
	if !hasUpstream {
		return ""
	}
	a := fmt.Sprintf("%s%d", ui.SymUp, ahead)
	b := fmt.Sprintf("↓%d", behind)
	switch {
	case ahead > 0 && behind > 0:
		return ui.Fail.Render(a + " " + b)
	case behind > 0:
		return a + " " + ui.Warn.Render(b)
	default:
		return a + " " + b
	}
}

// classifyFiles counts modified (any non-untracked change) and untracked files.
func classifyFiles(files []git.FileStatus) (modified, untracked int) {
	for _, f := range files {
		if f.Index == '?' && f.Worktree == '?' {
			untracked++
		} else {
			modified++
		}
	}
	return
}
