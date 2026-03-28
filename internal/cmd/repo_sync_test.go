package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/geoffamey/wtg/internal/git"
	"github.com/geoffamey/wtg/internal/ui"
)

// syncRunner builds a testRunner configured for sync scenarios.
// statusSeq is a list of RepoStatus values returned in order on successive
// calls to Status (before fetch, after fetch).
func syncRunner(defaultBranch string, statusSeq []git.RepoStatus, fetchErr, ffErr error) *testRunner {
	callCount := 0
	return &testRunner{
		defaultBranchFn: func(string) (string, error) { return defaultBranch, nil },
		statusFn: func(string) (git.RepoStatus, error) {
			s := statusSeq[callCount]
			callCount++
			return s, nil
		},
		fetchFn:       func(string) error { return fetchErr },
		fastForwardFn: func(string, string) error { return ffErr },
	}
}

func runSync(t *testing.T, root string, runner *testRunner, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	if err := RunSync(discoverCfg(root, 2), runner, args, &out); err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	return out.String()
}

// --- syncOne ---

func TestSyncOne_UpToDate(t *testing.T) {
	root := t.TempDir()
	clean := git.RepoStatus{Branch: "main"}
	r := syncRunner("main", []git.RepoStatus{clean, clean}, nil, nil)
	sym, msg := syncOne(root, r)
	if sym != ui.SymOK {
		t.Errorf("sym: got %q, want %q", sym, ui.SymOK)
	}
	if !strings.Contains(msg, "up to date") {
		t.Errorf("msg: %q", msg)
	}
}

func TestSyncOne_FastForwarded(t *testing.T) {
	root := t.TempDir()
	before := git.RepoStatus{Branch: "main"}
	after := git.RepoStatus{Branch: "main", Behind: 3}
	r := syncRunner("main", []git.RepoStatus{before, after}, nil, nil)
	sym, msg := syncOne(root, r)
	if sym != ui.SymUp {
		t.Errorf("sym: got %q, want %q", sym, ui.SymUp)
	}
	if !strings.Contains(msg, "3 commits") {
		t.Errorf("msg: %q", msg)
	}
	if !strings.Contains(msg, "origin/main") {
		t.Errorf("msg missing branch: %q", msg)
	}
}

func TestSyncOne_FastForwarded_OneCommit(t *testing.T) {
	root := t.TempDir()
	before := git.RepoStatus{Branch: "main"}
	after := git.RepoStatus{Branch: "main", Behind: 1}
	r := syncRunner("main", []git.RepoStatus{before, after}, nil, nil)
	_, msg := syncOne(root, r)
	if !strings.Contains(msg, "1 commit") || strings.Contains(msg, "1 commits") {
		t.Errorf("singular commit: %q", msg)
	}
}

func TestSyncOne_SkippedDirty(t *testing.T) {
	root := t.TempDir()
	dirty := git.RepoStatus{
		Branch: "main",
		Files:  []git.FileStatus{{Path: "dirty.go", Index: '?', Worktree: '?'}},
	}
	r := &testRunner{
		defaultBranchFn: func(string) (string, error) { return "main", nil },
		statusFn:        func(string) (git.RepoStatus, error) { return dirty, nil },
	}
	sym, msg := syncOne(root, r)
	if sym != ui.SymWarn {
		t.Errorf("sym: got %q, want %q", sym, ui.SymWarn)
	}
	if !strings.Contains(msg, "dirty") {
		t.Errorf("msg: %q", msg)
	}
}

func TestSyncOne_SkippedWrongBranch(t *testing.T) {
	root := t.TempDir()
	r := &testRunner{
		defaultBranchFn: func(string) (string, error) { return "main", nil },
		statusFn:        func(string) (git.RepoStatus, error) { return git.RepoStatus{Branch: "feature"}, nil },
	}
	sym, msg := syncOne(root, r)
	if sym != ui.SymWarn {
		t.Errorf("sym: got %q, want %q", sym, ui.SymWarn)
	}
	if !strings.Contains(msg, "feature") || !strings.Contains(msg, "main") {
		t.Errorf("msg should mention both branches: %q", msg)
	}
}

func TestSyncOne_FetchError(t *testing.T) {
	root := t.TempDir()
	clean := git.RepoStatus{Branch: "main"}
	r := syncRunner("main", []git.RepoStatus{clean}, fmt.Errorf("network error"), nil)
	sym, msg := syncOne(root, r)
	if sym != ui.SymFail {
		t.Errorf("sym: got %q, want %q", sym, ui.SymFail)
	}
	if !strings.Contains(msg, "network error") {
		t.Errorf("msg: %q", msg)
	}
}

func TestSyncOne_FastForwardError(t *testing.T) {
	root := t.TempDir()
	before := git.RepoStatus{Branch: "main"}
	after := git.RepoStatus{Branch: "main", Behind: 2}
	r := syncRunner("main", []git.RepoStatus{before, after}, nil, fmt.Errorf("conflict"))
	sym, msg := syncOne(root, r)
	if sym != ui.SymFail {
		t.Errorf("sym: got %q, want %q", sym, ui.SymFail)
	}
	if !strings.Contains(msg, "conflict") {
		t.Errorf("msg: %q", msg)
	}
}

// --- RunSync ---

func TestRunSync_AllRepos(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "api")
	makeRepo(t, root, "frontend")

	clean := git.RepoStatus{Branch: "main"}
	calls := 0
	r := &testRunner{
		defaultBranchFn: func(string) (string, error) { return "main", nil },
		statusFn:        func(string) (git.RepoStatus, error) { calls++; return clean, nil },
		fetchFn:         func(string) error { return nil },
		fastForwardFn:   func(string, string) error { return nil },
	}

	got := runSync(t, root, r)
	if !strings.Contains(got, "api") || !strings.Contains(got, "frontend") {
		t.Errorf("output missing repo names: %q", got)
	}
}

func TestRunSync_NamedRepos(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "api")
	makeRepo(t, root, "frontend")

	clean := git.RepoStatus{Branch: "main"}
	r := &testRunner{
		defaultBranchFn: func(string) (string, error) { return "main", nil },
		statusFn:        func(string) (git.RepoStatus, error) { return clean, nil },
		fetchFn:         func(string) error { return nil },
		fastForwardFn:   func(string, string) error { return nil },
	}

	got := runSync(t, root, r, "api") // only api
	if !strings.Contains(got, "api") {
		t.Errorf("output missing api: %q", got)
	}
	if strings.Contains(got, "frontend") {
		t.Errorf("output should not contain frontend: %q", got)
	}
}

func TestRunSync_UnknownRepo(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	err := RunSync(discoverCfg(root, 2), &testRunner{}, []string{"no-such-repo"}, &out)
	if err == nil {
		t.Fatal("expected error for unknown repo")
	}
}

func TestRunSync_NoRootDir(t *testing.T) {
	var out bytes.Buffer
	err := RunSync(discoverCfg("", 2), &testRunner{}, nil, &out)
	if err == nil {
		t.Fatal("expected error when root_dir is empty")
	}
}
