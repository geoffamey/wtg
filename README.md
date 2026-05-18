# wtg

`wtg` manages feature branches that span multiple repos. When a feature touches several repos at once, `wtg` checks out a shared branch across all of them as [git worktrees](https://git-scm.com/docs/git-worktree) and wires them together with a `go.work` file — giving you an isolated, ready-to-build workspace per feature without cloning anything new.

> New to worktrees? The [GitKraken worktree guide](https://www.gitkraken.com/learn/git/git-worktree) is a good primer.

## Installation

```sh
go install github.com/geoffamey/wtg@latest
```

### Shell completions

**bash** — add to `~/.bashrc`:
```bash
source <(wtg completion bash)
wcd() { cd "$(wtg path "$1")"; }
```

**zsh** — add to `~/.zshrc`:
```zsh
source <(wtg completion zsh)
wcd() { cd "$(wtg path "$1")"; }
```

**fish** — add to `~/.config/fish/conf.d/wtg.fish`:
```fish
if status is-interactive
    wtg completion fish | source
    function wcd
        cd (wtg path $argv[1])
    end
    complete -c wcd -f -a '(wtg path --generate-shell-completion 2>/dev/null)'
end
```

`wcd <workspace>` is a shell helper that `cd`s into the workspace root. Since
`cd` must run in the current shell it can't be a standalone command. The fish
`complete` line gives `wcd` the same workspace-name tab completion as `wtg path`.

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
  branch_prefix: ""       # prepended to workspace names, e.g. "yourname/"
```

`discovery.root_dir` should contain your regular repo clones, each sitting on
their default branch (`main`, `master`, etc.) and otherwise left untouched.
`wtg` creates worktrees alongside them — it never modifies the main clones.

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

### `wtg new <workspace> <repo>...`

Create a workspace. At least one repo must be specified. For each repo, `wtg`
creates or checks out a branch named `<branch_prefix><workspace>` as a linked
worktree. A `go.work` file is written automatically for repos that have a
`go.mod`.

```sh
wtg new my-feature api payments frontend
wtg new my-feature api --branch yourname/main  # check out an existing branch
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

Show workspace status. Without arguments, shows all workspaces — the one
containing the current directory is listed first. Pass `--long` / `-l` to
expand file-level changes per repo.

```sh
wtg status
wtg status my-feature
wtg status my-feature --long
```

```
my-feature  ~/workspaces/my-feature
  api       [geoff/my-feature]  ✓ clean      ↑2
  payments  [geoff/my-feature]  ! 2 modified
  frontend  [geoff/my-feature]  ✓ clean
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
