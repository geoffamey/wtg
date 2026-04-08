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

	"github.com/urfave/cli/v3"

	"github.com/geoffamey/wtg/internal/git"
	"github.com/geoffamey/wtg/internal/state"
	"github.com/geoffamey/wtg/internal/ui"
)

// RemoveCommand returns the `wtg remove` command.
func RemoveCommand(runner git.Runner) *cli.Command {
	return &cli.Command{
		Name:    "remove",
		Aliases: []string{"rm"},
		Usage:     "remove repos from a workspace",
		ArgsUsage: "<workspace> <repo>...",
		Description: `Removes the specified repos' worktrees from a workspace and updates go.work.
By default, branches are left untouched — use --delete-branch (-d) or
--force-delete-branch (-D) to also delete them.

Prompts for confirmation if any repo has uncommitted changes or unpushed
commits. To remove all repos at once, use wtg delete instead.`,
		ShellComplete: completeSpaceMembers,
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
			if cmd.Args().Len() < 2 {
				return fmt.Errorf("usage: wtg remove <workspace> <repo>...")
			}
			return RunSpaceRemove(runner, SpaceRemoveArgs{
				Name:         cmd.Args().First(),
				Repos:        cmd.Args().Tail(),
				DeleteBranch: cmd.Bool("delete-branch"),
				ForceBranch:  cmd.Bool("force-delete-branch"),
			}, os.Stdin, os.Stdout)
		},
	}
}

// SpaceRemoveArgs holds the parsed arguments for RunSpaceRemove.
type SpaceRemoveArgs struct {
	Name         string
	Repos        []string // short names of repos to remove (required)
	DeleteBranch bool     // -d: delete branch if fully merged
	ForceBranch  bool     // -D: force-delete branch regardless of merge state
}

// RunSpaceRemove removes one or more repos from a workspace by deleting their
// worktrees and updating the go.work file and state. It mirrors the safety
// checks of RunSpaceDelete: uncommitted changes and unpushed commits prompt for
// confirmation before proceeding.
func RunSpaceRemove(runner git.Runner, args SpaceRemoveArgs, in io.Reader, out io.Writer) error {
	sp, err := state.Load(args.Name)
	if err != nil {
		return fmt.Errorf("load space %q: %w", args.Name, err)
	}

	// Index existing repos by name for fast lookup.
	byName := make(map[string]state.RepoEntry, len(sp.Repos))
	for _, r := range sp.Repos {
		byName[r.Name] = r
	}

	var toRemove []state.RepoEntry
	for _, name := range args.Repos {
		r, ok := byName[name]
		if !ok {
			return fmt.Errorf("repo %q is not in space %q", name, args.Name)
		}
		toRemove = append(toRemove, r)
	}

	if len(toRemove) == len(sp.Repos) {
		return fmt.Errorf("cannot remove all repos from space %q; use `wtg delete` instead", args.Name)
	}

	// Pre-flight: gather warnings about uncommitted or unpushed work.
	var warnings []string
	for _, r := range toRemove {
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
		ok, err := confirm(bufio.NewReader(in), out, "Remove repos anyway?")
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
	}

	// Remove worktrees. A SymFail result means the worktree is still present,
	// so we must not update go.work or state.
	hadError := false
	tbl := ui.NewTableWriter(out)
	for _, r := range toRemove {
		sym, msg := deleteOne(runner, r, sp.Branch, args.DeleteBranch, args.ForceBranch, needsForce)
		tbl.Row(r.Name, sym+" "+msg)
		if sym == ui.SymFail {
			hadError = true
		}
	}
	tbl.Flush()

	if hadError {
		return fmt.Errorf("some worktrees could not be removed; space %q unchanged", args.Name)
	}

	// Compute the repos that remain after removal.
	removeSet := make(map[string]bool, len(toRemove))
	for _, r := range toRemove {
		removeSet[r.Name] = true
	}
	var keepEntries []state.RepoEntry
	for _, r := range sp.Repos {
		if !removeSet[r.Name] {
			keepEntries = append(keepEntries, r)
		}
	}

	// Update go.work when this space has one. Rewrite it for the remaining
	// repos; if none of them have a go.mod, remove the file entirely.
	// go.work.sum is always removed because its entries are stale once the
	// module set changes — the user can regenerate it with `go work sync`.
	goWorkPath := filepath.Join(sp.Path, "go.work")
	goWorkSumPath := filepath.Join(sp.Path, "go.work.sum")
	remainingGoWorkspace := false
	if sp.GoWorkspace {
		remainingTargets := targetsFromState(&state.Space{Repos: keepEntries})
		hasGoMod, anyGoMod := detectGoMods(remainingTargets)
		if anyGoMod {
			remainingGoWorkspace = true
			goVersion := detectGoVersion(remainingTargets, hasGoMod)
			if err := writeGoWork(goWorkPath, sp.Path, remainingTargets, hasGoMod, goVersion); err != nil {
				fmt.Fprintf(out, "  %s could not update go.work: %v\n", ui.SymWarn, err)
			}
		} else {
			if err := os.Remove(goWorkPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
				fmt.Fprintf(out, "  %s could not remove go.work: %v\n", ui.SymWarn, err)
			}
		}
		if err := os.Remove(goWorkSumPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(out, "  %s could not remove go.work.sum: %v\n", ui.SymWarn, err)
		}
	}

	updated := *sp
	updated.Repos = keepEntries
	updated.GoWorkspace = remainingGoWorkspace
	if err := state.Save(&updated); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	fmt.Fprintf(out, "%s removed from space %q\n", ui.SymOK, args.Name)
	return nil
}
