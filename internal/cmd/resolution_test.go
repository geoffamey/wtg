package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- repoInSet ---

func TestRepoInSet_ExactMatchWins(t *testing.T) {
	names := []string{"org/api", "api"}
	got, ok, err := repoInSet(names, "org/api")
	if err != nil || !ok || got != "org/api" {
		t.Fatalf("repoInSet(org/api) = %q, ok=%v, err=%v", got, ok, err)
	}
}

func TestRepoInSet_UniqueBasename(t *testing.T) {
	names := []string{"github.com/suhlig/dspictl", "github.com/speisehof/caterbill"}
	got, ok, err := repoInSet(names, "dspictl")
	if err != nil || !ok || got != "github.com/suhlig/dspictl" {
		t.Fatalf("repoInSet(dspictl) = %q, ok=%v, err=%v", got, ok, err)
	}
}

func TestRepoInSet_AmbiguousBasename(t *testing.T) {
	names := []string{"aaa/dup", "bbb/dup"}
	got, ok, err := repoInSet(names, "dup")
	if ok || got != "" {
		t.Fatalf("expected no match, got %q ok=%v", got, ok)
	}
	var ae *ambiguousRepoError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *ambiguousRepoError, got %T", err)
	}
	if !strings.Contains(err.Error(), "ambiguous") || !strings.Contains(err.Error(), "aaa/dup") {
		t.Errorf("error should list candidates: %v", err)
	}
}

func TestRepoInSet_NotFound(t *testing.T) {
	got, ok, err := repoInSet([]string{"aaa/dup"}, "nope")
	if ok || got != "" || err != nil {
		t.Fatalf("expected no match and no error, got %q ok=%v err=%v", got, ok, err)
	}
}

// --- resolveRepoName ---

func TestResolveRepoName_NotFoundUnder(t *testing.T) {
	_, err := resolveRepoName("/repos", []string{"org/api"}, "nope")
	if err == nil || !strings.Contains(err.Error(), "not found under /repos") {
		t.Fatalf("expected 'not found under /repos', got %v", err)
	}
}

// --- repoNamesIndex ---

func TestRepoNamesIndex_SlashNormalized(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sub") // ensure rel is non-trivial
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	p := filepath.Join(root, "org", "api")
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	names, byName := repoNamesIndex(root, []string{p})
	if len(names) != 1 || names[0] != "org/api" {
		t.Fatalf("names = %v, want [org/api]", names)
	}
	if byName["org/api"] != p {
		t.Errorf("byName[org/api] = %q, want %q", byName["org/api"], p)
	}
}

// --- completionName ---

func TestCompletionName_UniqueBasename(t *testing.T) {
	names := []string{"org/api", "org/frontend"}
	if got := completionName(names, "org/api"); got != "api" {
		t.Errorf("completionName(org/api) = %q, want api", got)
	}
}

func TestCompletionName_AmbiguousKeepsFullPath(t *testing.T) {
	names := []string{"aaa/dup", "bbb/dup"}
	if got := completionName(names, "aaa/dup"); got != "aaa/dup" {
		t.Errorf("completionName(aaa/dup) = %q, want aaa/dup", got)
	}
}

// --- removeEmptyParents ---

func TestRemoveEmptyParents_PrunesAllEmptyLevels(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "github.com", "suhlig", "dspictl")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Simulate git worktree remove having removed only the leaf.
	if err := os.Remove(repo); err != nil {
		t.Fatalf("remove leaf: %v", err)
	}

	removeEmptyParents(repo, root)

	for _, name := range []string{"github.com", "github.com/suhlig"} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Errorf("expected %s pruned: %v", name, err)
		}
	}
	if _, err := os.Stat(root); err != nil {
		t.Errorf("space root should remain: %v", err)
	}
}

func TestRemoveEmptyParents_StopsAtNonEmptySibling(t *testing.T) {
	root := t.TempDir()
	// org/keep has content, org/gone does not — only gone may be pruned.
	keep := filepath.Join(root, "org", "keep")
	if err := os.MkdirAll(keep, 0o755); err != nil {
		t.Fatalf("mkdir keep: %v", err)
	}
	if err := os.WriteFile(filepath.Join(keep, "f"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	goneRepo := filepath.Join(root, "org", "gone", "repo")
	if err := os.MkdirAll(goneRepo, 0o755); err != nil {
		t.Fatalf("mkdir gone: %v", err)
	}
	if err := os.Remove(goneRepo); err != nil {
		t.Fatalf("remove leaf: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "org", "gone")); err != nil {
		t.Fatalf("remove gone: %v", err)
	}

	removeEmptyParents(goneRepo, root)

	if _, err := os.Stat(filepath.Join(root, "org", "keep")); err != nil {
		t.Errorf("keep should remain intact: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "org", "gone")); !os.IsNotExist(err) {
		t.Errorf("gone should be pruned: %v", err)
	}
}
