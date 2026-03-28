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
	if err := RunStatus(&testRunner{}, nil, &out); err != nil {
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
	if err := RunStatus(r, nil, &out); err != nil {
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
	if err := RunStatus(r, nil, &out); err != nil {
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
	if err := RunStatus(r, nil, &out); err != nil {
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
	if err := RunStatus(r, nil, &out); err != nil {
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
	if err := RunStatus(r, nil, &out); err != nil {
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
	if err := RunStatus(r, []string{"feat"}, &out); err != nil {
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
	err := RunStatus(&testRunner{}, []string{"nonexistent"}, &out)
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
	if err := RunStatus(r, nil, &out); err != nil {
		t.Fatalf("RunStatus: %v", err)
	}
	got := out.String()
	if strings.Index(got, "alpha") > strings.Index(got, "zebra") {
		t.Errorf("spaces should be sorted alphabetically:\n%s", got)
	}
	// A blank line should separate the two space blocks.
	if !strings.Contains(got, "\n\n") {
		t.Errorf("spaces should be separated by a blank line:\n%s", got)
	}
}
