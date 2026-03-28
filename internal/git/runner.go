// Package git provides the Runner interface and its system-git implementation.
package git

import "errors"

// WorktreeInfo describes a single git worktree.
type WorktreeInfo struct {
	Path   string
	HEAD   string // commit hash
	Branch string // empty if HEAD is detached
	Bare   bool
	Locked bool
}

// FileStatus holds the porcelain v2 XY status codes for a single file.
type FileStatus struct {
	Path     string
	Index    byte // X: staged change ('.' = clean, 'M' = modified, 'A' = added, etc.)
	Worktree byte // Y: unstaged change
}

// RepoStatus summarises the working-tree and branch state of a repository.
type RepoStatus struct {
	Branch   string
	Upstream string // empty if no upstream is configured
	Ahead    int
	Behind   int
	Files    []FileStatus
}

// ErrRepairUnsupported is returned by WorktreeRepair when the installed git
// version predates 2.29 and does not support the repair subcommand.
var ErrRepairUnsupported = errors.New("git worktree repair requires git ≥ 2.29")

// Runner is the interface through which all git operations are performed.
// The production implementation shells out to system git; tests use a mock or a
// real git repo created with internal/git/testhelper.
type Runner interface {
	// Worktrees
	WorktreeAdd(repoPath, worktreePath, branch string, createBranch bool) error
	WorktreeRemove(repoPath, worktreePath string, force bool) error
	WorktreeList(repoPath string) ([]WorktreeInfo, error)
	WorktreeRepair(repoPath string, paths ...string) error // ErrRepairUnsupported on git < 2.29

	// Branches
	BranchExists(repoPath, branch string) (bool, error)
	BranchDelete(repoPath, branch string, force bool) error
	BranchMerged(repoPath, branch string) (bool, error) // is branch merged into HEAD?

	// Status
	Status(repoPath string) (RepoStatus, error) // git status --porcelain=v2 --branch

	// Sync
	DefaultBranch(repoPath string) (string, error)
	Fetch(repoPath string) error
	FastForward(repoPath, branch string) error

	// Info
	RemoteURL(repoPath, remote string) (string, error)
}
