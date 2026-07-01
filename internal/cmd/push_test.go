package cmd

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/geoffamey/wtg/internal/ui"
)

func TestRunSpacePush_PushesAllRepos(t *testing.T) {
	isolateState(t)
	makeSpace(t, "feat", "geoff/feat", t.TempDir(), []string{"api", "svc"}, "/repos")

	r := &testRunner{pushFn: func(string, string) error { return nil }}

	var out bytes.Buffer
	if err := RunSpacePush(r, "feat", &out); err != nil {
		t.Fatalf("RunSpacePush: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "api") || !strings.Contains(got, "svc") {
		t.Errorf("output missing repo names: %q", got)
	}
	if !strings.Contains(got, ui.SymOK) {
		t.Errorf("output missing success symbol: %q", got)
	}
}

func TestRunSpacePush_PushError(t *testing.T) {
	isolateState(t)
	makeSpace(t, "feat", "geoff/feat", t.TempDir(), []string{"api"}, "/repos")

	r := &testRunner{pushFn: func(string, string) error { return fmt.Errorf("rejected") }}

	var out bytes.Buffer
	err := RunSpacePush(r, "feat", &out)
	if err == nil {
		t.Fatal("expected error when push fails")
	}
	if !strings.Contains(err.Error(), "push failed") {
		t.Errorf("error should mention push failed: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, ui.SymFail) {
		t.Errorf("output missing fail symbol: %q", got)
	}
	if !strings.Contains(got, "rejected") {
		t.Errorf("output missing error message: %q", got)
	}
}

func TestRunSpacePush_SkipsSymlinks(t *testing.T) {
	isolateState(t)
	spacePath := t.TempDir()
	makeSpaceWithSymlink(t, "feat", "geoff/feat", spacePath, "/repos", "api", "shared")

	var pushed []string
	r := &testRunner{pushFn: func(repoPath, branch string) error {
		pushed = append(pushed, repoPath)
		return nil
	}}

	var out bytes.Buffer
	if err := RunSpacePush(r, "feat", &out); err != nil {
		t.Fatalf("RunSpacePush: %v", err)
	}
	if len(pushed) != 1 || !strings.Contains(pushed[0], "api") {
		t.Errorf("expected push only for the worktree repo, got: %v", pushed)
	}
	got := out.String()
	if !strings.Contains(got, "shared") || !strings.Contains(got, "skipped") {
		t.Errorf("expected skip notice for symlink repo: %q", got)
	}
}

func TestRunSpacePush_UnknownSpace(t *testing.T) {
	isolateState(t)
	var out bytes.Buffer
	if err := RunSpacePush(&testRunner{}, "nonexistent", &out); err == nil {
		t.Fatal("expected error for unknown space")
	}
}

func TestRunSpacePush_Parallel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		isolateState(t)
		makeSpace(t, "feat", "geoff/feat", t.TempDir(), []string{"api", "svc", "web"}, "/repos")

		gate := make(chan struct{})
		var started atomic.Int32

		r := &testRunner{pushFn: func(string, string) error {
			started.Add(1)
			<-gate
			return nil
		}}

		done := make(chan error, 1)
		go func() {
			done <- RunSpacePush(r, "feat", io.Discard)
		}()

		synctest.Wait()

		if n := started.Load(); n != 3 {
			t.Errorf("expected 3 concurrent pushes, got %d", n)
		}

		close(gate)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
}
