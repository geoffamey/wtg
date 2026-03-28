package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geoffamey/wtg/internal/config"
	"github.com/geoffamey/wtg/internal/state"
)

// seedSpace saves a space to state with the given repos already in it.
func seedSpace(t *testing.T, name, branch, spacePath string, repoNames []string, repoRoot string) *state.Space {
	t.Helper()
	sp := &state.Space{
		Name:      name,
		Branch:    branch,
		Path:      spacePath,
		CreatedAt: time.Now(),
	}
	for _, n := range repoNames {
		sp.Repos = append(sp.Repos, state.RepoEntry{
			Name:         n,
			RepoPath:     filepath.Join(repoRoot, n),
			WorktreePath: filepath.Join(spacePath, n),
		})
	}
	if err := state.Save(sp); err != nil {
		t.Fatalf("seedSpace: %v", err)
	}
	return sp
}

// --- validation ---

func TestRunSpaceAdd_NoDiscoveryRoot(t *testing.T) {
	isolateState(t)
	cfg := &config.Config{}
	var out bytes.Buffer
	err := RunSpaceAdd(cfg, &testRunner{}, SpaceAddArgs{Name: "feat", Repos: []string{"api"}}, &out)
	if err == nil {
		t.Fatal("expected error when discovery.root_dir is empty")
	}
}

func TestRunSpaceAdd_NoReposSpecified(t *testing.T) {
	root := t.TempDir()
	isolateState(t)
	cfg := spaceCreateCfg(root, t.TempDir())
	var out bytes.Buffer
	err := RunSpaceAdd(cfg, &testRunner{}, SpaceAddArgs{Name: "feat"}, &out)
	if err == nil {
		t.Fatal("expected error when no repos specified")
	}
}

func TestRunSpaceAdd_SpaceNotFound(t *testing.T) {
	root := t.TempDir()
	isolateState(t)
	cfg := spaceCreateCfg(root, t.TempDir())
	var out bytes.Buffer
	err := RunSpaceAdd(cfg, &testRunner{}, SpaceAddArgs{Name: "nonexistent", Repos: []string{"api"}}, &out)
	if err == nil {
		t.Fatal("expected error when space does not exist")
	}
}

func TestRunSpaceAdd_RepoAlreadyInSpace(t *testing.T) {
	root := t.TempDir()
	spacesRoot := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")
	seedSpace(t, "feat", "feat", filepath.Join(spacesRoot, "feat"), []string{"api"}, root)
	cfg := spaceCreateCfg(root, spacesRoot)
	var out bytes.Buffer
	err := RunSpaceAdd(cfg, &testRunner{}, SpaceAddArgs{Name: "feat", Repos: []string{"api"}}, &out)
	if err == nil {
		t.Fatal("expected error when repo is already in the space")
	}
	if !strings.Contains(err.Error(), "already in space") {
		t.Errorf("error should mention already in space: %v", err)
	}
}

func TestRunSpaceAdd_UnknownRepo(t *testing.T) {
	root := t.TempDir()
	spacesRoot := t.TempDir()
	isolateState(t)
	spacePath := filepath.Join(spacesRoot, "feat")
	seedSpace(t, "feat", "feat", spacePath, nil, root)
	cfg := spaceCreateCfg(root, spacesRoot)
	var out bytes.Buffer
	err := RunSpaceAdd(cfg, &testRunner{}, SpaceAddArgs{Name: "feat", Repos: []string{"no-such-repo"}}, &out)
	if err == nil {
		t.Fatal("expected error for unknown repo")
	}
	if !strings.Contains(err.Error(), "no-such-repo") {
		t.Errorf("error should mention repo name: %v", err)
	}
}

// --- happy path ---

func TestRunSpaceAdd_Success(t *testing.T) {
	root := t.TempDir()
	spacesRoot := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")
	makeRepo(t, root, "frontend")
	spacePath := filepath.Join(spacesRoot, "feat")
	seedSpace(t, "feat", "feat", spacePath, []string{"api"}, root)
	cfg := spaceCreateCfg(root, spacesRoot)

	var added []string
	r := &testRunner{
		branchExistsFn: func(_, _ string) (bool, error) { return false, nil },
		worktreeAddFn: func(_, worktreePath, _ string, _ bool) error {
			added = append(added, worktreePath)
			return nil
		},
	}

	var out bytes.Buffer
	if err := RunSpaceAdd(cfg, r, SpaceAddArgs{Name: "feat", Repos: []string{"frontend"}}, &out); err != nil {
		t.Fatalf("RunSpaceAdd: %v", err)
	}

	// Only frontend should be added.
	if len(added) != 1 || !strings.Contains(added[0], "frontend") {
		t.Errorf("expected one worktree add for frontend, got %v", added)
	}

	// State should now have both repos.
	sp, err := state.Load("feat")
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	if len(sp.Repos) != 2 {
		t.Errorf("expected 2 repos in state, got %d", len(sp.Repos))
	}
}

func TestRunSpaceAdd_UsesSpaceBranch(t *testing.T) {
	root := t.TempDir()
	spacesRoot := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "frontend")
	spacePath := filepath.Join(spacesRoot, "feat")
	seedSpace(t, "feat", "geoff/feat", spacePath, nil, root)
	cfg := spaceCreateCfg(root, spacesRoot)

	var usedBranch string
	r := &testRunner{
		branchExistsFn: func(_, _ string) (bool, error) { return false, nil },
		worktreeAddFn:  func(_, _, branch string, _ bool) error { usedBranch = branch; return nil },
	}

	var out bytes.Buffer
	if err := RunSpaceAdd(cfg, r, SpaceAddArgs{Name: "feat", Repos: []string{"frontend"}}, &out); err != nil {
		t.Fatalf("RunSpaceAdd: %v", err)
	}
	if usedBranch != "geoff/feat" {
		t.Errorf("expected branch geoff/feat from space state, got %q", usedBranch)
	}
}

// --- go.work update ---

func TestRunSpaceAdd_GoWorkUpdated(t *testing.T) {
	root := t.TempDir()
	spacesRoot := t.TempDir()
	isolateState(t)

	// api already in space, with go.mod.
	apiPath := makeRepo(t, root, "api")
	if err := os.WriteFile(filepath.Join(apiPath, "go.mod"),
		[]byte("module example.com/api\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	// frontend being added, also has go.mod.
	frontendPath := makeRepo(t, root, "frontend")
	if err := os.WriteFile(filepath.Join(frontendPath, "go.mod"),
		[]byte("module example.com/frontend\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	spacePath := filepath.Join(spacesRoot, "feat")
	// Seed with existing go.work referencing only api.
	if err := os.MkdirAll(spacePath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	oldGoWork := "go 1.24\n\nuse (\n\t./api\n)\n"
	if err := os.WriteFile(filepath.Join(spacePath, "go.work"), []byte(oldGoWork), 0o644); err != nil {
		t.Fatalf("write old go.work: %v", err)
	}
	seedSpace(t, "feat", "feat", spacePath, []string{"api"}, root)
	cfg := spaceCreateCfg(root, spacesRoot)

	var out bytes.Buffer
	if err := RunSpaceAdd(cfg, createRunner(), SpaceAddArgs{
		Name:  "feat",
		Repos: []string{"frontend"},
	}, &out); err != nil {
		t.Fatalf("RunSpaceAdd: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(spacePath, "go.work"))
	if err != nil {
		t.Fatalf("read go.work: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "./api") {
		t.Errorf("go.work missing ./api: %q", content)
	}
	if !strings.Contains(content, "./frontend") {
		t.Errorf("go.work missing ./frontend: %q", content)
	}
}

// --- rollback ---

func TestRunSpaceAdd_RollbackRestoresState(t *testing.T) {
	root := t.TempDir()
	spacesRoot := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "frontend")
	spacePath := filepath.Join(spacesRoot, "feat")
	seedSpace(t, "feat", "feat", spacePath, []string{"api"}, root)
	cfg := spaceCreateCfg(root, spacesRoot)

	r := &testRunner{
		branchExistsFn:   func(_, _ string) (bool, error) { return false, nil },
		worktreeAddFn:    func(_, _, _ string, _ bool) error { return fmt.Errorf("disk full") },
		worktreeRemoveFn: func(_, _ string, _ bool) error { return nil },
		branchDeleteFn:   func(_, _ string, _ bool) error { return nil },
	}

	var out bytes.Buffer
	if err := RunSpaceAdd(cfg, r, SpaceAddArgs{Name: "feat", Repos: []string{"frontend"}}, &out); err == nil {
		t.Fatal("expected error")
	}

	// State should still have only the original repo.
	sp, err := state.Load("feat")
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	if len(sp.Repos) != 1 || sp.Repos[0].Name != "api" {
		t.Errorf("state should be restored to original: got repos %v", sp.Repos)
	}
}

func TestRunSpaceAdd_RollbackRestoresGoWork(t *testing.T) {
	root := t.TempDir()
	spacesRoot := t.TempDir()
	isolateState(t)

	apiPath := makeRepo(t, root, "api")
	if err := os.WriteFile(filepath.Join(apiPath, "go.mod"),
		[]byte("module example.com/api\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	frontendPath := makeRepo(t, root, "frontend")
	if err := os.WriteFile(filepath.Join(frontendPath, "go.mod"),
		[]byte("module example.com/frontend\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	spacePath := filepath.Join(spacesRoot, "feat")
	if err := os.MkdirAll(spacePath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	oldGoWork := "go 1.24\n\nuse (\n\t./api\n)\n"
	if err := os.WriteFile(filepath.Join(spacePath, "go.work"), []byte(oldGoWork), 0o644); err != nil {
		t.Fatalf("write old go.work: %v", err)
	}
	seedSpace(t, "feat", "feat", spacePath, []string{"api"}, root)

	// Make the state file read-only so state.Save (WriteFile) fails while
	// state.Load (ReadFile) still works.
	stateFile := filepath.Join(state.DataDir(), "feat.yaml")
	if err := os.Chmod(stateFile, 0o444); err != nil {
		t.Fatalf("chmod stateFile: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(stateFile, 0o644) })

	cfg := spaceCreateCfg(root, spacesRoot)
	r := &testRunner{
		branchExistsFn:   func(_, _ string) (bool, error) { return false, nil },
		worktreeAddFn:    func(_, _, _ string, _ bool) error { return nil },
		worktreeRemoveFn: func(_, _ string, _ bool) error { return nil },
		branchDeleteFn:   func(_, _ string, _ bool) error { return nil },
	}

	var out bytes.Buffer
	if err := RunSpaceAdd(cfg, r, SpaceAddArgs{Name: "feat", Repos: []string{"frontend"}}, &out); err == nil {
		t.Fatal("expected error when state save fails")
	}

	// go.work should be restored to its original content.
	data, err := os.ReadFile(filepath.Join(spacePath, "go.work"))
	if err != nil {
		t.Fatalf("read go.work after rollback: %v", err)
	}
	if string(data) != oldGoWork {
		t.Errorf("go.work not restored:\ngot:  %q\nwant: %q", string(data), oldGoWork)
	}
}
