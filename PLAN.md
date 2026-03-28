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
- [ ] `wtg space delete`
- [ ] `wtg status`

## Phase 3 — Polish

- [ ] Shell completions (bash, zsh, fish)
- [ ] CI (GitHub Actions: build + test)
- [ ] Release workflow (goreleaser)
