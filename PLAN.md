# Implementation Plan

## Phase 1 — Foundation

- [x] Package layout and module setup
- [x] Config types + koanf loading (file, env, flags)
- [x] `Runner` interface + shell-out implementation (`internal/git/`)
- [x] `Runner` integration tests (real git repos in `t.TempDir()`)
- [x] State file types + read/write (`internal/state/`)
- [ ] Saga implementation + tests (`internal/saga/`)
- [ ] UI helpers — shared lipgloss styles and output formatting (`internal/ui/`)

## Phase 2 — Commands

Each command includes unit tests (mock `Runner`) in the same session.

- [ ] `wtg init`
- [ ] `wtg repo discover` / `wtg repo list`
- [ ] `wtg repo sync`
- [ ] `wtg space create`
- [ ] `wtg space list`
- [ ] `wtg space add`
- [ ] `wtg space delete`
- [ ] `wtg status`

## Phase 3 — Polish

- [ ] Shell completions (bash, zsh, fish)
- [ ] CI (GitHub Actions: build + test)
- [ ] Release workflow (goreleaser)
