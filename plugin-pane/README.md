# ai-specview-pane

A Claude Code mod that shows the ai-specview spec and code in a pane beside the
chat. It works with the `ai-specview` plugin and does not replace it: it hooks
that plugin's MCP tools by name, so the model and the `spec-elaborate` skill
stay the same.

## Modes

| Mode | What happens |
|---|---|
| `both` (default) | The MCP bridge serves every tool and the browser page updates as before. The pane then shows the same spec, quote and code, using the bridge's resolved code ranges. |
| `pane` | The mod answers `spec_show` and `focus` itself, so the daemon gets no request for them. `ask`, `ask_wait` and `spec_done` still go to the bridge and the browser page. Before the first of them, the mod sends the bridge the last `spec_show` once. |
| `browser` | The mod does nothing. |

Set the default with the plugin's `mode` option. `/specview-mode [both|pane|browser]`
shows or changes the mode for the current session, and `/specview-pane` opens
the pane again.

## The pane

- The spec and the code side by side when the pane is docked and at least 100
  cells wide, one above the other otherwise.
- A section picker with every heading and "Whole spec". Changed sections are
  marked and have buttons of their own.
- The paragraph that holds the focused quote is framed in yellow, with the
  quote in bold, and the pane scrolls to it.
- One tab per code ref.

## Limits

- In `pane` mode, quotes match exactly or with whitespace collapsed, and Go
  symbols are found with a regular expression. The bridge's fuzzy matching and
  `go/packages` lookup are more exact.
- Questions (`ask`) are not drawn in the pane yet.
- For a folder spec, only the primary file is shown.
- A mod cannot colour a span inside markdown; the frame stands in for the
  browser's highlight.

## Develop

```bash
claude plugin validate ./plugin-pane
```

```bash
claude plugin test ./plugin-pane
```

To try a change, run a session with `claude --plugin-dir ./plugin-pane`; edits
reload when a turn ends. Claude Code writes the API types to
`.claude-plugin/types/` the first time it loads the mod; `tsc -p plugin-pane`
type-checks it after that.
