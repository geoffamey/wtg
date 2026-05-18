package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/geoffamey/wtg/internal/git"
	"github.com/geoffamey/wtg/internal/ui"
)

func statusRunner(st git.RepoStatus, defaultBranch string) *testRunner {
	return &testRunner{
		statusFn:        func(string) (git.RepoStatus, error) { return st, nil },
		defaultBranchFn: func(string) (string, error) { return defaultBranch, nil },
	}
}

// --- branchCol ---

func TestBranchCol_DefaultBranch(t *testing.T) {
	got := branchCol("main", "main")
	if !strings.Contains(got, "main") {
		t.Errorf("missing branch name: %q", got)
	}
	// Should use muted style (same content as Muted.Render)
	if got != ui.Muted.Render("[main]") {
		t.Errorf("expected muted style: %q", got)
	}
}

func TestBranchCol_NonDefault(t *testing.T) {
	got := branchCol("feature", "main")
	if got != ui.Fail.Render("[feature]") {
		t.Errorf("expected fail style for non-default branch: %q", got)
	}
}

func TestBranchCol_NoRemote(t *testing.T) {
	// defaultBranch empty (no remote) — branch shown as muted, not warned
	got := branchCol("main", "")
	if got != ui.Muted.Render("[main]") {
		t.Errorf("expected muted when no remote: %q", got)
	}
}

func TestBranchCol_Detached(t *testing.T) {
	got := branchCol("", "main")
	if !strings.Contains(got, "detached") {
		t.Errorf("expected detached label: %q", got)
	}
}

// --- statusCol ---

func TestStatusCol_Clean(t *testing.T) {
	got := statusCol(nil)
	if got != ui.OK.Render(ui.SymOK+" clean") {
		t.Errorf("clean: %q", got)
	}
}

func TestStatusCol_Modified(t *testing.T) {
	files := []git.FileStatus{
		{Path: "a.go", Index: 'M', Worktree: '.'},
		{Path: "b.go", Index: '.', Worktree: 'M'},
	}
	got := statusCol(files)
	if !strings.Contains(got, "2 modified") {
		t.Errorf("expected 2 modified: %q", got)
	}
}

func TestStatusCol_Untracked(t *testing.T) {
	files := []git.FileStatus{
		{Path: "new.go", Index: '?', Worktree: '?'},
	}
	got := statusCol(files)
	if !strings.Contains(got, "1 untracked") {
		t.Errorf("expected 1 untracked: %q", got)
	}
}

func TestStatusCol_Mixed(t *testing.T) {
	files := []git.FileStatus{
		{Path: "a.go", Index: 'M', Worktree: '.'},
		{Path: "new.go", Index: '?', Worktree: '?'},
		{Path: "b.go", Index: '?', Worktree: '?'},
	}
	got := statusCol(files)
	if !strings.Contains(got, "1 modified") {
		t.Errorf("expected 1 modified: %q", got)
	}
	if !strings.Contains(got, "2 untracked") {
		t.Errorf("expected 2 untracked: %q", got)
	}
}

// --- aheadBehindCol ---

func TestAheadBehindCol_NoUpstream(t *testing.T) {
	if got := aheadBehindCol(0, 0, false, false); got != ui.Muted.Render("(local)") {
		t.Errorf("expected (local) without upstream, got %q", got)
	}
}

func TestAheadBehindCol_Merged(t *testing.T) {
	if got := aheadBehindCol(0, 0, false, true); got != ui.Muted.Render("(merged)") {
		t.Errorf("expected (merged), got %q", got)
	}
}

func TestAheadBehindCol_Clean(t *testing.T) {
	if got := aheadBehindCol(0, 0, true, false); got != "" {
		t.Errorf("expected empty for in-sync branch, got %q", got)
	}
}

func TestAheadBehindCol_Behind(t *testing.T) {
	got := aheadBehindCol(0, 3, true, false)
	// ↓3 should be warn-coloured
	if !strings.Contains(got, ui.Warn.Render("↓3")) {
		t.Errorf("expected warn-coloured ↓3: %q", got)
	}
}

func TestAheadBehindCol_Diverged(t *testing.T) {
	got := aheadBehindCol(2, 3, true, false)
	// Both should be fail-coloured
	if !strings.Contains(got, ui.Fail.Render("↑2 ↓3")) {
		t.Errorf("expected fail-coloured diverged: %q", got)
	}
}

// --- isMergedIntoRemote ---

func TestIsMergedIntoRemote_RemoteGone(t *testing.T) {
	// Remote ref absent → merged.
	r := &testRunner{
		remoteBranchExistsFn: func(_, _ string) (bool, error) { return false, nil },
	}
	if !isMergedIntoRemote(r, "/repo", "feat") {
		t.Error("expected merged=true when remote ref is gone")
	}
}

func TestIsMergedIntoRemote_RemoteExistsAndMerged(t *testing.T) {
	// Remote ref present and is ancestor of HEAD → merged.
	r := &testRunner{
		remoteBranchExistsFn: func(_, _ string) (bool, error) { return true, nil },
		branchMergedFn:       func(_, _ string) (bool, error) { return true, nil },
	}
	if !isMergedIntoRemote(r, "/repo", "feat") {
		t.Error("expected merged=true when remote ref is ancestor of HEAD")
	}
}

func TestIsMergedIntoRemote_RemoteExistsNotMerged(t *testing.T) {
	// Remote ref present but not yet merged.
	r := &testRunner{
		remoteBranchExistsFn: func(_, _ string) (bool, error) { return true, nil },
		branchMergedFn:       func(_, _ string) (bool, error) { return false, nil },
	}
	if isMergedIntoRemote(r, "/repo", "feat") {
		t.Error("expected merged=false when remote ref exists and is not ancestor")
	}
}

// --- RunRepoStatus ---

func TestRunRepoStatus_Output(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "api")
	makeRepo(t, root, "myorg/frontend")

	r := statusRunner(git.RepoStatus{Branch: "main", Upstream: "origin/main"}, "main")

	var out bytes.Buffer
	if err := RunRepoStatus(discoverCfg(root, 2), r, nil, false, &out); err != nil {
		t.Fatalf("RunRepoStatus: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "api") {
		t.Errorf("missing api: %q", got)
	}
	if !strings.Contains(got, "myorg/frontend") {
		t.Errorf("missing myorg/frontend: %q", got)
	}
}

func TestRunRepoStatus_NamedRepo(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "api")
	makeRepo(t, root, "other")

	r := statusRunner(git.RepoStatus{Branch: "main"}, "main")

	var out bytes.Buffer
	if err := RunRepoStatus(discoverCfg(root, 2), r, []string{"api"}, false, &out); err != nil {
		t.Fatalf("RunRepoStatus: %v", err)
	}
	got := out.String()
	if strings.Contains(got, "other") {
		t.Errorf("output should not contain 'other': %q", got)
	}
}

func TestRunRepoStatus_NoRootDir(t *testing.T) {
	var out bytes.Buffer
	if err := RunRepoStatus(discoverCfg("", 2), &testRunner{}, nil, false, &out); err == nil {
		t.Fatal("expected error for empty root_dir")
	}
}

func TestRunRepoStatus_Long(t *testing.T) {
	root := t.TempDir()
	apiPath := makeRepo(t, root, "api")

	r := &testRunner{
		statusFn: func(string) (git.RepoStatus, error) {
			return git.RepoStatus{Branch: "main", Upstream: "origin/main"}, nil
		},
		defaultBranchFn: func(string) (string, error) { return "main", nil },
		remoteURLFn: func(repoPath, remote string) (string, error) {
			return "https://github.com/org/api.git", nil
		},
	}

	var out bytes.Buffer
	if err := RunRepoStatus(discoverCfg(root, 2), r, nil, true, &out); err != nil {
		t.Fatalf("RunRepoStatus: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "https://github.com/org/api.git") {
		t.Errorf("long output missing remote URL: %q", got)
	}
	if !strings.Contains(got, apiPath) {
		t.Errorf("long output missing local path: %q", got)
	}
}
