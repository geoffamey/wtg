---
name: wtg-setup
description: Install, configure, and onboard a new user to wtg, a tool for managing multi-repo feature workspaces using git worktrees. Use when someone asks how to install wtg, set it up for the first time, revisit an existing setup, or get started with the multi-repo workspace workflow.
---

# wtg (WorkTrees for Go) Setup

Walk the user through installing and configuring wtg, then help them create space instruction templates that give their coding agent context in every new workspace.

## Re-running this skill

This skill is also how a user revisits an existing setup. A common reason is a new wtg
release: they heard there are new features and want them. So on a re-run, treat updating
and reconciling the config as the main job, not a fresh install.

- Note the installed version (`wtg --version`), then run the install step — `go install
  @latest` is idempotent and pulls the newest release. If the version changed, new config
  settings may exist.
- Reconcile the config against the new version's scaffold rather than editing the old
  file blind. Step 5's existing-config branch covers the merge: regenerate the current
  scaffold, keep the user's settings, and surface anything new.
- Every other step must be safe to re-run too: for shell integration, check the rc file
  for the lines before adding them so you don't duplicate.

If the user came back just to tweak one setting and isn't chasing an upgrade, you can skip
straight to reading their config and editing it — but a version bump is cheap to check.

## Step 1: Check Go

wtg is installed via `go install`, so Go must be present first:

```bash
go version
```

If this fails, direct the user to https://go.dev/doc/install and wait for confirmation before proceeding.

## Step 2: Install wtg

```bash
go install github.com/geoffamey/wtg@latest
```

## Step 3: Check PATH

Go installs binaries to `~/go/bin`. Verify wtg is reachable:

```bash
which wtg
```

If this fails, `~/go/bin` is not in PATH. Show the fix for their shell:

**bash** — add to `~/.bashrc`:
```bash
export PATH="$HOME/go/bin:$PATH"
```

**zsh** — add to `~/.zshrc`:
```zsh
export PATH="$HOME/go/bin:$PATH"
```

**fish** — run once:
```fish
fish_add_path ~/go/bin
```

Open a new terminal (or source the config file) before continuing.

## Step 4: Shell Integration

Add completions and the `wcd` helper. `wcd <space>` changes directory into a workspace — it must be a shell function because `cd` can't run inside a subprocess.

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

Remind the user to open a new terminal (or source the file) before continuing.

## Step 5: Configure wtg

Before scaffolding the config, scan the user's home directory for existing git repos so
you can give an informed suggestion for where wtg should scan for them:

```bash
find ~ -maxdepth 3 -name ".git" -type d 2>/dev/null | sed 's|/.git||' | sort
```

Look at the results and identify patterns:
- If repos are already concentrated in a folder like `~/repos`, `~/code`, or `~/projects`,
  suggest that as the directory wtg scans.
- If repos are scattered across the home directory or Desktop, suggest consolidating them.
  Propose a folder (e.g. `~/repos`) and offer to move them there and switch each to its
  default branch — but **always ask first and never proceed without explicit confirmation**.
  Before moving anything, check each repo for uncommitted changes or unpushed commits and
  warn the user. Skip any repo that isn't clean.

A good prompt: "I found git repositories in [locations]. wtg works best when all repos
live in one folder and track their default branch. Want me to create `~/repos/` and move
them there? I'll only touch repos with no uncommitted or unpushed changes."

### Scaffold and edit the config

`wtg config init` writes a commented TOML template (every setting shown commented out
with its default) to `~/.config/wtg/config.toml`. There's no interactive wizard — the
file is the interface, so the work is generating it and editing it with the user.

**First check whether a config already exists** (`~/.config/wtg/config.toml`, or the
manager's source path for dotfile users). `config init` refuses to overwrite, by design.

If there's no config yet, scaffold one:

```bash
wtg config init
```

**If a config already exists, reconcile it with the current scaffold** instead of editing
the old file blind — a newer wtg may have added settings the user's file never had. The
merge, given that this skill only ever *uncomments* lines (never restructures):

1. Generate the current scaffold to stdout and capture it: `wtg config init -o -`. This is
   the new base — every setting at its default, with up-to-date comments.
2. Find the user's overrides: the uncommented `key = value` lines in their existing
   config. Those are the only things they changed.
3. Diff the two to spot what's new: settings present in the scaffold but absent from the
   user's config. Those are the new features the re-run is probably about.
4. Build the merged config: take the new scaffold as the base, then uncomment/set each of
   the user's overrides on it (preserving their values). The result keeps the user's
   settings and gains the new scaffold's comments and any new settings (still commented).
5. Walk the user through the new settings from step 3 and ask whether to enable any.
6. Write the merged config back (to the manager's source for dotfile users) and run `wtg
   status` to confirm it loads (`wtg config` only dumps the file, it doesn't validate).

Never `--force` a blank scaffold over a populated config — that wipes their values.

### Editing the config

Whichever branch you took, edit the scaffold **in place** with the user: uncomment the
lines you're setting and change their values, and leave everything else untouched. Do not
rewrite the file or replace its comments with your own summaries — the verbose inline
documentation is the point of the scaffold, and the user will read it later. The diff
should only touch the handful of lines you set.

Don't work from a list of settings memorized here — it drifts as wtg changes. The
scaffold you just generated documents every setting and its default in its own comments;
that file is the source of truth. Read it, then walk the user through the settings it
actually contains, deciding each together and uncommenting the ones they want to change.

Bring the context the file can't know:

- For wherever repos are discovered, use what the home-directory scan above told you.
- Whichever setting copies files into each new space should point at the instruction
  template or templates you'll write in Step 6 (for example,
  `~/.config/wtg/space-template/AGENTS.md` and/or `CLAUDE.md`).
- The setting that runs a script on space events is covered in Step 5a — ask the user
  before leaving it empty.
- Ask whether they want a branch prefix, and what it should be.

If a setting in the scaffold isn't covered above, lean on its comment and ask the user.
Run `wtg config --help` if you need current subcommand details.

Edit the file with the Edit tool. To confirm it's valid, run `wtg status` (or any
command) — wtg parses the config only when a command loads it, so a syntax or type error
surfaces there. `wtg config` with no subcommand just dumps the file's raw contents and
does not validate.

**If the user manages dotfiles with a tool** (chezmoi, yadm, a bare git repo, stow): they
won't want files edited in their final location, and you should never hand-write a
replacement config in the manager's source — that loses the scaffold's comments. Use
`config init`'s `-o` flag to scaffold straight into the manager's source, then edit it
there (uncommenting lines only) and apply. For chezmoi:

```bash
wtg config init -o ~/.local/share/chezmoi/dot_config/wtg/config.toml
```

(`-o -` writes the scaffold to stdout if you just want to read it.) Then edit it the same
way as above — uncommenting lines only — and `chezmoi apply`. The same principle applies
to the `on-event` script and space template: write them into the manager's source and
apply, rather than authoring into the live location.

## Step 5a: Lifecycle hook (always.run)

`always.run` points at an executable that wtg runs **after** a space operation completes
(`create`, `add`, `remove`, `delete`). It's best-effort — a non-zero exit just prints a
warning, the operation still succeeds. The script is invoked directly (not via a shell),
so it needs a shebang and `chmod +x`.

This is worth calling out during onboarding: ask the user whether they want anything to
happen automatically when a space is created or changed. Common uses:
- `direnv allow` the space and open it in their editor on `create`
- Run `npm install` / `go mod download` in each repo of a fresh space
- Send a notification on `delete`

If they want one, write the script (default `~/.config/wtg/on-event`), `chmod +x` it, and
set `always.run` to its path. The script receives its context (event type, space name and
path, branch, repos) through environment variables. Don't reproduce the variable list
here — it drifts. Read the authoritative reference before writing the script:

https://raw.githubusercontent.com/geoffamey/wtg/main/docs/always.md

That doc names every variable and its format and shows how to dispatch on the event type.
If the user isn't sure they want a hook, leave `always.run` unset — it's easy to add
later by editing the config.

## Step 6: Create Space Instruction Templates

Ask which coding agents the user wants each space to support: Claude Code, Codex, or both.
Create the matching template files under a directory such as
`~/.config/wtg/space-template/`, then include every template path in the `always.files`
setting from Step 5.

- Claude Code reads `CLAUDE.md`.
- Codex reads `AGENTS.md`.
- For both, put the shared content in `AGENTS.md` and make `CLAUDE.md` import it with
  `@AGENTS.md`. This avoids maintaining two copies.

The instruction file's job is small: announce that the directory is a wtg space so the
`wtg` skill loads, and make loading it a precondition for writing code or touching git.
The skill carries the wtg mechanics (what a space is, the commands, the read-only-symlink
rule, and the edit-in-the-space rule), so the instruction file must not restate them.
Duplicating the skill adds context cost to every session run in the space.

Use this shared content for `AGENTS.md`, or for `CLAUDE.md` when supporting Claude Code
alone:

```markdown
# Space

This is a **wtg feature space** — a multi-repo workspace where each subdirectory is a git
worktree of a different repo, all on the same branch.

Before writing or editing any code, or running git, in this space, load the installed
`wtg` workspace skill and follow it. It carries the rules for working across the space's
worktrees.
```

That wording trips the skill's trigger and makes loading it a precondition for acting.
The skill takes over from there. Do not embed `wtg --help` output, a command list, or the
worktree rules themselves; those belong in the skill and drift over time.

### Optional space-specific additions

Add only what neither the skill nor the user's existing instructions already cover. Ask
what else the coding agent should know, but skip anything redundant. Every line here
loads in every session in the space:

- **Team conventions** (PR title prefixes, PR template, issue tracker, build/test
  commands): add these only if the user doesn't already keep them in a global instruction
  file that loads every session. If they do, don't copy them here.
- **Always-present repos**: if repos are symlinked into every space via the repos setting,
  a one-line note on which are read-only context can help.
- **Space naming**: if spaces are named after tickets (e.g. `lin-123-thing`), a line
  telling the coding agent to look the ticket up in their tracker gives it free context.

One or two questions, not a checklist. Default to leaving things out unless they earn
their place.

Once written, make sure the config setting that copies files into new spaces points at its
path (you may have set this in Step 5). If not, add it and run `wtg status` to confirm the
config loads.
