### Added

- In `pane` mode the `ai-specview-pane` mod asks the questions in the pane:
  options, "Decide later", Other, notes, and "Send answers", "Back to chat" and
  "Skip". It also serves `spec_done`, so `pane` mode no longer starts the
  daemon.
- In `pane` mode the mod blocks the built-in `AskUserQuestion` while a spec is
  open.
- The pane shows a "Spec finished" banner with the summary after `spec_done`.
