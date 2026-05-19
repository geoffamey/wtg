package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/geoffamey/wtg/internal/git"
	"github.com/geoffamey/wtg/internal/state"
	"github.com/geoffamey/wtg/internal/ui"
)

// DeleteCommand returns the `wtg delete` command.
func DeleteCommand(runner git.Runner) *cli.Command {
	return &cli.Command{
		Name:      "delete",
		Usage:     "delete a workspace and optionally its branches",
		ArgsUsage: "<workspace>",
		Description: `Removes all worktrees in the workspace. By default, branches are left
untouched — use --delete-branch (-d) to delete merged branches or
--force-delete-branch (-D) to force-delete regardless of merge state.

Prompts for confirmation if any repo has uncommitted changes or unpushed
commits. The workspace root directory is removed if it is empty after
worktrees are cleaned up.`,
		ShellComplete: completeSpaces,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "delete-branch",
				Aliases: []string{"d"},
				Usage:   "delete branch if fully merged into upstream",
			},
			&cli.BoolFlag{
				Name:    "force-delete-branch",
				Aliases: []string{"D"},
				Usage:   "force-delete branch regardless of merge state",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() == 0 {
				return fmt.Errorf("missing required argument: <workspace>")
			}
			return RunSpaceDelete(runner, SpaceDeleteArgs{
				Name:         cmd.Args().First(),
				DeleteBranch: cmd.Bool("delete-branch"),
				ForceBranch:  cmd.Bool("force-delete-branch"),
			}, os.Stdin, os.Stdout)
		},
	}
}

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
	// Symlink entries point to the shared main clone; their state is not
	// specific to this space, so they are excluded from pre-flight checks.
	var warnings []string
	for _, r := range sp.Repos {
		if r.Symlink {
			continue
		}
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
		ok, err := confirm(bufio.NewReader(in), out, "Delete workspace anyway?")
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
		goWorkSumPath := filepath.Join(sp.Path, "go.work.sum")
		if err := os.Remove(goWorkSumPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(out, "  %s could not remove go.work.sum: %v\n", ui.SymWarn, err)
		}
	}

	// Remove the space root directory. Only succeeds when empty; if the user
	// placed other files there, report a warning so they can clean up manually.
	if err := os.Remove(sp.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(out, "  %s could not remove space directory %s: %v\n", ui.SymWarn, sp.Path, err)
	}

	return state.Delete(args.Name)
}
