package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/mod/modfile"

	"github.com/geoffamey/wtg/internal/git"
	"github.com/geoffamey/wtg/internal/saga"
	"github.com/geoffamey/wtg/internal/state"
	"github.com/geoffamey/wtg/internal/ui"
)

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
			parentDir := filepath.Dir(t.worktreePath)
			if err := os.MkdirAll(parentDir, 0o755); err != nil {
				return fmt.Errorf("create parent dir: %w", err)
			}
			if err := runner.WorktreeAdd(t.repoPath, t.worktreePath, branch, base, t.createBranch); err != nil {
				// Clean up the dir we just created; ignore error (may not be empty
				// if another worktree in the same space already populated it).
				_ = os.Remove(parentDir)
				return err
			}
			return nil
		},
		Undo: func(ctx context.Context) error {
			removeErr := runner.WorktreeRemove(t.repoPath, t.worktreePath, true)
			// Clean up the parent dir created by MkdirAll; os.Remove is a no-op
			// if the dir is non-empty or does not exist.
			_ = os.Remove(filepath.Dir(t.worktreePath))
			if removeErr != nil {
				return removeErr
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
