// Package testhelper creates real git repositories in t.TempDir() for use in
// integration tests of the git.Runner implementation.
package testhelper

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Repo is a real git repository created in a temp directory.
type Repo struct {
	Path string // absolute path to the repo root
	t    *testing.T
}

// RepoAt wraps an existing directory as a Repo for use in tests.
// Use this when you need a Repo handle for a path created by the Runner
// (e.g. a worktree added via WorktreeAdd).
func RepoAt(t *testing.T, path string) *Repo {
	t.Helper()
	return &Repo{Path: path, t: t}
}

// Init creates a new bare-minimum git repository in t.TempDir().
// The repo has one commit on "main" and origin/HEAD pointing to main.
func Init(t *testing.T) *Repo {
	t.Helper()
	dir := t.TempDir()
	r := &Repo{Path: dir, t: t}

	r.GitCmd("init", "-b", "main")
	r.GitCmd("config", "user.email", "test@example.com")
	r.GitCmd("config", "user.name", "Test")
	r.WriteFile("README.md", "# test\n")
	r.GitCmd("add", ".")
	r.GitCmd("commit", "-m", "initial commit")

	return r
}

// InitWithRemote creates a local repo plus a bare "origin" clone so fetch/push work.
func InitWithRemote(t *testing.T) (local *Repo, remote *Repo) {
	t.Helper()
	remote = &Repo{Path: t.TempDir(), t: t}
	remote.GitCmd("init", "--bare", "-b", "main")

	// Seed the remote with one commit via a temporary clone.
	seed := &Repo{Path: t.TempDir(), t: t}
	seed.GitCmd("clone", remote.Path, ".")
	seed.GitCmd("config", "user.email", "test@example.com")
	seed.GitCmd("config", "user.name", "Test")
	seed.WriteFile("README.md", "# test\n")
	seed.GitCmd("add", ".")
	seed.GitCmd("commit", "-m", "initial commit")
	seed.GitCmd("push", "--set-upstream", "origin", "main")

	// Clone into the local repo dir.
	local = &Repo{Path: t.TempDir(), t: t}
	local.GitCmd("clone", remote.Path, ".")
	local.GitCmd("config", "user.email", "test@example.com")
	local.GitCmd("config", "user.name", "Test")
	// Ensure origin/HEAD is set for DefaultBranch().
	local.GitCmd("remote", "set-head", "origin", "--auto")

	return local, remote
}

// CreateBranch creates a new branch from HEAD without checking it out.
func (r *Repo) CreateBranch(name string) {
	r.t.Helper()
	r.GitCmd("branch", name)
}

// Commit adds a file and creates a commit, advancing HEAD.
func (r *Repo) Commit(message string) {
	r.t.Helper()
	r.WriteFile("file-"+message+".txt", message)
	r.GitCmd("add", ".")
	r.GitCmd("commit", "-m", message)
}

// WriteFile writes content to a file inside the repo (creating parent dirs as needed).
func (r *Repo) WriteFile(name, content string) {
	r.t.Helper()
	full := filepath.Join(r.Path, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		r.t.Fatalf("testhelper: mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		r.t.Fatalf("testhelper: write %s: %v", name, err)
	}
}

// GitCmd runs a git command in the repo directory, failing the test on error.
func (r *Repo) GitCmd(args ...string) {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Path
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("testhelper: git %v: %v\n%s", args, err, out)
	}
}
