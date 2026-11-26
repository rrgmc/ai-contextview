### Added

- A question can point at any document of the project, not only the spec:
  a markdown passage by quote or by lines, or another text file (YAML, JSON,
  SQL) by lines, shown as source with the lines highlighted. When a question
  has several documents, tabs switch between them.
- `AI_SPECVIEW_ASK_MODE=always` turns ask mode on for every session.

### Changed

- When no question of a set has a document and no spec is shown, the
  questions and the code take the whole page. The layout changes only when a
  new question set arrives.
- The README and the plugin description present ai-specview as a front end
  for Claude's questions, with spec elaboration built on it.
