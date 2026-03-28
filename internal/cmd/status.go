package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"

	"github.com/geoffamey/wtg/internal/git"
	"github.com/geoffamey/wtg/internal/state"
	"github.com/geoffamey/wtg/internal/ui"
)

// StatusCommand returns the top-level `wtg status` command.
func StatusCommand(runner git.Runner) *cli.Command {
	return &cli.Command{
		Name:          "status",
		Usage:         "show status of all workspace worktrees",
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
	}
}

// RunStatus shows workspace status. With no arguments it detects whether the
// current directory is inside a known space and shows that space in detail; if
// not, it prints a one-line summary for every space. With a named space it
// always shows full per-repo detail. The --detailed flag adds individual
// modified-file listings under each repo row.
func RunStatus(runner git.Runner, names []string, detailed bool, out io.Writer) error {
	if len(names) == 0 {
		spaces, err := state.List()
		if err != nil {
			return fmt.Errorf("list spaces: %w", err)
		}
		sort.Slice(spaces, func(i, j int) bool { return spaces[i].Name < spaces[j].Name })

		if sp := spaceContainingCWD(spaces); sp != nil {
			return printSpaceDetail(runner, sp, detailed, out)
		}
		return printSummary(spaces, out)
	}

	for i, name := range names {
		if i > 0 {
			fmt.Fprintln(out)
		}
		sp, err := state.Load(name)
		if err != nil {
			return fmt.Errorf("load space %q: %w", name, err)
		}
		if err := printSpaceDetail(runner, sp, detailed, out); err != nil {
			return err
		}
	}
	return nil
}

// spaceContainingCWD returns the first space whose path contains the current
// working directory, or nil if the CWD is not inside any known space.
func spaceContainingCWD(spaces []*state.Space) *state.Space {
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	for _, sp := range spaces {
		rel, err := filepath.Rel(sp.Path, cwd)
		if err == nil && !strings.HasPrefix(rel, "..") {
			return sp
		}
	}
	return nil
}

// printSummary prints a one-line metadata row for each space without running
// any git commands.
func printSummary(spaces []*state.Space, out io.Writer) error {
	tbl := ui.NewTableWriter(out)
	for _, sp := range spaces {
		tbl.Row(sp.Name, sp.Branch, sp.Path, fmt.Sprintf("%d repos", len(sp.Repos)))
	}
	tbl.Flush()
	return nil
}

// repoStatusResult holds the git status (or error) for one worktree.
type repoStatusResult struct {
	entry state.RepoEntry
	st    git.RepoStatus
	err   error
}

// printSpaceDetail prints the space header followed by a per-repo status table.
// When detailed is true it appends modified-file listings under each dirty repo.
func printSpaceDetail(runner git.Runner, sp *state.Space, detailed bool, out io.Writer) error {
	fmt.Fprintf(out, "%s  %s  %s  %d repos\n",
		ui.Bold.Render(sp.Name),
		ui.Muted.Render(sp.Branch),
		ui.Muted.Render(sp.Path),
		len(sp.Repos),
	)

	results := make([]repoStatusResult, len(sp.Repos))
	var g errgroup.Group
	for i, r := range sp.Repos {
		g.Go(func() error {
			st, err := runner.Status(r.WorktreePath)
			results[i] = repoStatusResult{entry: r, st: st, err: err}
			return nil
		})
	}
	_ = g.Wait() // goroutines always return nil; outcomes are written to results[i]

	// Render repo rows into a buffer so tabwriter computes column widths across
	// all repos before we interleave file lines in detailed mode.
	var buf bytes.Buffer
	tbl := ui.NewTableWriter(&buf)
	for _, rs := range results {
		tbl.Row(append([]string{"  " + rs.entry.Name}, worktreeStatusCols(rs.st, rs.err, sp.Branch)...)...)
	}
	tbl.Flush()

	if !detailed {
		_, err := io.Copy(out, &buf)
		return err
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	for i, rs := range results {
		if i < len(lines) {
			fmt.Fprintln(out, lines[i])
		}
		if rs.err == nil {
			for _, f := range rs.st.Files {
				x, y := f.Index, f.Worktree
				if x == '.' {
					x = ' '
				}
				if y == '.' {
					y = ' '
				}
				fmt.Fprintf(out, "    %c%c  %s\n", x, y, f.Path)
			}
		}
	}
	return nil
}

// worktreeStatusCols returns the branch, status, and ahead/behind columns for
// one worktree row. The expected branch is the space's branch.
func worktreeStatusCols(st git.RepoStatus, err error, spaceBranch string) []string {
	if err != nil {
		return []string{ui.Fail.Render(ui.SymFail + " " + err.Error())}
	}
	return []string{
		branchCol(st.Branch, spaceBranch),
		statusCol(st.Files),
		aheadBehindCol(st.Ahead, st.Behind, st.Upstream != ""),
	}
}

