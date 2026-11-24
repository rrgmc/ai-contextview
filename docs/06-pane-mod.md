# 06 — Pane mod

`plugin-pane/` is a second plugin, `ai-specview-pane`: a Claude Code mod that
draws the spec, the code and the questions in a pane beside the chat. It adds a viewer; it does
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

| Mode | `spec_show`, `focus` | `ask`, `ask_wait` | `spec_done` | `AskUserQuestion` |
|---|---|---|---|---|
| `browser` | Bridge | Bridge | Bridge | `ai-specview hook` |
| `both` (default) | Bridge, then the pane mirrors the call | Bridge | Bridge, then the pane shows the banner | `ai-specview hook` |
| `pane` | The mod | The mod | The mod | Blocked by the mod while a spec is open |

- **`both`.** The hook calls `next(e)`, so the bridge runs first. On success it
  reads the spec file and, for `focus`, the bridge's resolved code ranges
  (`code[].start_line`, `end_line` in the result JSON). A failure in the mirror
  shows in the status line and never fails the tool call.
- **`pane`.** The hook answers without `next(e)`. Its result is the JSON text
  the bridge would send (an MCP tool's result must be text, not an object). A
  quote not found or a bad code ref fails the call with the bridge's
  "fix these and call again" list.
- **No daemon in `pane` mode.** The bridge gets no call at all, so the daemon
  is never started. `ai-specview hook` asks the daemon whether a spec is open
  and allows `AskUserQuestion` when no daemon answers, so the mod blocks it
  itself, with the hook's message, until `spec_done`.
- **Interrupted calls.** If a hook fails or the person interrupts a call the
  pane serves, the mod's `.catch` refuses the call rather than passing it to
  the bridge, which knows no spec in that mode.

The mode comes from the plugin's `mode` option. `/specview-mode` changes it for
the session; the value lives in session state (`$.state`) and survives a reload
of the mod.

## 3. Questions in `pane` mode

- **Input.** `ask` checks its input with the rules of `internal/validate`
  (1-6 questions, 2-6 options, at most one recommended, refs required, quote
  length) and turns a "(Recommended)" label suffix into the flag. Each
  question's quote and code refs are resolved as `focus` resolves them; any
  problem fails the call with the bridge's "fix these and call again" list.
- **Waiting.** The question set is stored in session state and the pane shows
  it. The `tool.call` hook then waits for the person's choice, checking once a
  second, for up to 10 minutes (the bridge's `AI_SPECVIEW_ASK_WAIT` default).
  Past that it answers `pending` with the question set id, and `ask_wait` waits
  again. A hook's own time is capped at 10 seconds and a `$.clock` wait counts
  toward it, while time inside any other `$` call does not (up to 20 minutes on
  `tool.call`). So the hook pauses between checks with `/bin/sleep 1` through
  `$.process.run`. A hidden tool of the mod's own does not lift the cap: its
  `tool.run` hook is held to the same 10 seconds.
- **Answers.** The form's buttons store the outcome in session state and in a
  map the waiting hook reads. `ask_wait` reads session state too, so an answer
  given while the mod reloaded is not lost. The result has the bridge's shape
  and messages: `answered` with `answers` (and `deferred` for "Decide later"),
  `in_chat` for "Back to chat", which also copies the questions to the
  clipboard, and `dismissed` for "Skip". "Send answers" names the questions
  still unanswered and goes to the first.
- **`spec_done`.** Checks the summary's length (8,000 bytes), clears the focus
  and shows a "Spec finished" banner with the summary. The next `spec_show` or
  `ask` reopens the spec.

## 4. Matching without the bridge

`hooks/spec.ts` is the subset of `internal/specsrc` and `internal/resolve` that
`pane` mode needs:

- Headings, with inline code marks dropped as the bridge drops them.
- Quotes: exact, then with runs of whitespace collapsed. No fuzzy stage and no
  heading fallback.
- Code refs: `N` or `N-M` line ranges, and Go symbols found by a regular
  expression over the file (`func`, method receiver, `type`, `var`, `const`),
  doc comment included. A symbol in another file of the package is not found.

## 5. The pane

| Part | What it does |
|---|---|
| Header | Spec path; the mode, where the last update came from and the quote match |
| Section picker | `Select` over "Whole spec" and every heading, indented by level; changed sections are marked |
| Changed buttons | One per entry of `changed_sections`; shows that section |
| Back to the focus | Shown when another section is picked |
| Spec column | The section shown, as markdown. The block that holds the quote is drawn in a yellow frame with the quote in bold |
| Side column | In `pane` mode, the question form; then one `Button` tab per code ref and the `Code` of the active one, with line numbers |
| Banner | After `spec_done`, "Spec finished" with the summary |

- **Columns.** Side by side (55% and 45%) when the pane is docked and at least
  100 cells wide. Otherwise the question form comes first, then the spec, then
  the code.
- **The current question.** While questions wait, the spec and code columns
  show the current question's quote and code, and moving between questions
  moves the section shown.
- **Focus block.** `splitAround` cuts the section into before, focus and after
  at blank lines outside code fences, so each part is whole markdown. After a
  `focus` the pane scrolls the frame into view.
- **State.** `view` holds the last spec and focus; `ui` holds the section shown
  and the code tab; `ask` holds the question set, the drafts and the outcome. Both are session state with a shape tag, so a reload keeps
  them and a changed shape discards the old value.

## 6. Tests

`claude plugin test ./plugin-pane` runs `tests/pane.test.tsx` against Claude
Code's test kit. It covers the three modes, the error paths, the column layout at
two widths, the section picker, the code tabs, the block splitter, the symbol
lookup, and in `pane` mode answering, "Decide later", "Back to chat", "Skip",
the `pending` timeout and `ask_wait`, input checks, the `AskUserQuestion` block
and the `spec_done` banner. The tests use `mock.clock`, which holds the wait
until the test moves the clock. The test kit does not check an MCP tool's
result shape; the live run did.
