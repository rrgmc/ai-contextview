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

[Unreleased]: https://github.com/rrgmc/ai-specview/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/rrgmc/ai-specview/releases/tag/v0.5.0
