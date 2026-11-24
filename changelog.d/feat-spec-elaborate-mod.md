### Added

- The `ai-specview:spec-elaborate-mod` skill: the spec workflow with the spec,
  the code and the questions in a pane beside the chat instead of the browser
  page. It needs neither the binary nor the daemon.
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
