package cmd

import (
	"fmt"

	"github.com/geoffamey/wtg/internal/git"
)

// testRunner is a configurable mock of git.Runner for command unit tests.
// Only set the function fields your test actually needs; unset fields panic
// with a descriptive message if called unexpectedly.
type testRunner struct {
	worktreeAddFn    func(repoPath, worktreePath, branch string, createBranch bool) error
	worktreeRemoveFn func(repoPath, worktreePath string, force bool) error
	worktreeListFn   func(repoPath string) ([]git.WorktreeInfo, error)
	worktreeRepairFn func(repoPath string, paths ...string) error
	branchExistsFn   func(repoPath, branch string) (bool, error)
	branchDeleteFn   func(repoPath, branch string, force bool) error
	branchMergedFn   func(repoPath, branch string) (bool, error)
	statusFn         func(repoPath string) (git.RepoStatus, error)
	defaultBranchFn  func(repoPath string) (string, error)
	fetchFn          func(repoPath string) error
	fastForwardFn    func(repoPath, branch string) error
	remoteURLFn      func(repoPath, remote string) (string, error)
}

func (r *testRunner) WorktreeAdd(repoPath, worktreePath, branch string, createBranch bool) error {
	if r.worktreeAddFn == nil {
		panic(fmt.Sprintf("unexpected WorktreeAdd(%q, %q, %q, %v)", repoPath, worktreePath, branch, createBranch))
	}
	return r.worktreeAddFn(repoPath, worktreePath, branch, createBranch)
}

func (r *testRunner) WorktreeRemove(repoPath, worktreePath string, force bool) error {
	if r.worktreeRemoveFn == nil {
		panic(fmt.Sprintf("unexpected WorktreeRemove(%q, %q, %v)", repoPath, worktreePath, force))
	}
	return r.worktreeRemoveFn(repoPath, worktreePath, force)
}

func (r *testRunner) WorktreeList(repoPath string) ([]git.WorktreeInfo, error) {
	if r.worktreeListFn == nil {
		panic(fmt.Sprintf("unexpected WorktreeList(%q)", repoPath))
	}
	return r.worktreeListFn(repoPath)
}

func (r *testRunner) WorktreeRepair(repoPath string, paths ...string) error {
	if r.worktreeRepairFn == nil {
		panic(fmt.Sprintf("unexpected WorktreeRepair(%q, %v)", repoPath, paths))
	}
	return r.worktreeRepairFn(repoPath, paths...)
}

func (r *testRunner) BranchExists(repoPath, branch string) (bool, error) {
	if r.branchExistsFn == nil {
		panic(fmt.Sprintf("unexpected BranchExists(%q, %q)", repoPath, branch))
	}
	return r.branchExistsFn(repoPath, branch)
}

func (r *testRunner) BranchDelete(repoPath, branch string, force bool) error {
	if r.branchDeleteFn == nil {
		panic(fmt.Sprintf("unexpected BranchDelete(%q, %q, %v)", repoPath, branch, force))
	}
	return r.branchDeleteFn(repoPath, branch, force)
}

func (r *testRunner) BranchMerged(repoPath, branch string) (bool, error) {
	if r.branchMergedFn == nil {
		panic(fmt.Sprintf("unexpected BranchMerged(%q, %q)", repoPath, branch))
	}
	return r.branchMergedFn(repoPath, branch)
}

func (r *testRunner) Status(repoPath string) (git.RepoStatus, error) {
	if r.statusFn == nil {
		panic(fmt.Sprintf("unexpected Status(%q)", repoPath))
	}
	return r.statusFn(repoPath)
}

func (r *testRunner) DefaultBranch(repoPath string) (string, error) {
	if r.defaultBranchFn == nil {
		panic(fmt.Sprintf("unexpected DefaultBranch(%q)", repoPath))
	}
	return r.defaultBranchFn(repoPath)
}

func (r *testRunner) Fetch(repoPath string) error {
	if r.fetchFn == nil {
		panic(fmt.Sprintf("unexpected Fetch(%q)", repoPath))
	}
	return r.fetchFn(repoPath)
}

func (r *testRunner) FastForward(repoPath, branch string) error {
	if r.fastForwardFn == nil {
		panic(fmt.Sprintf("unexpected FastForward(%q, %q)", repoPath, branch))
	}
	return r.fastForwardFn(repoPath, branch)
}

func (r *testRunner) RemoteURL(repoPath, remote string) (string, error) {
	if r.remoteURLFn == nil {
		panic(fmt.Sprintf("unexpected RemoteURL(%q, %q)", repoPath, remote))
	}
	return r.remoteURLFn(repoPath, remote)
}
