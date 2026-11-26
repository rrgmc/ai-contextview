---
name: ask-mode-mod
description: Turn on ai-specview ask mode in the pane beside the chat (a Claude Code mod; no browser page or daemon) for the rest of the session, so every question Claude asks goes to the pane instead of the chat, with or without a spec. Use when the user runs /ask-mode-mod or asks to route all questions through the ai-specview pane from now on; "/ask-mode-mod off" turns it off. Do NOT use for writing a spec (use spec-elaborate-mod), or when the user wants the browser viewer (use ask-mode).
argument-hint: "[off]"
---

# Ask mode in the ai-specview pane

Ask mode sends every question you ask the user for the rest of this session
to the ai-specview pane beside the chat. The user answers there, next to the
code each question is about.

## The pane's tools

Use only the pane's tools: `mcp__ai-specview__ask_mode`,
`mcp__ai-specview__ask` and `mcp__ai-specview__ask_wait`. They are deferred:
load them first with ToolSearch, as
`select:mcp__ai-specview__ask_mode,mcp__ai-specview__ask,mcp__ai-specview__ask_wait`.
Never call the `mcp__plugin_ai-specview_ai-specview__…` tools in this skill:
they drive the browser viewer. Below, `ask_mode`, `ask` and `ask_wait` mean
the pane's tools.

## Turn it on or off

- With no argument, call `ask_mode` with no input. The pane opens. Tell the
  user in one line that ask mode is on, then wait for their next instruction.
  Do not start any other work.
- With the argument `off`, call `ask_mode` with `{"off": true}` and tell the
  user in one line that ask mode is off.

## While ask mode is on

- Ask **every** question with `ask`: clarifying questions, choices between
  approaches, confirmations. Never ask in plain chat text and never with
  another question tool. The mod blocks the built-in question tool while ask
  mode is on.
- Refs are optional. Add `code_refs` when the question is about code (for Go,
  prefer `symbol`; else `lines`), and `doc_refs` when it is about a markdown
  document (give its `path` and copy the quote verbatim). Leave both out for
  a question about neither. `spec_quote` needs a spec shown with
  `spec_show`.
- Give 2 to 6 options with short labels and the trade-off in each
  `description`. Mark the option you recommend with `"recommended": true` and
  say why. Set `allow_other` when a free answer makes sense.
- Results:
  - `answered`: act on the answers.
  - `in_chat`: stop and wait for the user's message.
  - `pending`: call `ask_wait` with the returned `question_set_id`.
  - `dismissed`: continue without that decision, or ask in chat if it is
    essential.
  - `deferred` (with `answered`): the user chose **Decide later**. Continue
    without the decision and ask again before you finish the task.
- If the user starts a spec, follow the spec-elaborate-mod skill as usual;
  ask mode stays on after `spec_done`.
- Ask mode lasts until the user turns it off or the session ends.
