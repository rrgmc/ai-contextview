### Changed

- `/ai-contextview:ask-mode off` and `/ai-contextview:spec-mode off` (and
  their `-mod` versions) now hand the questions back to the chat for the rest
  of the session, even while a spec from `spec-elaborate` is open. The spec
  stays in the viewer. Before, an open spec kept blocking the built-in
  question tool until it was finished. Turning ask mode on again, or showing
  another spec, ends this.
