### Changed

- `/ai-contextview:spec-mode <path>` shows the spec at once when the file
  already exists, for example when Claude implements a spec written earlier.
  Before, it showed only after Claude wrote to the file. A glob still waits
  for a write.
- The ask-mode and spec-mode skills tell Claude to quote the document passage
  a question is about, such as a requirement of the spec being implemented,
  with an absolute path when the file is in another git worktree.
