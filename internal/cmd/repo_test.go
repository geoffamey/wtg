package cmd

import (
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
	makeRepo(t, root, "shallow")  // depth 1 — included at maxDepth=1
	makeRepo(t, root, "org/deep") // depth 2 — excluded at maxDepth=1

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

// --- resolveRepoPath ---

func TestResolveRepoPath_ExactPath(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "github.com/suhlig/dspictl")
	p, err := resolveRepoPath(root, 3, "github.com/suhlig/dspictl")
	if err != nil {
		t.Fatalf("resolveRepoPath: %v", err)
	}
	if !strings.HasSuffix(p, "github.com/suhlig/dspictl") {
		t.Errorf("got %q", p)
	}
}

func TestResolveRepoPath_UniqueBasename(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "github.com/suhlig/dspictl")
	makeRepo(t, root, "github.com/speisehof/caterbill")
	p, err := resolveRepoPath(root, 3, "dspictl")
	if err != nil {
		t.Fatalf("resolveRepoPath: %v", err)
	}
	if !strings.HasSuffix(p, "github.com/suhlig/dspictl") {
		t.Errorf("got %q", p)
	}
}

func TestResolveRepoPath_AmbiguousBasename(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "aaa/dup")
	makeRepo(t, root, "bbb/dup")
	_, err := resolveRepoPath(root, 3, "dup")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguous error, got %v", err)
	}
}

func TestResolveRepoPath_Unknown(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "api")
	_, err := resolveRepoPath(root, 2, "nope")
	if err == nil || !strings.Contains(err.Error(), "not found under") {
		t.Fatalf("expected not-found error, got %v", err)
	}
}
