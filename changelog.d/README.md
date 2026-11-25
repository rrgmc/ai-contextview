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

1. On a branch from `master`, merge the change files into `CHANGELOG.md`:

   ```bash
   go run ./tools/changelog release X.Y.Z
   ```

   This adds a `## [X.Y.Z] - YYYY-MM-DD` section under `## [Unreleased]`, with
   the entries of every change file grouped under one heading per kind, in the
   order listed above. It updates the compare links at the end of the file and
   deletes the change files, keeping this `README.md`. It also sets
   `version` in `plugin/.claude-plugin/plugin.json` to `X.Y.Z`, so the plugin
   and the release share one version. For a stable release it also sets the
   two release tags in the install section of the repository `README.md` (the
   example tag and the `go install …@vX.Y.Z` command) to `vX.Y.Z`; a
   prerelease such as `X.Y.Z-rc.1` leaves the README alone. `-date` sets another date.
2. Read the result, commit it and fast-forward `master` to it.
3. Tag that commit on `master` and push the tag:

   ```bash
   git tag vX.Y.Z && git push origin vX.Y.Z
   ```

The `Release` workflow (`.github/workflows/release.yml`) then runs. It stops
unless the tag is on `master` and `go run ./tools/changelog check vX.Y.Z`
passes, which needs the `[X.Y.Z]` section and links, no change files left and
the plugin version `X.Y.Z` and, for a stable release, the README install tag
`vX.Y.Z`.
It runs the tests, builds `ai-specview` for Linux, macOS and Windows (amd64 and arm64),
and creates the GitHub release with the archives and the `[X.Y.Z]` section as
its notes. A tag with a suffix such as `v1.2.3-rc.1` is marked as a
prerelease. Last, it asks the Go module proxy for the version, so
`go install …@latest` finds it at once.

Run `go run ./tools/changelog check vX.Y.Z` before tagging to catch a failed
check early. A tag whose workflow failed has to be deleted
(`git push --delete origin vX.Y.Z`) and pushed again after the fix.
