### Changed

- The project is renamed from ai-specview to ai-contextview. The binary,
  the Go module (`github.com/rrgmc/ai-contextview`), the Claude Code plugin
  and marketplace, the MCP tool names, the `/contextview-pane` command and the
  socket and cache folders all use the new name. The environment variables
  are now `AI_CONTEXTVIEW_*`; the old `AI_SPECVIEW_*` names are no longer
  read.
- To upgrade, stop the old daemon (`ai-specview serve --stop`), uninstall the
  `ai-specview` plugin and marketplace, then install the binary and the
  plugin under the new name as the README describes. Rename any
  `AI_SPECVIEW_*` variables in your settings.
