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
