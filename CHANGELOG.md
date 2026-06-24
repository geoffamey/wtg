# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- `wtg push` skips symlinked repos instead of reporting an expected failure (the workspace branch never exists in the shared main clone)
- `wtg exec` labels symlinked repos as `(symlink → main clone)` in the per-repo header so their output is clearly from the main clone

## [1.3.0] - 2026-06-21

### Added

- `wtg config` command group: `wtg config` prints the resolved config file, `wtg config init` scaffolds a commented TOML template (`--force`, `-o PATH`, `-o -`), `wtg config path` prints the resolved path
- TOML config support; the parser is selected by file extension, so existing `config.yaml` files keep loading
- `always.run` config key — an executable run after a space is created, changed, or deleted (see `docs/always.md`)

### Changed

- Config is now TOML-primary: the default path is `$XDG_CONFIG_HOME/wtg/config.toml`

### Removed

- `wtg init` interactive wizard, replaced by `wtg config init`

## [1.2.0] - 2026-05-19

### Added

- `always.repos` and `always.files` config keys — repos symlinked into and files copied into every new space

## [1.1.0] - 2026-05-18

### Changed

- `wtg status` always shows all spaces, with the current space sorted first
- `wtg new` requires at least one repo argument, and saves workspace state before the saga runs so cleanup covers a partial creation

### Fixed

- `wtg push` sets the upstream on first push and detects remote-only branches

## [1.0.0] - 2026-04-08

Initial public release.

### Added

- `wtg new` — create a workspace: one linked worktree per repo, all on the same branch, with an auto-generated `go.work` for Go modules
- `wtg delete` — remove all worktrees in a workspace; optionally delete branches with `-d` (merged only) or `-D` (force)
- `wtg add` / `wtg remove` — add or remove repos from an existing workspace; `go.work` is kept in sync
- `wtg status` — combined view of main repo clones and all workspaces; `--long` expands to per-file changes
- `wtg exec <workspace> -- <cmd>` — run a command in each worktree sequentially, continuing past failures
- `wtg push` — push all workspace branches to origin in parallel
- `wtg repo status` / `fetch` / `sync` — inspect and update main repo clones across the discovery root
- `wtg init` — interactive setup wizard
- Shell completions for bash, zsh, and fish; `wcd <workspace>` helper to `cd` into a workspace root
- Saga-based rollback: failures during `wtg new` or `wtg add` clean up any worktrees and branches created so far
- XDG Base Directory-compliant config (`$XDG_CONFIG_HOME/wtg/config.yaml`) and state (`$XDG_DATA_HOME/wtg/spaces/`)
- `--config` flag and `WTG_CONFIG` environment variable for config file override

[Unreleased]: https://github.com/geoffamey/wtg/compare/v1.3.0...HEAD
[1.3.0]: https://github.com/geoffamey/wtg/compare/v1.2.0...v1.3.0
[1.2.0]: https://github.com/geoffamey/wtg/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/geoffamey/wtg/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/geoffamey/wtg/releases/tag/v1.0.0
