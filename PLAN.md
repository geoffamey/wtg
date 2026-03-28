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
- [x] `wtg status`

## Phase 4 — Gap closure

- [ ] `wtg status` with no args detects if CWD is inside a workspace and shows that space; otherwise shows summary of all spaces
- [ ] `wtg status <name>` shows per-repo detail (branch, clean/dirty, ahead/behind) — current impl shows summary row only
- [ ] `wtg space list` includes repo names alongside count

## Phase 3 — Polish

- [x] Shell completions (bash, zsh, fish)
- [x] CI (GitHub Actions: build + test)
- [x] Release workflow — skipped; `go install` is sufficient for a Go developer audience
