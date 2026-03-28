package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geoffamey/wtg/internal/config"
)

// makeRepo creates a fake git repo (a directory with a .git subdir) under root.
func makeRepo(t *testing.T, root string, relPath string) string {
	t.Helper()
	dir := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("makeRepo %s: %v", relPath, err)
	}
	return dir
}

func discoverCfg(rootDir string, maxDepth int) *config.Config {
	return &config.Config{
		Discovery: config.DiscoveryConfig{RootDir: rootDir, MaxDepth: maxDepth},
	}
}

// --- discoverRepoPaths ---

func TestDiscoverRepoPaths_Flat(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "api")
	makeRepo(t, root, "frontend")

	paths, err := discoverRepoPaths(root, 2)
	if err != nil {
		t.Fatalf("discoverRepoPaths: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("got %d paths, want 2: %v", len(paths), paths)
	}
}

func TestDiscoverRepoPaths_Nested(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "myorg/api")
	makeRepo(t, root, "myorg/frontend")
	makeRepo(t, root, "infra")

	paths, err := discoverRepoPaths(root, 2)
	if err != nil {
		t.Fatalf("discoverRepoPaths: %v", err)
	}
	if len(paths) != 3 {
		t.Fatalf("got %d paths, want 3: %v", len(paths), paths)
	}
}

func TestDiscoverRepoPaths_MaxDepthRespected(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "shallow")       // depth 1 — included at maxDepth=1
	makeRepo(t, root, "org/deep")      // depth 2 — excluded at maxDepth=1

	paths, err := discoverRepoPaths(root, 1)
	if err != nil {
		t.Fatalf("discoverRepoPaths: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("got %d paths, want 1: %v", len(paths), paths)
	}
	if !strings.HasSuffix(paths[0], "shallow") {
		t.Errorf("expected shallow, got %q", paths[0])
	}
}

func TestDiscoverRepoPaths_DoesNotRecurseIntoRepo(t *testing.T) {
	root := t.TempDir()
	// api is a repo that happens to contain another .git — should not be returned twice.
	makeRepo(t, root, "api")
	makeRepo(t, root, "api/vendor/lib") // nested; should be ignored

	paths, err := discoverRepoPaths(root, 3)
	if err != nil {
		t.Fatalf("discoverRepoPaths: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("got %d paths, want 1: %v", len(paths), paths)
	}
}

func TestDiscoverRepoPaths_SkipsHiddenDirs(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "visible")
	makeRepo(t, root, ".hidden") // should be skipped

	paths, err := discoverRepoPaths(root, 2)
	if err != nil {
		t.Fatalf("discoverRepoPaths: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("got %d paths, want 1: %v", len(paths), paths)
	}
}

func TestDiscoverRepoPaths_Empty(t *testing.T) {
	root := t.TempDir()
	paths, err := discoverRepoPaths(root, 2)
	if err != nil {
		t.Fatalf("discoverRepoPaths: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("got %d paths, want 0", len(paths))
	}
}

// --- RunDiscover ---

func TestRunDiscover_Output(t *testing.T) {
	root := t.TempDir()
	apiPath := makeRepo(t, root, "api")
	frontendPath := makeRepo(t, root, "myorg/frontend")

	runner := &testRunner{
		remoteURLFn: func(repoPath, remote string) (string, error) {
			urls := map[string]string{
				apiPath:      "https://github.com/org/api.git",
				frontendPath: "https://github.com/org/frontend.git",
			}
			return urls[repoPath], nil
		},
	}

	var out bytes.Buffer
	err := RunDiscover(discoverCfg(root, 2), runner, &out)
	if err != nil {
		t.Fatalf("RunDiscover: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "api") {
		t.Errorf("output missing 'api': %q", got)
	}
	if !strings.Contains(got, "myorg/frontend") {
		t.Errorf("output missing 'myorg/frontend': %q", got)
	}
	if !strings.Contains(got, "https://github.com/org/api.git") {
		t.Errorf("output missing remote URL: %q", got)
	}
}

func TestRunDiscover_NoRootDir(t *testing.T) {
	err := RunDiscover(&config.Config{}, &testRunner{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error when root_dir is empty")
	}
}

func TestRunDiscover_NoRemote(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "local-only")

	runner := &testRunner{
		remoteURLFn: func(repoPath, remote string) (string, error) {
			return "", nil // no remote configured
		},
	}

	var out bytes.Buffer
	if err := RunDiscover(discoverCfg(root, 2), runner, &out); err != nil {
		t.Fatalf("RunDiscover: %v", err)
	}
	if !strings.Contains(out.String(), "local-only") {
		t.Errorf("expected local-only in output: %q", out.String())
	}
}

func TestRunDiscover_SortedOutput(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "zzz")
	makeRepo(t, root, "aaa")
	makeRepo(t, root, "mmm")

	runner := &testRunner{
		remoteURLFn: func(string, string) (string, error) { return "", nil },
	}

	var out bytes.Buffer
	if err := RunDiscover(discoverCfg(root, 2), runner, &out); err != nil {
		t.Fatalf("RunDiscover: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if !strings.HasPrefix(strings.TrimSpace(lines[0]), "aaa") {
		t.Errorf("first line should be aaa, got %q", lines[0])
	}
	if !strings.HasPrefix(strings.TrimSpace(lines[2]), "zzz") {
		t.Errorf("last line should be zzz, got %q", lines[2])
	}
}
