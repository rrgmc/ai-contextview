### Changed

- `/ai-contextview:spec-mode <path>` shows the spec at once when the file
  already exists, for example when Claude implements a spec written earlier.
  Before, it showed only after Claude wrote to the file. A glob still waits
  for a write. A relative path that is not in the session's folder is found
  in the repository's other git worktrees, the most recently changed copy
  first, so `/ai-contextview:spec-mode docs/specs/SW-1.md` works from any
  worktree.
- The ask-mode and spec-mode skills tell Claude to quote the document passage
  a question is about, such as a requirement of the spec being implemented,
  with an absolute path when the file is in another git worktree.
