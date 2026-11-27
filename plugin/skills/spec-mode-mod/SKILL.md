---
name: spec-mode-mod
description: Turn on ai-contextview spec mode for the rest of the session - ask mode that expects a spec. Every question goes to the ai-contextview pane beside the chat, and the spec shows there as the working document as soon as it is written, whichever skill writes it (a company's spec skill, spec-elaborate, or none). Use when the user runs /spec-mode-mod, optionally with the spec's path or a glob, or asks to show the spec they are about to write in ai-contextview; "/spec-mode-mod off" turns it off and hands the questions back to the chat, even with a spec open. Do NOT use to write the spec itself (use the user's spec skill, or spec-elaborate-mod), or when the user wants the browser viewer (use spec-mode).
argument-hint: "[path or glob | off]"
---

# Spec mode in the ai-contextview pane

Spec mode is ask mode that expects a spec. It sends every question you ask
the user for the rest of this session to the ai-contextview pane beside the chat, and shows the
spec there as the working document as soon as it is written. The user
answers next to the spec and the code each question is about.

## The pane's tools

Use only the pane's tools: `mcp__ai-contextview__ask_mode`,
`mcp__ai-contextview__ask` and `mcp__ai-contextview__ask_wait`. They are deferred:
load them first with ToolSearch, as
`select:mcp__ai-contextview__ask_mode,mcp__ai-contextview__ask,mcp__ai-contextview__ask_wait`.
Never call the `mcp__plugin_ai-contextview_ai-contextview__…` tools in this skill:
they drive the browser viewer. Below, `ask_mode`, `ask` and `ask_wait` mean
the pane's tools.

## Turn it on or off

- With no argument, or with a path or glob, call `ask_mode` with
  `{"spec": true}`, adding `"paths": ["<the argument>"]` when there is one
  (relative to the project directory, or absolute; `**` matches any folders,
  for example `docs/specs/**`). The first file you write that matches becomes
  the working document. A path, not a glob, to a spec that already exists
  shows it at once, for example when you implement a written spec: the
  result then has `shown`, and `spec_quote` quotes that file. Without
  paths, ai-contextview picks the written file
  that looks most like a spec. The pane opens. Tell the user in one line that
  spec mode is on, then wait for their next instruction. Do not start any
  other work.
- With the argument `off`, call `ask_mode` with `{"off": true}` and tell the
  user in one line that spec mode is off and questions come back to the chat.
  This also releases a spec that is open, for example one from
  spec-elaborate. From then on, ask every question in the chat or with the
  built-in question tool, never with `ask`, as the result's `next` says.

## While spec mode is on

- Ask **every** question with `ask`: clarifying questions, choices between
  approaches, confirmations. Never ask in plain chat text and never with
  another question tool. The mod blocks the built-in question tool while spec
  mode is on.
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
- **A question about a document quotes it.** When the question names or
  depends on a passage of a document, such as a requirement, an acceptance
  criterion or a section of the spec you are implementing, put that passage
  in `doc_refs`, even when another session wrote the document. The user
  reads the question beside it. Give an absolute `path` when the file is
  outside the session's folder, for example in another git worktree.
- Give 2 to 6 options with short labels and the trade-off in each
  `description`. Mark the option you recommend with `"recommended": true` and
  say why.
- Results:
  - `answered`: act on the answers.
  - `in_chat`: stop and wait for the user's message.
  - `pending`: call `ask_wait` with the returned `question_set_id`.
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
- Write the spec with the user's spec skill, or with spec-elaborate-mod if they
  ask for it. Spec mode adds no workflow of its own: follow that skill's
  steps, but ask every question with `ask`, quoting the spec with
  `spec_quote` once it shows.
- When the spec is finished, call `spec_done` with a summary (a
  **Decided** and a **Still open** bullet list), unless the user's skill
  says otherwise. Spec mode stays on until the user turns it off or the
  session ends.
