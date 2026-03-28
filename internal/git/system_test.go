package git_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/geoffamey/wtg/internal/git"
	"github.com/geoffamey/wtg/internal/git/testhelper"
)

func runner() *git.SystemRunner { return git.New() }

// --- WorktreeList / WorktreeAdd / WorktreeRemove ---

func TestWorktreeAdd_NewBranch(t *testing.T) {
	repo := testhelper.Init(t)
	r := runner()

	wtPath := filepath.Join(t.TempDir(), "wt")
	if err := r.WorktreeAdd(repo.Path, wtPath, "feature", true); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}

	wts, err := r.WorktreeList(repo.Path)
	if err != nil {
		t.Fatalf("WorktreeList: %v", err)
	}

	resolvedWtPath, _ := filepath.EvalSymlinks(wtPath)
	var found bool
	for _, wt := range wts {
		resolvedPath, _ := filepath.EvalSymlinks(wt.Path)
		if resolvedPath == resolvedWtPath && wt.Branch == "feature" {
			found = true
		}
	}
	if !found {
		t.Errorf("worktree not found in list: %+v", wts)
	}
}

func TestWorktreeAdd_ExistingBranch(t *testing.T) {
	repo := testhelper.Init(t)
	r := runner()
	repo.CreateBranch("existing")

	wtPath := filepath.Join(t.TempDir(), "wt")
	if err := r.WorktreeAdd(repo.Path, wtPath, "existing", false); err != nil {
		t.Fatalf("WorktreeAdd existing branch: %v", err)
	}

	wts, err := r.WorktreeList(repo.Path)
	if err != nil {
		t.Fatalf("WorktreeList: %v", err)
	}
	var found bool
	for _, wt := range wts {
		if wt.Branch == "existing" {
			found = true
		}
	}
	if !found {
		t.Error("worktree for existing branch not found")
	}
}

func TestWorktreeRemove(t *testing.T) {
	repo := testhelper.Init(t)
	r := runner()

	wtPath := filepath.Join(t.TempDir(), "wt")
	if err := r.WorktreeAdd(repo.Path, wtPath, "feature", true); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	if err := r.WorktreeRemove(repo.Path, wtPath, false); err != nil {
		t.Fatalf("WorktreeRemove: %v", err)
	}

	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Error("worktree directory still exists after remove")
	}
}

func TestWorktreeRemove_Force(t *testing.T) {
	repo := testhelper.Init(t)
	r := runner()

	wtPath := filepath.Join(t.TempDir(), "wt")
	if err := r.WorktreeAdd(repo.Path, wtPath, "feature", true); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	// Write an untracked file to make the worktree dirty, requiring --force.
	testhelper.RepoAt(t, wtPath).WriteFile("dirty.go", "package main\n")

	if err := r.WorktreeRemove(repo.Path, wtPath, true); err != nil {
		t.Fatalf("WorktreeRemove(force): %v", err)
	}
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Error("worktree directory still exists after force remove")
	}
}

func TestWorktreeList_MainWorktree(t *testing.T) {
	repo := testhelper.Init(t)
	wts, err := runner().WorktreeList(repo.Path)
	if err != nil {
		t.Fatalf("WorktreeList: %v", err)
	}
	if len(wts) != 1 {
		t.Fatalf("got %d worktrees, want 1", len(wts))
	}
	// git resolves symlinks in worktree paths; use EvalSymlinks to compare fairly
	// (on macOS /var is a symlink to /private/var).
	gotPath, _ := filepath.EvalSymlinks(wts[0].Path)
	wantPath, _ := filepath.EvalSymlinks(repo.Path)
	if gotPath != wantPath {
		t.Errorf("Path: got %q, want %q", wts[0].Path, repo.Path)
	}
	if wts[0].Branch != "main" {
		t.Errorf("Branch: got %q, want %q", wts[0].Branch, "main")
	}
}

// --- BranchExists / BranchDelete / BranchMerged ---

func TestBranchExists(t *testing.T) {
	repo := testhelper.Init(t)
	r := runner()

	exists, err := r.BranchExists(repo.Path, "main")
	if err != nil {
		t.Fatalf("BranchExists(main): %v", err)
	}
	if !exists {
		t.Error("main branch should exist")
	}

	exists, err = r.BranchExists(repo.Path, "no-such-branch")
	if err != nil {
		t.Fatalf("BranchExists(no-such-branch): %v", err)
	}
	if exists {
		t.Error("no-such-branch should not exist")
	}
}

func TestBranchDelete(t *testing.T) {
	repo := testhelper.Init(t)
	r := runner()
	repo.CreateBranch("to-delete")

	if err := r.BranchDelete(repo.Path, "to-delete", false); err != nil {
		t.Fatalf("BranchDelete: %v", err)
	}

	exists, err := r.BranchExists(repo.Path, "to-delete")
	if err != nil {
		t.Fatalf("BranchExists after delete: %v", err)
	}
	if exists {
		t.Error("branch still exists after delete")
	}
}

func TestBranchDelete_Force(t *testing.T) {
	repo := testhelper.Init(t)
	r := runner()

	// Add a commit on a worktree so the branch is unmerged, requiring -D.
	wtPath := filepath.Join(t.TempDir(), "wt")
	if err := r.WorktreeAdd(repo.Path, wtPath, "unmerged", true); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	testhelper.RepoAt(t, wtPath).Commit("unmerged commit")
	if err := r.WorktreeRemove(repo.Path, wtPath, false); err != nil {
		t.Fatalf("WorktreeRemove: %v", err)
	}

	// Safe delete should fail (unmerged).
	if err := r.BranchDelete(repo.Path, "unmerged", false); err == nil {
		t.Fatal("expected error for unmerged branch with -d")
	}
	// Force delete should succeed.
	if err := r.BranchDelete(repo.Path, "unmerged", true); err != nil {
		t.Fatalf("BranchDelete(force): %v", err)
	}
	exists, err := r.BranchExists(repo.Path, "unmerged")
	if err != nil {
		t.Fatalf("BranchExists: %v", err)
	}
	if exists {
		t.Error("branch still exists after force delete")
	}
}

func TestBranchMerged(t *testing.T) {
	repo := testhelper.Init(t)
	r := runner()

	// A branch created from HEAD is immediately merged (its tip == HEAD).
	repo.CreateBranch("merged-branch")
	merged, err := r.BranchMerged(repo.Path, "merged-branch")
	if err != nil {
		t.Fatalf("BranchMerged(merged-branch): %v", err)
	}
	if !merged {
		t.Error("branch at HEAD should be considered merged")
	}
}

func TestBranchMerged_NotMerged(t *testing.T) {
	repo := testhelper.Init(t)
	r := runner()

	// Create a worktree on a new branch, add a commit — branch is ahead of main.
	wtPath := filepath.Join(t.TempDir(), "wt")
	if err := r.WorktreeAdd(repo.Path, wtPath, "ahead-branch", true); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	wt := testhelper.RepoAt(t, wtPath)
	wt.Commit("extra commit")

	merged, err := r.BranchMerged(repo.Path, "ahead-branch")
	if err != nil {
		t.Fatalf("BranchMerged(ahead-branch): %v", err)
	}
	if merged {
		t.Error("branch with unmerged commits should not be considered merged")
	}
}

// --- Status ---

func TestStatus_Clean(t *testing.T) {
	repo := testhelper.Init(t)
	s, err := runner().Status(repo.Path)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if s.Branch != "main" {
		t.Errorf("Branch: %q", s.Branch)
	}
	if len(s.Files) != 0 {
		t.Errorf("expected no files, got %d", len(s.Files))
	}
}

func TestStatus_Dirty(t *testing.T) {
	repo := testhelper.Init(t)
	repo.WriteFile("dirty.go", "package main\n")

	s, err := runner().Status(repo.Path)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(s.Files) != 1 {
		t.Fatalf("got %d files, want 1", len(s.Files))
	}
	if s.Files[0].Path != "dirty.go" {
		t.Errorf("Files[0].Path: %q", s.Files[0].Path)
	}
	if s.Files[0].Index != '?' || s.Files[0].Worktree != '?' {
		t.Errorf("XY: %c%c, want ??", s.Files[0].Index, s.Files[0].Worktree)
	}
}

// --- DefaultBranch / Fetch / FastForward ---

func TestDefaultBranch(t *testing.T) {
	local, _ := testhelper.InitWithRemote(t)
	branch, err := runner().DefaultBranch(local.Path)
	if err != nil {
		t.Fatalf("DefaultBranch: %v", err)
	}
	if branch != "main" {
		t.Errorf("DefaultBranch: got %q, want %q", branch, "main")
	}
}

func TestFetchAndFastForward(t *testing.T) {
	local, remote := testhelper.InitWithRemote(t)
	r := runner()

	// Push a new commit to remote via a second clone.
	second := testhelper.RepoAt(t, t.TempDir())
	second.GitCmd("clone", remote.Path, ".")
	second.GitCmd("config", "user.email", "test@example.com")
	second.GitCmd("config", "user.name", "Test")
	second.Commit("remote-commit")
	second.GitCmd("push", "origin", "main")

	if err := r.Fetch(local.Path); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if err := r.FastForward(local.Path, "main"); err != nil {
		t.Fatalf("FastForward: %v", err)
	}

	s, err := r.Status(local.Path)
	if err != nil {
		t.Fatalf("Status after fast-forward: %v", err)
	}
	if s.Ahead != 0 || s.Behind != 0 {
		t.Errorf("expected ahead=0 behind=0, got %d/%d", s.Ahead, s.Behind)
	}
}

// --- RemoteURL ---

func TestRemoteURL(t *testing.T) {
	local, remote := testhelper.InitWithRemote(t)
	url, err := runner().RemoteURL(local.Path, "origin")
	if err != nil {
		t.Fatalf("RemoteURL: %v", err)
	}
	if url != remote.Path {
		t.Errorf("RemoteURL: got %q, want %q", url, remote.Path)
	}
}

// --- WorktreeRepair ---

func TestWorktreeRepair_Supported(t *testing.T) {
	repo := testhelper.Init(t)
	// On any supported git version (≥ 2.29), repair with no args should be a no-op.
	err := runner().WorktreeRepair(repo.Path)
	if err != nil && !errors.Is(err, git.ErrRepairUnsupported) {
		t.Fatalf("WorktreeRepair: %v", err)
	}
}
