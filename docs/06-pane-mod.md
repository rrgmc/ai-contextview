# 06 — Pane mod

`plugin-pane/` is a second plugin, `ai-specview-pane`: a Claude Code mod that
draws the spec and the code in a pane beside the chat. It adds a viewer; it does
not replace the browser page, the bridge or the daemon.

## 1. Why a mod

A mod is a plugin with a TypeScript hooks module that Claude Code runs in its
own process. It can hook tool calls, keep session state and draw panes with
Claude Code's own elements (`Box`, `Text`, `Markdown`, `Code`, `Select`,
`Button`). The pane sits in the Claude window, so the person does not switch to
a browser tab.

What a mod cannot do, and the browser page can:

- Colour a span inside rendered markdown. `Markdown` and `Code` take whole
  text, with no ranges.
- Reuse the page's HTML, CSS and JavaScript.
- Draw more than about 100,000 characters at once.
- Show the workspaces of several Claude sessions in one place.

## 2. How it hooks ai-specview

The mod registers no tools. Its `tool.call` hook matches the ai-specview MCP
tools by name (`mcp__…ai-specview__spec_show`, `focus`, `ask`, `ask_wait`,
`spec_done`), so the model, the tool descriptions and the `spec-elaborate`
skill are the same in every mode.

| Mode | `spec_show`, `focus` | `ask`, `ask_wait`, `spec_done` |
|---|---|---|
| `browser` | Bridge | Bridge |
| `both` (default) | Bridge, then the pane mirrors the call | Bridge |
| `pane` | The mod answers; the bridge gets nothing | Bridge, after one replay of the last `spec_show` |

- **`both`.** The hook calls `next(e)`, so the bridge runs first. On success it
  reads the spec file and, for `focus`, the bridge's resolved code ranges
  (`code[].start_line`, `end_line` in the result JSON). A failure in the mirror
  shows in the status line and never fails the tool call.
- **`pane`.** The hook answers without `next(e)`. Its result is the JSON text
  the bridge would send (an MCP tool's result must be text, not an object). A
  quote not found or a bad code ref fails the call with the bridge's
  "fix these and call again" list.
- **Replay before `ask`.** The bridge needs the spec to resolve a question's
  refs. In `pane` mode it never saw `spec_show`, so before the first `ask`,
  `ask_wait` or `spec_done` after a `spec_show` the mod sends it that
  `spec_show` once, with its own hook passing the call through.

The mode comes from the plugin's `mode` option. `/specview-mode` changes it for
the session; the value lives in session state (`$.state`) and survives a reload
of the mod.

## 3. Matching without the bridge

`hooks/spec.ts` is the subset of `internal/specsrc` and `internal/resolve` that
`pane` mode needs:

- Headings, with inline code marks dropped as the bridge drops them.
- Quotes: exact, then with runs of whitespace collapsed. No fuzzy stage and no
  heading fallback.
- Code refs: `N` or `N-M` line ranges, and Go symbols found by a regular
  expression over the file (`func`, method receiver, `type`, `var`, `const`),
  doc comment included. A symbol in another file of the package is not found.

## 4. The pane

| Part | What it does |
|---|---|
| Header | Spec path; the mode, where the last update came from and the quote match |
| Section picker | `Select` over "Whole spec" and every heading, indented by level; changed sections are marked |
| Changed buttons | One per entry of `changed_sections`; shows that section |
| Back to the focus | Shown when another section is picked |
| Spec column | The section shown, as markdown. The block that holds the quote is drawn in a yellow frame with the quote in bold |
| Code column | One `Button` tab per code ref and the `Code` of the active one, with line numbers |

- **Columns.** Side by side (55% and 45%) when the pane is docked and at least
  100 cells wide; otherwise the code is under the spec.
- **Focus block.** `splitAround` cuts the section into before, focus and after
  at blank lines outside code fences, so each part is whole markdown. After a
  `focus` the pane scrolls the frame into view.
- **State.** `view` holds the last spec and focus; `ui` holds the section shown
  and the code tab. Both are session state with a shape tag, so a reload keeps
  them and a changed shape discards the old value.

## 5. Tests

`claude plugin test ./plugin-pane` runs `tests/pane.test.tsx` against Claude
Code's test kit. It covers the three modes, the replay before `ask`, the error
path, the column layout at two widths, the section picker, the code tabs, the
block splitter and the symbol lookup. The test kit does not check an MCP tool's
result shape; the live run did.
