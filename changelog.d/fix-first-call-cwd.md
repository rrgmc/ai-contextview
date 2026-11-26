### Fixed

- In a desktop-app worktree session, relative paths in the first ai-specview
  call (for example `/spec-mode docs/specs/**`) resolve against the worktree,
  not the main checkout. Before, the spec-mode glob matched nothing until
  ask mode was turned on a second time.
