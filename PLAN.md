# Implementation Plan

## Phase 6 — CLI Restructure

Rework the command hierarchy so the implicit subject at root level is always a
space, and `repo` is an explicit utility namespace. Each item is an independent
change — work through them one at a time.

### Command promotion (space operations → root)

- [x] **C1** — Promote `wtg space create` → `wtg create`
- [x] **C2** — Promote `wtg space delete` → `wtg delete` (keep `rm` alias)
- [x] **C3** — Promote `wtg space add` → `wtg add`
- [x] **C4** — Promote `wtg space remove` → `wtg remove`
- [x] **C5** — Promote `wtg space exec` → `wtg exec`
- [x] **C6** — Promote `wtg space push` → `wtg push`
- [x] **C7** — Promote `wtg space path` → `wtg path`
- [x] **C8** — Promote `wtg space list` → `wtg list` _(skipped; see C19)_

### Removals

- [x] **C9** — Remove `wtg space rebase` (covered by `wtg exec <name> -- git rebase origin/main`; parallel rebase makes conflict recovery harder, not easier)
- [x] **C10** — Remove `wtg space status` alias (redundant with `wtg status`)

### exec pass-through convention

- [x] **C15** — Change `wtg exec` (currently `wtg space exec`) to use `--` as the pass-through separator instead of `SkipFlagParsing`. Signature becomes `wtg exec <name> -- <cmd> [args...]`.

### Flag and command polish

- [x] **C16** — Standardize verbose flag to `--long` / `-l` on both `status` and `repo status` (currently `status` uses `--detailed`, `repo status` uses `--long`; align on `--long` to match git conventions and avoid collision with `-d`)
- [x] **C17** — Add long-form aliases `--delete-branch` / `--force-delete-branch` to the `-d` / `-D` flags on `delete` and `remove`
- [x] **C18** — Hide `wtg path` from help output (`Hidden: true`); keep it functional for use in shell `wcd` functions
- [x] **C19** — Remove `wtg list` (redundant with `wtg status`)
- [x] **C20** — Replace `wtg version` subcommand with a `--version` flag on the root command

### Cleanup

- [x] **C11** — Remove the now-empty `wtg space` command group (after C1–C8 and C9–C10)
- [ ] **C12** — Update README and all help strings to reflect new command paths
- [ ] **C13** — Update shell completion handlers for renamed/moved commands
