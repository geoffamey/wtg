# wtg

A CLI tool for managing multi-repo feature workflows using git worktrees and Go workspaces.

When working on a feature that spans multiple repositories, `wtg` lets you create a named
workspace that checks out a consistent branch across all relevant repos as git worktrees,
and wires them together with a `go.work` file so they compile against each other.

## Concepts

- **Repos**: git repositories discovered under a configured root directory (e.g. `~/repos`)
- **Space**: a named workspace containing worktrees of selected repos on a shared branch, with a `go.work` tying them together
- **Worktree**: a git linked worktree — a separate working directory for a branch, sharing the same object store as the main clone

## Installation

```sh
go install github.com/geoffamey/wtg@latest
```

## Configuration

Config file location (XDG): `~/.config/wtg/config.yaml`

Override with `--config <path>` or `WTG_CONFIG=<path>`.

```yaml
discovery:
  root_dir: ~/repos      # where to scan for git repositories
  max_depth: 2           # how deep to scan (default: 2)

spaces:
  root_dir: ~/workspaces # where to create workspace directories

git:
  branch_prefix: ""      # optional prefix for created branches, e.g. "yourname/"
```

Individual config keys can be overridden with environment variables using the `WTG_` prefix
and `_` separators, e.g. `WTG_GIT_BRANCH_PREFIX=geoff/`.

## Commands

### `wtg repo discover`

Scan `discovery.root_dir` up to `discovery.max_depth` for git repositories and print them.
No cache is maintained — the scan is fast at typical depths.

Repo names are relative paths from `discovery.root_dir`, so nested repos are unambiguous:

```
$ wtg repo discover
api             https://github.com/myorg/api.git       /Users/geoff/repos/api
myorg/frontend  https://github.com/myorg/frontend.git  /Users/geoff/repos/myorg/frontend
myorg/payments  https://github.com/myorg/payments.git  /Users/geoff/repos/myorg/payments
infra           https://github.com/myorg/infra.git     /Users/geoff/repos/infra
```

`wtg repo list` is an alias for `wtg repo discover`.

### `wtg repo sync [<repo>...]`

Fetch and fast-forward each repo's default branch from origin. Operates on the main clones
in `repos_dir`, not on workspace worktrees.

Skips repos whose main clone is dirty or not on the default branch, with a warning.
If no repos are specified, syncs all discovered repos.

Run this before creating a new space to ensure you branch from the latest upstream.

```
$ wtg repo sync
api          ✓ up to date
frontend     ↑ fast-forwarded to origin/main (3 commits)
payments     ⚠ skipped — working tree is dirty
```

### `wtg space create <name> <repo1> [<repo2>...] [--branch <branch>]`

Create a new workspace. For each repo:

1. Creates (or checks out) a branch named `<prefix><name>` (or `--branch` if specified)
2. Creates a git worktree at `<spaces.root_dir>/<name>/<repo>`
3. If the repo has a `go.mod` at its root, adds it to a `go.work` in the workspace root

If the branch already exists in a repo (e.g. a teammate's branch you're picking up),
it is checked out rather than created fresh.

Repos without `go.mod` are included as worktrees but silently excluded from `go.work`.
Fails with an error if the worktree path already exists.

```
$ wtg space create myfeature api frontend payments
Creating space 'myfeature' with branch 'geoff/myfeature'
  api       ✓ branch created, worktree added
  frontend  ✓ branch created, worktree added
  payments  ✓ branch created, worktree added
go.work written (3 modules)

Space ready at ~/workspaces/myfeature
```

### `wtg space list`

List all known spaces.

```
$ wtg space list
myfeature    3 repos   api, frontend, payments
otherthing   2 repos   api, infra
```

### `wtg space add <name> <repo>`

Add a repo to an existing space. Creates (or checks out) the space's branch in that repo
and creates a worktree. Updates `go.work` if applicable.

### `wtg space delete <name> [-d | -D]`

Remove a workspace. Checks for unmerged commits and dirty working trees across all worktrees.
If any are found, prints a summary and prompts for confirmation before proceeding.

Removes all worktrees and the workspace directory. Branches are left untouched by default.

| Flag | Behavior |
|------|----------|
| (none) | Remove worktrees only, keep branches |
| `-d` | Also delete branches that have been fully merged (safe, equivalent to `git branch -d`) |
| `-D` | Force-delete branches regardless of merge status (equivalent to `git branch -D`) |

### `wtg status [<name>] [--all]`

Show workspace status.

- **No args, inside a workspace**: detailed status of the current space
- **No args, outside a workspace**: summary of all spaces
- **`<name>`**: detailed status of the named space
- **`--all`**: detailed status of all spaces

Summary view:
```
myfeature    3 repos   api, frontend, payments   ! dirty
otherthing   2 repos   api, infra                ✓ clean
```

Detailed view:
```
Space: myfeature  ~/workspaces/myfeature

  api       [geoff/myfeature]  ✓ clean     ↑ 2 ahead of origin
  frontend  [geoff/myfeature]  ! 3 modified, 1 untracked
  payments  [geoff/myfeature]  ✓ clean     ↑ 0
```

Status always validates against the actual filesystem and git state — it does not blindly
trust the stored space metadata.

## Data Layout

```
~/.config/wtg/config.yaml          # configuration (XDG config)
~/.local/share/wtg/spaces/          # space state (XDG data)
  myfeature.yaml
  otherthing.yaml

~/repos/                            # your main clones (managed by you, discovered by wtg)
  api/
  frontend/

~/workspaces/                       # worktrees created by wtg
  myfeature/
    api/                            # linked worktree of ~/repos/api
    frontend/                       # linked worktree of ~/repos/frontend
    go.work
```

## Future Work

- `space rebase <name>` — rebase all worktrees onto updated default branch
- `space exec <name> <cmd>` — run a command in each repo of a space
- `space rename <old> <new>` — rename a space, its directory, and optionally its branches
- `repo clone <url>` — clone into `repos_dir` and make available for discovery
- Submodule handling
- `--base <branch>` flag on `space create` to branch from something other than default
- Shell completions (bash, zsh, fish)
