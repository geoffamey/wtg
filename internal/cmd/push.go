package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"

	"github.com/geoffamey/wtg/internal/git"
	"github.com/geoffamey/wtg/internal/state"
	"github.com/geoffamey/wtg/internal/ui"
)

// PushCommand returns the `wtg push` command.
func PushCommand(runner git.Runner) *cli.Command {
	return &cli.Command{
		Name:      "push",
		Usage:     "push all branches in a workspace to origin",
		ArgsUsage: "<workspace>",
		Description: `Pushes the workspace's branch from each repo's worktree to origin in
parallel. Repos that fail are reported individually; others are not affected.`,
		ShellComplete: completeSpaces,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() == 0 {
				return fmt.Errorf("missing required argument: <workspace>")
			}
			return RunSpacePush(runner, cmd.Args().First(), os.Stdout)
		},
	}
}

// RunSpacePush pushes each repo's branch to origin in parallel.
func RunSpacePush(runner git.Runner, spaceName string, out io.Writer) error {
	sp, err := state.Load(spaceName)
	if err != nil {
		return fmt.Errorf("load space %q: %w", spaceName, err)
	}

	results := make([]opResult, len(sp.Repos))

	var g errgroup.Group
	for i, r := range sp.Repos {
		if r.Symlink {
			results[i] = opResult{r.Name, ui.SymWarn, "skipped — symlink (always.repos)"}
			continue
		}
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
		tbl.Row(r.name, r.render())
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
