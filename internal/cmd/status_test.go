package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/geoffamey/wtg/internal/git"
	"github.com/geoffamey/wtg/internal/ui"
)

func alwaysStatus(st git.RepoStatus) func(string) (git.RepoStatus, error) {
	return func(_ string) (git.RepoStatus, error) { return st, nil }
}

// --- no spaces ---

func TestRunStatus_NoSpaces(t *testing.T) {
	isolateState(t)
	var out bytes.Buffer
	if err := RunSpaceStatus(&testRunner{}, nil, false, &out); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	if out.String() != "" {
		t.Errorf("expected empty output with no spaces, got %q", out.String())
	}
}

// --- single space ---

func TestRunStatus_ShowsSpaceHeader(t *testing.T) {
	isolateState(t)
	sp := t.TempDir()
	makeSpace(t, "feat", "geoff/feat", sp, []string{"api"}, "/repos")

	r := &testRunner{statusFn: alwaysStatus(git.RepoStatus{Branch: "geoff/feat", Upstream: "origin/geoff/feat"})}
	var out bytes.Buffer
	if err := RunSpaceStatus(r, []string{"feat"}, false, &out); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "feat") {
		t.Errorf("output missing space name: %q", got)
	}
	if !strings.Contains(got, "geoff/feat") {
		t.Errorf("output missing branch: %q", got)
	}
	if !strings.Contains(got, "api") {
		t.Errorf("output missing repo name: %q", got)
	}
}

func TestRunStatus_OnSpaceBranch_ShowsMuted(t *testing.T) {
	isolateState(t)
	sp := t.TempDir()
	makeSpace(t, "feat", "geoff/feat", sp, []string{"api"}, "/repos")

	r := &testRunner{statusFn: alwaysStatus(git.RepoStatus{Branch: "geoff/feat"})}
	var out bytes.Buffer
	if err := RunSpaceStatus(r, []string{"feat"}, false, &out); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	want := ui.Muted.Render("[geoff/feat]")
	if !strings.Contains(out.String(), want) {
		t.Errorf("worktree on space branch should be muted: %q", out.String())
	}
}

func TestRunStatus_WrongBranch_ShowsFail(t *testing.T) {
	isolateState(t)
	sp := t.TempDir()
	makeSpace(t, "feat", "geoff/feat", sp, []string{"api"}, "/repos")

	r := &testRunner{statusFn: alwaysStatus(git.RepoStatus{Branch: "other-branch"})}
	var out bytes.Buffer
	if err := RunSpaceStatus(r, []string{"feat"}, false, &out); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	want := ui.Fail.Render("[other-branch]")
	if !strings.Contains(out.String(), want) {
		t.Errorf("worktree on wrong branch should be shown as fail: %q", out.String())
	}
}

func TestRunStatus_DirtyWorktree(t *testing.T) {
	isolateState(t)
	sp := t.TempDir()
	makeSpace(t, "feat", "geoff/feat", sp, []string{"api"}, "/repos")

	dirty := git.RepoStatus{
		Branch: "geoff/feat",
		Files:  []git.FileStatus{{Path: "main.go", Index: 'M', Worktree: '.'}},
	}
	r := &testRunner{statusFn: alwaysStatus(dirty)}
	var out bytes.Buffer
	if err := RunSpaceStatus(r, []string{"feat"}, false, &out); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	if !strings.Contains(out.String(), "modified") {
		t.Errorf("output should mention modified files: %q", out.String())
	}
}

func TestRunStatus_StatusError_ShowsFailRow(t *testing.T) {
	isolateState(t)
	sp := t.TempDir()
	makeSpace(t, "feat", "geoff/feat", sp, []string{"api"}, "/repos")

	r := &testRunner{statusFn: func(_ string) (git.RepoStatus, error) {
		return git.RepoStatus{}, fmt.Errorf("worktree missing")
	}}
	var out bytes.Buffer
	if err := RunSpaceStatus(r, []string{"feat"}, false, &out); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	if !strings.Contains(out.String(), "worktree missing") {
		t.Errorf("output should show status error: %q", out.String())
	}
}

// --- named spaces ---

func TestRunStatus_NamedSpaces(t *testing.T) {
	isolateState(t)
	makeSpace(t, "feat", "feat", t.TempDir(), []string{"api"}, "/repos")
	makeSpace(t, "other", "other", t.TempDir(), []string{"api"}, "/repos")

	r := &testRunner{statusFn: alwaysStatus(git.RepoStatus{Branch: "feat"})}
	var out bytes.Buffer
	if err := RunSpaceStatus(r, []string{"feat"}, false, &out); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "feat") {
		t.Errorf("missing requested space: %q", got)
	}
	if strings.Contains(got, "other") {
		t.Errorf("output should not include unrequested space: %q", got)
	}
}

func TestRunStatus_UnknownSpace_Error(t *testing.T) {
	isolateState(t)
	var out bytes.Buffer
	err := RunSpaceStatus(&testRunner{}, []string{"nonexistent"}, false, &out)
	if err == nil {
		t.Fatal("expected error for unknown space")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention space name: %v", err)
	}
}

// --- multiple spaces ---

func TestRunStatus_MultipleSpaces_SortedAndSeparated(t *testing.T) {
	isolateState(t)
	makeSpace(t, "zebra", "zebra", t.TempDir(), []string{"api"}, "/repos")
	makeSpace(t, "alpha", "alpha", t.TempDir(), []string{"api"}, "/repos")

	r := &testRunner{statusFn: alwaysStatus(git.RepoStatus{Branch: "feat"})}
	var out bytes.Buffer
	if err := RunSpaceStatus(r, nil, false, &out); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	got := out.String()
	if strings.Index(got, "alpha") > strings.Index(got, "zebra") {
		t.Errorf("spaces should be sorted alphabetically:\n%s", got)
	}
}

// --- summary vs detail ---

func TestRunStatus_NoArg_OutsideSpace_ShowsAllSpaces(t *testing.T) {
	isolateState(t)
	// Use a path that does not contain the test's CWD.
	makeSpace(t, "feat", "geoff/feat", "/nonexistent/path/feat", []string{"api"}, "/repos")

	r := &testRunner{statusFn: alwaysStatus(git.RepoStatus{Branch: "geoff/feat"})}
	var out bytes.Buffer
	if err := RunSpaceStatus(r, nil, false, &out); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "feat") {
		t.Errorf("missing space name: %q", got)
	}
	// Full detail is shown, so the per-repo branch column is present.
	if !strings.Contains(got, ui.Muted.Render("[geoff/feat]")) {
		t.Errorf("should show per-repo branch column: %q", got)
	}
}

func TestRunStatus_NoArg_InsideSpace_ShowsDetail(t *testing.T) {
	isolateState(t)
	dir := t.TempDir()
	t.Chdir(dir)
	makeSpace(t, "feat", "geoff/feat", dir, []string{"api"}, "/repos")
	makeSpace(t, "other", "other", t.TempDir(), []string{"svc"}, "/repos")

	r := &testRunner{statusFn: alwaysStatus(git.RepoStatus{Branch: "geoff/feat"})}
	var out bytes.Buffer
	if err := RunSpaceStatus(r, nil, false, &out); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	got := out.String()
	// Both spaces should be shown.
	if !strings.Contains(got, "feat") {
		t.Errorf("current space should be shown: %q", got)
	}
	if !strings.Contains(got, "other") {
		t.Errorf("other space should also be shown: %q", got)
	}
	// Current space (CWD inside it) should appear before the other.
	if strings.Index(got, "feat") > strings.Index(got, "other") {
		t.Errorf("current space should appear before other spaces:\n%s", got)
	}
}

// --- merged branch detection ---

func TestRunStatus_MergedBranch_ShowsMerged(t *testing.T) {
	isolateState(t)
	sp := t.TempDir()
	makeSpace(t, "feat", "geoff/feat", sp, []string{"api"}, "/repos")

	r := &testRunner{
		// No upstream → triggers merged check.
		statusFn:             alwaysStatus(git.RepoStatus{Branch: "geoff/feat"}),
		remoteBranchExistsFn: func(_, _ string) (bool, error) { return false, nil }, // remote gone
	}
	var out bytes.Buffer
	if err := RunSpaceStatus(r, []string{"feat"}, false, &out); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	if !strings.Contains(out.String(), ui.Muted.Render("(merged)")) {
		t.Errorf("merged branch should show (merged): %q", out.String())
	}
}

// --- --detailed flag ---

func TestRunStatus_Detailed_ShowsFiles(t *testing.T) {
	isolateState(t)
	sp := t.TempDir()
	makeSpace(t, "feat", "geoff/feat", sp, []string{"api"}, "/repos")

	dirty := git.RepoStatus{
		Branch: "geoff/feat",
		Files: []git.FileStatus{
			{Path: "main.go", Index: 'M', Worktree: '.'},
			{Path: "new.go", Index: '?', Worktree: '?'},
		},
	}
	r := &testRunner{statusFn: alwaysStatus(dirty)}
	var out bytes.Buffer
	if err := RunSpaceStatus(r, []string{"feat"}, true, &out); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "main.go") {
		t.Errorf("detailed output should list modified file: %q", got)
	}
	if !strings.Contains(got, "new.go") {
		t.Errorf("detailed output should list untracked file: %q", got)
	}
}

func TestRunStatus_Detailed_CleanRepo_NoFileLines(t *testing.T) {
	isolateState(t)
	sp := t.TempDir()
	makeSpace(t, "feat", "geoff/feat", sp, []string{"api"}, "/repos")

	r := &testRunner{statusFn: alwaysStatus(git.RepoStatus{Branch: "geoff/feat"})}
	var out bytes.Buffer
	if err := RunSpaceStatus(r, []string{"feat"}, true, &out); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	// Clean repo should produce no file lines (no extra indented content).
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	// Expect: 1 header line + 1 repo row = 2 lines total.
	if len(lines) != 2 {
		t.Errorf("clean repo in detailed mode should produce 2 lines, got %d:\n%s", len(lines), out.String())
	}
}
