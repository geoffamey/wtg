# Design Notes

Key architectural decisions and rationale. This is a living document — update it when decisions change.

## Library Choices

| Concern | Library | Rationale |
|---------|---------|-----------|
| CLI framework | `github.com/urfave/cli/v2` | Lightweight, idiomatic, good subcommand support |
| Configuration | `github.com/knadh/koanf/v2` | Multi-source (file, env, flags), clean layering |
| Git operations | `github.com/go-git/go-git/v6` | Pure Go, no dependency on system git version |
| Terminal UI / colors | `github.com/charmbracelet/lipgloss` | Composable styling, no TUI required |
| Confirmation prompts | `github.com/charmbracelet/huh` | Fits CLI use case, charm ecosystem consistency |

**Why go-git v6 (pre-release)?**
go-git v5 (stable) does not implement linked worktree creation/removal (`git worktree add/remove`).
v6 exposes this via `x/plumbing/worktree`. The `x/` prefix means the API is subject to change,
but the underlying git worktree format is stable. This is preferable to shelling out to git,
which would introduce a dependency on the host system's git version and break on older Ubuntu
systems with git < 2.5 (which introduced worktrees).

**go-git v6 worktree API limitations (see `spikes/go-git-worktrees/README.md`):**
The spike revealed that `xworktree.Add()` has two constraints incompatible with our needs:
it ties the worktree metadata name to the branch name, and it rejects names not matching
`^[a-zA-Z0-9\-]+$` (no underscores, no slashes). We implement our own thin worktree
creation layer (`internal/gitutil/worktree.go`) to remove both constraints. `Remove()` and
`List()` from the `x/` package are still used as they are simple and have no name validation.

## State Persistence

Space metadata is stored in `~/.local/share/wtg/spaces/<name>.yaml` (XDG data directory).

**Why not inside the workspace directory?**
If the workspace directory is deleted externally (e.g. `rm -rf ~/workspaces/myfeature`),
the space would disappear from the tool's awareness entirely. Storing state separately
allows the tool to detect and report orphaned or missing worktrees rather than silently
losing track of them.

**Consistency model**: State files are treated as a hint, not ground truth. Any command
that reads space state (`status`, `delete`, etc.) validates against the actual filesystem
and git state. If a worktree path doesn't exist or a branch is gone, the tool reports this
rather than crashing.

### Space state schema

```yaml
name: myfeature
path: /Users/geoff/workspaces/myfeature   # absolute, tilde-expanded
branch: geoff/myfeature                   # branch used across all repos in this space
created_at: 2026-03-27T10:00:00Z
repos:
  - name: myorg/api                       # short name (relative to repos_root)
    repo_path: /Users/geoff/repos/myorg/api
    worktree_path: /Users/geoff/workspaces/myfeature/myorg/api
    worktree_key: myfeature-myorg-api     # key used in .git/worktrees/<key>/ (no slashes)
    branch: geoff/myfeature               # per-repo branch (may differ in future)
go_workspace: true                        # whether a go.work was generated
```

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
~/workspaces/<space-name>/
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

### Git worktree metadata key

The metadata directory `.git/worktrees/<key>/` inside each repo's `.git` cannot contain
slashes (it is a single directory name). The key is derived from the space name and repo
short name by replacing `/` with `-`:

```
space "myfeature", repo "myorg/api"  →  key "myfeature-myorg-api"
space "myfeature", repo "api"        →  key "myfeature-api"
```

The key is stored in the space state file (`worktree_key`) and used during `space delete`
to clean up the metadata. The actual worktree path is independent of the key.

## Branch Strategy

- Default branch name: `<git.branch_prefix><space-name>` (prefix may be empty)
- `--branch` flag on `space create` overrides the generated name
- All repos in a space share the same branch name
- If the branch already exists when creating a worktree, it is checked out (not recreated)
  — this supports picking up a teammate's existing branch

## go.work Generation

When `space create` or `space add` runs:
- Each repo with a `go.mod` at its root gets a `use` directive
- Repos without `go.mod` are silently skipped
- `go.work` is written at `<workspaces_dir>/<space-name>/go.work`
- Only `use` directives are emitted; no `replace` directives
- Paths in `use` directives are relative to the workspace root and mirror the repo nesting:

```
# ~/workspaces/myfeature/go.work
go 1.26

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
