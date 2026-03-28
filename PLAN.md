# Implementation Plan

## Phase 1 — Foundation

- [x] Package layout and module setup
- [x] Config types + koanf loading (file, env, flags)
- [x] `Runner` interface + shell-out implementation (`internal/git/`)
- [x] `Runner` integration tests (real git repos in `t.TempDir()`)
- [x] State file types + read/write (`internal/state/`)
- [x] Saga implementation + tests (`internal/saga/`)
- [x] UI helpers — shared lipgloss styles and output formatting (`internal/ui/`)

## Phase 2 — Commands

Each command includes unit tests (mock `Runner`) in the same session.

- [x] `wtg init`
- [x] `wtg repo discover` / `wtg repo list`
- [x] `wtg repo sync`
- [x] `wtg space create`
- [x] `wtg space list`
- [x] `wtg space add`
- [x] `wtg space delete`
- [x] `wtg space path`
- [x] `wtg status`

## Phase 3 — Polish

- [x] Shell completions (bash, zsh, fish)
- [x] CI (GitHub Actions: build + test)
- [x] Release workflow — skipped; `go install` is sufficient for a Go developer audience

## Phase 4 — Gap closure

- [x] `wtg status` with no args detects if CWD is inside a workspace and shows that space; otherwise shows summary of all spaces
- [x] `wtg status <name>` shows per-repo detail (branch, clean/dirty, ahead/behind)
- [x] `wtg status <name> --detailed` lists individual modified files per repo
- [x] `wtg space list` includes repo names alongside count

## Phase 5 — Enhancements

- [x] Parallelize `repo sync` and `repo status` (currently sequential, network-bound)
- [x] `wtg repo fetch [<repo>...]` — fetch from origin without fast-forwarding (lighter-weight alternative to sync)
- [x] `wtg space exec <name> <cmd>` — run a shell command in each worktree of a space
- [x] `wtg space push <name>` — push all branches in a space to origin
- [x] `--base <branch>` flag on `wtg space create` — branch from something other than the default branch
- [x] `wtg space rebase <name>` — rebase all worktrees onto the updated default branch

## Code Review — Follow-up Work

Findings from review, ordered by priority. Work through these one at a time.

### High

- [x] **R1** — Dead `cfg` param in `RunSpaceRebase`: `cfg *config.Config` is accepted but never used inside the function; remove from signature and drop the `config.Load` call in the CLI action handler.
- [x] **R2** — Triplicate path-resolution block: `RunFetch`, `RunSync`, and `RunRepoStatus` each contain a verbatim ~15-line block that resolves repo paths from either all-discovered or named args. Extract a shared helper `resolveRepoPaths(cfg, args)`.
- [x] **R3** — Duplicate result struct: `fetchResult`, `syncResult`, `pushResult`, `rebaseResult` are all `struct { name, sym, msg string }`. Define one shared type (e.g. `opResult` in `cmd`).
- [x] **R4** — Duplicate test helpers with a path bug: `seedSpace` (`space_add_test.go`) and `statusSpace` (`status_test.go`) are nearly identical; `statusSpace` uses string concatenation instead of `filepath.Join`. Delete one, fix path construction in the survivor, update call sites.

### Medium

- [x] **R5** — `ui.NewTable()` is dead code: defined and exported but never called; every call site uses `NewTableWriter`. Remove it.
- [x] **R6** — `_ = g.Wait()` unexplained across five call sites: goroutines always return nil (by design); add a brief comment at each site explaining the invariant so it doesn't look like a swallowed error.
- [x] **R7** — Serial status in `printSpaceDetail` vs parallel elsewhere: `printSpaceDetail` calls `runner.Status` sequentially in a for loop; `RunRepoStatus` does the same work in parallel with errgroup. Make them consistent.
- [x] **R8** — Magic string `"go 1.24"` in `writeGoWork`: extract as a named constant so it doesn't get missed when the project's minimum Go version bumps.
- [x] **R9** — `RunSpacePush` / `RunSpaceRebase` swallow failures silently: both print failures in the table but always return `nil`, unlike `RunSpaceExec` which returns an error. **Decision needed**: adopt one policy (fail loudly vs. always nil) and apply it consistently.

### Low

- [x] **R10** — `forceRemove` misleading name in `RunSpaceDelete`: the variable represents "there are warnings and user has confirmed"; rename to `needsForce` or `confirmedForce`.
- [x] **R11** — `targets[i]` vs `t` in `checkBranchConflicts`: `targets` is `[]*repoTarget`; `t` from the range is already the pointer, so `t.createBranch = ...` is equivalent and more idiomatic than `targets[i].createBranch = ...`.
- [x] **R12** — Hardcoded `/tmp/feat` in `space_create_test.go:113`: not portable; replace with `t.TempDir()`.
- [x] **R13** — Redundant push in `testhelper.InitWithRemote`: two push commands are used where one (`--set-upstream`) suffices.
- [x] **R14** — Fish completion gaps: `space exec`, `space push`, `space rebase`, and `space path` all take a space name as their first arg but have no dynamic completion registered.
- [x] **R15** — `checkBranchConflicts` name doesn't reflect its side-effect: the function also sets `targets[i].createBranch`; consider renaming to `classifyBranchTargets` or splitting check and mutation.
