package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geoffamey/wtg/internal/config"
	"github.com/geoffamey/wtg/internal/git"
	"github.com/geoffamey/wtg/internal/state"
)

// spaceCreateCfg returns a config with both discovery and spaces root set.
func spaceCreateCfg(root, spacesRoot string) *config.Config {
	return &config.Config{
		Discovery: config.DiscoveryConfig{RootDir: root, MaxDepth: 2},
		Spaces:    config.SpacesConfig{RootDir: spacesRoot},
	}
}

// isolateState redirects XDG_DATA_HOME to a temp dir for the duration of the test.
func isolateState(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
}

// createRunner returns a testRunner that accepts any branch create without conflict.
func createRunner() *testRunner {
	return &testRunner{
		branchExistsFn: func(_, _ string) (bool, error) { return false, nil },
		worktreeAddFn:  func(_, _, _ string, _ bool) error { return nil },
	}
}

// --- validation errors ---

func TestRunSpaceCreate_NoSpacesRoot(t *testing.T) {
	isolateState(t)
	cfg := &config.Config{
		Discovery: config.DiscoveryConfig{RootDir: t.TempDir(), MaxDepth: 2},
	}
	var out bytes.Buffer
	if err := RunSpaceCreate(cfg, &testRunner{}, SpaceCreateArgs{Name: "feat"}, &out); err == nil {
		t.Fatal("expected error when spaces.root_dir is empty and no --path")
	}
}

func TestRunSpaceCreate_NoDiscoveryRoot(t *testing.T) {
	isolateState(t)
	cfg := &config.Config{
		Spaces: config.SpacesConfig{RootDir: t.TempDir()},
	}
	var out bytes.Buffer
	if err := RunSpaceCreate(cfg, &testRunner{}, SpaceCreateArgs{Name: "feat"}, &out); err == nil {
		t.Fatal("expected error when discovery.root_dir is empty")
	}
}

func TestRunSpaceCreate_PathFlagBypassesSpacesRoot(t *testing.T) {
	root := t.TempDir()
	spacePath := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")
	// cfg has no spaces.root_dir — should be OK because --path is supplied
	cfg := &config.Config{
		Discovery: config.DiscoveryConfig{RootDir: root, MaxDepth: 2},
	}
	var out bytes.Buffer
	if err := RunSpaceCreate(cfg, createRunner(), SpaceCreateArgs{
		Name: "feat",
		Path: spacePath,
	}, &out); err != nil {
		t.Fatalf("expected success with --path override: %v", err)
	}
}

func TestRunSpaceCreate_NoRepos(t *testing.T) {
	root := t.TempDir()
	isolateState(t)
	cfg := spaceCreateCfg(root, t.TempDir())
	var out bytes.Buffer
	if err := RunSpaceCreate(cfg, &testRunner{}, SpaceCreateArgs{Name: "feat"}, &out); err == nil {
		t.Fatal("expected error when no repos found")
	}
}

func TestRunSpaceCreate_UnknownRepo(t *testing.T) {
	root := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")
	cfg := spaceCreateCfg(root, t.TempDir())
	var out bytes.Buffer
	err := RunSpaceCreate(cfg, &testRunner{}, SpaceCreateArgs{
		Name:  "feat",
		Repos: []string{"nonexistent"},
	}, &out)
	if err == nil {
		t.Fatal("expected error for unknown repo")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention unknown repo name: %v", err)
	}
}

func TestRunSpaceCreate_SpaceAlreadyExists(t *testing.T) {
	root := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")
	cfg := spaceCreateCfg(root, t.TempDir())
	if err := state.Save(&state.Space{Name: "feat", Path: "/tmp/feat", Branch: "feat"}); err != nil {
		t.Fatalf("state.Save: %v", err)
	}
	var out bytes.Buffer
	err := RunSpaceCreate(cfg, &testRunner{}, SpaceCreateArgs{Name: "feat"}, &out)
	if err == nil {
		t.Fatal("expected error when space already exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention already exists: %v", err)
	}
}

// --- branch conflict ---

func TestRunSpaceCreate_BranchConflict(t *testing.T) {
	root := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")
	cfg := spaceCreateCfg(root, t.TempDir())
	r := &testRunner{
		branchExistsFn: func(_, _ string) (bool, error) { return true, nil },
		worktreeListFn: func(_ string) ([]git.WorktreeInfo, error) {
			return []git.WorktreeInfo{
				{Path: "/some/worktree", Branch: "feat"},
			}, nil
		},
	}
	var out bytes.Buffer
	err := RunSpaceCreate(cfg, r, SpaceCreateArgs{Name: "feat", Branch: "feat"}, &out)
	if err == nil {
		t.Fatal("expected error for branch already checked out")
	}
	if !strings.Contains(err.Error(), "already checked out") {
		t.Errorf("error should mention already checked out: %v", err)
	}
}

func TestRunSpaceCreate_ExistingBranchNotCheckedOut(t *testing.T) {
	// Branch exists but is not checked out anywhere — should succeed and use existing branch.
	root := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")
	cfg := spaceCreateCfg(root, t.TempDir())

	var createBranch bool
	r := &testRunner{
		branchExistsFn: func(_, _ string) (bool, error) { return true, nil },
		worktreeListFn: func(_ string) ([]git.WorktreeInfo, error) {
			// Branch is not checked out in any worktree.
			return []git.WorktreeInfo{
				{Path: "/repo/api", Branch: "main"},
			}, nil
		},
		worktreeAddFn: func(_, _, _ string, create bool) error {
			createBranch = create
			return nil
		},
	}
	var out bytes.Buffer
	if err := RunSpaceCreate(cfg, r, SpaceCreateArgs{Name: "feat", Branch: "feat"}, &out); err != nil {
		t.Fatalf("RunSpaceCreate: %v", err)
	}
	if createBranch {
		t.Error("should not create branch when it already exists")
	}
}

// --- happy path ---

func TestRunSpaceCreate_Success(t *testing.T) {
	root := t.TempDir()
	spacesRoot := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")
	makeRepo(t, root, "frontend")
	cfg := spaceCreateCfg(root, spacesRoot)

	var added [][2]string
	r := &testRunner{
		branchExistsFn: func(_, _ string) (bool, error) { return false, nil },
		worktreeAddFn: func(repoPath, worktreePath, _ string, _ bool) error {
			added = append(added, [2]string{repoPath, worktreePath})
			return nil
		},
	}

	var out bytes.Buffer
	if err := RunSpaceCreate(cfg, r, SpaceCreateArgs{Name: "feat"}, &out); err != nil {
		t.Fatalf("RunSpaceCreate: %v", err)
	}
	if len(added) != 2 {
		t.Errorf("expected 2 worktree adds, got %d", len(added))
	}
	got := out.String()
	if !strings.Contains(got, "feat") {
		t.Errorf("output missing space name: %q", got)
	}

	sp, err := state.Load("feat")
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	if sp.Branch != "feat" {
		t.Errorf("branch: got %q, want %q", sp.Branch, "feat")
	}
	if len(sp.Repos) != 2 {
		t.Errorf("expected 2 repos in state, got %d", len(sp.Repos))
	}
}

func TestRunSpaceCreate_DefaultBranch(t *testing.T) {
	root := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")
	cfg := spaceCreateCfg(root, t.TempDir())
	cfg.Git.BranchPrefix = "geoff/"

	var usedBranch string
	r := &testRunner{
		branchExistsFn: func(_, _ string) (bool, error) { return false, nil },
		worktreeAddFn: func(_, _, branch string, _ bool) error {
			usedBranch = branch
			return nil
		},
	}

	var out bytes.Buffer
	if err := RunSpaceCreate(cfg, r, SpaceCreateArgs{Name: "feat"}, &out); err != nil {
		t.Fatalf("RunSpaceCreate: %v", err)
	}
	if usedBranch != "geoff/feat" {
		t.Errorf("branch: got %q, want %q", usedBranch, "geoff/feat")
	}
}

func TestRunSpaceCreate_NamedRepos(t *testing.T) {
	root := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")
	makeRepo(t, root, "frontend")
	cfg := spaceCreateCfg(root, t.TempDir())

	var added []string
	r := &testRunner{
		branchExistsFn: func(_, _ string) (bool, error) { return false, nil },
		worktreeAddFn: func(repoPath, _, _ string, _ bool) error {
			added = append(added, repoPath)
			return nil
		},
	}

	var out bytes.Buffer
	if err := RunSpaceCreate(cfg, r, SpaceCreateArgs{
		Name:  "feat",
		Repos: []string{"api"},
	}, &out); err != nil {
		t.Fatalf("RunSpaceCreate: %v", err)
	}
	if len(added) != 1 {
		t.Errorf("expected 1 worktree add (api only), got %d", len(added))
	}
}

// --- saga rollback ---

func TestRunSpaceCreate_RollbackOnFailure(t *testing.T) {
	root := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")
	makeRepo(t, root, "frontend")
	cfg := spaceCreateCfg(root, t.TempDir())

	addCount := 0
	var removed []string
	r := &testRunner{
		branchExistsFn: func(_, _ string) (bool, error) { return false, nil },
		worktreeAddFn: func(_, worktreePath, _ string, _ bool) error {
			addCount++
			if addCount == 2 {
				return fmt.Errorf("disk full")
			}
			return nil
		},
		worktreeRemoveFn: func(_, worktreePath string, _ bool) error {
			removed = append(removed, worktreePath)
			return nil
		},
	}

	var out bytes.Buffer
	err := RunSpaceCreate(cfg, r, SpaceCreateArgs{Name: "feat"}, &out)
	if err == nil {
		t.Fatal("expected error on second worktree add failure")
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Errorf("error should mention original cause: %v", err)
	}
	// First worktree (api) should have been rolled back.
	if len(removed) != 1 {
		t.Errorf("expected 1 worktree removed in rollback, got %d: %v", len(removed), removed)
	}
	// State should NOT be saved.
	if _, err := state.Load("feat"); err == nil {
		t.Error("state should not be saved when saga fails")
	}
}

// --- go.work ---

func TestRunSpaceCreate_GoWork(t *testing.T) {
	root := t.TempDir()
	spacesRoot := t.TempDir()
	isolateState(t)
	apiPath := makeRepo(t, root, "api")
	if err := os.WriteFile(filepath.Join(apiPath, "go.mod"),
		[]byte("module example.com/api\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	makeRepo(t, root, "frontend") // no go.mod

	cfg := spaceCreateCfg(root, spacesRoot)
	var out bytes.Buffer
	if err := RunSpaceCreate(cfg, createRunner(), SpaceCreateArgs{Name: "feat"}, &out); err != nil {
		t.Fatalf("RunSpaceCreate: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(spacesRoot, "feat", "go.work"))
	if err != nil {
		t.Fatalf("go.work not written: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "./api") {
		t.Errorf("go.work should reference ./api: %q", content)
	}
	if strings.Contains(content, "frontend") {
		t.Errorf("go.work should not reference frontend (no go.mod): %q", content)
	}
}

func TestRunSpaceCreate_NoGoWork_WhenNoGoMods(t *testing.T) {
	root := t.TempDir()
	spacesRoot := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api") // no go.mod
	cfg := spaceCreateCfg(root, spacesRoot)

	var out bytes.Buffer
	if err := RunSpaceCreate(cfg, createRunner(), SpaceCreateArgs{Name: "feat"}, &out); err != nil {
		t.Fatalf("RunSpaceCreate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(spacesRoot, "feat", "go.work")); err == nil {
		t.Error("go.work should not be created when no repos have go.mod")
	}

	sp, err := state.Load("feat")
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	if sp.GoWorkspace {
		t.Error("GoWorkspace should be false when no go.mod found")
	}
}
