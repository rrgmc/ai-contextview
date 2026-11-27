### Changed

- The `ask` tool description, the hook's message when it blocks the
  built-in question tool, and the skills tell Claude to ask for approval of
  an action that needs your permission in plain chat, not in the viewer. An
  answer in the viewer reaches Claude as a tool result, which Claude Code's
  permission checks do not take as your approval.
