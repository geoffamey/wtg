package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"

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
		worktreeAddFn:  func(_, _, _, _ string, _ bool) error { return nil },
	}
}

// createRunnerMkdir is like createRunner but creates the worktree directory so
// filesystem copies (e.g. always.secrets) can land under it.
func createRunnerMkdir() *testRunner {
	return &testRunner{
		branchExistsFn: func(_, _ string) (bool, error) { return false, nil },
		worktreeAddFn: func(_, worktreePath, _, _ string, _ bool) error {
			return os.MkdirAll(worktreePath, 0o755)
		},
		worktreeRemoveFn: func(_, worktreePath string, _ bool) error {
			return os.RemoveAll(worktreePath)
		},
		branchDeleteFn: func(_, _ string, _ bool) error { return nil },
	}
}

// newApp wraps NewCommand in a minimal parent app so Action can call cmd.Root().
func newApp(runner git.Runner) *cli.Command {
	return &cli.Command{
		Name: "wtg",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config"},
		},
		Commands: []*cli.Command{NewCommand(runner)},
	}
}

// --- CLI action validation ---

func TestNewCommand_NoRepoArg(t *testing.T) {
	err := newApp(&testRunner{}).Run(context.Background(), []string{"wtg", "new", "feat"})
	if err == nil {
		t.Fatal("expected error when no repo argument given")
	}
	if !strings.Contains(err.Error(), "at least one repo") {
		t.Errorf("error should mention repo requirement: %v", err)
	}
}

func TestNewCommand_NoSpaceArg(t *testing.T) {
	err := newApp(&testRunner{}).Run(context.Background(), []string{"wtg", "new"})
	if err == nil {
		t.Fatal("expected error when no space argument given")
	}
}

// --- validation errors ---

func TestRunSpaceNew_NoSpacesRoot(t *testing.T) {
	isolateState(t)
	cfg := &config.Config{
		Discovery: config.DiscoveryConfig{RootDir: t.TempDir(), MaxDepth: 2},
	}
	var out bytes.Buffer
	if err := RunSpaceNew(cfg, &testRunner{}, SpaceNewArgs{Name: "feat"}, &out); err == nil {
		t.Fatal("expected error when spaces.root_dir is empty and no --path")
	}
}

func TestRunSpaceNew_NoDiscoveryRoot(t *testing.T) {
	isolateState(t)
	cfg := &config.Config{
		Spaces: config.SpacesConfig{RootDir: t.TempDir()},
	}
	var out bytes.Buffer
	if err := RunSpaceNew(cfg, &testRunner{}, SpaceNewArgs{Name: "feat"}, &out); err == nil {
		t.Fatal("expected error when discovery.root_dir is empty")
	}
}

func TestRunSpaceNew_PathFlagBypassesSpacesRoot(t *testing.T) {
	root := t.TempDir()
	spacePath := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")
	// cfg has no spaces.root_dir — should be OK because --path is supplied
	cfg := &config.Config{
		Discovery: config.DiscoveryConfig{RootDir: root, MaxDepth: 2},
	}
	var out bytes.Buffer
	if err := RunSpaceNew(cfg, createRunner(), SpaceNewArgs{
		Name: "feat",
		Path: spacePath,
	}, &out); err != nil {
		t.Fatalf("expected success with --path override: %v", err)
	}
}

func TestRunSpaceNew_NoRepos(t *testing.T) {
	root := t.TempDir()
	isolateState(t)
	cfg := spaceCreateCfg(root, t.TempDir())
	var out bytes.Buffer
	if err := RunSpaceNew(cfg, &testRunner{}, SpaceNewArgs{Name: "feat"}, &out); err == nil {
		t.Fatal("expected error when no repos found")
	}
}

func TestRunSpaceNew_UnknownRepo(t *testing.T) {
	root := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")
	cfg := spaceCreateCfg(root, t.TempDir())
	var out bytes.Buffer
	err := RunSpaceNew(cfg, &testRunner{}, SpaceNewArgs{
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

func TestRunSpaceNew_SpaceAlreadyExists(t *testing.T) {
	root := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")
	cfg := spaceCreateCfg(root, t.TempDir())
	if err := state.Save(&state.Space{Name: "feat", Path: t.TempDir(), Branch: "feat"}); err != nil {
		t.Fatalf("state.Save: %v", err)
	}
	var out bytes.Buffer
	err := RunSpaceNew(cfg, &testRunner{}, SpaceNewArgs{Name: "feat"}, &out)
	if err == nil {
		t.Fatal("expected error when space already exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention already exists: %v", err)
	}
}

// --- branch conflict ---

func TestRunSpaceNew_BranchConflict(t *testing.T) {
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
	err := RunSpaceNew(cfg, r, SpaceNewArgs{Name: "feat", Branch: "feat"}, &out)
	if err == nil {
		t.Fatal("expected error for branch already checked out")
	}
	if !strings.Contains(err.Error(), "already checked out") {
		t.Errorf("error should mention already checked out: %v", err)
	}
}

func TestRunSpaceNew_ExistingBranchNotCheckedOut(t *testing.T) {
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
		worktreeAddFn: func(_, _, _, _ string, create bool) error {
			createBranch = create
			return nil
		},
	}
	var out bytes.Buffer
	if err := RunSpaceNew(cfg, r, SpaceNewArgs{Name: "feat", Branch: "feat"}, &out); err != nil {
		t.Fatalf("RunSpaceNew: %v", err)
	}
	if createBranch {
		t.Error("should not create branch when it already exists")
	}
}

func TestRunSpaceNew_RemoteOnlyBranch(t *testing.T) {
	// Branch exists on remote but not locally — should create a local branch from the remote ref.
	root := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")
	cfg := spaceCreateCfg(root, t.TempDir())

	var gotBase string
	var gotCreate bool
	r := &testRunner{
		branchExistsFn:       func(_, _ string) (bool, error) { return false, nil },
		remoteBranchExistsFn: func(_, _ string) (bool, error) { return true, nil },
		worktreeAddFn: func(_, _, _, base string, create bool) error {
			gotBase = base
			gotCreate = create
			return nil
		},
	}
	var out bytes.Buffer
	if err := RunSpaceNew(cfg, r, SpaceNewArgs{Name: "feat", Branch: "feat"}, &out); err != nil {
		t.Fatalf("RunSpaceNew: %v", err)
	}
	if !gotCreate {
		t.Error("should create local branch when only remote exists")
	}
	if gotBase != "origin/feat" {
		t.Errorf("base: got %q, want %q", gotBase, "origin/feat")
	}
}

// --- happy path ---

func TestRunSpaceNew_Success(t *testing.T) {
	root := t.TempDir()
	spacesRoot := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")
	makeRepo(t, root, "frontend")
	cfg := spaceCreateCfg(root, spacesRoot)

	var added [][2]string
	r := &testRunner{
		branchExistsFn: func(_, _ string) (bool, error) { return false, nil },
		worktreeAddFn: func(repoPath, worktreePath, _, _ string, _ bool) error {
			added = append(added, [2]string{repoPath, worktreePath})
			return nil
		},
	}

	var out bytes.Buffer
	if err := RunSpaceNew(cfg, r, SpaceNewArgs{Name: "feat"}, &out); err != nil {
		t.Fatalf("RunSpaceNew: %v", err)
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

func TestRunSpaceNew_DefaultBranch(t *testing.T) {
	root := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")
	cfg := spaceCreateCfg(root, t.TempDir())
	cfg.Git.BranchPrefix = "geoff/"

	var usedBranch string
	r := &testRunner{
		branchExistsFn: func(_, _ string) (bool, error) { return false, nil },
		worktreeAddFn: func(_, _, branch, _ string, _ bool) error {
			usedBranch = branch
			return nil
		},
	}

	var out bytes.Buffer
	if err := RunSpaceNew(cfg, r, SpaceNewArgs{Name: "feat"}, &out); err != nil {
		t.Fatalf("RunSpaceNew: %v", err)
	}
	if usedBranch != "geoff/feat" {
		t.Errorf("branch: got %q, want %q", usedBranch, "geoff/feat")
	}
}

func TestRunSpaceNew_BaseFlag(t *testing.T) {
	root := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")
	cfg := spaceCreateCfg(root, t.TempDir())

	var gotBase string
	r := &testRunner{
		branchExistsFn: func(_, _ string) (bool, error) { return false, nil },
		worktreeAddFn: func(_, _, _, base string, _ bool) error {
			gotBase = base
			return nil
		},
	}

	var out bytes.Buffer
	if err := RunSpaceNew(cfg, r, SpaceNewArgs{Name: "feat", Base: "origin/main"}, &out); err != nil {
		t.Fatalf("RunSpaceNew: %v", err)
	}
	if gotBase != "origin/main" {
		t.Errorf("base: got %q, want %q", gotBase, "origin/main")
	}
}

func TestRunSpaceNew_NoBase_PassesEmpty(t *testing.T) {
	root := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")
	cfg := spaceCreateCfg(root, t.TempDir())

	var gotBase string
	r := &testRunner{
		branchExistsFn: func(_, _ string) (bool, error) { return false, nil },
		worktreeAddFn: func(_, _, _, base string, _ bool) error {
			gotBase = base
			return nil
		},
	}

	var out bytes.Buffer
	if err := RunSpaceNew(cfg, r, SpaceNewArgs{Name: "feat"}, &out); err != nil {
		t.Fatalf("RunSpaceNew: %v", err)
	}
	if gotBase != "" {
		t.Errorf("base should be empty when not set, got %q", gotBase)
	}
}

func TestRunSpaceNew_NamedRepos(t *testing.T) {
	root := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")
	makeRepo(t, root, "frontend")
	cfg := spaceCreateCfg(root, t.TempDir())

	var added []string
	r := &testRunner{
		branchExistsFn: func(_, _ string) (bool, error) { return false, nil },
		worktreeAddFn: func(repoPath, _, _, _ string, _ bool) error {
			added = append(added, repoPath)
			return nil
		},
	}

	var out bytes.Buffer
	if err := RunSpaceNew(cfg, r, SpaceNewArgs{
		Name:  "feat",
		Repos: []string{"api"},
	}, &out); err != nil {
		t.Fatalf("RunSpaceNew: %v", err)
	}
	if len(added) != 1 {
		t.Errorf("expected 1 worktree add (api only), got %d", len(added))
	}
}

// --- saga rollback ---

func TestRunSpaceNew_RollbackOnFailure(t *testing.T) {
	root := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")
	makeRepo(t, root, "frontend")
	cfg := spaceCreateCfg(root, t.TempDir())

	addCount := 0
	var removed []string
	r := &testRunner{
		branchExistsFn: func(_, _ string) (bool, error) { return false, nil },
		worktreeAddFn: func(_, worktreePath, _, _ string, _ bool) error {
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
		branchDeleteFn: func(_, _ string, _ bool) error { return nil },
	}

	var out bytes.Buffer
	err := RunSpaceNew(cfg, r, SpaceNewArgs{Name: "feat"}, &out)
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

func TestRunSpaceNew_RollbackDeletesCreatedBranch(t *testing.T) {
	root := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")
	makeRepo(t, root, "frontend")
	cfg := spaceCreateCfg(root, t.TempDir())

	addCount := 0
	var deletedBranches []string
	r := &testRunner{
		branchExistsFn: func(_, _ string) (bool, error) { return false, nil }, // new branch
		worktreeAddFn: func(_, _, _, _ string, _ bool) error {
			addCount++
			if addCount == 2 {
				return fmt.Errorf("disk full")
			}
			return nil
		},
		worktreeRemoveFn: func(_, _ string, _ bool) error { return nil },
		branchDeleteFn: func(_, branch string, _ bool) error {
			deletedBranches = append(deletedBranches, branch)
			return nil
		},
	}

	var out bytes.Buffer
	_ = RunSpaceNew(cfg, r, SpaceNewArgs{Name: "feat"}, &out)

	if len(deletedBranches) != 1 || deletedBranches[0] != "feat" {
		t.Errorf("expected created branch to be deleted on rollback, got %v", deletedBranches)
	}
}

func TestRunSpaceNew_RollbackPreservesExistingBranch(t *testing.T) {
	root := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")
	makeRepo(t, root, "frontend")
	cfg := spaceCreateCfg(root, t.TempDir())

	addCount := 0
	branchDeleted := false
	r := &testRunner{
		// Branch already exists but is not checked out.
		branchExistsFn: func(_, _ string) (bool, error) { return true, nil },
		worktreeListFn: func(_ string) ([]git.WorktreeInfo, error) {
			return []git.WorktreeInfo{{Path: "/repo", Branch: "main"}}, nil
		},
		worktreeAddFn: func(_, _, _, _ string, _ bool) error {
			addCount++
			if addCount == 2 {
				return fmt.Errorf("disk full")
			}
			return nil
		},
		worktreeRemoveFn: func(_, _ string, _ bool) error { return nil },
		branchDeleteFn: func(_, _ string, _ bool) error {
			branchDeleted = true
			return nil
		},
	}

	var out bytes.Buffer
	_ = RunSpaceNew(cfg, r, SpaceNewArgs{Name: "feat", Branch: "feat"}, &out)

	if branchDeleted {
		t.Error("pre-existing branch should not be deleted on rollback")
	}
}

// --- nested repos ---

func TestRunSpaceNew_NestedRepoLayout(t *testing.T) {
	root := t.TempDir()
	spacesRoot := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "myorg/api")
	makeRepo(t, root, "myorg/frontend")
	cfg := spaceCreateCfg(root, spacesRoot)

	var worktreePaths []string
	r := &testRunner{
		branchExistsFn: func(_, _ string) (bool, error) { return false, nil },
		worktreeAddFn: func(_, worktreePath, _, _ string, _ bool) error {
			worktreePaths = append(worktreePaths, worktreePath)
			return nil
		},
	}

	var out bytes.Buffer
	if err := RunSpaceNew(cfg, r, SpaceNewArgs{Name: "feat"}, &out); err != nil {
		t.Fatalf("RunSpaceNew: %v", err)
	}

	spaceRoot := filepath.Join(spacesRoot, "feat")
	want := []string{
		filepath.Join(spaceRoot, "myorg", "api"),
		filepath.Join(spaceRoot, "myorg", "frontend"),
	}
	if len(worktreePaths) != len(want) {
		t.Fatalf("got %d worktree paths, want %d: %v", len(worktreePaths), len(want), worktreePaths)
	}
	for i, got := range worktreePaths {
		if got != want[i] {
			t.Errorf("worktree[%d]: got %q, want %q", i, got, want[i])
		}
	}

	// State should record the same paths.
	sp, err := state.Load("feat")
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	for _, repo := range sp.Repos {
		expected := filepath.Join(spaceRoot, filepath.FromSlash(repo.Name))
		if repo.WorktreePath != expected {
			t.Errorf("state WorktreePath for %s: got %q, want %q", repo.Name, repo.WorktreePath, expected)
		}
	}
}

// --- go.work ---

func TestRunSpaceNew_GoWork(t *testing.T) {
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
	if err := RunSpaceNew(cfg, createRunner(), SpaceNewArgs{Name: "feat"}, &out); err != nil {
		t.Fatalf("RunSpaceNew: %v", err)
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
	// go directive should be read from the module's go.mod, not hardcoded.
	if !strings.Contains(content, "go 1.24") {
		t.Errorf("go.work should carry go version from go.mod: %q", content)
	}
}

func TestRunSpaceNew_GoWork_MaxVersion(t *testing.T) {
	// When multiple modules declare different go versions, the highest wins.
	root := t.TempDir()
	spacesRoot := t.TempDir()
	isolateState(t)
	for name, ver := range map[string]string{"api": "1.22", "svc": "1.24", "lib": "1.21"} {
		p := makeRepo(t, root, name)
		if err := os.WriteFile(filepath.Join(p, "go.mod"),
			[]byte("module example.com/"+name+"\n\ngo "+ver+"\n"), 0o644); err != nil {
			t.Fatalf("write go.mod: %v", err)
		}
	}

	cfg := spaceCreateCfg(root, spacesRoot)
	var out bytes.Buffer
	if err := RunSpaceNew(cfg, createRunner(), SpaceNewArgs{Name: "feat"}, &out); err != nil {
		t.Fatalf("RunSpaceNew: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(spacesRoot, "feat", "go.work"))
	if err != nil {
		t.Fatalf("go.work not written: %v", err)
	}
	if !strings.Contains(string(data), "go 1.24") {
		t.Errorf("go.work should use max go version (1.24): %q", string(data))
	}
}

func TestCmpGoVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want int // sign only: <0, 0, >0
	}{
		{"1.21", "1.21", 0},
		{"1.22", "1.21", 1},
		{"1.21", "1.22", -1},
		{"1.21.0", "1.21", 0},
		{"1.21.1", "1.21.0", 1},
		{"1.24", "1.9", 1},
	}
	for _, c := range cases {
		got := cmpGoVersion(c.a, c.b)
		switch {
		case c.want == 0 && got != 0:
			t.Errorf("cmpGoVersion(%q, %q) = %d, want 0", c.a, c.b, got)
		case c.want > 0 && got <= 0:
			t.Errorf("cmpGoVersion(%q, %q) = %d, want >0", c.a, c.b, got)
		case c.want < 0 && got >= 0:
			t.Errorf("cmpGoVersion(%q, %q) = %d, want <0", c.a, c.b, got)
		}
	}
}

func TestRunSpaceNew_GoWorkRemovedOnRollback(t *testing.T) {
	root := t.TempDir()
	spacesRoot := t.TempDir()
	apiPath := makeRepo(t, root, "api")
	if err := os.WriteFile(filepath.Join(apiPath, "go.mod"),
		[]byte("module example.com/api\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	cfg := spaceCreateCfg(root, spacesRoot)

	// Make state.Save fail by pointing XDG_DATA_HOME at a regular file so
	// os.MkdirAll cannot create the state directory beneath it.
	blockFile := filepath.Join(t.TempDir(), "block")
	if err := os.WriteFile(blockFile, nil, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	t.Setenv("XDG_DATA_HOME", blockFile)

	var out bytes.Buffer
	err := RunSpaceNew(cfg, createRunner(), SpaceNewArgs{Name: "feat"}, &out)
	if err == nil {
		t.Fatal("expected error when state save fails")
	}

	// go.work should have been removed by the saga undo.
	goWorkPath := filepath.Join(spacesRoot, "feat", "go.work")
	if _, statErr := os.Stat(goWorkPath); statErr == nil {
		t.Error("go.work should be removed when saga rolls back after state save failure")
	}
}

func TestRunSpaceNew_NoGoWork_WhenNoGoMods(t *testing.T) {
	root := t.TempDir()
	spacesRoot := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api") // no go.mod
	cfg := spaceCreateCfg(root, spacesRoot)

	var out bytes.Buffer
	if err := RunSpaceNew(cfg, createRunner(), SpaceNewArgs{Name: "feat"}, &out); err != nil {
		t.Fatalf("RunSpaceNew: %v", err)
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

// --- always.repos ---

func alwaysCfg(root, spacesRoot string, alwaysRepos []string) *config.Config {
	return &config.Config{
		Discovery: config.DiscoveryConfig{RootDir: root, MaxDepth: 2},
		Spaces:    config.SpacesConfig{RootDir: spacesRoot},
		Always:    config.AlwaysConfig{Repos: alwaysRepos},
	}
}

func TestRunSpaceNew_AlwaysRepos_SymlinkedIntoSpace(t *testing.T) {
	root := t.TempDir()
	spacesRoot := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")
	makeRepo(t, root, "docs")
	cfg := alwaysCfg(root, spacesRoot, []string{"docs"})

	var worktreeAdded []string
	r := &testRunner{
		branchExistsFn: func(_, _ string) (bool, error) { return false, nil },
		worktreeAddFn: func(_, worktreePath, _, _ string, _ bool) error {
			worktreeAdded = append(worktreeAdded, worktreePath)
			return nil
		},
	}

	var out bytes.Buffer
	if err := RunSpaceNew(cfg, r, SpaceNewArgs{Name: "feat", Repos: []string{"api"}}, &out); err != nil {
		t.Fatalf("RunSpaceNew: %v", err)
	}

	// Only api gets a worktree; docs is symlinked.
	if len(worktreeAdded) != 1 || !strings.Contains(worktreeAdded[0], "api") {
		t.Errorf("expected 1 worktree add for api, got %v", worktreeAdded)
	}

	// docs symlink should exist in the space.
	docsLink := filepath.Join(spacesRoot, "feat", "docs")
	target, err := os.Readlink(docsLink)
	if err != nil {
		t.Fatalf("docs symlink not created: %v", err)
	}
	if target != filepath.Join(root, "docs") {
		t.Errorf("symlink target: got %q, want %q", target, filepath.Join(root, "docs"))
	}

	// State should record docs as a symlink.
	sp, err := state.Load("feat")
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	if len(sp.Repos) != 2 {
		t.Fatalf("expected 2 repos in state, got %d", len(sp.Repos))
	}
	var docsEntry *state.RepoEntry
	for i := range sp.Repos {
		if sp.Repos[i].Name == "docs" {
			docsEntry = &sp.Repos[i]
		}
	}
	if docsEntry == nil {
		t.Fatal("docs not in state")
	}
	if !docsEntry.Symlink {
		t.Error("docs should be recorded as a symlink in state")
	}
}

func TestRunSpaceNew_AlwaysRepos_ExplicitOverridesSymlink(t *testing.T) {
	// When docs is in always.repos but also explicitly in args.Repos, it
	// should get a real worktree, not a symlink.
	root := t.TempDir()
	spacesRoot := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")
	makeRepo(t, root, "docs")
	cfg := alwaysCfg(root, spacesRoot, []string{"docs"})

	var worktreeAdded []string
	r := &testRunner{
		branchExistsFn: func(_, _ string) (bool, error) { return false, nil },
		worktreeAddFn: func(_, worktreePath, _, _ string, _ bool) error {
			worktreeAdded = append(worktreeAdded, worktreePath)
			return nil
		},
	}

	var out bytes.Buffer
	if err := RunSpaceNew(cfg, r, SpaceNewArgs{Name: "feat", Repos: []string{"api", "docs"}}, &out); err != nil {
		t.Fatalf("RunSpaceNew: %v", err)
	}

	// Both repos get worktrees; no symlink.
	if len(worktreeAdded) != 2 {
		t.Errorf("expected 2 worktree adds, got %d: %v", len(worktreeAdded), worktreeAdded)
	}
	docsLink := filepath.Join(spacesRoot, "feat", "docs")
	if fi, err := os.Lstat(docsLink); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		t.Error("docs should not be a symlink when explicitly passed as a repo")
	}
}

func TestRunSpaceNew_AlwaysRepos_UnknownRepo_Errors(t *testing.T) {
	root := t.TempDir()
	spacesRoot := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")
	cfg := alwaysCfg(root, spacesRoot, []string{"no-such-repo"})

	var out bytes.Buffer
	err := RunSpaceNew(cfg, createRunner(), SpaceNewArgs{Name: "feat", Repos: []string{"api"}}, &out)
	if err == nil {
		t.Fatal("expected error when always.repos entry not found")
	}
	if !strings.Contains(err.Error(), "no-such-repo") {
		t.Errorf("error should mention the unknown repo: %v", err)
	}
}

func TestRunSpaceNew_AlwaysRepos_NotInGoWork(t *testing.T) {
	// A symlinked always-repo with a go.mod should NOT appear in go.work.
	root := t.TempDir()
	spacesRoot := t.TempDir()
	isolateState(t)
	apiPath := makeRepo(t, root, "api")
	if err := os.WriteFile(filepath.Join(apiPath, "go.mod"),
		[]byte("module example.com/api\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	docsPath := makeRepo(t, root, "docs")
	if err := os.WriteFile(filepath.Join(docsPath, "go.mod"),
		[]byte("module example.com/docs\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	cfg := alwaysCfg(root, spacesRoot, []string{"docs"})

	var out bytes.Buffer
	if err := RunSpaceNew(cfg, createRunner(), SpaceNewArgs{Name: "feat", Repos: []string{"api"}}, &out); err != nil {
		t.Fatalf("RunSpaceNew: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(spacesRoot, "feat", "go.work"))
	if err != nil {
		t.Fatalf("go.work not written: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "./api") {
		t.Errorf("go.work should reference ./api: %q", content)
	}
	if strings.Contains(content, "docs") {
		t.Errorf("go.work should not reference symlinked docs repo: %q", content)
	}
}

func TestRunSpaceNew_AlwaysRepos_RollbackRemovesSymlink(t *testing.T) {
	root := t.TempDir()
	spacesRoot := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")
	makeRepo(t, root, "docs")
	cfg := alwaysCfg(root, spacesRoot, []string{"docs"})

	// Make the state save fail so the saga rolls back.
	blockFile := filepath.Join(t.TempDir(), "block")
	if err := os.WriteFile(blockFile, nil, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	t.Setenv("XDG_DATA_HOME", blockFile)

	var out bytes.Buffer
	err := RunSpaceNew(cfg, createRunner(), SpaceNewArgs{Name: "feat", Repos: []string{"api"}}, &out)
	if err == nil {
		t.Fatal("expected error when state save fails")
	}

	docsLink := filepath.Join(spacesRoot, "feat", "docs")
	if _, err := os.Lstat(docsLink); err == nil {
		t.Error("docs symlink should be removed on rollback")
	}
}

// --- always.files ---

func TestRunSpaceNew_AlwaysFiles_CopiedIntoSpace(t *testing.T) {
	root := t.TempDir()
	spacesRoot := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")

	// Create a source file to be copied.
	srcFile := filepath.Join(t.TempDir(), "CLAUDE.md")
	if err := os.WriteFile(srcFile, []byte("# context\n"), 0o644); err != nil {
		t.Fatalf("write src file: %v", err)
	}

	cfg := &config.Config{
		Discovery: config.DiscoveryConfig{RootDir: root, MaxDepth: 2},
		Spaces:    config.SpacesConfig{RootDir: spacesRoot},
		Always:    config.AlwaysConfig{Files: []string{srcFile}},
	}

	var out bytes.Buffer
	if err := RunSpaceNew(cfg, createRunner(), SpaceNewArgs{Name: "feat", Repos: []string{"api"}}, &out); err != nil {
		t.Fatalf("RunSpaceNew: %v", err)
	}

	dst := filepath.Join(spacesRoot, "feat", "CLAUDE.md")
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("copied file not found: %v", err)
	}
	if string(data) != "# context\n" {
		t.Errorf("copied file content: got %q", string(data))
	}
}

func TestRunSpaceNew_AlwaysFiles_MissingSource_Errors(t *testing.T) {
	root := t.TempDir()
	spacesRoot := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")

	cfg := &config.Config{
		Discovery: config.DiscoveryConfig{RootDir: root, MaxDepth: 2},
		Spaces:    config.SpacesConfig{RootDir: spacesRoot},
		Always:    config.AlwaysConfig{Files: []string{"/no/such/file.md"}},
	}

	r := &testRunner{
		branchExistsFn:   func(_, _ string) (bool, error) { return false, nil },
		worktreeAddFn:    func(_, _, _, _ string, _ bool) error { return nil },
		worktreeRemoveFn: func(_, _ string, _ bool) error { return nil },
		branchDeleteFn:   func(_, _ string, _ bool) error { return nil },
	}

	var out bytes.Buffer
	err := RunSpaceNew(cfg, r, SpaceNewArgs{Name: "feat", Repos: []string{"api"}}, &out)
	if err == nil {
		t.Fatal("expected error when always.files source is missing")
	}
}

// --- always.secrets ---

func TestRunSpaceNew_AlwaysSecrets_CopiesWhenPresent(t *testing.T) {
	root := t.TempDir()
	spacesRoot := t.TempDir()
	isolateState(t)
	api := makeRepo(t, root, "api")

	cfgDir := filepath.Join(api, "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	src := filepath.Join(cfgDir, "local.env")
	if err := os.WriteFile(src, []byte("SECRET=1\n"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	cfg := spaceCreateCfg(root, spacesRoot)
	cfg.Always.Secrets = []string{"config/local.env"}
	var out bytes.Buffer
	if err := RunSpaceNew(cfg, createRunnerMkdir(), SpaceNewArgs{Name: "feat", Repos: []string{"api"}}, &out); err != nil {
		t.Fatalf("RunSpaceNew: %v", err)
	}

	dst := filepath.Join(spacesRoot, "feat", "api", "config", "local.env")
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("copied file not found: %v", err)
	}
	if string(data) != "SECRET=1\n" {
		t.Errorf("copied content: got %q", string(data))
	}
}

func TestRunSpaceNew_AlwaysSecrets_MissingSkipped(t *testing.T) {
	root := t.TempDir()
	spacesRoot := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")

	cfg := spaceCreateCfg(root, spacesRoot)
	cfg.Always.Secrets = []string{"missing.env"}
	var out bytes.Buffer
	if err := RunSpaceNew(cfg, createRunnerMkdir(), SpaceNewArgs{Name: "feat", Repos: []string{"api"}}, &out); err != nil {
		t.Fatalf("RunSpaceNew: %v", err)
	}
}

func TestRunSpaceNew_AlwaysSecrets_DirectoryErrors(t *testing.T) {
	root := t.TempDir()
	spacesRoot := t.TempDir()
	isolateState(t)
	api := makeRepo(t, root, "api")
	if err := os.MkdirAll(filepath.Join(api, "secrets"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	cfg := spaceCreateCfg(root, spacesRoot)
	cfg.Always.Secrets = []string{"secrets"}
	var out bytes.Buffer
	err := RunSpaceNew(cfg, createRunnerMkdir(), SpaceNewArgs{Name: "feat", Repos: []string{"api"}}, &out)
	if err == nil {
		t.Fatal("expected error when always.secrets points at a directory")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Errorf("error should mention directory: %v", err)
	}
}

func TestRunSpaceNew_AlwaysSecrets_AlwaysRepoSymlink_Skipped(t *testing.T) {
	root := t.TempDir()
	spacesRoot := t.TempDir()
	isolateState(t)
	makeRepo(t, root, "api")
	docs := makeRepo(t, root, "docs")

	if err := os.WriteFile(filepath.Join(docs, "local.env"), []byte("DOCS=1\n"), 0o644); err != nil {
		t.Fatalf("write docs local.env: %v", err)
	}

	cfg := alwaysCfg(root, spacesRoot, []string{"docs"})
	cfg.Always.Secrets = []string{"local.env"}
	var out bytes.Buffer
	if err := RunSpaceNew(cfg, createRunnerMkdir(), SpaceNewArgs{Name: "feat", Repos: []string{"api"}}, &out); err != nil {
		t.Fatalf("RunSpaceNew: %v", err)
	}

	docsLink := filepath.Join(spacesRoot, "feat", "docs")
	fi, err := os.Lstat(docsLink)
	if err != nil {
		t.Fatalf("docs entry: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("docs should remain a symlink")
	}
}
