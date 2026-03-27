# Design Notes

Key architectural decisions and rationale. This is a living document — update it when decisions change.

## Library Choices

| Concern | Library | Rationale |
|---------|---------|-----------|
| CLI framework | `github.com/urfave/cli/v2` | Lightweight, idiomatic, good subcommand support |
| Configuration | `github.com/knadh/koanf/v2` | Multi-source (file, env, flags), clean layering |
| Git operations | System `git` via `os/exec` | See below |
| `go.work` files | `golang.org/x/mod/modfile` | Official parser/writer, no shelling out needed |
| Terminal UI / colors | `github.com/charmbracelet/lipgloss` | Composable styling, no TUI required |
| Confirmation prompts | `github.com/charmbracelet/huh` | Fits CLI use case, charm ecosystem consistency |

### Why system `git` instead of go-git

The initial design targeted `go-git/go-git/v6` (pre-release) to avoid a dependency on the
host system's git version. After a spike (see `spikes/go-git-worktrees/README.md`), we
concluded the tradeoffs favor shelling out to system git:

**go-git v6 costs:**
- Pre-release, unstable API (the `x/plumbing/worktree` package)
- Name character restriction (`^[a-zA-Z0-9\-]+$`) requiring a custom worktree layer
- Does not respect the host's credential helpers, SSH agents, or proxy config
- In-memory implementations help testing, but a clean interface (see below) achieves the same

**System git benefits:**
- All worktree subcommands needed (`add`, `list`, `remove`, `prune`, `move`) are available
  in git 2.5–2.18. Ubuntu 20.04 LTS ships git 2.25.1 — well within range
- `--porcelain` and `--porcelain=v2` output formats are stable across versions and locales;
  no fragile text parsing required
- Respects the user's existing git configuration (SSH, proxies, credential helpers)
- Battle-tested edge case handling
- `golang.org/x/mod/modfile` handles `go.work` in pure Go; no `go` binary shelling needed

**Minimum supported git version:** 2.25.1 (Ubuntu 20.04 LTS default). All features used
are available in this version. `git worktree repair` (added in 2.29) is attempted
opportunistically — see the State Validation section.

## Git Abstraction Layer

All git operations are routed through a `GitRunner` interface defined in `internal/git/runner.go`.
The production implementation shells out to system git. Tests use a mock or a real git repo
in a temp directory.

```go
// internal/git/runner.go

type WorktreeInfo struct {
    Path    string
    HEAD    string // commit hash
    Branch  string // empty if detached
    Bare    bool
    Locked  bool
}

type FileStatus struct {
    Path    string
    Index   byte // porcelain v2 XY code, X = index
    Worktree byte // Y = worktree
}

type RepoStatus struct {
    Branch    string
    Upstream  string // empty if no upstream
    Ahead     int
    Behind    int
    Files     []FileStatus
}

type Runner interface {
    // Worktrees
    WorktreeAdd(repoPath, worktreePath, branch string, createBranch bool) error
    WorktreeRemove(repoPath, worktreePath string, force bool) error
    WorktreeList(repoPath string) ([]WorktreeInfo, error)
    WorktreeRepair(repoPath string, paths ...string) error // no-op + warn on git < 2.29

    // Branches
    BranchExists(repoPath, branch string) (bool, error)
    BranchDelete(repoPath, branch string, force bool) error
    BranchMerged(repoPath, branch string) (bool, error) // merged into HEAD?

    // Status
    Status(repoPath string) (RepoStatus, error) // git status --porcelain=v2 --branch

    // Sync
    DefaultBranch(repoPath string) (string, error)
    Fetch(repoPath string) error
    FastForward(repoPath, branch string) error

    // Info
    RemoteURL(repoPath, remote string) (string, error)
}
```

### Scripting-safe git output formats

No free-text git output is parsed. All parsing uses structured, version-stable formats:

| Operation | Command | Format |
|-----------|---------|--------|
| Worktree list | `git worktree list --porcelain` | Record-per-worktree, blank-line separated |
| Working tree status + branch + ahead/behind | `git status --porcelain=v2 --branch` | `# branch.*` headers + XY-coded file lines |
| Branch existence | `git rev-parse --verify refs/heads/<branch>` | Exit code only |
| Remote URL | `git remote get-url origin` | Single line |
| Default branch | `git symbolic-ref refs/remotes/origin/HEAD` | `refs/remotes/origin/<branch>` |

### Testing strategy

Unit tests mock the `Runner` interface directly — no git binary required.

Integration tests use a helper (`internal/git/testhelper`) that:
1. Creates a real git repo in `t.TempDir()`
2. Seeds it with commits, branches, and worktrees as needed
3. Returns a production `Runner` pointed at it

This gives the same coverage benefit as go-git's in-memory repos while testing against
real git output formats.

## Transactions / Saga Pattern

Commands that mutate state (`space create`, `space add`, `space delete`) follow a
pre-flight + saga rollback pattern.

### Pre-flight checks

Before any mutation, verify:
1. All repos exist on disk and are valid git repos
2. Target worktree paths don't already exist (or are empty)
3. No space with this name already exists in the state file
4. Branch state in each repo (see Branch Strategy)

If any pre-flight check fails, the command errors immediately with no side effects.

### Rollback on mid-operation failure

Each mutating step registers a compensating action. If a later step fails, completed
steps are unwound in reverse order:

```go
// internal/saga/saga.go

type Step struct {
    Name string
    Do   func(ctx context.Context) error
    Undo func(ctx context.Context) error // called on rollback; errors are logged, not fatal
}

func Run(ctx context.Context, steps []Step) error {
    var done []Step
    for _, step := range steps {
        if err := step.Do(ctx); err != nil {
            for i := len(done) - 1; i >= 0; i-- {
                if cerr := done[i].Undo(ctx); cerr != nil {
                    // log compensation failure; continue unwinding
                }
            }
            return fmt.Errorf("step %q: %w", step.Name, err)
        }
        done = append(done, step)
    }
    return nil
}
```

No external saga library is used — the pattern is ~30 lines and carries no dependency risk.
Existing Go saga libraries have low adoption and awkward APIs.

Compensation failures (e.g. `git worktree remove` failing mid-rollback) are logged and
reported to the user but do not prevent the remaining rollback steps from running.

## State Persistence

Space metadata is stored in `~/.local/share/wtg/spaces/<name>.yaml` (XDG data directory).

**Why not inside the workspace directory?**
If the workspace directory is deleted externally (e.g. `rm -rf ~/workspaces/myfeature`),
the space would disappear from the tool's awareness entirely. Storing state separately
allows the tool to detect and report missing worktrees rather than silently losing track.

**Why a state file at all?**
Git worktrees record per-repo state (path, branch, HEAD), but git has no concept of a
"space" — grouping worktrees across multiple repos is entirely this tool's abstraction.
The state file is the only place that knows which repos belong to which space. It is kept
intentionally minimal.

### Space state schema

```yaml
name: myfeature
path: /Users/geoff/workspaces/myfeature   # absolute, tilde-expanded
branch: geoff/myfeature                   # branch shared across all repos in this space
created_at: 2026-03-27T10:00:00Z
repos:
  - name: myorg/api                       # short name (relative to repos_root)
    repo_path: /Users/geoff/repos/myorg/api
    worktree_path: /Users/geoff/workspaces/myfeature/myorg/api
go_workspace: true                        # whether a go.work was generated
```

### State validation and auto-heal

State files are hints, not ground truth. Every command that reads space state validates
against the actual filesystem and git state before acting.

| What state says | What reality shows | Action |
|---|---|---|
| Worktree exists | Path ✓, `git worktree list` ✓ | Healthy — proceed |
| Worktree exists | Path ✗, stale git entry | Run `git worktree prune`; warn user that data is gone |
| Worktree exists | Path ✓, not in git | Attempt `git worktree repair`; if git < 2.29, error with manual instructions |
| Worktree exists | Path ✗, not in git | Remove from state, warn user |

If a worktree directory is deleted externally, the data inside it is gone. There is nothing
to restore. "Repair" in this context means cleaning up stale references, not recovering data.

**`git worktree repair`** (added in git 2.29) is attempted automatically in the relevant case.
If git is too old, the error output from git will include the unknown-subcommand message;
`wtg` catches this and prints manual recovery instructions instead.

## Repository Discovery

Discovery scans `discovery.root_dir` for directories containing a `.git` entry up to
`discovery.max_depth` levels deep. No cache is maintained — the scan is fast at depth 2
and caching introduces staleness problems.

A repo's **short name** is its path relative to `discovery.root_dir`, using `/` as the
separator. This naturally disambiguates repos with the same directory name under different
parent directories:

```
~/repos/api/.git          → short name: api
~/repos/foo/api/.git      → short name: foo/api
~/repos/bar/api/.git      → short name: bar/api
```

In the standard (non-nested) case the short name is just the directory name, so there is
no added complexity for the common workflow. When used in commands like `space create`,
users can specify `foo/api` to be explicit or `api` if it is unambiguous.

Repo listings also include the remote `origin` URL to make repos easy to identify:

```
api          https://github.com/myorg/api.git        /Users/geoff/repos/api
foo/api      https://github.com/foo/api.git           /Users/geoff/repos/foo/api
bar/api      https://github.com/bar/api.git           /Users/geoff/repos/bar/api
```

## Worktree Layout

Workspace directories mirror the repo short name structure (relative path from `repos_root`).
For non-nested repos the result is a flat directory. For nested repos the org-level directory
is created automatically.

```
<space-root>/          # default: <spaces.root_dir>/<space-name>; overridable with --path
  <repo-short-name>/   # mirrors nesting from repos_root
  go.work
```

Examples:

```
# Non-nested repos (standard case) — flat workspace:
~/workspaces/myfeature/
  api/              ← repos/api
  frontend/         ← repos/frontend
  go.work

# Nested repos — org-level subdirectory created:
~/workspaces/myfeature/
  myorg/
    api/            ← repos/myorg/api
    frontend/       ← repos/myorg/frontend
  otherapg/
    api/            ← repos/otherapg/api
  go.work

# Mixed (common in practice):
~/workspaces/myfeature/
  infra/            ← repos/infra  (non-nested)
  myorg/
    api/            ← repos/myorg/api
    frontend/       ← repos/myorg/frontend
  go.work
```

This eliminates any collision risk: two repos with the same leaf name but different org
paths produce distinct worktree directories, matching the short name that identified them.

The default space root is `<spaces.root_dir>/<space-name>`. The `--path` flag on
`space create` overrides this for the specific space being created; `space add` inherits
the path stored in the space state.

## Branch Strategy

- Default branch name: `<git.branch_prefix><space-name>` (prefix may be empty)
- `--branch` flag on `space create` overrides the generated name
- All repos in a space share the same branch name

### Branch conflict rules

Evaluated per-repo during `space create` and `space add`:

| Branch state in repo | Action |
|---|---|
| Does not exist | Create from HEAD of the repo's default branch |
| Exists, not checked out anywhere | Check out in the new worktree (do not reset) |
| Exists, already checked out in another worktree | **Error** — git does not allow two worktrees on the same branch |
| Exists in some repos, not others | Create where missing, use existing where present |

These checks are part of the pre-flight phase so that no worktrees are created before a
conflict is detected.

## go.work Generation

When `space create` or `space add` runs, `golang.org/x/mod/modfile` is used to read and
write `go.work` files — no shelling out to the `go` binary required.

- Each repo with a `go.mod` at its root gets a `use` directive
- Repos without `go.mod` are silently skipped
- If no repos have `go.mod`, no `go.work` is created
- `go.work` is written at `<space-root>/go.work`
- Only `use` directives are emitted; no `replace` directives
- Paths in `use` directives are relative to the workspace root and mirror the repo nesting:

```
# ~/workspaces/myfeature/go.work
go 1.24

use (
  ./infra             # repos/infra
  ./myorg/api         # repos/myorg/api
  ./myorg/frontend    # repos/myorg/frontend
)
```

## `space delete` and Branch Safety

Following git's own `-d`/`-D` convention:
- No flag: remove worktrees only, leave branches
- `-d`: also delete branches if they are fully merged (wraps `git branch -d` semantics)
- `-D`: force-delete branches regardless of merge state

Before any destructive action, the tool checks:
1. Does any worktree have uncommitted changes (dirty working tree)?
2. Does any branch have commits not pushed to origin?

If either is true, a summary is printed and the user is prompted to confirm.

## `repo sync` Scope

`repo sync` operates exclusively on the main clones in `repos_dir` — it does NOT touch
workspace worktrees. This is intentional: worktrees are on feature branches, not the
default branch, and should be rebased/merged explicitly by the developer.

When `space create` runs, it branches from the current HEAD of the local default branch.
Users should run `repo sync` first to ensure they're branching from the latest upstream.

## `wtg init`

`wtg init` is an interactive setup wizard that creates `~/.config/wtg/config.yaml` for
new users. It prompts for:

- One or more root paths to scan for git repositories
- Max scan depth (default: 2)
- Workspace root directory (where space directories will be created)
- Optional branch name prefix (e.g. `yourname/`)

Running `wtg init` on an existing config prompts to confirm before overwriting.
