package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/urfave/cli/v2"

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

	tbl := ui.NewTableWriter(out)
	for _, p := range paths {
		name, _ := filepath.Rel(cfg.Discovery.RootDir, p)
		sym, msg := syncOne(p, runner)
		tbl.Row(name, sym+" "+msg)
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
