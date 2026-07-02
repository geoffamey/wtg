---
name: wtg
description: Use this skill whenever you are working inside a wtg workspace (space) — a directory where each subdirectory is a git worktree of a different repo, all on the same branch. Triggers when the space CLAUDE.md instructs you to use it, when the user asks about wtg commands or multi-repo operations, when you need to create or manage a workspace, or when you notice a go.work file alongside multiple git repo subdirectories. Use proactively — don't wait to be asked.
---

# wtg Workspace

A wtg space is a directory with one git worktree per repo, all sharing one branch
(`<prefix><space-name>`, e.g. `alice/my-feature`).

**Symlinked subdirectories are read-only context repos** (pulled in via `always.repos`).
Don't commit to or modify them unless the repo was added to the space with `wtg add`.

All repos share the branch name, but each is pushed and PR'd independently: one PR per
repo, coordinate the merges. wtg auto-writes a `go.work` at the space root when any repo
has a `go.mod`.

## Commands

wtg is actively developed — get current docs from the tool, don't trust stale memory:

```bash
wtg --help                  # command list
wtg <subcommand> --help     # flags and details for one command
```

`wcd <space>` (a shell function from setup) jumps into a space.
