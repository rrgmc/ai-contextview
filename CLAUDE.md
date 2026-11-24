# ai-specview

ai-specview shows the spec and the code side by side while Claude elaborates a
spec. It is one Go binary, `ai-specview`:

- `ai-specview mcp`: a stdio MCP server ("bridge"), one per Claude session. It
  offers the tools `spec_show`, `focus`, `ask`, `ask_wait` and `spec_done`.
- `ai-specview serve`: the viewer daemon, one per user. It serves the web page
  (`http://127.0.0.1:7878`) and talks to the bridges over a Unix socket.
- `ai-specview hook`: a Claude Code `PreToolUse` hook that blocks the built-in
  `AskUserQuestion` only in a session that has a spec open.
- `ai-specview fakeapp`: a terminal stand-in for the daemon, for testing.

The design is in [docs/](docs/README.md); the contract between the parts is
[docs/01-protocol.md](docs/01-protocol.md). Keep the docs in step with the
code: a change in behavior updates the matching doc, and
[docs/05-milestones.md](docs/05-milestones.md) records what was built.

## Always work in a git worktree

Never edit files, build or commit in the main checkout
(`ai-specview/`). Other sessions use it at the same time, and a
branch switch or an uncommitted edit there affects all of them. The main
checkout stays on `master` and clean.

For every change, before the first edit:

1. Create a branch and a worktree next to the main checkout, from `master`:
   ```bash
   git worktree add -b <type>/<name> ../ai-specview-<name> master
   ```
   A session the Claude desktop app started in its own worktree
   (`.claude/worktrees/…`) already has one: work there.
2. Work, build and test only in that worktree.
3. Stage files by name, never `git add -A` or `git add .`: other sessions'
   sample files must not end up in a commit.
4. Commit, merge and push only when the user asks. To merge, fast-forward
   `master` in the main checkout (`git merge --ff-only <branch>`); if `master`
   moved, rebase the branch first.
5. After the merge, remove the worktree (`git worktree remove`) and delete the
   branch.

Read-only work (reading code, answering questions) needs no worktree.

## Layout

| Path | What it holds |
|---|---|
| `cmd/ai-specview/` | Subcommands, `serve` flags (`--detach`, `--restart`, `--stop`), version string |
| `internal/protocol/` | Socket envelope and messages, MCP tool inputs and outputs, socket path |
| `internal/tools/` | The MCP tools: validation, quote pre-check, ref resolution, `ask` blocking |
| `internal/appconn/` | Bridge side of the socket: lazy connect, daemon start, reconnect and replay |
| `internal/daemon/` | Daemon: workspaces, socket server, HTTP and SSE, templates, `static/` front end |
| `internal/render/` | Markdown with quote highlights, code with Chroma, the Changes diff |
| `internal/specsrc/` | Quote matching (exact, normalized, fuzzy, heading), sections, spec roots |
| `internal/resolve/` | Code refs: line ranges, Go symbols via `go/packages` |
| `internal/hook/` | The `PreToolUse` hook |
| `internal/identity/` | Workspace id, project name, Claude and ancestor process ids |
| `internal/e2e/` | End-to-end tests, including the multi-process test |
| `protocol/` | Golden message fixtures and quote-matching fixtures |
| `plugin/` | The Claude Code plugin: `.mcp.json`, `hooks/`, `skills/spec-elaborate/`, launcher script |
| `plugin-pane/` | The pane mod `ai-specview-pane`: TypeScript hooks module, state contract, tests |
| `.claude-plugin/` | `marketplace.json`: the repository is a plugin marketplace |
| `examples/` | Manual setup files (`.mcp.json`, settings, CLAUDE.md snippet) |

## Build and test

```bash
go test ./...
```

- `go test -short ./...` skips the slow tests: the browser tests
  (`internal/daemon/browser_test.go`, headless Chrome via chromedp, also
  skipped when no Chrome is found; `CHROME_PATH` picks one) and
  `internal/e2e/multi_test.go` (builds the binary and runs three bridges
  against a real daemon).
- A protocol change updates the structs, `protocol/fixtures/` and
  docs/01-protocol.md together.
- After changing `plugin/` or `.claude-plugin/`, run
  `claude plugin validate ./plugin` and `claude plugin validate .`. Bump
  `version` in `plugin/.claude-plugin/plugin.json` when plugin files change,
  or installed copies keep the cached version.
- After changing `plugin-pane/`, run `claude plugin validate ./plugin-pane`
  and `claude plugin test ./plugin-pane`, and bump its version the same way.
  It hooks the MCP tools by name, so a renamed tool or a changed tool result
  updates `plugin-pane/hooks/register.tsx` too (docs/06-pane-mod.md).

To try a change locally, install it and replace the running daemon:

```bash
go install ./cmd/ai-specview && ai-specview serve --restart
```

## Things to know

- The front end is plain JavaScript and CSS in `internal/daemon/static/`, with
  Go templates in `internal/daemon/templates/`, both embedded in the binary.
  There is no build step. Colors are CSS tokens with light and dark values.
- The daemon keeps all state in memory. After a reconnect, bridges replay
  `spec_show`, the pending `ask` and the last `spec_done`.
- One daemon per socket: `serve` takes an exclusive lock on `<socket>.lock`
  before it listens.
- HTTP accepts loopback only, checks the `Host` header, checks `Origin` on
  POSTs and sends no CORS headers. Keep it that way.
- The environment variables (`AI_SPECVIEW_SOCKET`, `AI_SPECVIEW_ADDR`,
  `AI_SPECVIEW_ASK_WAIT`, `AI_SPECVIEW_OPEN`, `AI_SPECVIEW_DAEMON`) are listed in the
  README.
