package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/geoffamey/wtg/internal/state"
	"github.com/geoffamey/wtg/internal/ui"
)

// ExecCommand returns the `wtg exec` command.
func ExecCommand() *cli.Command {
	return &cli.Command{
		Name:      "exec",
		Usage:     "run a command in each repo of a workspace",
		ArgsUsage: "<workspace> -- <cmd> [<args>...]",
		Description: `Runs a command in each repo's worktree sequentially, streaming output as
it goes. A header line identifies each repo. Execution continues even if a
command fails — all repos are attempted and failures are reported at the end.

Use -- to separate the workspace name from the command:

   wtg exec myfeature -- git status
   wtg exec myfeature -- go test ./...`,
		ShellComplete: completeSpaceAtFirst,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() < 2 {
				return fmt.Errorf("usage: wtg exec <workspace> -- <cmd> [<args>...]")
			}
			return RunSpaceExec(cmd.Args().First(), cmd.Args().Tail(), os.Stdout)
		},
	}
}

// RunSpaceExec runs a command in each worktree of the named space sequentially,
// streaming output as it goes. A header line identifies each repo. Execution
// continues even if a command fails; failed repos are reported at the end.
func RunSpaceExec(spaceName string, args []string, out io.Writer) error {
	sp, err := state.Load(spaceName)
	if err != nil {
		return fmt.Errorf("load space %q: %w", spaceName, err)
	}

	var failed []string
	for i, r := range sp.Repos {
		if i > 0 {
			fmt.Fprintln(out)
		}
		fmt.Fprintf(out, "%s\n", ui.SectionHeader(r.Name))
		if r.Symlink {
			fmt.Fprintf(out, "%s\n", ui.Warn.Render(ui.SymWarn+" skipped — symlink (always.repos)"))
			continue
		}
		cmd := exec.Command(args[0], args[1:]...) //nolint:gosec
		cmd.Dir = r.WorktreePath
		cmd.Stdout = out
		cmd.Stderr = out
		if err := cmd.Run(); err != nil {
			failed = append(failed, r.Name)
			fmt.Fprintf(out, "%s\n", ui.Fail.Render(ui.SymFail+" failed"))
		} else {
			fmt.Fprintf(out, "%s\n", ui.OK.Render(ui.SymOK+" ok"))
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("command failed in: %s", strings.Join(failed, ", "))
	}
	return nil
}
