### Added

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

- `ai-specview hook` also runs after `Write`, `Edit` and `MultiEdit` and
  before `Skill`. The plugin sets this up; a manual setup adds the two hook
  entries shown in the README.
