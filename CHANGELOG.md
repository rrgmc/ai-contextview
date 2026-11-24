# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Changes that are not released yet live as one file per change in
[changelog.d/](changelog.d/README.md). They are merged into this file when a
release is tagged.

## [Unreleased]

See [changelog.d/](changelog.d/).

## [0.6.0] - 2026-11-24

### Added

- Each GitHub release has prebuilt `ai-specview` binaries for Linux and macOS
  (amd64 and arm64).
- `CHANGELOG.md`, with unreleased changes kept as one file per change in
  `changelog.d/`.
- The README explains how to install a release by its version tag.
- The `ai-specview:spec-elaborate-mod` skill: the spec workflow with the spec,
  the code and the questions in a pane beside the chat instead of the browser
  page. It needs neither the binary nor the daemon.
- The pane asks the questions itself: options, "Decide later", Other, notes,
  and "Send answers", "Back to chat" and "Skip".
- While a spec is open in the pane, the built-in `AskUserQuestion` is blocked.
- The pane shows a "Spec finished" banner with the summary after `spec_done`.
- The pane shows quotes from `doc_refs` in other markdown files, and has a file
  picker for folder specs and quoted files.

### Changed

- The pane is part of the `ai-specview` plugin, with tools of its own, instead
  of a separate `ai-specview-pane` plugin that hooked the browser viewer's
  tools.

### Removed

- The `ai-specview-pane` plugin, its `mode` option and `/specview-mode`: pick
  the pane or the browser by skill. Uninstall it with
  `claude plugin uninstall ai-specview-pane@ai-specview`.

## [0.5.0] - 2026-11-24

### Added

- The `ai-specview` binary, imported from the specview proof of concept: the
  `mcp` bridge, the `serve` viewer daemon, the `hook` subcommand and the
  `fakeapp` test stand-in.
- The `ai-specview-pane` mod, which shows the viewer in a Claude Code pane.
- The version comes from git tags. `ai-specview version` prints the tag, a Go
  pseudo-version between tags, or `devel`.

### Fixed

- The Changes tab stays on the file shown in the Spec tab.

[Unreleased]: https://github.com/rrgmc/ai-specview/compare/v0.6.0...HEAD
[0.6.0]: https://github.com/rrgmc/ai-specview/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/rrgmc/ai-specview/releases/tag/v0.5.0
