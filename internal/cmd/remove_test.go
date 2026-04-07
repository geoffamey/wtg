package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geoffamey/wtg/internal/git"
	"github.com/geoffamey/wtg/internal/state"
)

// removeRunner builds a testRunner for remove scenarios.
func removeRunner(statusFn func(string) (git.RepoStatus, error)) *testRunner {
	return &testRunner{
		statusFn:         statusFn,
		worktreeRemoveFn: func(_, _ string, _ bool) error { return nil },
	}
}

// --- validation ---

func TestRunSpaceRemove_SpaceNotFound(t *testing.T) {
	isolateState(t)
	var out bytes.Buffer
	err := RunSpaceRemove(&testRunner{}, SpaceRemoveArgs{Name: "nonexistent", Repos: []string{"api"}}, &bytes.Buffer{}, &out)
	if err == nil {
		t.Fatal("expected error when space does not exist")
	}
}

func TestRunSpaceRemove_RepoNotInSpace(t *testing.T) {
	isolateState(t)
	root := t.TempDir()
	spacePath := t.TempDir()
	makeSpace(t, "feat", "feat", spacePath, []string{"api"}, root)

	var out bytes.Buffer
	err := RunSpaceRemove(&testRunner{}, SpaceRemoveArgs{Name: "feat", Repos: []string{"frontend"}}, &bytes.Buffer{}, &out)
	if err == nil {
		t.Fatal("expected error when repo is not in space")
	}
	if !strings.Contains(err.Error(), "not in space") {
		t.Errorf("error should mention not in space: %v", err)
	}
}

func TestRunSpaceRemove_AllRepos_Rejected(t *testing.T) {
	isolateState(t)
	root := t.TempDir()
	spacePath := t.TempDir()
	makeSpace(t, "feat", "feat", spacePath, []string{"api"}, root)

	var out bytes.Buffer
	err := RunSpaceRemove(&testRunner{}, SpaceRemoveArgs{Name: "feat", Repos: []string{"api"}}, &bytes.Buffer{}, &out)
	if err == nil {
		t.Fatal("expected error when removing all repos")
	}
	if !strings.Contains(err.Error(), "wtg delete") {
		t.Errorf("error should suggest wtg delete: %v", err)
	}
}

// --- happy path ---

func TestRunSpaceRemove_Success(t *testing.T) {
	isolateState(t)
	root := t.TempDir()
	spacePath := t.TempDir()
	makeSpace(t, "feat", "feat", spacePath, []string{"api", "frontend"}, root)

	var removed []string
	r := removeRunner(cleanStatus)
	r.worktreeRemoveFn = func(_, worktreePath string, _ bool) error {
		removed = append(removed, worktreePath)
		return nil
	}

	var out bytes.Buffer
	if err := RunSpaceRemove(r, SpaceRemoveArgs{Name: "feat", Repos: []string{"frontend"}}, &bytes.Buffer{}, &out); err != nil {
		t.Fatalf("RunSpaceRemove: %v", err)
	}

	if len(removed) != 1 || !strings.Contains(removed[0], "frontend") {
		t.Errorf("expected one worktree remove for frontend, got %v", removed)
	}

	sp, err := state.Load("feat")
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	if len(sp.Repos) != 1 || sp.Repos[0].Name != "api" {
		t.Errorf("expected only api remaining, got %v", sp.Repos)
	}
}

func TestRunSpaceRemove_MultipleRepos(t *testing.T) {
	isolateState(t)
	root := t.TempDir()
	spacePath := t.TempDir()
	makeSpace(t, "feat", "feat", spacePath, []string{"api", "frontend", "infra"}, root)

	r := removeRunner(cleanStatus)

	var out bytes.Buffer
	if err := RunSpaceRemove(r, SpaceRemoveArgs{Name: "feat", Repos: []string{"frontend", "infra"}}, &bytes.Buffer{}, &out); err != nil {
		t.Fatalf("RunSpaceRemove: %v", err)
	}

	sp, err := state.Load("feat")
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	if len(sp.Repos) != 1 || sp.Repos[0].Name != "api" {
		t.Errorf("expected only api remaining, got %v", sp.Repos)
	}
}

// --- dirty / unpushed prompts ---

func TestRunSpaceRemove_DirtyWorktree_UserDeclines(t *testing.T) {
	isolateState(t)
	root := t.TempDir()
	spacePath := t.TempDir()
	makeSpace(t, "feat", "feat", spacePath, []string{"api", "frontend"}, root)

	dirtyStatus := func(_ string) (git.RepoStatus, error) {
		return git.RepoStatus{Files: []git.FileStatus{{Path: "a.go", Index: 'M'}}}, nil
	}
	r := removeRunner(dirtyStatus)

	var out bytes.Buffer
	if err := RunSpaceRemove(r, SpaceRemoveArgs{Name: "feat", Repos: []string{"frontend"}}, strings.NewReader("n\n"), &out); err != nil {
		t.Fatalf("RunSpaceRemove: %v", err)
	}
	// State should be unchanged.
	sp, err := state.Load("feat")
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	if len(sp.Repos) != 2 {
		t.Errorf("state should be unchanged when user declines, got %d repos", len(sp.Repos))
	}
	if !strings.Contains(out.String(), "uncommitted changes") {
		t.Errorf("output should mention uncommitted changes: %q", out.String())
	}
}

func TestRunSpaceRemove_DirtyWorktree_UserConfirms(t *testing.T) {
	isolateState(t)
	root := t.TempDir()
	spacePath := t.TempDir()
	makeSpace(t, "feat", "feat", spacePath, []string{"api", "frontend"}, root)

	dirtyStatus := func(_ string) (git.RepoStatus, error) {
		return git.RepoStatus{Files: []git.FileStatus{{Path: "a.go", Index: 'M'}}}, nil
	}
	var usedForce bool
	r := removeRunner(dirtyStatus)
	r.worktreeRemoveFn = func(_, _ string, force bool) error {
		usedForce = force
		return nil
	}

	var out bytes.Buffer
	if err := RunSpaceRemove(r, SpaceRemoveArgs{Name: "feat", Repos: []string{"frontend"}}, strings.NewReader("y\n"), &out); err != nil {
		t.Fatalf("RunSpaceRemove: %v", err)
	}
	if !usedForce {
		t.Error("WorktreeRemove should use force when user confirms despite dirty state")
	}
	sp, err := state.Load("feat")
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	if len(sp.Repos) != 1 {
		t.Errorf("expected 1 repo remaining after confirmed remove, got %d", len(sp.Repos))
	}
}

func TestRunSpaceRemove_UnpushedCommits_Warned(t *testing.T) {
	isolateState(t)
	root := t.TempDir()
	spacePath := t.TempDir()
	makeSpace(t, "feat", "feat", spacePath, []string{"api", "frontend"}, root)

	aheadStatus := func(_ string) (git.RepoStatus, error) {
		return git.RepoStatus{Ahead: 3}, nil
	}
	r := removeRunner(aheadStatus)

	var out bytes.Buffer
	_ = RunSpaceRemove(r, SpaceRemoveArgs{Name: "feat", Repos: []string{"frontend"}}, strings.NewReader("n\n"), &out)
	if !strings.Contains(out.String(), "unpushed") {
		t.Errorf("output should mention unpushed commits: %q", out.String())
	}
}

// --- branch deletion ---

func TestRunSpaceRemove_DeleteBranch(t *testing.T) {
	isolateState(t)
	root := t.TempDir()
	spacePath := t.TempDir()
	makeSpace(t, "feat", "mybranch", spacePath, []string{"api", "frontend"}, root)

	var deletedBranch string
	var deletedForce bool
	r := removeRunner(cleanStatus)
	r.branchDeleteFn = func(_, branch string, force bool) error {
		deletedBranch = branch
		deletedForce = force
		return nil
	}

	var out bytes.Buffer
	if err := RunSpaceRemove(r, SpaceRemoveArgs{Name: "feat", Repos: []string{"frontend"}, DeleteBranch: true}, &bytes.Buffer{}, &out); err != nil {
		t.Fatalf("RunSpaceRemove: %v", err)
	}
	if deletedBranch != "mybranch" {
		t.Errorf("expected branch mybranch to be deleted, got %q", deletedBranch)
	}
	if deletedForce {
		t.Error("-d should not force-delete branch")
	}
}

func TestRunSpaceRemove_ForceBranch(t *testing.T) {
	isolateState(t)
	root := t.TempDir()
	spacePath := t.TempDir()
	makeSpace(t, "feat", "mybranch", spacePath, []string{"api", "frontend"}, root)

	var deletedForce bool
	r := removeRunner(cleanStatus)
	r.branchDeleteFn = func(_, _ string, force bool) error {
		deletedForce = force
		return nil
	}

	var out bytes.Buffer
	if err := RunSpaceRemove(r, SpaceRemoveArgs{Name: "feat", Repos: []string{"frontend"}, ForceBranch: true}, &bytes.Buffer{}, &out); err != nil {
		t.Fatalf("RunSpaceRemove: %v", err)
	}
	if !deletedForce {
		t.Error("-D should force-delete branch")
	}
}

// --- go.work update ---

func TestRunSpaceRemove_GoWorkUpdated(t *testing.T) {
	isolateState(t)
	root := t.TempDir()
	spacesRoot := t.TempDir()

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
	oldGoWork := "go 1.24\n\nuse (\n\t./api\n\t./frontend\n)\n"
	if err := os.WriteFile(filepath.Join(spacePath, "go.work"), []byte(oldGoWork), 0o644); err != nil {
		t.Fatalf("write go.work: %v", err)
	}

	sp := makeSpace(t, "feat", "feat", spacePath, []string{"api", "frontend"}, root)
	sp.GoWorkspace = true
	if err := state.Save(sp); err != nil {
		t.Fatalf("state.Save: %v", err)
	}

	r := removeRunner(cleanStatus)

	var out bytes.Buffer
	if err := RunSpaceRemove(r, SpaceRemoveArgs{Name: "feat", Repos: []string{"frontend"}}, &bytes.Buffer{}, &out); err != nil {
		t.Fatalf("RunSpaceRemove: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(spacePath, "go.work"))
	if err != nil {
		t.Fatalf("read go.work: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "./api") {
		t.Errorf("go.work should still contain ./api: %q", content)
	}
	if strings.Contains(content, "./frontend") {
		t.Errorf("go.work should not contain ./frontend after removal: %q", content)
	}

	// go.work.sum should be removed.
	goWorkSum := filepath.Join(spacePath, "go.work.sum")
	if _, err := os.Stat(goWorkSum); err == nil {
		t.Error("go.work.sum should be removed after go.work update")
	}

	// GoWorkspace flag should remain true.
	loaded, err := state.Load("feat")
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	if !loaded.GoWorkspace {
		t.Error("GoWorkspace should remain true when go.mod repos still remain")
	}
}

func TestRunSpaceRemove_LastGoModRepo_RemovesGoWork(t *testing.T) {
	isolateState(t)
	root := t.TempDir()
	spacesRoot := t.TempDir()

	apiPath := makeRepo(t, root, "api")
	if err := os.WriteFile(filepath.Join(apiPath, "go.mod"),
		[]byte("module example.com/api\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	// frontend has no go.mod — stays in space after api is removed.
	makeRepo(t, root, "frontend")

	spacePath := filepath.Join(spacesRoot, "feat")
	if err := os.MkdirAll(spacePath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	goWorkPath := filepath.Join(spacePath, "go.work")
	goWorkSumPath := filepath.Join(spacePath, "go.work.sum")
	if err := os.WriteFile(goWorkPath, []byte("go 1.24\n\nuse (\n\t./api\n)\n"), 0o644); err != nil {
		t.Fatalf("write go.work: %v", err)
	}
	if err := os.WriteFile(goWorkSumPath, []byte(""), 0o644); err != nil {
		t.Fatalf("write go.work.sum: %v", err)
	}

	sp := makeSpace(t, "feat", "feat", spacePath, []string{"api", "frontend"}, root)
	sp.GoWorkspace = true
	if err := state.Save(sp); err != nil {
		t.Fatalf("state.Save: %v", err)
	}

	r := removeRunner(cleanStatus)

	var out bytes.Buffer
	if err := RunSpaceRemove(r, SpaceRemoveArgs{Name: "feat", Repos: []string{"api"}}, &bytes.Buffer{}, &out); err != nil {
		t.Fatalf("RunSpaceRemove: %v", err)
	}

	if _, err := os.Stat(goWorkPath); err == nil {
		t.Error("go.work should be removed when last go.mod repo is removed")
	}
	if _, err := os.Stat(goWorkSumPath); err == nil {
		t.Error("go.work.sum should be removed when last go.mod repo is removed")
	}

	loaded, err := state.Load("feat")
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	if loaded.GoWorkspace {
		t.Error("GoWorkspace should be false when no go.mod repos remain")
	}
	if len(loaded.Repos) != 1 || loaded.Repos[0].Name != "frontend" {
		t.Errorf("expected only frontend remaining, got %v", loaded.Repos)
	}
}

func TestRunSpaceRemove_GoWorkSumRemovedOnUpdate(t *testing.T) {
	isolateState(t)
	root := t.TempDir()
	spacesRoot := t.TempDir()

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
	infraPath := makeRepo(t, root, "infra")
	if err := os.WriteFile(filepath.Join(infraPath, "go.mod"),
		[]byte("module example.com/infra\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	spacePath := filepath.Join(spacesRoot, "feat")
	if err := os.MkdirAll(spacePath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	goWorkSumPath := filepath.Join(spacePath, "go.work.sum")
	if err := os.WriteFile(filepath.Join(spacePath, "go.work"),
		[]byte("go 1.24\n\nuse (\n\t./api\n\t./frontend\n\t./infra\n)\n"), 0o644); err != nil {
		t.Fatalf("write go.work: %v", err)
	}
	if err := os.WriteFile(goWorkSumPath, []byte("some stale sum entries\n"), 0o644); err != nil {
		t.Fatalf("write go.work.sum: %v", err)
	}

	sp := makeSpace(t, "feat", "feat", spacePath, []string{"api", "frontend", "infra"}, root)
	sp.GoWorkspace = true
	if err := state.Save(sp); err != nil {
		t.Fatalf("state.Save: %v", err)
	}

	r := removeRunner(cleanStatus)

	var out bytes.Buffer
	if err := RunSpaceRemove(r, SpaceRemoveArgs{Name: "feat", Repos: []string{"infra"}}, &bytes.Buffer{}, &out); err != nil {
		t.Fatalf("RunSpaceRemove: %v", err)
	}

	if _, err := os.Stat(goWorkSumPath); err == nil {
		t.Error("go.work.sum should be removed after go.work update")
	}
}

// --- partial failure ---

func TestRunSpaceRemove_WorktreeRemoveError_StatePreserved(t *testing.T) {
	isolateState(t)
	root := t.TempDir()
	spacePath := t.TempDir()
	makeSpace(t, "feat", "feat", spacePath, []string{"api", "frontend"}, root)

	r := removeRunner(cleanStatus)
	r.worktreeRemoveFn = func(_, _ string, _ bool) error {
		return fmt.Errorf("locked worktree")
	}

	var out bytes.Buffer
	err := RunSpaceRemove(r, SpaceRemoveArgs{Name: "feat", Repos: []string{"frontend"}}, &bytes.Buffer{}, &out)
	if err == nil {
		t.Fatal("expected error when worktree removal fails")
	}
	// State must be unchanged.
	sp, err := state.Load("feat")
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	if len(sp.Repos) != 2 {
		t.Errorf("state should be unchanged on worktree failure, got %d repos", len(sp.Repos))
	}
}

func TestRunSpaceRemove_PartialWorktreeFailure_StatePreserved(t *testing.T) {
	isolateState(t)
	root := t.TempDir()
	spacePath := t.TempDir()
	makeSpace(t, "feat", "feat", spacePath, []string{"api", "frontend", "infra"}, root)

	callCount := 0
	r := removeRunner(cleanStatus)
	r.worktreeRemoveFn = func(_, _ string, _ bool) error {
		callCount++
		if callCount == 2 {
			return fmt.Errorf("locked")
		}
		return nil
	}

	var out bytes.Buffer
	err := RunSpaceRemove(r, SpaceRemoveArgs{Name: "feat", Repos: []string{"frontend", "infra"}}, &bytes.Buffer{}, &out)
	if err == nil {
		t.Fatal("expected error on partial worktree failure")
	}
	sp, err := state.Load("feat")
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	if len(sp.Repos) != 3 {
		t.Errorf("state should be fully preserved on any worktree failure, got %d repos", len(sp.Repos))
	}
}

func TestRunSpaceRemove_StatusError_SkippedSilently(t *testing.T) {
	isolateState(t)
	root := t.TempDir()
	spacePath := t.TempDir()
	makeSpace(t, "feat", "feat", spacePath, []string{"api", "frontend"}, root)

	r := &testRunner{
		statusFn:         func(_ string) (git.RepoStatus, error) { return git.RepoStatus{}, fmt.Errorf("no such path") },
		worktreeRemoveFn: func(_, _ string, _ bool) error { return nil },
	}

	var out bytes.Buffer
	if err := RunSpaceRemove(r, SpaceRemoveArgs{Name: "feat", Repos: []string{"frontend"}}, &bytes.Buffer{}, &out); err != nil {
		t.Fatalf("RunSpaceRemove: %v", err)
	}
	if strings.Contains(out.String(), "[y/N]") {
		t.Errorf("should not prompt when status errors are skipped: %q", out.String())
	}
}
