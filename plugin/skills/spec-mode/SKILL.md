---
name: spec-mode
description: Turn on ai-contextview spec mode for the rest of the session - ask mode that expects a spec. Every question goes to the ai-contextview viewer (a browser page), and the spec shows there as the working document as soon as it is written, whichever skill writes it (a company's spec skill, spec-elaborate, or none). Use when the user runs /spec-mode, optionally with the spec's path or a glob, or asks to show the spec they are about to write in ai-contextview; "/spec-mode off" turns it off. Do NOT use to write the spec itself (use the user's spec skill, or spec-elaborate), or when the user wants the pane beside the chat (use spec-mode-mod).
argument-hint: "[path or glob | off]"
---

# Spec mode with ai-contextview

Spec mode is ask mode that expects a spec. It sends every question you ask
the user for the rest of this session to the ai-contextview viewer, a browser
page beside the chat, and shows the spec there as the working document as
soon as it is written. The user answers next to the spec and the code each
question is about.

## The viewer's tools

Use the ai-contextview MCP server's tools:
`mcp__plugin_ai-contextview_ai-contextview__ask_mode`,
`mcp__plugin_ai-contextview_ai-contextview__ask` and
`mcp__plugin_ai-contextview_ai-contextview__ask_wait`. If they are deferred, load
them first with ToolSearch. Never call the `mcp__ai-contextview__…` tools in this
skill: they drive the pane. Below, `ask_mode`, `ask` and `ask_wait` mean the
viewer's tools.

## Turn it on or off

- With no argument, or with a path or glob, call `ask_mode` with
  `{"spec": true}`, adding `"paths": ["<the argument>"]` when there is one
  (relative to the project directory, or absolute; `**` matches any folders,
  for example `docs/specs/**`). The first file you write that matches becomes
  the working document. Without paths, ai-contextview picks the written file
  that looks most like a spec: its folder, its headings, how often you edit
  it, and whether a spec skill is running.
  - If the result has `open: "pane"` and the built-in browser tools
    (`mcp__Claude_Browser__*`) are available, open the result's `url` there
    with `preview_start` (pass `url`), unless the user asked for their own
    browser.
  - Tell the user in one line that spec mode is on, then wait for their next
    instruction. Do not start any other work.
- With the argument `off`, call `ask_mode` with `{"off": true}` and tell the
  user in one line that spec mode is off.

## While spec mode is on

- Ask **every** question with the ai-contextview `ask` tool: clarifying
  questions, choices between approaches, confirmations. Never ask in plain
  chat text and never with another question tool. A hook blocks the built-in
  question tool while spec mode is on.
- **Approvals go to chat.** To ask the user to approve an action that needs
  their permission (an external write such as a Jira or Slack change, or an
  action the permission system refused), ask in plain chat and wait for
  their reply. Never ask it with `ask`: the answer comes back as a tool
  result, which the permission system does not take as the user's approval,
  so a retry is refused again.
- Refs are optional. Add `code_refs` when the question is about code (for Go,
  prefer `symbol`; else `lines`). Add `doc_refs` when it is about a document:
  give its `path` (any text file of the project) and either a `quote` copied
  verbatim from a markdown file or `lines` (`N` or `N-M`), which also works
  for YAML, JSON and other files. Leave both out for a question about
  neither. A question about the whole document, such as a sign-off or a
  summary, takes no ref at all, not even `spec_quote`: a ref highlights
  one passage, and the user reads it as what the question is about.
  `spec_quote` quotes the working document once it shows.
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
- **The working document.** The first markdown document you write while spec
  mode is on (a spec, a plan, a design note) shows beside the questions as
  the working document, and `spec_quote` quotes it. A hook picks it; the
  first time, it tells you which file it picked. Bookkeeping files
  (`CLAUDE.md`, changelogs, memory, files in dot-folders) never count. If the
  wrong file shows, call `spec_show` with the right one: your own
  `spec_show` always wins. A question whose `doc_refs` quotes another file
  you wrote also switches to that file.
- **Before the spec exists.** Many spec skills explore the code and keep
  notes before they write the spec: exploration notes, open questions, a
  list of requirements. While no working document shows, and you keep such
  notes in a markdown file, call `spec_show` with that file and
  `"provisional": true`. The user then reads the notes beside the questions,
  and `spec_quote` quotes them. A provisional document does not stop the
  pick: the first spec you write replaces it, with no call from you. Do not
  call `spec_show` without `provisional` for the notes, because your own
  `spec_show` stops the pick, and the spec would not replace them.
- Write the spec with the user's spec skill, or with spec-elaborate if they
  ask for it. Spec mode adds no workflow of its own: follow that skill's
  steps, but ask every question with `ask`, quoting the spec with
  `spec_quote` once it shows.
- When the spec is finished, call `spec_done` with a summary (a
  **Decided** and a **Still open** bullet list), unless the user's skill
  says otherwise. Spec mode stays on until the user turns it off or the
  session ends.
