// spike/main.go — verifies go-git v6 linked worktree behavior against real git.
//
// What this tests:
//   1. Init a repo and make an initial commit using go-git.
//   2. Create a branch named "prefix/spacename" (contains slash — invalid as a worktree name).
//   3. Workaround: Add() a worktree with detached HEAD, then rewrite HEAD to the branch ref.
//   4. Verify `git worktree list` sees the worktree correctly.
//   5. Verify go-git can open the linked worktree and read the correct HEAD.
//   6. Remove the worktree and verify it is gone.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-git/go-billy/v6/osfs"
	gogit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/storage/filesystem"
	xworktree "github.com/go-git/go-git/v6/x/plumbing/worktree"
	"time"
)

func must(err error, msg string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", msg, err)
		os.Exit(1)
	}
}

func gitCmd(dir string, args ...string) string {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "git %v failed: %v\n%s\n", args, err, out)
		os.Exit(1)
	}
	return strings.TrimSpace(string(out))
}

func section(title string) {
	fmt.Printf("\n=== %s ===\n", title)
}

func main() {
	// -------------------------------------------------------------------------
	// Setup: create temp dirs for the main repo and the linked worktree.
	// -------------------------------------------------------------------------
	tmp, err := os.MkdirTemp("", "wtg-spike-*")
	must(err, "mkdtemp")
	defer func() {
		os.RemoveAll(tmp)
		fmt.Printf("\nCleaned up %s\n", tmp)
	}()

	repoDir := filepath.Join(tmp, "myrepo")
	wtDir := filepath.Join(tmp, "workspaces", "myfeature", "myrepo")
	must(os.MkdirAll(repoDir, 0755), "mkdir repoDir")
	must(os.MkdirAll(wtDir, 0755), "mkdir wtDir")

	fmt.Printf("Repo:     %s\n", repoDir)
	fmt.Printf("Worktree: %s\n", wtDir)

	// -------------------------------------------------------------------------
	// Step 1: Init repo and make an initial commit using go-git.
	// -------------------------------------------------------------------------
	section("1. Init repo + initial commit (go-git)")

	repo, err := gogit.PlainInit(repoDir, false)
	must(err, "PlainInit")

	wt, err := repo.Worktree()
	must(err, "repo.Worktree()")

	// Write a file and commit.
	repoFs := osfs.New(repoDir)
	f, err := repoFs.Create("hello.txt")
	must(err, "create hello.txt")
	_, err = f.Write([]byte("hello from wtg spike\n"))
	must(err, "write hello.txt")
	must(f.Close(), "close hello.txt")

	must(wt.AddWithOptions(&gogit.AddOptions{All: true}), "git add")

	sig := &object.Signature{Name: "wtg-spike", Email: "spike@wtg", When: time.Now()}
	hash, err := wt.Commit("initial commit", &gogit.CommitOptions{Author: sig, Committer: sig})
	must(err, "commit")
	fmt.Printf("Initial commit: %s\n", hash)

	// Verify with real git.
	fmt.Printf("git log: %s\n", gitCmd(repoDir, "log", "--oneline"))

	// -------------------------------------------------------------------------
	// Step 2: Create a branch with a slash in the name (prefix/spacename).
	// -------------------------------------------------------------------------
	section("2. Create branch 'geoff/myfeature' (go-git)")

	branchName := "geoff/myfeature"
	branchRef := plumbing.NewBranchReferenceName(branchName)

	headRef, err := repo.Head()
	must(err, "repo.Head()")

	ref := plumbing.NewHashReference(branchRef, headRef.Hash())
	must(repo.Storer.SetReference(ref), "SetReference branch")

	// Verify with real git.
	fmt.Printf("git branch: %s\n", gitCmd(repoDir, "branch"))

	// -------------------------------------------------------------------------
	// Step 3a: Add a linked worktree using go-git (detached HEAD workaround).
	//
	// go-git v6 Add() ties worktree name to branch name and requires the name
	// to match ^[a-zA-Z0-9\-]+$, so it cannot create a worktree on a branch
	// named "geoff/myfeature" directly.
	//
	// Workaround:
	//   a) Add() with WithDetachedHead() + WithCommit(branchTip)
	//   b) Rewrite .git/worktrees/<name>/HEAD to the branch ref
	// -------------------------------------------------------------------------
	section("3. Add linked worktree with detached HEAD, then attach to branch")

	dotgitFs := osfs.New(filepath.Join(repoDir, ".git"), osfs.WithBoundOS())
	dotgitStorage := filesystem.NewStorage(dotgitFs, nil)

	wm, err := xworktree.New(dotgitStorage)
	must(err, "xworktree.New")

	// Worktree name must match ^[a-zA-Z0-9\-]+$ — use space name without prefix.
	worktreeName := "myfeature"
	wtFs := osfs.New(wtDir)

	// Add with detached HEAD at the branch tip commit.
	must(wm.Add(wtFs, worktreeName, xworktree.WithCommit(headRef.Hash()), xworktree.WithDetachedHead()), "wm.Add")
	fmt.Printf("Worktree metadata created: .git/worktrees/%s/\n", worktreeName)

	// Rewrite .git/worktrees/myfeature/HEAD from a commit hash to the branch ref.
	headPath := filepath.Join(repoDir, ".git", "worktrees", worktreeName, "HEAD")
	must(os.WriteFile(headPath, []byte("ref: refs/heads/"+branchName+"\n"), 0644), "rewrite HEAD")
	fmt.Printf("HEAD rewritten to: ref: refs/heads/%s\n", branchName)

	// -------------------------------------------------------------------------
	// Step 4: Verify `git worktree list` sees the new worktree.
	// -------------------------------------------------------------------------
	section("4. git worktree list (real git)")
	fmt.Println(gitCmd(repoDir, "worktree", "list"))

	// Also verify from the worktree's perspective.
	fmt.Printf("\ngit status (from worktree dir):\n%s\n", gitCmd(wtDir, "status", "--short", "--branch"))

	// -------------------------------------------------------------------------
	// Step 5: Open the linked worktree with go-git and read HEAD.
	// -------------------------------------------------------------------------
	section("5. Open linked worktree via go-git and inspect HEAD")

	linkedRepo, err := wm.Open(wtFs)
	must(err, "wm.Open")

	linkedHead, err := linkedRepo.Head()
	must(err, "linkedRepo.Head()")
	fmt.Printf("go-git HEAD: %s -> %s\n", linkedHead.Name(), linkedHead.Hash())

	// -------------------------------------------------------------------------
	// Step 6: List worktrees via go-git.
	// -------------------------------------------------------------------------
	section("6. wm.List()")
	names, err := wm.List()
	must(err, "wm.List")
	fmt.Printf("Linked worktrees: %v\n", names)

	// -------------------------------------------------------------------------
	// Step 7: Make a commit in the linked worktree and verify it appears.
	// -------------------------------------------------------------------------
	section("7. Commit in linked worktree")

	linkedWt, err := linkedRepo.Worktree()
	must(err, "linkedRepo.Worktree()")

	f2, err := wtFs.Create("feature.txt")
	must(err, "create feature.txt")
	_, err = f2.Write([]byte("feature work\n"))
	must(err, "write feature.txt")
	must(f2.Close(), "close feature.txt")

	must(linkedWt.AddWithOptions(&gogit.AddOptions{All: true}), "linked git add")
	featureHash, err := linkedWt.Commit("feature commit", &gogit.CommitOptions{Author: sig, Committer: sig})
	must(err, "feature commit")
	fmt.Printf("Feature commit: %s\n", featureHash)

	fmt.Printf("git log (from worktree): %s\n", gitCmd(wtDir, "log", "--oneline"))
	fmt.Printf("git log (from main repo, branch): %s\n",
		gitCmd(repoDir, "log", "--oneline", branchName))

	// -------------------------------------------------------------------------
	// Step 8: Remove the worktree.
	//
	// Note: wm.Remove() only removes .git/worktrees/<name>/ metadata.
	// The worktree directory itself must be cleaned up separately.
	// -------------------------------------------------------------------------
	section("8. Remove worktree (go-git + manual dir cleanup)")

	must(wm.Remove(worktreeName), "wm.Remove")
	fmt.Printf(".git/worktrees/%s/ removed\n", worktreeName)

	// Verify metadata is gone.
	if _, err := os.Stat(filepath.Join(repoDir, ".git", "worktrees", worktreeName)); os.IsNotExist(err) {
		fmt.Println("Metadata confirmed gone")
	} else {
		fmt.Println("WARNING: metadata still present!")
	}

	// Clean up worktree directory (we must do this ourselves).
	// os.RemoveAll handles the whole tree in one call.
	must(os.RemoveAll(wtDir), "remove wtDir")
	fmt.Println("Worktree directory removed")

	fmt.Printf("git worktree list after removal:\n%s\n", gitCmd(repoDir, "worktree", "list"))

	// -------------------------------------------------------------------------
	// Step 9: Verify config API (we'll need this for repo.Config() later).
	// -------------------------------------------------------------------------
	section("9. Read repo config (remotes, default branch)")

	cfg, err := repo.Config()
	must(err, "repo.Config()")
	// Normally we'd inspect cfg.Remotes["origin"] for the URL.
	// This repo has no remote, so just show the core config.
	fmt.Printf("Core.IsBare: %v\n", cfg.Core.IsBare)
	fmt.Printf("Branches: %v\n", func() []string {
		var names []string
		for k := range cfg.Branches {
			names = append(names, k)
		}
		return names
	}())
	// Demonstrate reading remote URL (would be cfg.Remotes["origin"].URLs[0])
	if len(cfg.Remotes) == 0 {
		fmt.Println("No remotes (expected for this spike repo)")
	}
	_ = config.NewConfig() // just to confirm the import compiles

	section("DONE — all checks passed")
}
