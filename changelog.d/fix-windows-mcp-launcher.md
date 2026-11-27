### Fixed

- On Windows, the plugin's MCP server starts even when `sh` is not on the
  `PATH`, as in the desktop app. Before, it failed with "Connection closed"
  and the `ask`, `ask_mode` and `spec_show` tools were missing.
