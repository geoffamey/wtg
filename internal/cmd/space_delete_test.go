package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/geoffamey/wtg/internal/git"
	"github.com/geoffamey/wtg/internal/state"
)

// deleteRunner builds a testRunner for delete scenarios. statusFn is called for
// each worktree path to check dirty/unpushed state.
func deleteRunner(statusFn func(string) (git.RepoStatus, error)) *testRunner {
	return &testRunner{
		statusFn:         statusFn,
		worktreeRemoveFn: func(_, _ string, _ bool) error { return nil },
	}
}

func cleanStatus(_ string) (git.RepoStatus, error) {
	return git.RepoStatus{}, nil
}

// --- space not found ---

func TestRunSpaceDelete_SpaceNotFound(t *testing.T) {
	isolateState(t)
	var out bytes.Buffer
	err := RunSpaceDelete(&testRunner{}, SpaceDeleteArgs{Name: "nonexistent"}, &bytes.Buffer{}, &out)
	if err == nil {
		t.Fatal("expected error when space does not exist")
	}
}

// --- clean delete, no branch removal ---

func TestRunSpaceDelete_RemovesWorktrees(t *testing.T) {
	isolateState(t)
	root := t.TempDir()
	spacePath := t.TempDir()
	makeSpace(t, "feat", "feat", spacePath, []string{"api", "frontend"}, root)

	var removed []string
	r := deleteRunner(cleanStatus)
	r.worktreeRemoveFn = func(_, worktreePath string, _ bool) error {
		removed = append(removed, worktreePath)
		return nil
	}

	var out bytes.Buffer
	if err := RunSpaceDelete(r, SpaceDeleteArgs{Name: "feat"}, &bytes.Buffer{}, &out); err != nil {
		t.Fatalf("RunSpaceDelete: %v", err)
	}
	if len(removed) != 2 {
		t.Errorf("expected 2 worktree removes, got %d", len(removed))
	}
	// State should be gone.
	if _, err := state.Load("feat"); err == nil {
		t.Error("state should be deleted after successful delete")
	}
}

func TestRunSpaceDelete_NotForced_WhenClean(t *testing.T) {
	isolateState(t)
	root := t.TempDir()
	spacePath := t.TempDir()
	makeSpace(t, "feat", "feat", spacePath, []string{"api"}, root)

	var usedForce bool
	r := deleteRunner(cleanStatus)
	r.worktreeRemoveFn = func(_, _ string, force bool) error {
		usedForce = force
		return nil
	}

	var out bytes.Buffer
	if err := RunSpaceDelete(r, SpaceDeleteArgs{Name: "feat"}, &bytes.Buffer{}, &out); err != nil {
		t.Fatalf("RunSpaceDelete: %v", err)
	}
	if usedForce {
		t.Error("WorktreeRemove should not use force when worktrees are clean")
	}
}

// --- dirty / unpushed prompts ---

func TestRunSpaceDelete_DirtyWorktree_UserDeclines(t *testing.T) {
	isolateState(t)
	root := t.TempDir()
	spacePath := t.TempDir()
	makeSpace(t, "feat", "feat", spacePath, []string{"api"}, root)

	dirtyStatus := func(_ string) (git.RepoStatus, error) {
		return git.RepoStatus{Files: []git.FileStatus{{Path: "a.go", Index: 'M'}}}, nil
	}
	r := deleteRunner(dirtyStatus)

	var out bytes.Buffer
	// "n\n" → user declines
	if err := RunSpaceDelete(r, SpaceDeleteArgs{Name: "feat"}, strings.NewReader("n\n"), &out); err != nil {
		t.Fatalf("RunSpaceDelete: %v", err)
	}
	// State should still exist (nothing deleted).
	if _, err := state.Load("feat"); err != nil {
		t.Error("state should not be deleted when user declines")
	}
	if !strings.Contains(out.String(), "uncommitted changes") {
		t.Errorf("output should mention uncommitted changes: %q", out.String())
	}
}

func TestRunSpaceDelete_DirtyWorktree_UserConfirms(t *testing.T) {
	isolateState(t)
	root := t.TempDir()
	spacePath := t.TempDir()
	makeSpace(t, "feat", "feat", spacePath, []string{"api"}, root)

	dirtyStatus := func(_ string) (git.RepoStatus, error) {
		return git.RepoStatus{Files: []git.FileStatus{{Path: "a.go", Index: 'M'}}}, nil
	}
	var usedForce bool
	r := deleteRunner(dirtyStatus)
	r.worktreeRemoveFn = func(_, _ string, force bool) error {
		usedForce = force
		return nil
	}

	var out bytes.Buffer
	if err := RunSpaceDelete(r, SpaceDeleteArgs{Name: "feat"}, strings.NewReader("y\n"), &out); err != nil {
		t.Fatalf("RunSpaceDelete: %v", err)
	}
	if !usedForce {
		t.Error("WorktreeRemove should use force when user confirms despite dirty state")
	}
	if _, err := state.Load("feat"); err == nil {
		t.Error("state should be deleted after confirmed delete")
	}
}

func TestRunSpaceDelete_UnpushedCommits_Warned(t *testing.T) {
	isolateState(t)
	root := t.TempDir()
	spacePath := t.TempDir()
	makeSpace(t, "feat", "feat", spacePath, []string{"api"}, root)

	aheadStatus := func(_ string) (git.RepoStatus, error) {
		return git.RepoStatus{Ahead: 2}, nil
	}
	r := deleteRunner(aheadStatus)

	var out bytes.Buffer
	_ = RunSpaceDelete(r, SpaceDeleteArgs{Name: "feat"}, strings.NewReader("n\n"), &out)
	if !strings.Contains(out.String(), "unpushed") {
		t.Errorf("output should mention unpushed commits: %q", out.String())
	}
}

// --- branch deletion ---

func TestRunSpaceDelete_DeleteBranch(t *testing.T) {
	isolateState(t)
	root := t.TempDir()
	spacePath := t.TempDir()
	makeSpace(t, "feat", "mybranch", spacePath, []string{"api"}, root)

	var deletedBranch string
	var deletedForce bool
	r := deleteRunner(cleanStatus)
	r.branchDeleteFn = func(_, branch string, force bool) error {
		deletedBranch = branch
		deletedForce = force
		return nil
	}

	var out bytes.Buffer
	if err := RunSpaceDelete(r, SpaceDeleteArgs{Name: "feat", DeleteBranch: true}, &bytes.Buffer{}, &out); err != nil {
		t.Fatalf("RunSpaceDelete: %v", err)
	}
	if deletedBranch != "mybranch" {
		t.Errorf("expected branch mybranch to be deleted, got %q", deletedBranch)
	}
	if deletedForce {
		t.Error("-d should not force-delete branch")
	}
}

func TestRunSpaceDelete_ForceBranch(t *testing.T) {
	isolateState(t)
	root := t.TempDir()
	spacePath := t.TempDir()
	makeSpace(t, "feat", "mybranch", spacePath, []string{"api"}, root)

	var deletedForce bool
	r := deleteRunner(cleanStatus)
	r.branchDeleteFn = func(_, _ string, force bool) error {
		deletedForce = force
		return nil
	}

	var out bytes.Buffer
	if err := RunSpaceDelete(r, SpaceDeleteArgs{Name: "feat", ForceBranch: true}, &bytes.Buffer{}, &out); err != nil {
		t.Fatalf("RunSpaceDelete: %v", err)
	}
	if !deletedForce {
		t.Error("-D should force-delete branch")
	}
}

func TestRunSpaceDelete_BranchDeleteFailure_StateStillDeleted(t *testing.T) {
	// BranchDelete fails (e.g. unmerged with -d) → SymWarn, but state is deleted
	// because the worktree was already removed successfully.
	isolateState(t)
	root := t.TempDir()
	spacePath := t.TempDir()
	makeSpace(t, "feat", "feat", spacePath, []string{"api"}, root)

	r := deleteRunner(cleanStatus)
	r.branchDeleteFn = func(_, _ string, _ bool) error {
		return fmt.Errorf("branch not fully merged")
	}

	var out bytes.Buffer
	if err := RunSpaceDelete(r, SpaceDeleteArgs{Name: "feat", DeleteBranch: true}, &bytes.Buffer{}, &out); err != nil {
		t.Fatalf("RunSpaceDelete should succeed when only branch deletion fails: %v", err)
	}
	// State should still be deleted — the worktree is gone.
	if _, stateErr := state.Load("feat"); stateErr == nil {
		t.Error("state should be deleted even when branch deletion fails")
	}
	// Output should indicate the branch issue.
	if !strings.Contains(out.String(), "branch not deleted") {
		t.Errorf("output should mention branch not deleted: %q", out.String())
	}
}

func TestRunSpaceDelete_NoBranchDelete_WhenNoFlag(t *testing.T) {
	isolateState(t)
	root := t.TempDir()
	spacePath := t.TempDir()
	makeSpace(t, "feat", "feat", spacePath, []string{"api"}, root)

	r := deleteRunner(cleanStatus)
	// branchDeleteFn intentionally not set — panics if called unexpectedly.

	var out bytes.Buffer
	if err := RunSpaceDelete(r, SpaceDeleteArgs{Name: "feat"}, &bytes.Buffer{}, &out); err != nil {
		t.Fatalf("RunSpaceDelete: %v", err)
	}
}

// --- multi-repo behaviour ---

func TestRunSpaceDelete_PartialFailure_StatePreserved(t *testing.T) {
	// First repo removed OK, second fails → hadError=true, state preserved.
	isolateState(t)
	root := t.TempDir()
	spacePath := t.TempDir()
	makeSpace(t, "feat", "feat", spacePath, []string{"api", "frontend"}, root)

	callCount := 0
	r := deleteRunner(cleanStatus)
	r.worktreeRemoveFn = func(_, _ string, _ bool) error {
		callCount++
		if callCount == 2 {
			return fmt.Errorf("locked")
		}
		return nil
	}

	var out bytes.Buffer
	err := RunSpaceDelete(r, SpaceDeleteArgs{Name: "feat"}, &bytes.Buffer{}, &out)
	if err == nil {
		t.Fatal("expected error when one worktree removal fails")
	}
	// State must survive because one worktree could not be removed.
	if _, stateErr := state.Load("feat"); stateErr != nil {
		t.Error("state should be preserved on partial failure")
	}
	// Both repos should appear in output.
	got := out.String()
	if !strings.Contains(got, "api") || !strings.Contains(got, "frontend") {
		t.Errorf("output should show both repos: %q", got)
	}
}

func TestRunSpaceDelete_MixedDirtyClean_PromptFires(t *testing.T) {
	// One repo dirty, one clean → prompt shown, both removed with force on confirm.
	isolateState(t)
	root := t.TempDir()
	spacePath := t.TempDir()
	makeSpace(t, "feat", "feat", spacePath, []string{"api", "frontend"}, root)

	statusCalls := map[string]git.RepoStatus{
		"api":      {Files: []git.FileStatus{{Path: "a.go", Index: 'M'}}},
		"frontend": {},
	}
	statusFn := func(path string) (git.RepoStatus, error) {
		for name, st := range statusCalls {
			if strings.Contains(path, name) {
				return st, nil
			}
		}
		return git.RepoStatus{}, nil
	}

	var forcedPaths []string
	r := &testRunner{
		statusFn: statusFn,
		worktreeRemoveFn: func(_, worktreePath string, force bool) error {
			if force {
				forcedPaths = append(forcedPaths, worktreePath)
			}
			return nil
		},
	}

	var out bytes.Buffer
	if err := RunSpaceDelete(r, SpaceDeleteArgs{Name: "feat"}, strings.NewReader("y\n"), &out); err != nil {
		t.Fatalf("RunSpaceDelete: %v", err)
	}
	// Both repos should be removed with force=true since any warning triggers force.
	if len(forcedPaths) != 2 {
		t.Errorf("expected both repos removed with force, got %d: %v", len(forcedPaths), forcedPaths)
	}
}

func TestRunSpaceDelete_StatusError_SkippedSilently(t *testing.T) {
	// Status fails (e.g. worktree externally deleted) — no prompt, delete proceeds.
	isolateState(t)
	root := t.TempDir()
	spacePath := t.TempDir()
	makeSpace(t, "feat", "feat", spacePath, []string{"api"}, root)

	r := &testRunner{
		statusFn:         func(_ string) (git.RepoStatus, error) { return git.RepoStatus{}, fmt.Errorf("no such path") },
		worktreeRemoveFn: func(_, _ string, _ bool) error { return nil },
	}

	var out bytes.Buffer
	if err := RunSpaceDelete(r, SpaceDeleteArgs{Name: "feat"}, &bytes.Buffer{}, &out); err != nil {
		t.Fatalf("RunSpaceDelete: %v", err)
	}
	// No prompt should have been written (no warnings).
	if strings.Contains(out.String(), "[y/N]") {
		t.Errorf("should not prompt when status errors are skipped: %q", out.String())
	}
}

func TestRunSpaceDelete_EmptySpace_DeletesState(t *testing.T) {
	isolateState(t)
	spacePath := t.TempDir()
	makeSpace(t, "feat", "feat", spacePath, nil, "")

	var out bytes.Buffer
	if err := RunSpaceDelete(&testRunner{}, SpaceDeleteArgs{Name: "feat"}, &bytes.Buffer{}, &out); err != nil {
		t.Fatalf("RunSpaceDelete: %v", err)
	}
	if _, err := state.Load("feat"); err == nil {
		t.Error("state should be deleted for empty space")
	}
}

// --- error handling ---

func TestRunSpaceDelete_WorktreeRemoveError_StatePreserved(t *testing.T) {
	isolateState(t)
	root := t.TempDir()
	spacePath := t.TempDir()
	makeSpace(t, "feat", "feat", spacePath, []string{"api"}, root)

	r := deleteRunner(cleanStatus)
	r.worktreeRemoveFn = func(_, _ string, _ bool) error {
		return fmt.Errorf("locked worktree")
	}

	var out bytes.Buffer
	err := RunSpaceDelete(r, SpaceDeleteArgs{Name: "feat"}, &bytes.Buffer{}, &out)
	if err == nil {
		t.Fatal("expected error when worktree removal fails")
	}
	// State must not be deleted.
	if _, stateErr := state.Load("feat"); stateErr != nil {
		t.Error("state should be preserved when worktree removal fails")
	}
	if !strings.Contains(out.String(), "locked worktree") {
		t.Errorf("output should mention the error: %q", out.String())
	}
}
