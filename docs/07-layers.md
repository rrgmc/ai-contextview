# 07 — Layers: questions first, specs on top

Status: proposed. Nothing in this document is built yet, except ask mode as
described in 04 and 06.

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
| Claude writes a document with them | Ask in words ("write the plan in the viewer"), or `/ai-specview:doc <path>` | Layers 1 and 2, until the document is done |
| A spec | `/ai-specview:spec-elaborate`, or ask in words ("spec this") | Layers 1, 2 and 3, until `done` |

Rules:

- **Ask mode is off by default.** It starts only when the user turns it on.
  `AI_SPECVIEW_ASK_MODE=always`, in the `env` block of the Claude Code
  settings, turns it on for every session.
- **Layer 2 from inside ask mode.** The ask-mode skill tells Claude: when you
  write or change a document together with the user, show it as the working
  document. The user rarely needs `/ai-specview:doc`.
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

## Changes by part

| Part | Change |
|---|---|
| Protocol (01) | `doc_refs` gains `lines`; a doc ref may name any text file in the project. The working-document tool is generalized from `spec_show` (a new name such as `doc_show`, with `spec_show` kept as an alias so installed skills keep working). `spec_done` likewise becomes `done` with an alias. |
| Bridge (02) | Resolves doc refs outside the spec folder and by line range. Reads `AI_SPECVIEW_ASK_MODE` and turns ask mode on at start. Reads `AI_SPECVIEW_VIEW`. |
| Daemon and page (03) | The document panel, document tabs, the per-set layout, source view for non-markdown documents. |
| Skills (04) | One skill per layer: `ask-mode`, `doc` (new), `spec-elaborate`. The `-mod` skills go away. |
| Pane mod (06) | The same layout rules and tools, drawn in the pane. |
| README and plugin description | Present ask mode as the main feature, with the working document and the spec workflow built on it. |

## Phases

Each phase can be released on its own.

1. **Questions with context (layer 1).** The document panel, document tabs,
   `lines` in doc refs, documents outside the spec folder, the per-set layout,
   the `AI_SPECVIEW_ASK_MODE` setting, and the new README and plugin
   description.
2. **Working document (layer 2).** Generalize `spec_show` and `spec_done`,
   add `/ai-specview:doc`, and have the ask-mode skill show a document Claude
   writes with the user.
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
- **Removing the `-mod` skills** changes what users type. The old skill names
  could stay for one release, pointing at the new ones.
