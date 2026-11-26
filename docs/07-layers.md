# 07 — Layers: questions first, specs on top

Status: phase 1 is built (see "Phases"); phases 2 and 3 are proposed. Ask
mode itself is described in 04 and 06.

## Why

ai-specview started as a spec viewer: the spec on the left, the questions and
the code on the right. In use, its main value is broader. It is a front end for
Claude's questions that shows each question next to its context: the code, a
document, or both. A spec is one source of context among several.

This document reorganizes the plugin around that use. It splits ai-specview
into three layers, says how the user selects them, and changes the viewer
layout so that any document, not only a spec, can be the main context.

## The three layers

Each layer includes the layers below it.

| Layer | What it is | Parts |
|---|---|---|
| 1. Questions with context | Every question goes to the viewer, with the code and documents it is about | `ask`, `ask_wait`, refs, the document panel, ask mode |
| 2. Working document | A document that Claude writes with the user during the session | Showing the document (today `spec_show`), change marking, the Changes tab, `focus`, `done` |
| 3. Spec workflow | Writing a spec | The `spec-elaborate` skill: draft location, templates, recording decisions, open questions |

Layer 2 is not specific to specs. The working document can be a plan, a design
note, a README or an RFC. Only layer 3 is about specs.

A spec session is therefore layer 1, plus a working document (the spec), plus
the spec workflow. A session that only asks questions uses layer 1 alone.

## Selecting a layer

The user picks the highest layer they need. The lower layers come with it.

| The user wants | How they select it | What is on |
|---|---|---|
| Questions with context in any session | `/ai-specview:ask-mode`, or the `AI_SPECVIEW_ASK_MODE=always` setting | Layer 1, until turned off |
| Claude writes a document with them | Ask mode finds it (see "Finding the working document"), or ask in words ("write the plan in the viewer"), or `/ai-specview:doc <path>` | Layers 1 and 2, until the document is done |
| A spec, written by any skill (their own, a company's, or none) | `/ai-specview:spec-mode [path or glob]` | Layers 1 and 2, with a stronger search for the spec, until turned off |
| A spec written with ai-specview's own workflow | `/ai-specview:spec-elaborate`, or ask in words ("spec this") | Layers 1, 2 and 3, until `done` |

Rules:

- **Ask mode is off by default.** It starts only when the user turns it on.
  `AI_SPECVIEW_ASK_MODE=always`, in the `env` block of the Claude Code
  settings, turns it on for every session.
- **Layer 2 from inside ask mode.** A hook finds the document Claude writes
  and shows it as the working document (see "Finding the working document").
  The user rarely needs `/ai-specview:doc`.
- **Spec mode** is ask mode that expects a spec. It suits a spec skill that
  is not ai-specview's own, such as a company's: that skill keeps its
  workflow, and ai-specview adds the questions and the working document.
- **Ending.** `done` ends layers 2 and 3. Ask mode stays on if the user turned
  it on. Otherwise question routing ends with the document, as it ends with
  the spec today.
- **Browser or pane** is a separate choice, made with one setting,
  `AI_SPECVIEW_VIEW=browser|pane` (default `browser`). There is one skill per
  layer, and each skill uses the tools of the chosen viewer. This replaces the
  `-mod` copies of the skills (`spec-elaborate-mod`, `ask-mode-mod`).

## The document panel

The left panel becomes a **document panel**. It shows the main document of the
current question, with the passage highlighted.

- **Which document.** The first `doc_refs` entry of the current question. In a
  session with a working document, a question with no doc ref shows the
  working document, as the spec is shown today.
- **More than one document.** The question's other documents open as tabs in
  the document panel.
- **Highlighting.** A quoted passage is highlighted as today. A doc ref may
  give `lines` (`N` or `N-M`) instead of a quote; the range is highlighted the
  same way as in code. This suits configuration files and long documents.
- **Which files are documents.** Any markdown file in the project, not only
  the files of the spec folder. Markdown is rendered. Other text files
  (YAML, JSON, SQL and so on) are shown as source with line numbers.
- **Code stays on the right.** A question with a document and code shows the
  document on the left and the code on the right, under the question.

### Layout per question set

The panel must not open and close between the questions of one set: the screen
would jump. The layout is therefore chosen once per question set.

- If any question in the set has a document, or a working document is shown,
  the document panel is shown for the whole set.
- If no question in the set has a document and there is no working document,
  the set uses the full width: the questions and the code take the whole page.
- A question with no document, in a set that shows the panel, keeps the last
  document shown, without a highlight. A short line says that the question has
  no document.
- The layout changes only when a new question set arrives. If this still feels
  jumpy in use, a set without documents can keep the panel of the previous set
  instead.

## Finding the working document

Claude does not tell ai-specview which document it is writing unless a skill
says so (`spec_show`). Another skill, such as a company's spec skill, has its
own instructions and will not. In one real session, a company skill's spec
still showed as the working document, but only because the `spec-elaborate`
instructions from earlier in the same session were still in Claude's context.
A fresh session would not have shown it. Phase 2 makes this reliable with a
hook, independent of the skill.

### The write hook

A `PostToolUse` hook on `Write`, `Edit` and `MultiEdit` reports each file
Claude writes to the daemon. The hook finds its session as `ai-specview hook`
does today, through the Claude Code process (04), and sends a new socket
message, for example `doc_written {pids, path}`. The pane mod sees the same
tool calls in its `tool.call` hook and applies the same rules without a
daemon. The hook runs only in ask mode or spec mode.

An explicit `spec_show` always wins over everything below.

### Which files count

Markdown files (`.md`, `.markdown`) count, except bookkeeping:

- `CLAUDE.md` and `AGENTS.md` files, `CHANGELOG.md` and `changelog.d/`
- Claude's memory (`~/.claude/projects/…`)
- files in dot-folders (`.agent/`, `.github/`, `.claude/`), except
  `.claude/specs/`

Outside the project, drafts in `~/.claude/specs/` and plans in
`~/.claude/plans/` count.

Nothing names a particular layout such as `docs/specs/`: every team and skill
keeps documents in other places. The optional setting
`AI_SPECVIEW_DOC_PATHS` (glob patterns, for example
`docs/specs/**,docs/rfc/**`) replaces these rules: when it is set, only
matching files count.

### Ask mode: the first file, then questions

1. The first markdown file Claude writes that counts becomes the working
   document. In a spec flow that is almost always the spec, since the skill
   writes it before anything else.
2. Later writes to other files do not switch it. They are noted as
   candidates.
3. A question whose `doc_refs` names a candidate other than the working
   document switches to that file: Claude is asking about it.
4. A write to another markdown file in the working document's folder refreshes
   the folder view (as a folder `spec_show` does) instead of switching. A
   spec's ADRs and open questions stay part of one document.
5. After the first pick, the hook tells Claude in one line: "`<file>` is now
   the working document in ai-specview; call `spec_show` to show another."
   Claude can correct a wrong pick.

So, in one session that runs `spec-elaborate` and then a company's spec skill
on the same ticket, the first spec stays the working document until the
company skill's first question quotes its own spec; then the panel moves to
it.

### Spec mode: a stronger search

`/ai-specview:spec-mode` turns on ask mode and says that a spec is coming.

- **With a path or glob** (`/ai-specview:spec-mode docs/specs/SW-1234.md`),
  there is no detection: the first write to a matching file makes it the
  working document. The file need not exist yet. This is the main way to use
  spec mode.
- **Without one,** each markdown write that counts gets a score, and the best
  candidate becomes the working document:

  | Signal | Weight | Notes |
  |---|---|---|
  | A spec skill is running | High | A `PreToolUse` hook on the `Skill` tool sees the skill's name; a name with `spec`, `rfc` or `design` marks the session as writing a spec from then on |
  | Questions quote the file | High | As step 3 above |
  | Where the file is | Medium | A folder named `specs`, `spec`, `rfc`, `rfcs`, `design` or `adr`, or `~/.claude/specs/`, `~/.claude/plans/` |
  | What the file holds | Medium | `kind: spec` frontmatter, or headings such as Requirements, Goals, Design, Acceptance criteria, Open questions |
  | How it grows | Medium | The file Claude keeps editing between questions, rather than writing once |

  The weights and the threshold are tuned in use; the first version can be a
  simple sum.
- **Limits.** Without a path, spec mode still guesses, with more information.
  A spec with unusual headings in an unusual folder may need the path, or one
  `spec_show` from Claude. Spec mode also cannot tell when a spec is
  finished: a skill other than `spec-elaborate` does not call `spec_done`. It
  ends when the user turns it off, starts another spec mode, or the session
  ends.

Spec mode adds no spec workflow (layer 3). It works under any spec skill:
`spec-elaborate`, a company's, or Claude with no skill.

## Changes by part

| Part | Change |
|---|---|
| Protocol (01) | `doc_refs` gains `lines`; a doc ref may name any text file in the project. The working-document tool is generalized from `spec_show` (a new name such as `doc_show`, with `spec_show` kept as an alias so installed skills keep working). `spec_done` likewise becomes `done` with an alias. New socket messages for the write hook (`doc_written`) and for spec mode (its path or glob). |
| Bridge (02) | Resolves doc refs outside the spec folder and by line range. Reads `AI_SPECVIEW_ASK_MODE` and turns ask mode on at start. Reads `AI_SPECVIEW_VIEW`. |
| Daemon and page (03) | The document panel, document tabs, the per-set layout, source view for non-markdown documents. |
| Hooks (04) | A `PostToolUse` hook on `Write`, `Edit` and `MultiEdit`, and a `PreToolUse` hook on `Skill` for spec mode, both in `ai-specview hook`. |
| Skills (04) | One skill per layer: `ask-mode`, `spec-mode` (new), `doc` (new), `spec-elaborate`. The `-mod` skills go away. |
| Pane mod (06) | The same layout rules and tools, drawn in the pane, and the same rules for finding the working document in its `tool.call` hook. |
| README and plugin description | Present ask mode as the main feature, with the working document and the spec workflow built on it. |

## Phases

Each phase can be released on its own.

1. **Questions with context (layer 1).** Built. The document panel, document
   tabs, `lines` in doc refs, documents outside the spec folder or with no
   spec, the per-set layout, the `AI_SPECVIEW_ASK_MODE` setting, and the new
   README and plugin description. In the pane, a source document shows the
   lines before, in and after the range as code; document tabs are the
   pane's file list.
2. **Working document (layer 2).** Generalize `spec_show` and `spec_done`,
   add `/ai-specview:doc`, the write hook with its file rules and the
   `AI_SPECVIEW_DOC_PATHS` setting, and spec mode (`/ai-specview:spec-mode`)
   with its path argument and its scoring.
3. **Skills and viewer choice.** One skill per layer, `AI_SPECVIEW_VIEW`
   choosing the viewer, and `spec-elaborate` as the spec workflow on top of
   layers 1 and 2.

## Open points

- **Snippet ref.** Much context is not in a file: test output, build errors,
  logs, diffs, and plans or option lists that Claude wrote in the chat. A
  snippet ref would carry inline text with a title and a kind (`text`,
  `markdown`, `diff`, or a language for highlighting). A markdown snippet
  would open in the document panel; the other kinds in the code area. It fits
  phase 1 or a later phase.
- **Diff ref.** A ref to a file's uncommitted changes (`git diff` of a path),
  if snippets prove too manual for changes.
- **Name of the working-document tool.** `doc_show` is a placeholder.
- **Spec-mode scoring.** The weights and the threshold need real sessions to
  tune. Until then, the path argument is the reliable way.
- **Removing the `-mod` skills** changes what users type. The old skill names
  could stay for one release, pointing at the new ones.
