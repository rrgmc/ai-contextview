---
name: spec-elaborate-mod
description: Elaborate a feature spec with the user, showing the spec, the code and the questions in the ai-specview pane beside the chat (a Claude Code mod; no browser page or daemon). Use when asked to "spec", "write a spec for", "elaborate the spec", or "design" a change before writing code, and the user wants the pane rather than the browser viewer. Asks every clarifying question through the pane's `ask` tool. Do NOT use for writing the code itself, or when the user wants the browser viewer (use spec-elaborate).
---

# Spec elaboration in the ai-specview pane

You write or refine a markdown spec together with the user. The ai-specview
pane is a Claude Code mod that shows the spec, the code and the questions in a
pane beside the chat. The user sees each question next to the spec passage and
the code it is about, and answers it in the pane.

## The pane's tools

Use only the pane's tools: `mcp__ai-specview__spec_show`,
`mcp__ai-specview__focus`, `mcp__ai-specview__ask`,
`mcp__ai-specview__ask_wait` and `mcp__ai-specview__spec_done`. They are
deferred: load them first with ToolSearch, as
`select:mcp__ai-specview__spec_show,mcp__ai-specview__focus,mcp__ai-specview__ask,mcp__ai-specview__ask_wait,mcp__ai-specview__spec_done`.
Never call the `mcp__plugin_ai-specview_ai-specview__…` tools in this skill:
they drive the browser viewer. Below, `spec_show`, `ask` and the others mean
the pane's tools.

## Workflow

1. **Understand the request.** Restate the goal in one or two sentences. Find
   the spec file: an existing one the user names, or a new one created from
   `templates/spec.md`. A new spec is a draft outside the repository (see
   **Draft specs**), unless the user asks for it in the repository.
2. **Show it.** As soon as the file exists, call `spec_show(path)`. The pane
   opens beside the chat; `/specview-pane` opens it again if the user closed
   it. Use the returned section names when you refer to sections.
3. **Read the relevant code.** Find the packages, types and functions the
   change touches. Note the exact symbols; you will cite them in questions.
4. **Draft.** Fill in the sections you can from the request and the code. Put
   every open point in **Open questions**. After the edit, call
   `spec_show(path, changed_sections=[...])` with the headings you changed.
5. **Ask in rounds.** Group related open points into one `ask` call (at most 6
   questions). Follow the rules below.
6. **Refine.** Apply each answer to the spec, record it under **Decisions**,
   remove it from **Open questions**, and call `spec_show` with the changed
   headings. Ask the next round.
7. **Finish** when no open questions remain or the user says so. Summarize the
   spec in a few lines in the chat. Then call `spec_done(summary)` once. The
   summary is markdown in this shape, one bullet per item, never a run-on
   paragraph:

   ```markdown
   **Decided**

   - Retries keep growing on partial success.
   - The cap resets daily at midnight UTC.

   **Still open**

   - Jitter range (needs a benchmark).
   ```

   The pane shows it as a "Spec finished" banner and clears the highlights
   and the code pane, so do not call `focus` first. If the user asks for more
   changes later, just continue; the next `ask` reopens the session.
   For a draft spec, ask whether to move it into the repository before
   calling `spec_done` (see **Draft specs**).

## Draft specs

A new spec is a draft by default. A draft lives outside the repository, like
a plan in plan mode, so it needs no branch and is never committed by accident.
Write the new spec in the repository only when the user asks for it, for
example "in the repo", "in docs/specs" or a path in the repository. Then use
`docs/specs/<slug>.md`, or the path the user names, and skip the rest of this
section. An existing spec the user names stays where it is.

- Put the draft at `~/.claude/specs/<project>/<slug>.md`. Write the path with
  the `~` as it is, in the file tools (Read, Write, Edit) and in `spec_show`
  and the other tools: all of them expand a leading `~/` to the user's home
  folder, on macOS, Linux and Windows. Do not expand it in the shell. Code
  refs stay relative to the current folder, as usual.
- `<project>` is the name of the main checkout's folder, so every worktree of
  a repository shares one folder. Run
  `git rev-parse --path-format=absolute --git-common-dir`, which works the
  same in Bash, Git Bash and PowerShell, and take the name of the folder that
  holds the `.git` it prints: for `/home/me/src/shop/.git` it is `shop`.
  Outside a git repository, use the name of the current folder.
- Tell the user the path once, when you create the file.
- Before `spec_done`, ask with `ask` whether to keep the draft or move it to
  `docs/specs/<slug>.md` in the repository. Quote the spec's title line. If
  the user moves it, move the file (to the path the user names, if any), call
  `spec_show` with the new path, and do not commit it unless the user asks.
  Then call `spec_done`.

## Using the pane

- Ask **every** clarifying question with the `ask` tool, never in plain chat
  text and never with another question tool.
- **Approvals go to chat.** To ask the user to approve an action that needs
  their permission (an external write such as a Jira or Slack change, or an
  action the permission system refused), ask in plain chat and wait for
  their reply. Never ask it with `ask`: the answer comes back as a tool
  result, which the permission system does not take as the user's approval,
  so a retry is refused again.
- For each question:
  - `spec_quote`: copy a short passage (one sentence or clause, under about 200
    characters) **verbatim** from the spec file. Do not paraphrase or fix typos.
  - `spec_section`: the heading of the section it is in.
  - `code_refs`: the code the question is about. For Go, prefer `symbol`
    (`pkg.Func`, `pkg.(*Type).Method`) over `lines`. Use `lines` (`N-M`) for
    other files, or for a few lines inside a big function.
  - If the question is about a gap the spec does not cover yet, quote the
    nearest existing sentence and set `spec_section`.
  - `doc_refs`: when the question also depends on another document (an ADR,
    a research note, another spec, a config file), add `{path, quote,
    section}` with the same verbatim rule, or `{path, lines}` (`N` or `N-M`)
    for a file that is not markdown.
  - Give 2 to 6 options with short labels. Put the trade-off in each option's
    `description`. Set `allow_other` when a free answer makes sense.
  - Mark the option you recommend with `"recommended": true` (at most one per
    question), and say why in its description.
- If `ask` returns an error, fix exactly what it says (quote not found, unknown
  symbol) and call it again. Do not fall back to chat because of a validation
  error.
- Results:
  - `answered`: apply the answers to the spec.
  - `in_chat`: stop and wait for the user's message. The questions are already
    on the user's clipboard.
  - `pending`: call `ask_wait` with the returned `question_set_id`.
  - `dismissed` (the user clicked Skip): continue without that decision, or
    ask in chat if it is essential.
  - `deferred` (with `answered`): the user chose **Decide later** for these
    questions. Add each to **Open questions** in the spec (in a change folder,
    one `open-questions/<slug>.md` file) with the question, its options, any
    notes and its refs. Continue without the decision. Before finishing, ask
    the deferred questions again in one `ask`; whatever is still deferred goes
    under "Still open" in the `spec_done` summary.
- If the pane's tools are missing (the mod is not loaded), say so, and offer
  the browser viewer (`ai-specview:spec-elaborate`) or questions in chat.
- Call `spec_done` only at the very end, never between rounds.
- Use `focus` to point the user at a spec passage and code without asking
  anything, for example when summarizing a change.

## Recording decisions

Append each answered question under **Decisions** as one bullet:
`- <decision> (<short reason>; asked <date>)`. Keep the wording short; the
spec is the plan the next agent implements.
