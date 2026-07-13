# wtg

- `README.md` — user-facing docs: install, usage, and configuration. Start here.
- `docs/` — longer-form docs. `DESIGN.md` records architecture decisions and rationale for contributors; feature guides (e.g. `always.md`, `wtginclude.md`) explain individual features in depth for users.

## Conventions

- Conventional commits (`feat:`, `fix:`, `docs:`, `test:`, `refactor:`)
- Each command implementation includes its unit tests in the same session/PR
- Ambiguous repo names → error with suggestions, not interactive prompt (keep scriptable)
- Update `CHANGELOG.md` as you add features: user-facing changes go under `## [Unreleased]` (Added/Changed/Fixed/Removed per Keep a Changelog). Skip dependency bumps, CI, and pure refactors.

## Releasing

Entries accrue under `## [Unreleased]` as features land (see above), so a release is just a stamp:

1. Rename `## [Unreleased]` → `## [X.Y.Z] - YYYY-MM-DD`, then add a fresh empty `## [Unreleased]` above it.
2. Update the compare links at the bottom: point `[Unreleased]` at `vX.Y.Z...HEAD` and add `[X.Y.Z]: ...compare/<prev>...vX.Y.Z`.
3. Commit (`chore(release): vX.Y.Z`), then `git tag vX.Y.Z` on that commit.

Pick X.Y.Z by what's in Unreleased: new features → minor bump, fixes only → patch.
