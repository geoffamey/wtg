# wtg

Manage multi-repo feature workflows using git worktrees and Go workspaces.

When a feature spans multiple repos, `wtg` checks out a shared branch across all of them as git worktrees and wires them together with a `go.work` file.

## Installation

```sh
go install github.com/geoffamey/wtg@latest
```

### Shell completions (fish)

```fish
wtg completion fish > ~/.config/fish/completions/wtg.fish
```

Or to source dynamically from your `config.fish` / `conf.d`:

```fish
if status is-interactive
    wtg completion fish | source
end
```

### `wcd` — cd into a space

Since `cd` must run in the current shell, add a wrapper function. `wtg space path <name>` prints the space's root path.

**fish:**
```fish
function wcd
    cd (wtg space path $argv[1])
end
```

**bash / zsh:**
```bash
wcd() {
    cd "$(wtg space path "$1")"
}
```

## Concepts

- **Repo** — a git repository discovered under `discovery.root_dir`
- **Space** — a named workspace: worktrees of selected repos on a shared branch, linked by a `go.work`

## Configuration

Run `wtg init` for an interactive setup, or create `~/.config/wtg/config.yaml` directly:

```yaml
discovery:
  root_dir: ~/repos       # where to scan for git repositories
  max_depth: 2            # scan depth (default: 2)

spaces:
  root_dir: ~/workspaces  # where to create workspace directories

git:
  branch_prefix: ""       # optional prefix for created branches, e.g. "yourname/"
```

Override any key with `--config <path>`, `WTG_CONFIG=<path>`, or env vars like `WTG_GIT_BRANCH_PREFIX=geoff/`.

## Commands

### `wtg repo discover`

Scan for git repos and print their names, remote URLs, and paths. `wtg repo list` is an alias.

```
api     https://github.com/myorg/api.git      /Users/geoff/repos/api
infra   https://github.com/myorg/infra.git    /Users/geoff/repos/infra
```

### `wtg repo sync [<repo>...]`

Fetch and fast-forward each repo's default branch. Skips repos that are dirty or not on the default branch. Syncs all discovered repos if none are specified.

### `wtg repo status [<repo>...]`

Show branch, working-tree state, and ahead/behind counts for each repo's main clone.

### `wtg space create <name> [<repo>...] [--branch <branch>] [--path <dir>]`

Create a workspace. For each repo, creates or checks out a branch named `<prefix><name>` and adds a linked worktree. Writes a `go.work` for any repos with a `go.mod`. Uses all discovered repos if none are specified.

**Branch behaviour per repo:**

| Branch state | Action |
|---|---|
| Does not exist | Created from HEAD of the repo's default branch |
| Exists, not checked out | Checked out in the new worktree |
| Exists, already checked out elsewhere | Error |

### `wtg space list`

List all spaces with their branch, path, and repo count.

### `wtg space add <name> <repo>...`

Add repos to an existing space. Creates worktrees on the space's branch and updates `go.work`.

### `wtg space delete <name> [-d | -D]`

Remove a space's worktrees. Prompts for confirmation if any worktree has uncommitted changes or unpushed commits.

| Flag | Branch behaviour |
|------|-----------------|
| (none) | Branches left untouched |
| `-d` | Delete branches fully merged into upstream |
| `-D` | Force-delete branches |

### `wtg status [<name>] [--detailed]`

Show workspace status.

- **No args, inside a workspace**: status of the current space
- **No args, outside a workspace**: summary of all spaces
- **`<name>`**: status of the named space

Summary view:
```
myfeature    geoff/myfeature   ~/workspaces/myfeature   3 repos
otherthing   geoff/otherthing  ~/workspaces/otherthing   2 repos
```

Space view (one row per repo):
```
myfeature   geoff/myfeature   ~/workspaces/myfeature   3 repos
  api       [geoff/myfeature]  ✓ clean     ↑2 ↓0
  frontend  [geoff/myfeature]  ✗ 3 modified, 1 untracked
  payments  [geoff/myfeature]  ✓ clean     ↑0 ↓0
```

Detailed view (`--detailed` — lists modified files per repo):
```
myfeature   geoff/myfeature   ~/workspaces/myfeature   3 repos
  api       [geoff/myfeature]  ✓ clean     ↑2 ↓0
  frontend  [geoff/myfeature]  ✗ 3 modified, 1 untracked
    M  src/handler.go
    M  src/middleware.go
    ?? src/newfile.go
  payments  [geoff/myfeature]  ✓ clean     ↑0 ↓0
```

## Data layout

```
~/.config/wtg/config.yaml        # configuration
~/.local/share/wtg/spaces/        # space state
  myfeature.yaml

~/repos/                          # your main clones (discovered by wtg)
  api/
  infra/

~/workspaces/myfeature/           # worktrees created by wtg
  api/
  infra/
  go.work
```

## Future work

- `space rebase <name>` — rebase all worktrees onto updated default branch
- `space exec <name> <cmd>` — run a command in each repo of a space
- `space rename <old> <new>` — rename a space and its directory
- `repo clone <url>` — clone into `discovery.root_dir` and make available immediately
