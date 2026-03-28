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

func TestRunSpaceRebase_RebasesAllRepos(t *testing.T) {
	isolateState(t)
	makeSpace(t, "feat", "geoff/feat", t.TempDir(), []string{"api", "svc"}, "/repos")

	r := &testRunner{
		defaultBranchFn: func(string) (string, error) { return "main", nil },
		rebaseFn:        func(string, string) error { return nil },
	}

	var out bytes.Buffer
	if err := RunSpaceRebase(r, "feat", &out); err != nil {
		t.Fatalf("RunSpaceRebase: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "api") || !strings.Contains(got, "svc") {
		t.Errorf("output missing repo names: %q", got)
	}
	if !strings.Contains(got, ui.SymOK) {
		t.Errorf("output missing success symbol: %q", got)
	}
	if !strings.Contains(got, "origin/main") {
		t.Errorf("output missing onto ref: %q", got)
	}
}

func TestRunSpaceRebase_RebaseError(t *testing.T) {
	isolateState(t)
	makeSpace(t, "feat", "geoff/feat", t.TempDir(), []string{"api"}, "/repos")

	r := &testRunner{
		defaultBranchFn: func(string) (string, error) { return "main", nil },
		rebaseFn:        func(string, string) error { return fmt.Errorf("conflict in foo.go") },
	}

	var out bytes.Buffer
	err := RunSpaceRebase(r, "feat", &out)
	if err == nil {
		t.Fatal("expected error when rebase fails")
	}
	if !strings.Contains(err.Error(), "rebase failed") {
		t.Errorf("error should mention rebase failed: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, ui.SymFail) {
		t.Errorf("output missing fail symbol: %q", got)
	}
	if !strings.Contains(got, "conflict in foo.go") {
		t.Errorf("output missing error message: %q", got)
	}
}

func TestRunSpaceRebase_DefaultBranchError(t *testing.T) {
	isolateState(t)
	makeSpace(t, "feat", "geoff/feat", t.TempDir(), []string{"api"}, "/repos")

	r := &testRunner{
		defaultBranchFn: func(string) (string, error) { return "", fmt.Errorf("no origin/HEAD") },
	}

	var out bytes.Buffer
	err := RunSpaceRebase(r, "feat", &out)
	if err == nil {
		t.Fatal("expected error when default branch detection fails")
	}
	got := out.String()
	if !strings.Contains(got, ui.SymFail) {
		t.Errorf("output missing fail symbol: %q", got)
	}
	if !strings.Contains(got, "no origin/HEAD") {
		t.Errorf("output missing error message: %q", got)
	}
}

func TestRunSpaceRebase_UnknownSpace(t *testing.T) {
	isolateState(t)
	var out bytes.Buffer
	if err := RunSpaceRebase(&testRunner{}, "nonexistent", &out); err == nil {
		t.Fatal("expected error for unknown space")
	}
}

func TestRunSpaceRebase_Parallel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		isolateState(t)
		makeSpace(t, "feat", "geoff/feat", t.TempDir(), []string{"api", "svc", "web"}, "/repos")

		gate := make(chan struct{})
		var started atomic.Int32

		r := &testRunner{
			defaultBranchFn: func(string) (string, error) { return "main", nil },
			rebaseFn: func(string, string) error {
				started.Add(1)
				<-gate
				return nil
			},
		}

		done := make(chan error, 1)
		go func() {
			done <- RunSpaceRebase(r, "feat", io.Discard)
		}()

		synctest.Wait()

		if n := started.Load(); n != 3 {
			t.Errorf("expected 3 concurrent rebases, got %d", n)
		}

		close(gate)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
}
