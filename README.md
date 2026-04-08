# wtg

`wtg` manages feature branches that span multiple repos. When a feature touches several repos at once, `wtg` checks out a shared branch across all of them as [git worktrees](https://git-scm.com/docs/git-worktree) and wires them together with a `go.work` file — giving you an isolated, ready-to-build workspace per feature without cloning anything new.

> New to worktrees? The [Atlassian worktree guide](https://www.atlassian.com/git/tutorials/git-worktree) is a good primer.

## Installation

```sh
go install github.com/geoffamey/wtg@latest
```

### Shell completions

**bash** — add to `~/.bashrc`:
```bash
source <(wtg completion bash)
```

**zsh** — add to `~/.zshrc`:
```zsh
source <(wtg completion zsh)
```

**fish** — add to `~/.config/fish/conf.d/wtg.fish`:
```fish
if status is-interactive
    wtg completion fish | source
end
```

### `wcd` — jump into a workspace

Since `cd` must run in the current shell, add a small wrapper function:

**fish:**
```fish
function wcd
    cd (wtg path $argv[1])
end
```

**bash / zsh:**
```bash
wcd() { cd "$(wtg path "$1")"; }
```

## Configuration

Run `wtg init` to create `~/.config/wtg/config.yaml` interactively:

```sh
wtg init
```

Or write the config directly:

```yaml
discovery:
  root_dir: ~/repos       # where wtg scans for git repos
  max_depth: 2

spaces:
  root_dir: ~/workspaces  # where workspaces are created

git:
  branch_prefix: ""       # prepended to workspace names, e.g. "geoff/"
```

Override with `--config <path>` or the `WTG_CONFIG` environment variable.

## Quick start

```sh
# Create a workspace for a new feature across three repos
wtg new my-feature api payments frontend

# Jump in
wcd my-feature

# ... do your work, then clean up
wtg delete my-feature --delete-branch
```

## Workspace commands

### `wtg new <workspace> [<repo>...]`

Create a workspace. For each repo, `wtg` creates or checks out a branch named
`<branch_prefix><workspace>` as a linked worktree. A `go.work` file is written
automatically for repos that have a `go.mod`. If no repos are specified, all
discovered repos are included.

```sh
wtg new my-feature api payments frontend
wtg new my-feature                          # include all discovered repos
wtg new my-feature api --branch geoff/main  # check out an existing branch
```

Branch behaviour per repo:

| Branch state | Action |
|---|---|
| Does not exist | Created from the repo's default branch |
| Exists, not checked out | Checked out in the new worktree |
| Exists, already checked out | Error |

### `wtg delete <workspace>`

Delete a workspace and remove its worktrees. Prompts for confirmation if any
repo has uncommitted changes or unpushed commits.

```sh
wtg delete my-feature            # remove worktrees, keep branches
wtg delete my-feature -d         # also delete branches if merged
wtg delete my-feature -D         # force-delete branches
```

### `wtg add <workspace> <repo>...`

Add repos to an existing workspace. Creates worktrees on the workspace's branch
and updates `go.work`.

```sh
wtg add my-feature infra logging
```

### `wtg remove <workspace> <repo>...`

Remove repos from a workspace. Prompts if there are uncommitted changes or
unpushed commits. Use `wtg delete` to remove the whole workspace.

```sh
wtg remove my-feature logging
wtg remove my-feature logging -d  # also delete the branch
```

### `wtg status [<workspace>]`

Show workspace status. Without arguments, shows all workspaces — or just the
current one if you're inside a workspace directory. Pass `--long` / `-l` to
expand file-level changes per repo.

```sh
wtg status
wtg status my-feature
wtg status my-feature --long
```

```
my-feature  geoff/my-feature  ~/workspaces/my-feature
  api       [geoff/my-feature]  ✓ clean      ↑2 ↓0
  payments  [geoff/my-feature]  ✗ 2 modified
  frontend  [geoff/my-feature]  ✓ clean      ↑0 ↓0
```

### `wtg exec <workspace> -- <cmd> [<args>...]`

Run a command in each repo's worktree sequentially. Execution continues even if
a command fails — all repos are attempted and failures are reported at the end.

```sh
wtg exec my-feature -- git status
wtg exec my-feature -- go test ./...
wtg exec my-feature -- git push origin HEAD
```

## Repo commands

These operate on your main repo clones, not workspace worktrees. Useful for
keeping clones up to date before starting a new feature.

### `wtg repo sync [<repo>...]`

Fetch and fast-forward each repo's default branch. Repos with local changes are
skipped with a warning.

```sh
wtg repo sync               # sync all repos
wtg repo sync api payments  # sync specific repos
```

### `wtg repo status [<repo>...]`

Show branch, dirty status, and ahead/behind counts for each main repo clone.

```sh
wtg repo status
wtg repo status --long  # also show remote URL and local path
```
