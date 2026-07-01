package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geoffamey/wtg/internal/state"
)

// execSpace creates a space whose worktree paths are real directories.
func execSpace(t *testing.T, name string, repos []string) *state.Space {
	t.Helper()
	sp := &state.Space{
		Name:      name,
		Branch:    name,
		Path:      t.TempDir(),
		CreatedAt: time.Now(),
	}
	for _, n := range repos {
		dir := filepath.Join(sp.Path, n)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		sp.Repos = append(sp.Repos, state.RepoEntry{
			Name:         n,
			RepoPath:     "/repos/" + n,
			WorktreePath: dir,
		})
	}
	if err := state.Save(sp); err != nil {
		t.Fatalf("execSpace save: %v", err)
	}
	return sp
}

func TestRunSpaceExec_RunsInEachWorktree(t *testing.T) {
	isolateState(t)
	execSpace(t, "feat", []string{"api", "svc"})

	var out bytes.Buffer
	if err := RunSpaceExec("feat", []string{"echo", "hello"}, &out); err != nil {
		t.Fatalf("RunSpaceExec: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "api") || !strings.Contains(got, "svc") {
		t.Errorf("output missing repo headers: %q", got)
	}
	if strings.Count(got, "hello") != 2 {
		t.Errorf("expected 'hello' twice (once per repo): %q", got)
	}
}

func TestRunSpaceExec_OutputInRepoOrder(t *testing.T) {
	isolateState(t)
	execSpace(t, "feat", []string{"api", "svc"})

	var out bytes.Buffer
	if err := RunSpaceExec("feat", []string{"echo", "hello"}, &out); err != nil {
		t.Fatalf("RunSpaceExec: %v", err)
	}
	got := out.String()
	if strings.Index(got, "api") > strings.Index(got, "svc") {
		t.Errorf("repos should appear in state order: %q", got)
	}
}

func TestRunSpaceExec_ContinuesAfterFailure(t *testing.T) {
	isolateState(t)
	execSpace(t, "feat", []string{"api", "svc"})

	var out bytes.Buffer
	// 'false' exits with code 1.
	err := RunSpaceExec("feat", []string{"false"}, &out)
	if err == nil {
		t.Fatal("expected error when command fails")
	}
	// Both repos should have been attempted despite the first failing.
	if !strings.Contains(err.Error(), "api") || !strings.Contains(err.Error(), "svc") {
		t.Errorf("error should name all failed repos: %v", err)
	}
}

func TestRunSpaceExec_UnknownSpace(t *testing.T) {
	isolateState(t)
	var out bytes.Buffer
	if err := RunSpaceExec("nonexistent", []string{"echo", "hi"}, &out); err == nil {
		t.Fatal("expected error for unknown space")
	}
}

func TestRunSpaceExec_SkipsSymlinks(t *testing.T) {
	isolateState(t)
	sp := execSpace(t, "feat", []string{"api"})
	sp.Repos = append(sp.Repos, state.RepoEntry{
		Name:         "shared",
		RepoPath:     "/repos/shared",
		WorktreePath: "/nonexistent/shared", // never accessed if skipped correctly
		Symlink:      true,
	})
	if err := state.Save(sp); err != nil {
		t.Fatalf("save: %v", err)
	}

	var out bytes.Buffer
	if err := RunSpaceExec("feat", []string{"echo", "hello"}, &out); err != nil {
		t.Fatalf("RunSpaceExec: %v", err)
	}
	got := out.String()
	if strings.Count(got, "hello") != 1 {
		t.Errorf("expected 'hello' once (symlink repo skipped): %q", got)
	}
	if !strings.Contains(got, "shared") || !strings.Contains(got, "skipped") {
		t.Errorf("expected skip notice for symlink repo: %q", got)
	}
}

func TestRunSpaceExec_PassesStdinThrough(t *testing.T) {
	isolateState(t)
	execSpace(t, "feat", []string{"api"})

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	if _, err := w.WriteString("hello from stdin"); err != nil {
		t.Fatalf("write pipe: %v", err)
	}
	w.Close()

	origStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	var out bytes.Buffer
	if err := RunSpaceExec("feat", []string{"cat"}, &out); err != nil {
		t.Fatalf("RunSpaceExec: %v", err)
	}
	if !strings.Contains(out.String(), "hello from stdin") {
		t.Errorf("expected child to read piped stdin, got: %q", out.String())
	}
}

func TestRunSpaceExec_RunsInWorktreeDir(t *testing.T) {
	isolateState(t)
	sp := execSpace(t, "feat", []string{"api"})

	var out bytes.Buffer
	if err := RunSpaceExec("feat", []string{"pwd"}, &out); err != nil {
		t.Fatalf("RunSpaceExec: %v", err)
	}
	// pwd output should be the worktree path.
	want := sp.Repos[0].WorktreePath
	if !strings.Contains(out.String(), want) {
		t.Errorf("expected cwd %q in output: %q", want, out.String())
	}
}
