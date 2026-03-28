package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/geoffamey/wtg/internal/git"
	"github.com/geoffamey/wtg/internal/state"
	"github.com/geoffamey/wtg/internal/ui"
)

// statusSpace saves a space to state with the given repos.
func statusSpace(t *testing.T, name, branch, spacePath string, repos []string, repoRoot string) *state.Space {
	t.Helper()
	sp := &state.Space{
		Name:      name,
		Branch:    branch,
		Path:      spacePath,
		CreatedAt: time.Now(),
	}
	for _, n := range repos {
		sp.Repos = append(sp.Repos, state.RepoEntry{
			Name:         n,
			RepoPath:     repoRoot + "/" + n,
			WorktreePath: spacePath + "/" + n,
		})
	}
	if err := state.Save(sp); err != nil {
		t.Fatalf("statusSpace save: %v", err)
	}
	return sp
}

func alwaysStatus(st git.RepoStatus) func(string) (git.RepoStatus, error) {
	return func(_ string) (git.RepoStatus, error) { return st, nil }
}

// --- no spaces ---

func TestRunStatus_NoSpaces(t *testing.T) {
	isolateState(t)
	var out bytes.Buffer
	if err := RunStatus(&testRunner{}, nil, false, &out); err != nil {
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
	statusSpace(t, "feat", "geoff/feat", sp, []string{"api"}, "/repos")

	r := &testRunner{statusFn: alwaysStatus(git.RepoStatus{Branch: "geoff/feat", Upstream: "origin/geoff/feat"})}
	var out bytes.Buffer
	if err := RunStatus(r, []string{"feat"}, false, &out); err != nil {
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
	statusSpace(t, "feat", "geoff/feat", sp, []string{"api"}, "/repos")

	r := &testRunner{statusFn: alwaysStatus(git.RepoStatus{Branch: "geoff/feat"})}
	var out bytes.Buffer
	if err := RunStatus(r, []string{"feat"}, false, &out); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	want := ui.Muted.Render("[geoff/feat]")
	if !strings.Contains(out.String(), want) {
		t.Errorf("worktree on space branch should be muted: %q", out.String())
	}
}

func TestRunStatus_WrongBranch_ShowsWarn(t *testing.T) {
	isolateState(t)
	sp := t.TempDir()
	statusSpace(t, "feat", "geoff/feat", sp, []string{"api"}, "/repos")

	r := &testRunner{statusFn: alwaysStatus(git.RepoStatus{Branch: "other-branch"})}
	var out bytes.Buffer
	if err := RunStatus(r, []string{"feat"}, false, &out); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	want := ui.Warn.Render("[other-branch]")
	if !strings.Contains(out.String(), want) {
		t.Errorf("worktree on wrong branch should be warned: %q", out.String())
	}
}

func TestRunStatus_DirtyWorktree(t *testing.T) {
	isolateState(t)
	sp := t.TempDir()
	statusSpace(t, "feat", "geoff/feat", sp, []string{"api"}, "/repos")

	dirty := git.RepoStatus{
		Branch: "geoff/feat",
		Files:  []git.FileStatus{{Path: "main.go", Index: 'M', Worktree: '.'}},
	}
	r := &testRunner{statusFn: alwaysStatus(dirty)}
	var out bytes.Buffer
	if err := RunStatus(r, []string{"feat"}, false, &out); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	if !strings.Contains(out.String(), "modified") {
		t.Errorf("output should mention modified files: %q", out.String())
	}
}

func TestRunStatus_StatusError_ShowsFailRow(t *testing.T) {
	isolateState(t)
	sp := t.TempDir()
	statusSpace(t, "feat", "geoff/feat", sp, []string{"api"}, "/repos")

	r := &testRunner{statusFn: func(_ string) (git.RepoStatus, error) {
		return git.RepoStatus{}, fmt.Errorf("worktree missing")
	}}
	var out bytes.Buffer
	if err := RunStatus(r, []string{"feat"}, false, &out); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	if !strings.Contains(out.String(), "worktree missing") {
		t.Errorf("output should show status error: %q", out.String())
	}
}

// --- named spaces ---

func TestRunStatus_NamedSpaces(t *testing.T) {
	isolateState(t)
	statusSpace(t, "feat", "feat", t.TempDir(), []string{"api"}, "/repos")
	statusSpace(t, "other", "other", t.TempDir(), []string{"api"}, "/repos")

	r := &testRunner{statusFn: alwaysStatus(git.RepoStatus{Branch: "feat"})}
	var out bytes.Buffer
	if err := RunStatus(r, []string{"feat"}, false, &out); err != nil {
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
	err := RunStatus(&testRunner{}, []string{"nonexistent"}, false, &out)
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
	statusSpace(t, "zebra", "zebra", t.TempDir(), []string{"api"}, "/repos")
	statusSpace(t, "alpha", "alpha", t.TempDir(), []string{"api"}, "/repos")

	r := &testRunner{statusFn: alwaysStatus(git.RepoStatus{Branch: "feat"})}
	var out bytes.Buffer
	if err := RunStatus(r, nil, false, &out); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	got := out.String()
	if strings.Index(got, "alpha") > strings.Index(got, "zebra") {
		t.Errorf("spaces should be sorted alphabetically:\n%s", got)
	}
}

// --- summary vs detail ---

func TestRunStatus_NoArg_OutsideSpace_ShowsSummary(t *testing.T) {
	isolateState(t)
	// Use a path that does not contain the test's CWD.
	statusSpace(t, "feat", "geoff/feat", "/nonexistent/path/feat", []string{"api"}, "/repos")

	r := &testRunner{statusFn: alwaysStatus(git.RepoStatus{Branch: "geoff/feat"})}
	var out bytes.Buffer
	if err := RunStatus(r, nil, false, &out); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	// Summary should show the space name but no per-repo branch column.
	got := out.String()
	if !strings.Contains(got, "feat") {
		t.Errorf("summary missing space name: %q", got)
	}
	// Summary does not run git, so no branch column rendered by worktreeStatusCols.
	if strings.Contains(got, "[geoff/feat]") {
		t.Errorf("summary should not show per-repo branch column: %q", got)
	}
}

func TestRunStatus_NoArg_InsideSpace_ShowsDetail(t *testing.T) {
	isolateState(t)
	dir := t.TempDir()
	t.Chdir(dir)
	statusSpace(t, "feat", "geoff/feat", dir, []string{"api"}, "/repos")

	r := &testRunner{statusFn: alwaysStatus(git.RepoStatus{Branch: "geoff/feat"})}
	var out bytes.Buffer
	if err := RunStatus(r, nil, false, &out); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	// Detail view runs git and shows per-repo branch column.
	want := ui.Muted.Render("[geoff/feat]")
	if !strings.Contains(out.String(), want) {
		t.Errorf("detail view should show per-repo branch column: %q", out.String())
	}
}

// --- --detailed flag ---

func TestRunStatus_Detailed_ShowsFiles(t *testing.T) {
	isolateState(t)
	sp := t.TempDir()
	statusSpace(t, "feat", "geoff/feat", sp, []string{"api"}, "/repos")

	dirty := git.RepoStatus{
		Branch: "geoff/feat",
		Files: []git.FileStatus{
			{Path: "main.go", Index: 'M', Worktree: '.'},
			{Path: "new.go", Index: '?', Worktree: '?'},
		},
	}
	r := &testRunner{statusFn: alwaysStatus(dirty)}
	var out bytes.Buffer
	if err := RunStatus(r, []string{"feat"}, true, &out); err != nil {
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
	statusSpace(t, "feat", "geoff/feat", sp, []string{"api"}, "/repos")

	r := &testRunner{statusFn: alwaysStatus(git.RepoStatus{Branch: "geoff/feat"})}
	var out bytes.Buffer
	if err := RunStatus(r, []string{"feat"}, true, &out); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	// Clean repo should produce no file lines (no extra indented content).
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	// Expect: 1 header line + 1 repo row = 2 lines total.
	if len(lines) != 2 {
		t.Errorf("clean repo in detailed mode should produce 2 lines, got %d:\n%s", len(lines), out.String())
	}
}
