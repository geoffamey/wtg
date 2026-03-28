package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"

	"github.com/geoffamey/wtg/internal/config"
	"github.com/geoffamey/wtg/internal/git"
	"github.com/geoffamey/wtg/internal/ui"
)

// RepoCommand returns the `wtg repo` command with its subcommands.
func RepoCommand(runner git.Runner) *cli.Command {
	return &cli.Command{
		Name:  "repo",
		Usage: "manage repositories",
		Subcommands: []*cli.Command{
			{
				Name:    "discover",
				Aliases: []string{"list"},
				Usage:   "scan discovery.root_dir for git repositories",
				Action: func(c *cli.Context) error {
					cfg, err := config.Load(c.String("config"))
					if err != nil {
						return err
					}
					return RunDiscover(cfg, runner, os.Stdout)
				},
			},
			{
				Name:      "status",
				Usage:     "show status of main repo clones",
				ArgsUsage: "[<repo>...]",
				Action: func(c *cli.Context) error {
					cfg, err := config.Load(c.String("config"))
					if err != nil {
						return err
					}
					return RunRepoStatus(cfg, runner, c.Args().Slice(), os.Stdout)
				},
			},
			{
				Name:      "sync",
				Usage:     "fetch and fast-forward repos to origin's default branch",
				ArgsUsage: "[<repo>...]",
				Action: func(c *cli.Context) error {
					cfg, err := config.Load(c.String("config"))
					if err != nil {
						return err
					}
					return RunSync(cfg, runner, c.Args().Slice(), os.Stdout)
				},
			},
		},
	}
}

// RunDiscover scans cfg.Discovery.RootDir for git repositories and prints them
// as an aligned table of name, remote URL, and absolute path.
func RunDiscover(cfg *config.Config, runner git.Runner, out io.Writer) error {
	if cfg.Discovery.RootDir == "" {
		return fmt.Errorf("discovery.root_dir is not set; run `wtg init` to configure")
	}

	paths, err := discoverRepoPaths(cfg.Discovery.RootDir, cfg.Discovery.MaxDepth)
	if err != nil {
		return fmt.Errorf("scan %s: %w", cfg.Discovery.RootDir, err)
	}

	sort.Strings(paths)

	tbl := ui.NewTableWriter(out)
	for _, p := range paths {
		name, _ := filepath.Rel(cfg.Discovery.RootDir, p)
		remote, _ := runner.RemoteURL(p, "origin") // empty if no remote configured
		tbl.Row(name, remote, p)
	}
	tbl.Flush()
	return nil
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

// RunSync fetches and fast-forwards each repo's default branch. If args is
// empty, all discovered repos are synced. Otherwise, args are short names
// (relative to discovery.root_dir) to sync.
func RunSync(cfg *config.Config, runner git.Runner, args []string, out io.Writer) error {
	if cfg.Discovery.RootDir == "" {
		return fmt.Errorf("discovery.root_dir is not set; run `wtg init` to configure")
	}

	var paths []string
	if len(args) == 0 {
		discovered, err := discoverRepoPaths(cfg.Discovery.RootDir, cfg.Discovery.MaxDepth)
		if err != nil {
			return fmt.Errorf("scan %s: %w", cfg.Discovery.RootDir, err)
		}
		paths = discovered
		sort.Strings(paths)
	} else {
		for _, name := range args {
			p, err := resolveRepoPath(cfg.Discovery.RootDir, name)
			if err != nil {
				return err
			}
			paths = append(paths, p)
		}
	}

	type syncResult struct {
		name string
		sym  string
		msg  string
	}
	results := make([]syncResult, len(paths))

	var g errgroup.Group
	for i, p := range paths {
		g.Go(func() error {
			name, _ := filepath.Rel(cfg.Discovery.RootDir, p)
			sym, msg := syncOne(p, runner)
			results[i] = syncResult{name, sym, msg}
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

// RunRepoStatus prints a status table for all discovered repos (or the named
// subset). Each row shows the repo name, current branch, working-tree state,
// and ahead/behind counts relative to origin.
func RunRepoStatus(cfg *config.Config, runner git.Runner, args []string, out io.Writer) error {
	if cfg.Discovery.RootDir == "" {
		return fmt.Errorf("discovery.root_dir is not set; run `wtg init` to configure")
	}

	var paths []string
	if len(args) == 0 {
		discovered, err := discoverRepoPaths(cfg.Discovery.RootDir, cfg.Discovery.MaxDepth)
		if err != nil {
			return fmt.Errorf("scan %s: %w", cfg.Discovery.RootDir, err)
		}
		paths = discovered
		sort.Strings(paths)
	} else {
		for _, name := range args {
			p, err := resolveRepoPath(cfg.Discovery.RootDir, name)
			if err != nil {
				return err
			}
			paths = append(paths, p)
		}
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
			results[i] = statusResult{name, repoStatusCols(p, runner)}
			return nil
		})
	}
	_ = g.Wait()

	tbl := ui.NewTableWriter(out)
	for _, r := range results {
		tbl.Row(append([]string{r.name}, r.cols...)...)
	}
	tbl.Flush()
	return nil
}

// repoStatusCols returns the branch, status, and ahead/behind columns for one repo.
func repoStatusCols(repoPath string, runner git.Runner) []string {
	st, err := runner.Status(repoPath)
	if err != nil {
		return []string{ui.Fail.Render(ui.SymFail + " " + err.Error())}
	}

	defaultBranch, _ := runner.DefaultBranch(repoPath) // empty if no remote

	return []string{
		branchCol(st.Branch, defaultBranch),
		statusCol(st.Files),
		aheadBehindCol(st.Ahead, st.Behind, st.Upstream != ""),
	}
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
