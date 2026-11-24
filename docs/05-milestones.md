# 05 — Milestones, testing, risks

Each milestone ends with something runnable. M1 and M2 can proceed in parallel
once M0 is done. M0–M7 are the POC; M8–M9 are the change-folder workflow (00
"Future").

## Status

| Milestone | State |
|---|---|
| M0–M5 | Done. |
| M6 | Done, except the dogfood session with 3+ rounds on a real project. |
| M7 | Done. `internal/e2e/multi_test.go` is the scripted three-session test. |
| M8, M9 | Not started. |
| Pane mod | Prototype in `plugin-pane/` (06). Checked live in `both` and `pane` modes. |

What changed from the plan while building and using it:

- **Five tools, not four.** `spec_done` marks the spec finished. The page
  shows a "Spec finished" banner with the summary (a **Decided** and a
  **Still open** bullet list), clears the highlights and closes the code tabs.
- **Panel buttons.** "Send answers", "Back to chat" (was "Answer in chat") and
  "Skip" (was "Dismiss"). Clicking an option text answers and moves to the next
  question; clicking the radio button waits for "Next". After the last question
  a review step lists all answers before sending.
- **"Decide later" replaced the "keep as open question" checkbox.** It is one
  more choice per question. Claude gets the deferred question ids
  (`deferred` in the result) and records them as open items in the spec.
  `keep_open` stays in the protocol for "Back to chat" and "Skip".
- **Recommended options.** An option can carry `recommended: true` (or a
  "(Recommended)" label suffix, which the bridge turns into the flag); the
  panel marks it.
- **Changes tab.** The doc pane has Spec and Changes tabs. Changes shows the
  whole document as a block diff (rendered) or a line diff (source), against
  the last answers or the start of the session. This settles the former open
  question about a spec diff. The tab always shows the document the Spec
  tab shows; a slow, older request no longer replaces a newer one.
- **"Show current passage" link** next to "open in editor" scrolls back to the
  highlighted passage.
- **Layout.** Spec on the left; questions on the right with the code below them.
- **The built-in question tool is blocked by a hook, not a deny rule.**
  `ai-specview hook` blocks `AskUserQuestion` only in the Claude session that has
  an unfinished spec open, so other sessions keep it (04, section 2).
- **Install.** The module path is the repository path, and the MCP server,
  skill and hook ship as a Claude Code plugin (README).
- **Pane mod.** A second plugin, `ai-specview-pane`, draws the spec and code
  in a Claude Code pane. In `both` mode it mirrors the bridge; in `pane` mode
  it answers `spec_show` and `focus` itself (06).
- **Front end.** Plain JavaScript instead of htmx; the web server lives in
  `internal/daemon`. Browser tests in `internal/daemon/browser_test.go` drive
  the panel in headless Chrome.

## M0 — Contract and skeleton

- Repo layout from the README: one `go.mod`, `cmd/ai-specview` with empty `mcp`,
  `serve`, `fakeapp` subcommands that build.
- `internal/protocol`: envelope and message structs for 01, including
  `DocRef`/`ResolvedDocRef`/`Artifact`.
- `protocol/fixtures/`: at least one valid and one invalid example per message.
- `protocol/quotes/`: the quote-matching fixtures (markdown source, quote,
  expected match quality and source range).
- MCP tool input schemas come from the Go structs via the go-sdk; no
  hand-written JSON Schema.

**Done when:** a test loads every fixture and round-trips it through the
structs.

## M1 — Bridge end to end with the fake daemon

- `ai-specview mcp` with all tools (four planned; `spec_done` came later),
  validation, `lines`-only code refs (no symbols yet), the `specsrc` quote
  pre-check (single file and folder roots, `doc_refs`), `appconn` with daemon
  start and reconnect.
- `ai-specview fakeapp`, which prints messages and prompts for answers in the
  terminal.
- Register it in a scratch project with `AI_SPECVIEW_DAEMON="ai-specview fakeapp"`;
  ask Claude to call `ask` with a sample question.

**Done when:**
- a real Claude Code session calls `ask`, the answer typed into fakeapp comes
  back as the tool result, and Claude continues
- `pending` → `ask_wait` works (with `AI_SPECVIEW_ASK_WAIT=10s`)
- killing fakeapp mid-question and restarting it re-delivers the question

## M2 — Daemon: socket, workspaces, doc pane

- `daemon` socket server, `Conn`, workspaces, single-instance guard, stale socket
  cleanup, `--detach`.
- `web` server with Host/Origin checks, index page, workspace page, SSE hub.
- `render/markdown`: goldmark rendering, section index, file watcher (re-add after
  atomic saves, debounced), scroll position kept across reloads.
- `spec_show` handling for a single file **and** a folder (artifact sidebar).

**Done when:** two Claude sessions in two projects each get their own workspace
page showing their own spec, and editing the spec in an editor (or by Claude)
updates the right page without jumping the scroll.

## M3 — Code pane

- `render/code`: chroma with line numbers, line anchors, highlighted ranges,
  light/dark themes; code tabs; reload on change; "open in editor" links.
- `focus` end to end with `lines` refs.

**Done when:** `focus` with two code refs opens two tabs with the right ranges
highlighted and centred.

## M4 — Question panel and blocking ask

- Question panel: stepper, chips, options, Other, notes, Decide later, a
  review step, three buttons (Send answers, Back to chat, Skip), keyboard
  shortcuts. POST handlers with first-answer-wins.
- Auto-focus of refs per question; `ask_shown` (with `viewers`) / `ask_result`;
  bridge opens the URL when nobody is watching.
- Replacing a pending question set (`dismissed` for the old one).
- History tab.
- Notifications: title/favicon badge, browser `Notification`.

**Done when:** the full loop works from a real Claude session: question shown,
spec and code highlighted, the user browses freely, answers, and Claude
receives the answers. The same works with "Back to chat".

## M5 — Quote matching and highlighting

- `specsrc.Match` stages exact → normalized (with the index map) → fuzzy (the
  32-character anchors) → heading, on markdown source.
- `render/highlight`: the goldmark AST transformer that wraps the matched source
  range in `<mark>`s, including ranges across inline formatting and paragraphs.
- Match-quality badge on ref chips; a `none` match → `ask_cancel` → tool error.
- Section change detection (hash per heading path) with ◆ changed markers.

**Done when:** all quote fixtures pass, and a quote containing bold/code/link
text and a line break still matches `exact` or `normalized` and is highlighted
in the right place. Bridge and daemon pass the same fixtures by construction
(same package).

## M6 — Go symbols and skill

- `resolve` with `go/packages`, caching, near-miss suggestions.
- Example skill `plugin/skills/spec-elaborate/`, CLAUDE.md snippet, and the
  `ai-specview hook` that blocks the built-in question tool while a spec is open.
- Dogfood: elaborate a real spec for one of your Go projects.

**Done when:** a dogfood session with at least 3 question rounds needs no manual
navigation; every question lands on the right spec text and code.

## M7 — Multi-instance hardening and install

- Archive on disconnect, auto-adopt on resume (same cwd, < 2h) with page
  redirect, the archived workspace limit.
- `ai-specview serve --restart`; version mismatch message between an old daemon and
  a new bridge.
- Optional idle exit.
- Installation notes in the repo README (`go install`, the plugin,
  `MCP_TOOL_TIMEOUT`).

**Done when:** a scripted test with three concurrent Claude sessions, daemon
restarts, and a `claude --resume` shows no cross-talk and no lost pending
questions.

**Result:** `TestMultiSession` builds the binary and runs three `ai-specview mcp`
processes in three projects, as three Claude sessions would. The bridges start
the daemon themselves. With a question pending in each, the test restarts the
daemon with `serve --restart`, then kills it with `kill -9`. Each page must show
only its own question, with the same id, and each answer must reach its own
session. Last, one session exits and a new bridge in the same directory takes
its workspace over, with its history.

The test found a bug: after a crash, all bridges start a daemon at the same
moment, and two could end up running, each on its own port. Each removed the
stale socket and listened, so the second removed the first one's socket. Now a
daemon takes an exclusive lock on `<socket>.lock` before it listens, and the
others exit.

Known limit: the daemon keeps its state in memory. After a restart the bridges
show their spec and pending question again, but the answer history and ended
sessions are gone, so `claude --resume` after a daemon restart starts a new
workspace.

## M8 — Change folders

- An OpenSpec custom schema (`openspec schema init`) that adds `adr/`,
  `research/` and `open-questions/` artifacts to OpenSpec's proposal / specs /
  design / tasks.
- Skill: a "start task" step that creates a numbered branch and
  `openspec/changes/<NNN-name>/` from that schema, then calls `spec_show` on the
  folder.
- Skill: answered questions that settle a design choice become ADR files
  (MADR-style, `kind: adr`). Questions the user marked "Decide later"
  (`deferred` in the result, 01) become `open-questions/` files. Research notes the skill writes go in
  `research/`.
- Skill rule: use `doc_refs` when a question depends on an ADR, research note or
  another spec.
- Viewer: artifact sidebar grouped by kind, ◆ changed per artifact, open-question
  count in the header.
- Bridge: if the `openspec` CLI is on the PATH, call
  `openspec status --change <name> --json` on `spec_show` and pass the phase and
  task progress to the viewer header. Without the CLI, everything still works
  from the folder alone.

**Done when:** a dogfood session in a change folder produces at least one ADR
and one open question, and a question with a `doc_refs` entry into an ADR
highlights the ADR next to the proposal and the code.

## M9 — Handoff from a merged plan

- A final skill command that reads a merged change folder and produces the
  handoff for the next agent: a Jira epic with stories (or an RFC), linking back
  to the change folder in Git. Existing epic and RFC skills can cover most of
  the output side.
- The open questions that remain are carried into the epic or RFC as explicit
  open items, not dropped.

**Done when:** one real task goes branch → plan MR → merge → Jira epic, and an
implementation agent can start from the epic without re-asking the questions
already answered in the ADRs.

---

## End-to-end manual test script

1. Start Claude in project A with the skill. Ask it to spec "add jittered
   retries". The bridge opens `http://127.0.0.1:7878/w/…` in the browser and the
   spec renders.
2. Claude edits the spec: the page updates in place and the edited section shows
   ◆ changed.
3. Claude asks 2 questions: the panel shows Q 1/2; the spec quote and
   `Retrier.Next` are highlighted; the title shows ● and a notification appears.
4. Scroll the spec away, switch code tabs, open Q 2/2, click Q1's chip: the view
   re-focuses Q1's refs.
5. Answer. Claude applies the answer, and the Decisions section gets ◆ changed.
6. Start Claude in project B at the same time and ask a question there: it shows
   on B's page only, and A's page is unchanged.
7. Kill `ai-specview serve` while B has a question pending: the bridge restarts it,
   B's page reconnects and shows the question again, and Claude B is still
   waiting.
8. Exit Claude A: A's page greys out. Run `claude --resume` in A: the page
   redirects to the adopted workspace, with its history intact.
9. (M8) Run the skill on a change folder: the artifact sidebar lists proposal,
   ADRs, research and open questions; a question with a doc ref into an ADR
   highlights it.

## Risks and mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Claude paraphrases a quote | Highlights miss | Bridge pre-check with the same matcher returns an error naming the file and nearest headings; skill rule says "verbatim"; fuzzy stage; heading fallback |
| MCP tool timeout during a long wait | Claude sees an error | `AI_SPECVIEW_ASK_WAIT` < `MCP_TOOL_TIMEOUT`; `pending` + `ask_wait` |
| Built-in question tool used anyway | Questions bypass the UI | `ai-specview hook` blocks it while a spec is open; skill rule |
| Question arrives with no page open | User doesn't notice | `viewers: 0` → bridge opens the URL; browser notification |
| Splitting goldmark text nodes is fiddly (ranges across inline nodes, entities) | Highlight lands wrong | Golden HTML tests from the quote fixtures; fall back to highlighting the containing block when a split isn't clean |
| Another local web page talks to the daemon | Forged answers | Loopback only; Host check (DNS rebinding); Origin check on POSTs; no CORS |
| Port 7878 taken | Daemon fails to start | Try the next 9 ports; URL comes back in `hello_ack` |
| `go/packages` slow on big modules | Slow `ask` | Per-package cache with mtime invalidation; load only the package containing the file, not `./...` |
| Atomic saves drop fsnotify watches | Spec stops updating | Watch the directory, not the file; also poll the mtime every 2s as a safety net |
| Bridge starts before the daemon and Claude times out the server start | MCP server marked failed | MCP stdio starts at once; the daemon connection is lazy and in the background |
| Old daemon still running after an upgrade | Protocol mismatch | Version in `hello`; clear error suggesting `ai-specview serve --restart` |
| Hard to identify the Claude PID | Wrong label | Informational only; isolation never depends on it |

## Decisions taken

- **"Back to chat" copies the question to the clipboard** (text + options), so
  it can be pasted into the reply.
- **Non-Go code refs stay `lines` only.** Revisit after dogfooding.
- **Change folders use an OpenSpec custom schema** (M8).
- **Open items come from "Decide later"**, a choice per question, instead of a
  "keep as open question" checkbox.
- **The spec diff is a Changes tab** in the doc pane, against the last answers
  or the session start.
- **The built-in question tool is blocked per session by a hook**, never by a
  global deny rule.
