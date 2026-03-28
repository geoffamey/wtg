package git

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// SystemRunner is the production Runner implementation that shells out to system git.
type SystemRunner struct{}

// New returns a SystemRunner.
func New() *SystemRunner { return &SystemRunner{} }

// runError wraps a failed git command with its exit code and stderr output.
type runError struct {
	args     []string
	exitCode int
	stderr   string
}

func (e *runError) Error() string {
	msg := fmt.Sprintf("git %s: exit %d", strings.Join(e.args, " "), e.exitCode)
	if s := strings.TrimSpace(e.stderr); s != "" {
		msg += ": " + s
	}
	return msg
}

// run executes git -C repoPath <args> and returns trimmed stdout.
// Any non-zero exit is returned as a *runError.
func (r *SystemRunner) run(repoPath string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repoPath}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		code := -1
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			code = exitErr.ExitCode()
		}
		return "", &runError{args: args, exitCode: code, stderr: stderr.String()}
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// exitCode extracts the exit code from an error returned by run, or -1 if unavailable.
func exitCode(err error) int {
	if re, ok := errors.AsType[*runError](err); ok {
		return re.exitCode
	}
	return -1
}

// --- Worktrees ---

func (r *SystemRunner) WorktreeAdd(repoPath, worktreePath, branch string, createBranch bool) error {
	var args []string
	if createBranch {
		// git worktree add -b <new-branch> <path>
		args = []string{"worktree", "add", "-b", branch, worktreePath}
	} else {
		// git worktree add <path> <branch>
		args = []string{"worktree", "add", worktreePath, branch}
	}
	_, err := r.run(repoPath, args...)
	return err
}

func (r *SystemRunner) WorktreeRemove(repoPath, worktreePath string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, worktreePath)
	_, err := r.run(repoPath, args...)
	return err
}

func (r *SystemRunner) WorktreeList(repoPath string) ([]WorktreeInfo, error) {
	out, err := r.run(repoPath, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parseWorktreeList(out)
}

func (r *SystemRunner) WorktreeRepair(repoPath string, paths ...string) error {
	args := append([]string{"worktree", "repair"}, paths...)
	_, err := r.run(repoPath, args...)
	if err != nil {
		if re, ok := errors.AsType[*runError](err); ok && strings.Contains(re.stderr, "unknown subcommand") {
			return ErrRepairUnsupported
		}
		return err
	}
	return nil
}

// --- Branches ---

func (r *SystemRunner) BranchExists(repoPath, branch string) (bool, error) {
	_, err := r.run(repoPath, "rev-parse", "--verify", "refs/heads/"+branch)
	if err != nil {
		// git exits 128 when the ref does not exist. Any other code (including
		// -1 for "git failed to start") is a real error we should surface.
		if exitCode(err) == 128 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *SystemRunner) BranchDelete(repoPath, branch string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := r.run(repoPath, "branch", flag, branch)
	return err
}

func (r *SystemRunner) BranchMerged(repoPath, branch string) (bool, error) {
	_, err := r.run(repoPath, "merge-base", "--is-ancestor", branch, "HEAD")
	if err != nil {
		if exitCode(err) == 1 {
			return false, nil // not an ancestor — not merged
		}
		return false, err
	}
	return true, nil
}

// --- Status ---

func (r *SystemRunner) Status(repoPath string) (RepoStatus, error) {
	out, err := r.run(repoPath, "status", "--porcelain=v2", "--branch")
	if err != nil {
		return RepoStatus{}, err
	}
	return parseStatus(out)
}

// --- Sync ---

func (r *SystemRunner) DefaultBranch(repoPath string) (string, error) {
	out, err := r.run(repoPath, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err != nil {
		return "", fmt.Errorf("cannot determine default branch (is origin/HEAD set?): %w", err)
	}
	// "refs/remotes/origin/main" → "main"
	_, branch, ok := strings.Cut(out, "refs/remotes/origin/")
	if !ok {
		return "", fmt.Errorf("unexpected symbolic-ref output: %q", out)
	}
	return branch, nil
}

func (r *SystemRunner) Fetch(repoPath string) error {
	_, err := r.run(repoPath, "fetch", "origin")
	return err
}

func (r *SystemRunner) FastForward(repoPath, branch string) error {
	_, err := r.run(repoPath, "merge", "--ff-only", "origin/"+branch)
	return err
}

func (r *SystemRunner) Push(repoPath, branch string) error {
	_, err := r.run(repoPath, "push", "origin", branch)
	return err
}

// --- Info ---

func (r *SystemRunner) RemoteURL(repoPath, remote string) (string, error) {
	return r.run(repoPath, "remote", "get-url", remote)
}
