# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/geoffamey/wtg/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/geoffamey/wtg/releases/tag/v1.0.0
