# go-git v6 Worktree Spike Results

Spike program: `spikes/go-git-worktrees/main.go`

## Goal

Verify that `github.com/go-git/go-git/v6` (pre-release) can handle all worktree
operations we need — create, open, commit, list, remove — entirely in pure Go,
without shelling out to `git`. Validate the results against real `git` at each step.

## What Was Tested

1. Init a repo and make an initial commit using go-git
2. Create a branch with a slash in the name (`geoff/myfeature`)
3. Create a linked worktree for that branch
4. Verify `git worktree list` recognises the worktree and branch
5. Open the worktree with go-git and read HEAD
6. Make a commit inside the linked worktree
7. Verify the commit appears on the branch from the main repo
8. Remove the worktree (metadata + directory)
9. Verify `git worktree list` shows only the main worktree afterwards

## Results

All operations succeeded. Real `git` agreed with go-git at every verification point.

```
git worktree list output (step 4):
  /tmp/.../myrepo               cba2e18 [master]
  /tmp/.../workspaces/myfeature/myrepo  cba2e18 [geoff/myfeature]

go-git Head() after attach (step 5):
  refs/heads/geoff/myfeature -> cba2e18...

git log on branch after worktree commit (step 7):
  7338b02 feature commit
  cba2e18 initial commit
```

## Constraints and Workarounds

### 1. `Add()` ties worktree name to branch name

`xworktree.Add(fs, name, opts...)` uses `name` as both:
- the key in `.git/worktrees/<name>/`
- the branch it creates and checks out

There is no `WithBranch` option. This means you cannot pass `geoff/myfeature` as the
name and get a worktree on a branch of that name in a single call.

**Workaround (verified):**
1. Create the branch manually using go-git's reference API
2. Call `Add()` with `WithDetachedHead()` and `WithCommit(branchTipHash)`
3. Overwrite `.git/worktrees/<name>/HEAD` from the commit hash to the branch ref:
   ```
   ref: refs/heads/geoff/myfeature
   ```

Real `git` and go-git both read this correctly. The worktree behaves exactly as if
`git worktree add -b geoff/myfeature <path>` had been run.

### 2. Worktree name character restriction

`xworktree.Add()` validates the `name` parameter against:

```go
var worktreeNameRE = regexp.MustCompile(`^[a-zA-Z0-9\-]+$`)
```

This is a **go-git constraint only** — git itself has no such restriction. Git stores
the worktree metadata under `.git/worktrees/<name>/`, which is just a directory; any
valid directory name works. The restriction exists in go-git to be conservative, but
it rules out commonly-used characters.

Characters **not allowed** by go-git, but **valid in git** and common in space names:

| Character | Example | Notes |
|-----------|---------|-------|
| `_` (underscore) | `my_feature`, `JIRA_1234` | Very common, especially for ticket names |
| `.` (dot) | `v1.2`, `fix.login` | Less common but valid |
| `@` | rare | Valid directory name |

The underscore case is the most likely to affect real users, particularly when naming
spaces after issue tracker tickets (`PROJ_1234`, `my_feature_name`) or using snake_case
conventions.

#### Options

**Option A — Validate at `space create` time**
Reject space names containing characters outside `[a-zA-Z0-9-]`. Simple and predictable,
but `my_feature` produces an unhelpful hard error. Users who habitually use underscores
will be annoyed every time.

**Option B — Sanitize the worktree name**
Replace disallowed characters with `-` before passing to `Add()`. `my_feature` → `my-feature`
as the git metadata key, while the space name remains `my_feature` in wtg's state file.
This is transparent in the common case, but two different space names could theoretically
sanitize to the same key (e.g. `my-feature` and `my_feature` both → `my-feature`).
The collision could be detected and rejected at creation time.

**Option C — Implement our own worktree layer**
Bypass `xworktree.Add()` entirely and write the metadata files directly. The go-git v6
source reveals the full operation is ~35 lines of straightforward file writes:

```
.git/worktrees/<name>/
  commondir    ← "../.."
  gitdir       ← absolute path to <worktree>/.git
  HEAD         ← "ref: refs/heads/<branch>" (or commit hash for detached)
  ORIG_HEAD    ← commit hash
  refs/        ← empty directory
<worktree>/.git ← "gitdir: .git/worktrees/<name>"
```

Then check out the branch using go-git's existing `Worktree.Checkout()`. This removes
the name restriction entirely, and we already need the HEAD-rewrite logic from
workaround #1 anyway. The implementation lives in `internal/gitutil/worktree.go`.

#### Recommendation: Option C

We already need to write the HEAD rewrite logic (workaround #1). Implementing our own
thin layer adds roughly 40 lines and gives us full control: no name restriction, no
dependency on the `x/` experimental API's stability, and one fewer workaround. The git
worktree metadata format is stable and well-documented.

`wm.Remove()` and `wm.List()` from go-git v6 can still be used — they have no name
restriction and are simple enough to trust. If they ever cause problems, replacing them
is trivial.

### 3. `Remove()` is metadata-only

`wm.Remove(name)` removes `.git/worktrees/<name>/` but does **not** delete the worktree
directory on disk. We must call `os.RemoveAll(worktreePath)` ourselves after calling
`wm.Remove()`.

This matches real `git worktree remove` behaviour when called without `--force` on a
clean worktree — git also only removes a clean worktree directory. We are responsible
for the safety check (dirty/unmerged) before calling either.

## Library Status

go-git v6 is pre-release (`v6.0.0-20260324221343-cd85c8c75d34`). The `x/plumbing/worktree`
package carries an `x/` prefix indicating experimental/unstable API. Given that we are
implementing our own worktree layer (Option C above), our dependency on the `x/` package
is reduced to `wm.Remove()` and `wm.List()`, both of which are thin wrappers around
filesystem operations. The risk is acceptable for a developer CLI tool.

All other go-git v6 APIs used (init, commit, branch refs, checkout, status, config) are
in the stable surface and behave identically to v5.

## Decision: go-git not adopted

After this spike, the project decided to shell out to system `git` instead of using go-git.

**Reasons:**
- The pre-release API risk and workarounds identified here were not worth the "pure Go" benefit
- System git is universally available on developer machines and respects credential helpers,
  SSH agents, and proxy config that go-git silently bypasses
- All git features needed (`worktree add/list/remove/prune`) are available in git 2.25.1
  (Ubuntu 20.04 LTS default, well within the support target)
- `--porcelain` and `--porcelain=v2` output formats provide structured, stable output
  that does not require fragile text parsing
- A clean `Runner` interface (see `docs/DESIGN.md`) provides the same testability benefit
  as go-git's in-memory implementations

This spike was valuable: the constraints and workarounds discovered here directly informed
the decision. The spike code has been removed; this README is retained as a record.
