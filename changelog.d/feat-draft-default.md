### Changed

- Both skills now write a new spec as a draft in
  `~/.claude/specs/<project>/`, outside the repository, so speccing needs no
  git branch. Before finishing, they ask whether to move it into
  `docs/specs/`. Ask for the repository ("in the repo", "in docs/specs") to
  write it there from the start.

### Added

- `spec_show`, `focus` and `ask` accept paths that start with `~/`, as
  Claude Code's file tools do. On Windows they also accept `~\` and Git Bash
  paths such as `/c/Users/me`. The pane's tools accept `~/` too.
