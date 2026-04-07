# Implementation Plan

## Phase 6 — CLI Restructure

Rework the command hierarchy so the implicit subject at root level is always a
space, and `repo` is an explicit utility namespace. Each item is an independent
change — work through them one at a time.

### Command promotion (space operations → root)

- [ ] **C1** — Promote `wtg space create` → `wtg create`
- [ ] **C2** — Promote `wtg space delete` → `wtg delete` (keep `rm` alias)
- [ ] **C3** — Promote `wtg space add` → `wtg add`
- [ ] **C4** — Promote `wtg space remove` → `wtg remove`
- [ ] **C5** — Promote `wtg space exec` → `wtg exec`
- [ ] **C6** — Promote `wtg space push` → `wtg push`
- [ ] **C7** — Promote `wtg space path` → `wtg path`
- [ ] **C8** — Promote `wtg space list` → `wtg list`

### Removals

- [ ] **C9** — Remove `wtg space rebase` (covered by `wtg exec <name> git rebase origin/main`; parallel rebase makes conflict recovery harder, not easier)
- [ ] **C10** — Remove `wtg space status` alias (redundant with `wtg status`)

### exec pass-through convention

- [ ] **C15** — Change `wtg exec` (currently `wtg space exec`) to use `--` as the pass-through separator instead of `SkipFlagParsing`. Signature becomes `wtg exec <name> -- <cmd> [args...]`.

### Flag and command polish

- [ ] **C16** — Standardize verbose flag to `--long` / `-l` on both `status` and `repo status` (currently `status` uses `--detailed`, `repo status` uses `--long`; align on `--long` to match git conventions and avoid collision with `-d`)
- [ ] **C17** — Add long-form aliases `--delete-branch` / `--force-delete-branch` to the `-d` / `-D` flags on `delete` and `remove`
- [ ] **C18** — Hide `wtg path` from help output (`Hidden: true`); keep it functional for use in shell `wcd` functions
- [ ] **C19** — Remove `wtg list` (redundant with `wtg status`)
- [ ] **C20** — Replace `wtg version` subcommand with a `--version` flag on the root command

### Cleanup

- [ ] **C11** — Remove the now-empty `wtg space` command group (after C1–C8 and C9–C10)
- [ ] **C12** — Update README and all help strings to reflect new command paths
- [ ] **C13** — Update shell completion handlers for renamed/moved commands
