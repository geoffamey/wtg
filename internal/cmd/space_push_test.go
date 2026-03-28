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
	statusSpace(t, "feat", "geoff/feat", t.TempDir(), []string{"api", "svc"}, "/repos")

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
	statusSpace(t, "feat", "geoff/feat", t.TempDir(), []string{"api"}, "/repos")

	r := &testRunner{pushFn: func(string, string) error { return fmt.Errorf("rejected") }}

	var out bytes.Buffer
	if err := RunSpacePush(r, "feat", &out); err != nil {
		t.Fatalf("RunSpacePush should not return error on push failure: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, ui.SymFail) {
		t.Errorf("output missing fail symbol: %q", got)
	}
	if !strings.Contains(got, "rejected") {
		t.Errorf("output missing error message: %q", got)
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
		statusSpace(t, "feat", "geoff/feat", t.TempDir(), []string{"api", "svc", "web"}, "/repos")

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
