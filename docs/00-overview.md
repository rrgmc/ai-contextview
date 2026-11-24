# 00 — Overview

## Problem

When doing spec-driven development with Claude, the clarifying questions refer
to both the spec ("the retry section says…") and the code ("`Retrier.Next`
currently…"). Answering well means finding both places by hand, for every
question. The terminal shows neither.

## Goal

A local viewer that, during a spec-elaboration session:

1. **Shows the current spec all the time.** It is rendered, follows edits as they
   happen, and marks the sections that changed.
2. **For each question**, it:
   - highlights the part of the spec being asked about, located from a verbatim
     quote rather than line numbers
   - opens the referenced code at the right place, with the range highlighted
     (Go symbols are resolved to ranges)
   - shows the question and its options in a **docked, non-modal panel**, so the
     user can keep browsing the spec and code while deciding
3. Sends the user's answer back to Claude as the tool result, or lets the user
   answer in the chat instead.
4. Keeps **several Claude instances apart**. Each gets its own workspace, and
   their data never mixes.

## Non-goals (POC)

- No editing of spec or code inside ai-specview. It only views.
- No support for clients other than Claude Code. Any MCP client should work,
  but only Claude Code is tested.
- No language-aware symbol resolution beyond Go. Other languages use `path:line`
  (decided: no LSP or tree-sitter for now).
- No native desktop app. The viewer is a page in the browser (see "Key
  decisions").
- No JavaScript build step (no npm, no bundler).
- No MCP Apps (inline chat UI) support.

## Architecture

```
┌──────────────┐ stdio  ┌──────────────────┐
│ Claude Code A│◀──────▶│ ai-specview mcp     │──┐
└──────────────┘  MCP   │ workspace W1     │  │
                        └──────────────────┘  │  Unix socket
┌──────────────┐ stdio  ┌──────────────────┐  │  (NDJSON, 01-protocol.md)
│ Claude Code B│◀──────▶│ ai-specview mcp     │──┤
└──────────────┘  MCP   │ workspace W2     │  │
                        └──────────────────┘  ▼
                              ┌─────────────────────────────────┐
                              │ ai-specview serve (Go daemon)      │
                              │  workspaces W1, W2              │
                              │  markdown, quotes, highlighting │
                              │  HTTP on 127.0.0.1 + SSE        │
                              └───────────────┬─────────────────┘
                                              │ http://127.0.0.1:7878/w/<ws>
                                   ┌──────────▼───────────┐
                                   │ Browser tab per ws   │
                                   │ spec | code | Q-dock │
                                   └──────────────────────┘
```

Both processes are the **same Go binary**, `ai-specview`, with two subcommands.

### Components

**`ai-specview mcp`** (the bridge): Claude Code launches it as a stdio MCP server.
There is one per Claude instance.
- Implements the MCP tools: `spec_show`, `focus`, `ask`, `ask_wait`.
- Gives its instance an identity (workspace ID, cwd, parent PID).
- Resolves Go symbols to `file + line range` with `go/packages`, and validates
  the inputs (including the quote pre-check).
- Forwards everything to the daemon over a local socket, and blocks while
  waiting for answers.
- Starts the daemon if it isn't running, and reconnects if it restarts.

**`ai-specview serve`** (the viewer daemon): long-lived, one per user.
- Holds one workspace per connected bridge.
- Renders markdown (goldmark) and code (chroma) to HTML on the server, matches
  quotes, computes section changes, watches files.
- Serves a small web UI on `127.0.0.1` and pushes updates to open pages with
  Server-Sent Events. Answers come back as plain HTTP POSTs.
- Knows nothing about MCP. It only speaks the socket protocol and HTTP.

Because all the logic (quote matching, markdown, change detection) is Go, the
bridge and the daemon share packages. There is no second implementation to keep
in sync, which was the main cost of the Qt plan.

## Key decisions

| Decision | Choice | Why |
|---|---|---|
| UI tech | **Browser page served by a Go daemon** (`html/template` + htmx + a little vanilla JS, embedded with `go:embed`) | Far less to build than a native app; one language for everything; the browser already gives tabs, windows, multi-monitor and notifications. |
| MCP layer | Go, official `modelcontextprotocol/go-sdk` | Also provides `go/packages` for symbol resolution |
| MCP transport | **stdio**, one process per Claude instance | The process tree gives identity and lifetime for free. HTTP session IDs change on reconnect |
| Bridge↔daemon IPC | Unix socket, NDJSON | Local-only, no ports; the open connection *is* the workspace's lifetime |
| Daemon→browser push | Server-Sent Events | One-way push is all that's needed; answers go back as POSTs. No WebSocket library |
| Spec location | **Verbatim quote**, matched on normalized markdown source; highlighted by splitting goldmark text nodes | Line numbers drift on every edit; matching the source in Go means the bridge pre-check and the viewer use the same code |
| Code location | Go symbol → range (resolved in the bridge); `path:line` fallback | Survives edits; keeps the daemon language-agnostic |
| Spec refresh | Daemon file watcher (primary) + `spec_show` (intent / changed sections) | Doesn't depend on Claude remembering to call a tool |
| Answer flow | Blocking `ask` + "back to chat" escape hatch + `ask_wait` for timeouts | Simple result path, without trapping the user |
| Multi-instance | Workspace per bridge process; archive on disconnect; adopt on resume | Isolation without Claude having to pass IDs |
| Spec shape | A single markdown file **or a folder of artifacts** (01 `spec_show`, `doc_refs`) | Leaves room for change folders (below) without breaking the protocol |

## Future: change folders and branch-per-task

The POC works with one spec file. The longer-term workflow is:

1. Each task starts on its **own Git branch**, containing an OpenSpec **change
   folder** built from a custom schema, e.g.:
   ```
   openspec/changes/004-jittered-retries/
   ├── proposal.md          # the spec / plan (plus OpenSpec's specs/, design, tasks)
   ├── adr/                 # decisions made while elaborating (one file each)
   ├── research/            # findings, links, benchmarks
   └── open-questions/      # questions not answered yet (one file each)
   ```
2. The spec skill elaborates the change with ai-specview. An **answered** question
   that settles a design choice becomes an ADR; a **dismissed** or **in-chat**
   question becomes an entry in `open-questions/` when the user ticks "keep as
   open question".
3. ai-specview shows the whole folder. Questions can point at any artifact
   (`doc_refs`), so "this conflicts with ADR-002" highlights ADR-002 next to the
   proposal and the code.
4. The branch goes up as an MR **whose diff is the plan**, not code. Review
   happens on the plan.
5. After merge, a final skill command turns the change folder into the handoff
   for the next agent: a Jira epic with stories, or an RFC.

The protocol already allows this (01). The folder convention lives in the skill
and is added in M8; the handoff is M9 (05). The layout is an **OpenSpec custom
schema** that adds `adr/`, `research/` and `open-questions/`. The daemon doesn't
depend on OpenSpec: it only needs the artifact kind hints. The bridge reads
`openspec status --json` when the CLI is installed.

## Glossary

- **Workspace**: the daemon-side state for one Claude instance: spec root (file
  or folder), open code refs, current/pending question set, Q&A history.
- **Question set**: one `ask` call. It can contain several questions, which are
  answered together.
- **Ref**: a pointer into a markdown document (`spec_quote` / `doc_refs[]`) or
  into code (`code_refs[]`).
- **Change folder**: a directory of markdown artifacts for one task (see
  "Future" above).
- **Artifact kind**: `spec`, `adr`, `research`, `open_question` or `other`,
  inferred from the folder name or frontmatter. Used for labels and grouping.
