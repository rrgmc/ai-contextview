# Unreleased changes

Each change that is not released yet gets its own file in this folder. One
file per change means two branches never edit the same lines, so merges to
`master` do not conflict on the changelog.

## Adding an entry

Create `changelog.d/<branch-name>.md`, with the `/` in the branch name replaced
by `-` (for example `fix-changes-tab.md`). Use one or more
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) sections, with the
same headings as [CHANGELOG.md](../CHANGELOG.md):

```markdown
### Fixed

- The Changes tab stays on the file shown in the Spec tab.
```

The headings are `Added`, `Changed`, `Deprecated`, `Removed`, `Fixed` and
`Security`. Write for users of `ai-specview`, not for its developers. A change
with no visible effect (refactoring, tests, internal docs) needs no entry.

## Releasing

Before tagging `vX.Y.Z`:

1. Add a `## [X.Y.Z] - YYYY-MM-DD` section to `CHANGELOG.md`, under
   `## [Unreleased]`.
2. Merge the entries of every file in this folder into it, one heading per
   kind of change, in the order listed above.
3. Delete those files. Keep this `README.md`.
4. Add the `[X.Y.Z]` compare link at the end of `CHANGELOG.md` and move the
   `[Unreleased]` link to start from `vX.Y.Z`.
5. Commit, then tag that commit.
