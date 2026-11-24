# ai-specview-pane

A Claude Code mod that shows the ai-specview spec, code and questions in a pane
beside the chat. It works with the `ai-specview` plugin and does not replace it: it hooks
that plugin's MCP tools by name, so the model and the `spec-elaborate` skill
stay the same.

## Modes

| Mode | What happens |
|---|---|
| `both` (default) | The MCP bridge serves every tool and the browser page updates as before. The pane then shows the same spec, quote and code, using the bridge's resolved code ranges. Questions are answered in the browser. |
| `pane` | The mod serves every tool itself and the daemon is not used. Questions are answered in the pane, and the built-in `AskUserQuestion` is blocked while a spec is open. |
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
- In `pane` mode, a question form: the options (the recommended one marked),
  "Decide later", an Other field when the question allows it, notes, steps
  between questions, and "Send answers", "Back to chat" (copies the questions
  to the clipboard) and "Skip". Each question frames its own quote and shows
  its own code. `ask` waits up to 10 minutes, then answers `pending`, and
  `ask_wait` waits again.
- After `spec_done`, a "Spec finished" banner with the summary.

## Limits

- In `pane` mode, quotes match exactly or with whitespace collapsed, and Go
  symbols are found with a regular expression. The bridge's fuzzy matching and
  `go/packages` lookup are more exact.
- The form has no review step before sending, and no per-question "keep open"
  choice for "Back to chat" and "Skip".
- `doc_refs` into other files are checked but not shown.
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
