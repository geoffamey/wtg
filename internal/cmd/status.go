package cmd

import (
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/urfave/cli/v2"

	"github.com/geoffamey/wtg/internal/git"
	"github.com/geoffamey/wtg/internal/state"
	"github.com/geoffamey/wtg/internal/ui"
)

// StatusCommand returns the top-level `wtg status` command.
func StatusCommand(runner git.Runner) *cli.Command {
	return &cli.Command{
		Name:      "status",
		Usage:     "show status of all workspace worktrees",
		ArgsUsage: "[<space>...]",
		Action: func(c *cli.Context) error {
			return RunStatus(runner, c.Args().Slice(), os.Stdout)
		},
	}
}

// RunStatus prints a per-repo status table for each space. If names is empty
// all spaces are shown, sorted by name. Each repo row shows branch, working-tree
// state, and ahead/behind relative to upstream.
func RunStatus(runner git.Runner, names []string, out io.Writer) error {
	spaces, err := loadSpaces(names)
	if err != nil {
		return err
	}

	for i, sp := range spaces {
		if i > 0 {
			fmt.Fprintln(out)
		}
		fmt.Fprintf(out, "%s  %s\n",
			ui.Bold.Render(sp.Name),
			ui.Muted.Render("("+sp.Branch+")"),
		)
		tbl := ui.NewTableWriter(out)
		for _, r := range sp.Repos {
			tbl.Row(append([]string{"  " + r.Name}, spaceStatusCols(r, sp.Branch, runner)...)...)
		}
		tbl.Flush()
	}
	return nil
}

// spaceStatusCols returns the branch, status, and ahead/behind columns for one
// repo's worktree. The expected branch is the space's branch so that a worktree
// on the right branch is shown muted and any deviation is highlighted.
func spaceStatusCols(r state.RepoEntry, spaceBranch string, runner git.Runner) []string {
	st, err := runner.Status(r.WorktreePath)
	if err != nil {
		return []string{ui.Fail.Render(ui.SymFail + " " + err.Error())}
	}
	return []string{
		branchCol(st.Branch, spaceBranch),
		statusCol(st.Files),
		aheadBehindCol(st.Ahead, st.Behind, st.Upstream != ""),
	}
}

// loadSpaces loads the requested spaces in order, or all spaces sorted by name.
func loadSpaces(names []string) ([]*state.Space, error) {
	if len(names) == 0 {
		spaces, err := state.List()
		if err != nil {
			return nil, fmt.Errorf("list spaces: %w", err)
		}
		sort.Slice(spaces, func(i, j int) bool { return spaces[i].Name < spaces[j].Name })
		return spaces, nil
	}
	spaces := make([]*state.Space, 0, len(names))
	for _, name := range names {
		sp, err := state.Load(name)
		if err != nil {
			return nil, fmt.Errorf("load space %q: %w", name, err)
		}
		spaces = append(spaces, sp)
	}
	return spaces, nil
}
