---
name: wtg
description: Use for wtg commands, workspace creation or management, adding or removing repos, and coordinated multi-repo workspace operations. Do not use merely because the current directory is an existing wtg workspace; ordinary code edits, tests, and Git commands inside existing worktrees do not require this skill.
---

# wtg Workspace

A wtg space is a directory with one git worktree per repo, all sharing one branch
(`<prefix><space-name>`, e.g. `alice/my-feature`).

**Symlinked subdirectories are read-only context repos** (pulled in via `always.repos`).
Don't commit to or modify them unless the repo was added to the space with `wtg add`.

**Edit inside the space, never in `~/repos`.** Each repo subdirectory here is a git
worktree, and the same repo usually also has an ordinary checkout at `~/repos/<repo>` on a
different branch. Read `~/repos` freely (e.g. to inspect a repo the space doesn't include),
but make every edit — and run every git command — in the space's copy
(`<space>/<repo>/`). Editing `~/repos/<repo>` silently lands your work on the wrong branch,
in a worktree the space never sees. If you need to change a repo that isn't in the space
yet, add it with `wtg add` first; do not reach into `~/repos` to edit it.

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
