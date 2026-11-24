# 01 — Protocol (the contract)

There are three interfaces:

1. **MCP tools**: what Claude sees (Claude ↔ `ai-specview mcp`).
2. **Socket protocol**: bridge ↔ daemon (`ai-specview mcp` ↔ `ai-specview serve`).
3. **HTTP/SSE**: daemon ↔ browser. Internal to the daemon and its embedded UI,
   so it is described in 03, not versioned here.

1 and 2 are versioned. Both sides of 2 are Go, so the message structs live in
one shared package (`internal/protocol`). Golden fixtures in `protocol/fixtures/`
still pin the wire format, so a non-Go client can be written against them.

---

## 1. MCP tools

### `spec_show`

Declares or refreshes what is shown for this session: a single markdown file,
or a **folder of markdown artifacts** (a change folder, see 00).

```json
{
  "path": "docs/specs/retry-policy.md",
  "changed_sections": ["Retry policy", "Error budget"]
}
```

```json
{
  "path": "changes/004-jittered-retries",
  "primary": "proposal.md",
  "changed": [{"path": "adr/002-cap-resets-daily.md"}]
}
```

- `path` (required): a `.md`/`.markdown` file or a directory, relative to the
  Claude cwd or absolute.
- `primary` (optional, folders only): the file shown first and used when a ref
  has no `path`. Default: the first of `proposal.md`, `spec.md`, `README.md`
  that exists, else the first file alphabetically.
- `changed_sections` (optional): heading texts the caller just changed in the
  primary file. Only a hint; the daemon computes changes itself (03).
- `changed` (optional, folders only): `[{path, sections?}]` for other files the
  caller just created or edited.

For a folder, the daemon includes every `*.md` file up to 3 levels deep, skipping
dot-files and dot-directories. New files that appear later are picked up by the
watcher.

Result:

```json
{"ok": true,
 "primary": "proposal.md",
 "sections": ["Overview", "Retry policy"],
 "artifacts": [
   {"path": "proposal.md", "kind": "spec", "sections": ["Overview", "Retry policy"]},
   {"path": "adr/002-cap-resets-daily.md", "kind": "adr", "sections": ["Context", "Decision"]}
 ]}
```

`sections` is always the primary file's headings, so single-file callers can
ignore `artifacts`. Returning headings lets Claude use exact section names
later.

**Artifact kind** is inferred, first match wins:
1. frontmatter `kind:` (`spec | adr | research | open_question | other`)
2. any parent directory named `adr`/`adrs`/`decisions` → `adr`;
   `research` → `research`; `open-questions`/`questions` → `open_question`;
   `specs` → `spec`
3. file name `proposal.md`, `spec.md`, `design.md` → `spec`
4. `other`

Errors: the path doesn't exist; a file that isn't markdown; a folder with no
markdown files; `primary` not inside the folder.

### `focus`

Highlights something without asking anything.

```json
{
  "spec_quote": "retries use exponential backoff capped at 30s",
  "doc_refs": [{"path": "adr/002-cap-resets-daily.md", "quote": "the cap resets at midnight UTC"}],
  "code_refs": [{"path": "internal/sync/retry.go", "symbol": "sync.(*Retrier).Next"}]
}
```

Result: `{"ok": true, "docs": [{"path": "...", "match": "exact|normalized|fuzzy|heading|none"}], "code": [ResolvedRef...]}`

`spec_match` is also returned, equal to the match of the first doc ref, for
callers that only use `spec_quote`.

### `ask`

Shows a question set and **blocks** until the user acts or the wait limit is
reached.

```json
{
  "title": "Retry semantics",
  "questions": [
    {
      "id": "q1",
      "text": "Should retries reset the backoff on partial success?",
      "options": [
        {"label": "Reset", "description": "Start again from the base delay", "recommended": true},
        {"label": "Keep growing"},
        {"label": "Configurable"}
      ],
      "multi_select": false,
      "allow_other": true,
      "spec_quote": "retries use exponential backoff capped at 30s",
      "spec_section": "Retry policy",
      "doc_refs": [
        {"path": "adr/002-cap-resets-daily.md", "quote": "the cap resets at midnight UTC", "section": "Decision"}
      ],
      "code_refs": [
        {"path": "internal/sync/retry.go", "symbol": "sync.(*Retrier).Next"},
        {"path": "internal/sync/client.go", "lines": "88-104"}
      ]
    }
  ]
}
```

`spec_quote` + `spec_section` are shorthand for a doc ref into the primary file.
Internally the bridge turns them into `doc_refs[0]`. Both forms may be used
together.

Rules, enforced by the bridge, which returns an MCP tool error so Claude
retries:
- 1–6 questions; each has 2–6 options. At most one option per question has
  `recommended: true`. A label ending in "(Recommended)" is accepted too: the
  bridge strips the suffix and sets the flag.
- Each question has **at least one of** `spec_quote`, `doc_refs` or `code_refs`.
- At most 4 doc refs and 4 code refs per question.
- Each quote is 3–300 characters and must pre-match its file's source (normalized
  match, see 02). If it doesn't match, the error names the file and lists its
  closest headings.
- A doc ref `path` is relative to the `spec_show` folder (or to cwd when a single
  file is shown); omitted means the primary file. It must be markdown and exist.
- Each code ref has `path` and exactly one of `symbol`/`lines`. A symbol that
  can't be resolved is an error that names the candidates found.

Result, one of:
```json
{"status": "answered",
 "answers": [{"id": "q1", "selected": ["Keep growing"], "other": null, "notes": "but cap resets daily"}]}

{"status": "in_chat", "keep_open": ["q2"],
 "message": "The user will answer in the chat. Wait for their message."}

{"status": "pending", "question_set_id": "qs_7f3a",
 "message": "Still waiting for the user. Call ask_wait with this id to keep waiting."}

{"status": "dismissed", "keep_open": ["q1"],
 "message": "The user skipped these questions. Continue without these decisions; ask in chat only if one is essential."}
```

`deferred` (optional, `answered` only) lists the questions the user chose
**Decide later** for. Each also appears in `answers` with `"deferred": true`,
an empty `selected` and the user's `notes`, if any. Claude records them as open
questions and asks them again before finishing.

`keep_open` (optional, `in_chat` and `dismissed` only, older viewers) lists the questions the
user ticked "keep as open question" for. In a change folder the skill records
each of them as an `open-questions/` file (04, M8). Omitted when empty.

### `ask_wait`

`{"question_set_id": "qs_7f3a"}` → same result shape as `ask`. It exists because
an MCP tool call can't block forever (see the timeouts in 02).

### `spec_done`

Tells the viewer the spec is finished and no more questions are coming.

```json
{"summary": "**Decided**\n\n- Retries reset on partial success.\n- The cap stays at 30s.\n\n**Still open**\n\n- The jitter range."}
```

- `summary` (optional): short markdown, at most 8000 bytes. The viewer renders
  it in a "Spec finished" banner.
- A pending question set is withdrawn (`dismissed`).
- Highlights (from `focus` or questions) are cleared and the code tabs close.
- A later `ask` clears the finished state.

Result: `{"ok": true, "url": "..."}`.

### Shared types

```
CodeRef        = { path: string, symbol?: string, lines?: "N" | "N-M" }
ResolvedRef    = { path: abs string, start_line: int, end_line: int,
                   symbol?: string, kind?: "func"|"method"|"type"|"var"|"const",
                   source: "symbol"|"lines" }
DocRef         = { path?: string, quote: string, section?: string }
ResolvedDocRef = { path: abs string, quote: string, section?: string,
                   kind: ArtifactKind }
Artifact       = { path: string (relative to root), kind: ArtifactKind,
                   sections: [string] }
ArtifactKind   = "spec" | "adr" | "research" | "open_question" | "other"
Match          = "exact" | "normalized" | "fuzzy" | "heading" | "none"
```

---

## 2. Socket protocol (bridge ↔ daemon)

### Transport

- Unix socket at a fixed path: `$XDG_RUNTIME_DIR/ai-specview.sock`, or on macOS
  `~/Library/Caches/ai-specview/ai-specview.sock`. Override with `$AI_SPECVIEW_SOCKET`.
  The daemon creates it with mode `0600`.
- Framing: **NDJSON**: one JSON object per line, UTF-8, `\n` terminated. Maximum
  line length 4 MiB.
- One connection per bridge; that connection is the workspace.

### Envelope

```json
{"v": 1, "type": "ask", "id": "m_12", "ws": "ws_4c1e…", "body": { … }}
```
- `id` is unique per sender. Replies set `"re": "<id>"`.
- `ws`: workspace ID, a UUIDv4 generated by the bridge at startup.

### Messages: bridge → daemon

| type | body | reply |
|---|---|---|
| `hello` | `{protocol: 1, ws, cwd, claude_pid, bridge_pid, bridge_version, tty?, project_name}` | `hello_ack {daemon_version, url, adopted_from?}` |
| `spec_show` | `{root_abs, is_dir, primary?, changed_sections?, changed?: [{path, sections?}]}` | `result {primary, sections[], artifacts[]}` or `error` |
| `focus` | `{docs: [ResolvedDocRef], code: [ResolvedRef]}` | `result {docs: [{path, match}]}` |
| `ask` | `{qs_id, title?, questions: [ {…, docs: [ResolvedDocRef], code: [ResolvedRef]} ]}` | `ask_shown {matches}` right away, **then later** `ask_result` |
| `ask_cancel` | `{qs_id, reason}` | `result` |
| `done` | `{summary?}` | `result {}` |
| `bye` | `{}` | (none; the bridge closes) |

In `ask`, `spec_quote`/`spec_section` have already been folded into `docs` by
the bridge, so the daemon only sees `docs`.

### Messages: daemon → bridge

| type | body |
|---|---|
| `hello_ack` | `{daemon_version, url, adopted_from?: ws}` (`url` is the workspace page, e.g. `http://127.0.0.1:7878/w/ws_A`) |
| `result` | `{…}` (reply to `re`) |
| `error` | `{code, message}` (reply to `re`) |
| `ask_shown` | `{qs_id, viewers: int, matches: [{id, doc: int, match}]}` (`doc` indexes the question's `docs`; `viewers` is the number of open browser pages) |
| `ask_result` | `{qs_id, status: "answered"\|"in_chat"\|"dismissed", answers?, deferred?: [id], keep_open?: [id]}` |

Error codes: `bad_request`, `file_not_found`, `not_markdown`, `empty_folder`,
`unknown_qs`, `busy` (another question set is pending in this workspace),
`internal`.

### Sequencing rules

- **At most one pending question set per workspace.** A second `ask` replaces
  the first: the daemon sends `ask_result{status:"dismissed"}` for the old one,
  then shows the new one. This handles Claude giving up on a question and asking
  again.
- **`ask_shown` with a `none` match** triggers an `ask_cancel` from the bridge,
  which returns a tool error to Claude ("quote not found in <file>; closest
  sections: …"). Bridge and daemon use the same matching package, so this only
  happens if the file changed between the pre-check and the show.
- **`ask_shown` with `viewers: 0`**: the bridge opens the workspace `url` in the
  default browser (unless `AI_SPECVIEW_OPEN=never`), so a question never waits on a
  page nobody is looking at.
- **If the daemon restarts,** the bridge reconnects, re-sends `hello` with the
  same `ws`, re-sends the last `spec_show`, any pending `ask`, and the last
  `done` (unless an `ask` came after it).
- **Disconnect** (`bye` or EOF): the daemon archives the workspace (03).

### Example session

```
→ {"v":1,"type":"hello","id":"b1","ws":"ws_A","body":{"protocol":1,"cwd":"/Users/r/src/shop",...}}
← {"v":1,"type":"hello_ack","re":"b1","body":{"daemon_version":"v0.1.0","url":"http://127.0.0.1:7878/w/ws_A"}}
→ {"v":1,"type":"spec_show","id":"b2","ws":"ws_A","body":{"root_abs":"/Users/r/src/shop/docs/specs/retry.md","is_dir":false}}
← {"v":1,"type":"result","re":"b2","body":{"primary":"retry.md","sections":["Overview","Retry policy"],"artifacts":[…]}}
→ {"v":1,"type":"ask","id":"b3","ws":"ws_A","body":{"qs_id":"qs_1","questions":[…]}}
← {"v":1,"type":"ask_shown","re":"b3","body":{"qs_id":"qs_1","viewers":1,"matches":[{"id":"q1","doc":0,"match":"exact"}]}}
   … user browses, clicks …
← {"v":1,"type":"ask_result","body":{"qs_id":"qs_1","status":"answered","answers":[…]}}
```

## Versioning

- `v` is the envelope version and `protocol` (in `hello`) is the message-set
  version. The daemon rejects a `hello` with an unknown major version, and the
  bridge then returns a tool error telling the user to update. Since bridge and
  daemon ship in one binary this only happens when an old daemon is still
  running; the error says to run `ai-specview serve --restart`.
- Adding new optional fields is allowed without a version bump. Both sides
  ignore fields they don't know. (`doc_refs`, folder `spec_show`, `viewers` and
  `url` were added this way before any release, so protocol stays at 1.)
