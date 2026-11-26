---
name: ask-mode
description: Turn on ai-specview ask mode for the rest of the session, so every question Claude asks goes to the ai-specview viewer (a browser page) instead of the chat, with or without a spec. Use when the user runs /ask-mode or asks to route, send or ask all questions through ai-specview or the viewer from now on; "/ask-mode off" turns it off. Do NOT use for writing a spec (use spec-elaborate), or when the user wants the questions in the pane beside the chat (use ask-mode-mod).
argument-hint: "[off]"
---

# Ask mode with ai-specview

Ask mode sends every question you ask the user for the rest of this session
to the ai-specview viewer, a browser page beside the chat. The user answers
there, next to the code each question is about.

## The viewer's tools

Use the ai-specview MCP server's tools:
`mcp__plugin_ai-specview_ai-specview__ask_mode`,
`mcp__plugin_ai-specview_ai-specview__ask` and
`mcp__plugin_ai-specview_ai-specview__ask_wait`. If they are deferred, load
them first with ToolSearch. Never call the `mcp__ai-specview__…` tools in this
skill: they drive the pane. Below, `ask_mode`, `ask` and `ask_wait` mean the
viewer's tools.

## Turn it on or off

- With no argument, call `ask_mode` with no input. Then:
  - If the result has `open: "pane"` and the built-in browser tools
    (`mcp__Claude_Browser__*`) are available, open the result's `url` there
    with `preview_start` (pass `url`), unless the user asked for their own
    browser.
  - Tell the user in one line that ask mode is on, then wait for their next
    instruction. Do not start any other work.
- With the argument `off`, call `ask_mode` with `{"off": true}` and tell the
  user in one line that ask mode is off.

## While ask mode is on

- Ask **every** question with the ai-specview `ask` tool: clarifying
  questions, choices between approaches, confirmations. Never ask in plain
  chat text and never with another question tool. A hook blocks the built-in
  question tool while ask mode is on.
- Refs are optional. Add `code_refs` when the question is about code (for Go,
  prefer `symbol`; else `lines`). Add `doc_refs` when it is about a document:
  give its `path` (any text file of the project) and either a `quote` copied
  verbatim from a markdown file or `lines` (`N` or `N-M`), which also works
  for YAML, JSON and other files. Leave both out for a question about
  neither. `spec_quote` needs a spec shown with
  `spec_show`.
- Give 2 to 6 options with short labels and the trade-off in each
  `description`. Mark the option you recommend with `"recommended": true` and
  say why. Set `allow_other` when a free answer makes sense.
- Results:
  - `answered`: act on the answers.
  - `in_chat`: stop and wait for the user's message.
  - `pending`: call `ask_wait` with the returned `question_set_id`. If the
    result also has `url`, no viewer page is open: reopen `url` in the
    built-in browser first and tell the user in one line that questions are
    waiting there.
  - `dismissed`: continue without that decision, or ask in chat if it is
    essential.
  - `deferred` (with `answered`): the user chose **Decide later**. Continue
    without the decision and ask again before you finish the task.
- **The working document.** The first markdown document you write while ask
  mode is on (a spec, a plan, a design note) shows beside the questions as
  the working document, and `spec_quote` quotes it. A hook picks it; the
  first time, it tells you which file it picked. Bookkeeping files
  (`CLAUDE.md`, changelogs, memory, files in dot-folders) never count. If the
  wrong file shows, call `spec_show` with the right one: your own
  `spec_show` always wins. A question whose `doc_refs` quotes another file
  you wrote also switches to that file.
- If the user starts a spec, follow the spec-elaborate skill as usual; ask
  mode stays on after `spec_done`.
- Ask mode lasts until the user turns it off or the session ends.
