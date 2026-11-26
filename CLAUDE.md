# ai-specview

ai-specview shows the spec and the code side by side while Claude elaborates a
spec. It is one Go binary, `ai-specview`:

- `ai-specview mcp`: a stdio MCP server ("bridge"), one per Claude session. It
  offers the tools `spec_show`, `focus`, `ask`, `ask_wait`, `ask_mode` and
  `spec_done`.
- `ai-specview serve`: the viewer daemon, one per user. It serves the web page
  (`http://127.0.0.1:7878`) and talks to the bridges over a Unix socket.
- `ai-specview hook`: a Claude Code `PreToolUse` hook that blocks the built-in
  `AskUserQuestion` only in a session that has a spec open or is in ask mode.
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
| `plugin/` | The Claude Code plugin: `.mcp.json`, launcher script, `skills/spec-elaborate/` and `skills/ask-mode/` (browser), `skills/spec-elaborate-mod/` and `skills/ask-mode-mod/` (pane), `hooks/` (the `PreToolUse` hook and the pane mod's TypeScript module), `types/` and `tests/` of the mod |
| `.claude-plugin/` | `marketplace.json`: the repository is a plugin marketplace |
| `tools/changelog/` | Release tool: merges `changelog.d/` into `CHANGELOG.md`, checks a tag, prints release notes |
| `.github/workflows/` | `ci.yml` (tests) and `release.yml` (on `v*` tags) |
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
  `claude plugin validate ./plugin` and `claude plugin validate .`. Never
  edit `version` in `plugin/.claude-plugin/plugin.json` by hand: it is the
  release version, and only the release command sets it.
- After changing the pane mod (`plugin/hooks/*.ts*`, `plugin/types/`), also
  run `claude plugin test ./plugin`. Its tools mirror the bridge's, so a
  changed tool input, result or message updates `plugin/hooks/tools.ts` and
  `register.tsx`, a changed input rule in `internal/validate` updates
  `plugin/hooks/validate.ts`, and a change to one skill's workflow updates the
  other skill too (docs/06-pane-mod.md).

The `CI` workflow (`.github/workflows/ci.yml`) runs gofmt, `go vet` and
`go test -race ./...` on Linux and macOS for pushes to `master` and pull
requests. A push to `master` that only changes markdown files or `docs/` skips
it. It does not run the `claude plugin` checks.

Versions come from git tags (`cmd/ai-specview/version.go`). To release, run
`go run ./tools/changelog release X.Y.Z`, commit, then tag `master` with
`vX.Y.Z` and push the tag. The `Release` workflow checks the changelog, tests,
builds the binaries and creates the GitHub release
([changelog.d/README.md](changelog.d/README.md)). The release command also
sets `version` in `plugin/.claude-plugin/plugin.json` to `X.Y.Z` and, for a
stable release, the install tag in `README.md` to `vX.Y.Z`; the workflow
fails unless they match. Never edit the README install tag by hand.
Installed plugins update only when the plugin version changes, so plugin
changes reach users at the next release.

A change that users notice adds a file to `changelog.d/` (one per branch);
never edit `CHANGELOG.md` directly outside a release. Before tagging, merge
the files into `CHANGELOG.md` as
[changelog.d/README.md](changelog.d/README.md) describes.

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
  `AI_SPECVIEW_ASK_MODE`,
  `AI_SPECVIEW_ASK_WAIT`, `AI_SPECVIEW_OPEN`, `AI_SPECVIEW_DAEMON`) are listed in the
  README.
