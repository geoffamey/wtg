# `.wtginclude`: per-repo local file seeding

A repo can ship a `.wtginclude` file at its root listing local files that should
be copied from the main checkout into each new worktree when that repo is added
to a space (`wtg new` or `wtg add`).

This is for files that live in your day-to-day clone but are not (or should not
be) on the feature branch — `.env`, machine-local config, secrets templates, and
similar.

## Format

One relative path per line. Blank lines are ignored. `#` starts a comment
(full-line or trailing).

```
# seed local env into every worktree of this repo
.env
config/local.env  # nested path is preserved
```

Rules:

- Paths must be relative to the repo root (no absolute paths, no `..`).
- Files only — directories are rejected.
- Missing listed sources are a hard error (the space create/add rolls back).
- Absence of `.wtginclude` is a no-op.

## Behaviour

```sh
# In the main clone of api:
#   api/.wtginclude  →  lists .env
#   api/.env         →  SECRET=…
wtg new my-feature api
# my-feature/api/ is a worktree, then:
#   my-feature/api/.env  ← copy of api/.env
```

- Copies overwrite an existing destination file.
- Parent directories under the worktree are created as needed.
- `always.repos` **symlinks** do not run `.wtginclude` (the symlink already
  points at the main clone). Upgrading a symlink to a worktree with `wtg add`
  does apply `.wtginclude`.

## Compared to `always.files`

| | `.wtginclude` | `always.files` |
|---|---|---|
| Defined in | each source repo | global wtg config |
| Destination | that repo's worktree (relative paths kept) | space root (basename only) |
| Typical use | per-repo local/untracked files | shared templates (CLAUDE.md, `.envrc`) |

See [always.md](always.md) for the config-side seeding and hooks.
