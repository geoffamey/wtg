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

func runFetch(t *testing.T, root string, runner *testRunner, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	if err := RunFetch(discoverCfg(root, 2), runner, args, &out); err != nil {
		t.Fatalf("RunFetch: %v", err)
	}
	return out.String()
}

func TestRunFetch_AllRepos(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "api")
	makeRepo(t, root, "frontend")

	r := &testRunner{fetchFn: func(string) error { return nil }}
	got := runFetch(t, root, r)
	if !strings.Contains(got, "api") || !strings.Contains(got, "frontend") {
		t.Errorf("output missing repo names: %q", got)
	}
	if !strings.Contains(got, ui.SymOK) {
		t.Errorf("output missing success symbol: %q", got)
	}
}

func TestRunFetch_NamedRepo(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "api")
	makeRepo(t, root, "frontend")

	r := &testRunner{fetchFn: func(string) error { return nil }}
	got := runFetch(t, root, r, "api")
	if !strings.Contains(got, "api") {
		t.Errorf("missing api: %q", got)
	}
	if strings.Contains(got, "frontend") {
		t.Errorf("should not include frontend: %q", got)
	}
}

func TestRunFetch_FetchError(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "api")

	r := &testRunner{fetchFn: func(string) error { return fmt.Errorf("network error") }}
	got := runFetch(t, root, r)
	if !strings.Contains(got, ui.SymFail) {
		t.Errorf("output missing fail symbol: %q", got)
	}
	if !strings.Contains(got, "network error") {
		t.Errorf("output missing error message: %q", got)
	}
}

func TestRunFetch_UnknownRepo(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	err := RunFetch(discoverCfg(root, 2), &testRunner{}, []string{"no-such-repo"}, &out)
	if err == nil {
		t.Fatal("expected error for unknown repo")
	}
}

func TestRunFetch_NoRootDir(t *testing.T) {
	var out bytes.Buffer
	err := RunFetch(discoverCfg("", 2), &testRunner{}, nil, &out)
	if err == nil {
		t.Fatal("expected error when root_dir is empty")
	}
}

func TestRunFetch_Parallel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		root := t.TempDir()
		makeRepo(t, root, "api")
		makeRepo(t, root, "svc")
		makeRepo(t, root, "web")

		gate := make(chan struct{})
		var started atomic.Int32

		r := &testRunner{fetchFn: func(string) error {
			started.Add(1)
			<-gate
			return nil
		}}

		done := make(chan error, 1)
		go func() {
			done <- RunFetch(discoverCfg(root, 1), r, nil, io.Discard)
		}()

		synctest.Wait()

		if n := started.Load(); n != 3 {
			t.Errorf("expected 3 concurrent fetches, got %d", n)
		}

		close(gate)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
}
