package git

import (
	"strings"
	"testing"
)

// --- runError ---

func TestRunError_WithStderr(t *testing.T) {
	e := &runError{args: []string{"status"}, exitCode: 128, stderr: "  fatal: not a git repo  "}
	got := e.Error()
	if !strings.Contains(got, "git status") {
		t.Errorf("missing command: %q", got)
	}
	if !strings.Contains(got, "exit 128") {
		t.Errorf("missing exit code: %q", got)
	}
	if !strings.Contains(got, "fatal: not a git repo") {
		t.Errorf("missing stderr: %q", got)
	}
}

func TestRunError_NoStderr(t *testing.T) {
	e := &runError{args: []string{"fetch", "origin"}, exitCode: 1, stderr: ""}
	got := e.Error()
	if !strings.Contains(got, "git fetch origin") {
		t.Errorf("missing command: %q", got)
	}
	if strings.Contains(got, ":  ") {
		t.Errorf("should not have trailing colon+space with empty stderr: %q", got)
	}
}

// --- parseWorktreeList ---

func TestParseWorktreeList_Main(t *testing.T) {
	input := `worktree /repos/api
HEAD abc123
branch refs/heads/main

`
	wts, err := parseWorktreeList(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wts) != 1 {
		t.Fatalf("got %d worktrees, want 1", len(wts))
	}
	wt := wts[0]
	if wt.Path != "/repos/api" {
		t.Errorf("Path: %q", wt.Path)
	}
	if wt.HEAD != "abc123" {
		t.Errorf("HEAD: %q", wt.HEAD)
	}
	if wt.Branch != "main" {
		t.Errorf("Branch: %q", wt.Branch)
	}
	if wt.Bare || wt.Locked {
		t.Errorf("unexpected Bare/Locked: %+v", wt)
	}
}

func TestParseWorktreeList_Multiple(t *testing.T) {
	input := `worktree /repos/api
HEAD aaa111
branch refs/heads/main

worktree /workspaces/feat/api
HEAD bbb222
branch refs/heads/feat/my-feature

worktree /workspaces/other/api
HEAD ccc333
detached

`
	wts, err := parseWorktreeList(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wts) != 3 {
		t.Fatalf("got %d worktrees, want 3", len(wts))
	}
	if wts[1].Branch != "feat/my-feature" {
		t.Errorf("worktree[1].Branch: %q", wts[1].Branch)
	}
	if wts[2].Branch != "" {
		t.Errorf("detached worktree should have empty Branch, got %q", wts[2].Branch)
	}
}

func TestParseWorktreeList_BareAndLocked(t *testing.T) {
	input := `worktree /repos/bare.git
HEAD ddd444
bare
locked reason text

`
	wts, err := parseWorktreeList(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wts) != 1 {
		t.Fatalf("got %d worktrees, want 1", len(wts))
	}
	if !wts[0].Bare {
		t.Error("want Bare=true")
	}
	if !wts[0].Locked {
		t.Error("want Locked=true")
	}
}

func TestParseWorktreeList_NoTrailingNewline(t *testing.T) {
	// git porcelain output may not always have a trailing blank line.
	input := `worktree /repos/api
HEAD abc123
branch refs/heads/main`

	wts, err := parseWorktreeList(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wts) != 1 {
		t.Fatalf("got %d worktrees, want 1", len(wts))
	}
}

func TestParseWorktreeList_Empty(t *testing.T) {
	wts, err := parseWorktreeList("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wts) != 0 {
		t.Errorf("got %d worktrees, want 0", len(wts))
	}
}

// --- parseStatus ---

func TestParseStatus_Clean(t *testing.T) {
	input := `# branch.oid abc123
# branch.head main
# branch.upstream origin/main
# branch.ab +0 -0
`
	s, err := parseStatus(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Branch != "main" {
		t.Errorf("Branch: %q", s.Branch)
	}
	if s.Upstream != "origin/main" {
		t.Errorf("Upstream: %q", s.Upstream)
	}
	if s.Ahead != 0 || s.Behind != 0 {
		t.Errorf("Ahead/Behind: %d/%d", s.Ahead, s.Behind)
	}
	if len(s.Files) != 0 {
		t.Errorf("got %d files, want 0", len(s.Files))
	}
}

func TestParseStatus_AheadBehind(t *testing.T) {
	input := `# branch.oid abc123
# branch.head main
# branch.upstream origin/main
# branch.ab +3 -1
`
	s, err := parseStatus(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Ahead != 3 {
		t.Errorf("Ahead: got %d, want 3", s.Ahead)
	}
	if s.Behind != 1 {
		t.Errorf("Behind: got %d, want 1", s.Behind)
	}
}

func TestParseStatus_ModifiedAndUntracked(t *testing.T) {
	input := `# branch.oid abc123
# branch.head feature
# branch.ab +0 -0
1 .M N... 100644 100644 100644 aaa bbb src/handler.go
1 M. N... 100644 100644 100644 ccc ddd src/middleware.go
? src/newfile.go
`
	s, err := parseStatus(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s.Files) != 3 {
		t.Fatalf("got %d files, want 3", len(s.Files))
	}

	// worktree-modified
	if s.Files[0].Path != "src/handler.go" {
		t.Errorf("Files[0].Path: %q", s.Files[0].Path)
	}
	if s.Files[0].Index != '.' || s.Files[0].Worktree != 'M' {
		t.Errorf("Files[0] XY: %c%c", s.Files[0].Index, s.Files[0].Worktree)
	}

	// index-modified
	if s.Files[1].Index != 'M' || s.Files[1].Worktree != '.' {
		t.Errorf("Files[1] XY: %c%c", s.Files[1].Index, s.Files[1].Worktree)
	}

	// untracked
	if s.Files[2].Path != "src/newfile.go" {
		t.Errorf("Files[2].Path: %q", s.Files[2].Path)
	}
	if s.Files[2].Index != '?' || s.Files[2].Worktree != '?' {
		t.Errorf("Files[2] XY: %c%c", s.Files[2].Index, s.Files[2].Worktree)
	}
}

func TestParseStatus_Renamed(t *testing.T) {
	input := `# branch.oid abc123
# branch.head main
# branch.ab +0 -0
2 R. N... 100644 100644 100644 aaa bbb R100 new/path.go	old/path.go
`
	s, err := parseStatus(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s.Files) != 1 {
		t.Fatalf("got %d files, want 1", len(s.Files))
	}
	if s.Files[0].Path != "new/path.go" {
		t.Errorf("renamed path: got %q, want %q", s.Files[0].Path, "new/path.go")
	}
	if s.Files[0].Index != 'R' {
		t.Errorf("Index: %c", s.Files[0].Index)
	}
}

func TestParseStatus_DetachedHead(t *testing.T) {
	input := `# branch.oid abc123
# branch.head (detached)
# branch.ab +0 -0
`
	s, err := parseStatus(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Branch != "" {
		t.Errorf("detached HEAD: Branch should be empty, got %q", s.Branch)
	}
}

func TestParseStatus_NoUpstream(t *testing.T) {
	input := `# branch.oid abc123
# branch.head feature
`
	s, err := parseStatus(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Upstream != "" {
		t.Errorf("Upstream: got %q, want empty", s.Upstream)
	}
}
