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

## [1.3.5] - 2026-11-26

### Fixed

- In spec mode without a path, a running spec skill no longer makes the first
  markdown file it writes the working document on its own. The file also
  needs a spec folder, spec headings or a second write, so a README or notes
  file written before the spec does not take its place.

## [1.3.4] - 2026-11-26

### Fixed

- In a desktop-app worktree session, relative paths in the first ai-specview
  call (for example `/spec-mode docs/specs/**`) resolve against the worktree,
  not the main checkout. Before, the spec-mode glob matched nothing until
  ask mode was turned on a second time.

## [1.3.3] - 2026-11-26

### Fixed

- In a desktop-app worktree session, the write hook no longer ignores every
  file: a worktree lives under `.claude/worktrees/`, which the hook took for a
  dot-folder.
- In a worktree session, relative paths (doc and code refs, `spec_show`,
  spec-mode paths) resolve against the worktree instead of the main checkout.
  The hook now also runs before ai-specview's own tools to report the
  session's folder; a manual setup updates the `PreToolUse` matcher as the
  README shows.
- A document picked from a write now appears on an open viewer page at once,
  without a reload.

## [1.3.2] - 2026-11-26

### Fixed

- Turning ask mode or spec mode off clears a document that was picked from a
  write, in the viewer and in the pane, so the next question set starts from
  a clean page. A spec shown by Claude stays.

## [1.3.1] - 2026-11-26

### Fixed

- A document picked from a write no longer counts as an open spec after ask
  mode or spec mode is turned off, so the built-in question tool works again.
- A question with no document says so above the document it keeps showing,
  also while a spec is open.

## [1.3.0] - 2026-11-26

### Added

- Ask mode: run `/ai-specview:ask-mode` at the start of a session to send
  every question Claude asks to the ai-specview viewer, with or without a
  spec. Questions point at code only when they are about it, and the
  built-in question tool is blocked while ask mode is on.
  `/ai-specview:ask-mode off` turns it off, and `/ai-specview:ask-mode-mod`
  does the same in the pane.
- A question can point at any document of the project, not only the spec:
  a markdown passage by quote or by lines, or another text file (YAML, JSON,
  SQL) by lines, shown as source with the lines highlighted. When a question
  has several documents, tabs switch between them.
- `AI_SPECVIEW_ASK_MODE=always` turns ask mode on for every session.
- In ask mode, the first markdown document Claude writes (a spec, a plan, a
  design note) shows beside the questions as the working document, with
  change marks and the Changes tab, whichever skill writes it. Bookkeeping
  files never count, and `AI_SPECVIEW_DOC_PATHS` limits which files do.
- Spec mode: `/ai-specview:spec-mode [path or glob]` is ask mode that expects
  a spec, for use with your own or your company's spec skill. With a path or
  glob, the first matching file shows; without one, the written file that
  looks most like a spec. `/ai-specview:spec-mode-mod` does the same in the
  pane.

### Changed

- When no question of a set has a document and no spec is shown, the
  questions and the code take the whole page. The layout changes only when a
  new question set arrives.
- The README and the plugin description present ai-specview as a front end
  for Claude's questions, with spec elaboration built on it.
- `ai-specview hook` also runs after `Write`, `Edit` and `MultiEdit` and
  before `Skill`. The plugin sets this up; a manual setup adds the two hook
  entries shown in the README.

## [1.2.0] - 2026-11-26

### Changed

- A question that already has its own "Decide later" option (or "Defer",
  "Later") no longer shows a second "Decide later" choice, in the browser
  viewer and in the pane.
- When new questions arrive and no viewer page is open, for example because
  you closed the built-in browser pane, Claude reopens the viewer in the
  pane. Without the pane, the default browser opens after 10 seconds.

## [1.1.0] - 2026-11-25

### Added

- `spec_show`, `focus` and `ask` accept paths that start with `~/`, as
  Claude Code's file tools do. On Windows they also accept `~\` and Git Bash
  paths such as `/c/Users/me`. The pane's tools accept `~/` too.

### Changed

- Both skills now write a new spec as a draft in
  `~/.claude/specs/<project>/`, outside the repository, so speccing needs no
  git branch. Before finishing, they ask whether to move it into
  `docs/specs/`. Ask for the repository ("in the repo", "in docs/specs") to
  write it there from the start.

## [1.0.0] - 2026-11-25

### Added

- Releases include Windows binaries for amd64 and arm64, as `.zip` archives
  holding `ai-specview.exe`.

### Fixed

- On Windows, the bridge opens the viewer in the default browser when no page
  is open, as it does on Linux and macOS.
- `ai-specview` builds and runs on Windows 10 and later: `go install` no
  longer fails there, and the daemon, the MCP bridge and the hook work with
  Windows processes and paths.
- The plugin finds the binary on Windows when it is installed under a
  `GOPATH` such as `C:\Users\you\go`.

## [0.9.0] - 2026-11-24

### Changed

- In the Claude desktop app, the browser viewer opens in the app's built-in
  browser instead of the default browser. `AI_SPECVIEW_OPEN=browser`, or
  asking for it in the prompt, keeps the default browser. In the CLI and IDEs
  the default browser opens as before.
- The pane's answer review sets the answers apart from the questions: the
  questions are dimmed, the answers are bold and colored, and notes sit on
  their own line.

## [0.8.0] - 2026-11-24

### Changed

- The ai-specview pane gives the spec the whole width when there is no
  question or code to show beside it.
- In two columns, the question form and the code share the side column
  through tabs, so the form no longer pushes the code out of view.
- The pane's header shows the file and the section on one line, and the jump
  links (changed sections, quotes in other files, back to the question) get
  their own row.
- The question form's buttons are in two rows. "Send answers" is the main
  button only once every question has an answer.
- A recommended option is marked in green, the quote's caption names the
  question, and in one column the form repeats the quote.

## [0.7.0] - 2026-11-24

### Added

- Draft specs: ask either spec skill for a draft spec and it is written to
  `~/.claude/specs/<project>/` instead of the repository. Before it finishes,
  the skill asks whether to move it into `docs/specs/`.

### Changed

- The `ai-specview` plugin now has the same version as the release, starting
  with this one. Plugin changes reach installed copies at each release.

### Fixed

- In the dark theme, names and punctuation in the code pane are readable again;
  they kept the light theme's dark text color.
- In the pane, an answer option's description shows under its button and
  wraps, instead of running into the label and being cut off.
- In the pane (`spec-elaborate-mod`), answering the last question shows a
  review of every answer before sending, as the browser page does.

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

[Unreleased]: https://github.com/rrgmc/ai-specview/compare/v1.3.5...HEAD
[1.3.5]: https://github.com/rrgmc/ai-specview/compare/v1.3.4...v1.3.5
[1.3.4]: https://github.com/rrgmc/ai-specview/compare/v1.3.3...v1.3.4
[1.3.3]: https://github.com/rrgmc/ai-specview/compare/v1.3.2...v1.3.3
[1.3.2]: https://github.com/rrgmc/ai-specview/compare/v1.3.1...v1.3.2
[1.3.1]: https://github.com/rrgmc/ai-specview/compare/v1.3.0...v1.3.1
[1.3.0]: https://github.com/rrgmc/ai-specview/compare/v1.2.0...v1.3.0
[1.2.0]: https://github.com/rrgmc/ai-specview/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/rrgmc/ai-specview/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/rrgmc/ai-specview/compare/v0.9.0...v1.0.0
[0.9.0]: https://github.com/rrgmc/ai-specview/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/rrgmc/ai-specview/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/rrgmc/ai-specview/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/rrgmc/ai-specview/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/rrgmc/ai-specview/releases/tag/v0.5.0
