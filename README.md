# ai-specview

ai-specview shows **the spec and the code side by side** while Claude elaborates a
spec. When a spec skill creates or edits the spec, the viewer shows it at once.
When the skill asks a question, the viewer highlights the spec passage being
asked about, opens the code at the right lines, and shows the question in a
docked panel that never blocks the rest of the page.

The viewer is a local web page served by a Go daemon. The MCP server and the
daemon are one binary, `ai-specview`. The plan and design are in [docs/](docs/README.md).

## Install

Install the binary from the latest release:

```bash
go install github.com/rrgmc/ai-specview/cmd/ai-specview@latest
```

Releases are git tags such as `v0.5.0` (see the
[tags](https://github.com/rrgmc/ai-specview/tags)). To install one release, put
its tag after the `@`:

```bash
go install github.com/rrgmc/ai-specview/cmd/ai-specview@v0.5.0
```

Each release on the
[releases page](https://github.com/rrgmc/ai-specview/releases) also has
prebuilt binaries for Linux and macOS. `@master` installs the newest commit
instead. `ai-specview version` prints the
installed version. After an upgrade, run `ai-specview serve --restart` so a
running daemon switches to the new binary.

Then install the Claude Code plugin. It registers the MCP server, the
`spec-elaborate` skill and the `ai-specview hook` (below) in one step:

```bash
claude plugin marketplace add rrgmc/ai-specview
```

```bash
claude plugin install ai-specview@ai-specview
```

In a session the same works with `/plugin marketplace add …` and
`/plugin install ai-specview@ai-specview`. In the desktop app, use **+** next to the
prompt, then **Add plugins**.

The plugin finds the binary on the `PATH`, in `$GOBIN`, in `$(go env GOPATH)/bin`
or in `~/go/bin`. Set `AI_SPECVIEW_BIN` to use another path. The plugin's skill is
named `ai-specview:spec-elaborate`.

If you set ai-specview up by hand before, remove the old pieces so they don't run
twice: `claude mcp remove ai-specview`, the `ai-specview hook` entry in
`~/.claude/settings.json`, and `~/.claude/skills/spec-elaborate`.

Also raise the MCP tool timeout (see below); a plugin can't set it.

### Optional: the pane

`ai-specview-pane` is a Claude Code mod that shows the spec and the code in a
pane beside the chat, as well as or instead of the browser page:

```bash
claude plugin install ai-specview-pane@ai-specview
```

Its `mode` option is `both` (default: the browser page and the pane), `pane`
(the pane alone for `spec_show` and `focus`) or `browser`. `/specview-mode`
changes it for one session. See [plugin-pane/README.md](plugin-pane/README.md)
and [docs/06-pane-mod.md](docs/06-pane-mod.md).

### Manual setup

Without the plugin, the binary must be on the `PATH` that Claude Code sees.

Register the MCP server for every project:

```bash
claude mcp add --scope user ai-specview -- ai-specview mcp
```

Or per project, in `.mcp.json` (see [examples/mcp.example.json](examples/mcp.example.json)):

```json
{
  "mcpServers": {
    "ai-specview": {
      "command": "ai-specview",
      "args": ["mcp"],
      "env": { "AI_SPECVIEW_ASK_WAIT": "600s" }
    }
  }
}
```

Raise the MCP tool timeout so a question can wait for the user. Set it in your
shell profile or in Claude Code's settings `env`:

```json
{ "env": { "MCP_TOOL_TIMEOUT": "1800000" } }
```

`AI_SPECVIEW_ASK_WAIT` must stay **below** `MCP_TOOL_TIMEOUT`. Then a long wait
ends with a `pending` result that Claude continues with `ask_wait`, instead of a
timeout error.

To keep Claude from falling back to its built-in question tool while a spec
is open, add the `ai-specview hook` PreToolUse hook to `~/.claude/settings.json`
(see [examples/settings.example.json](examples/settings.example.json)). It blocks
`AskUserQuestion` only in the Claude session that has a spec open in ai-specview,
so other sessions are not affected. Don't deny `AskUserQuestion` globally.

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "AskUserQuestion",
        "hooks": [{ "type": "command", "command": "ai-specview hook" }]
      }
    ]
  }
}
```

Copy [plugin/skills/spec-elaborate/](plugin/skills/spec-elaborate/) into
`.claude/skills/` (project) or `~/.claude/skills/` (user).

## Use it

1. Optionally add [examples/CLAUDE.md.snippet](examples/CLAUDE.md.snippet) to
   your `CLAUDE.md`.
2. Ask Claude to spec a change. The skill calls `spec_show`; the bridge starts
   the daemon if needed and Claude tells you the page URL
   (`http://127.0.0.1:7878/w/<workspace>`).
3. Each question arrives with its spec passage and code highlighted. Browse
   freely, then answer in the panel, choose "Decide later", go "Back to chat",
   or "Skip".

`http://127.0.0.1:7878/` lists all sessions. Each Claude instance gets its own
workspace page; ended sessions stay readable, and `claude --resume` in the same
directory within 2 hours takes the old workspace over.

## Commands

| Command | What it does |
|---|---|
| `ai-specview mcp` | The stdio MCP server. Claude Code starts one per instance. |
| `ai-specview serve` | The viewer daemon. `--detach` starts it in the background, `--restart` replaces a running one (or starts one) in the background, `--stop` stops it, `--addr` sets the address (default `127.0.0.1:7878`, the next 9 ports are tried), `--idle-exit 30m` exits when unused. |
| `ai-specview fakeapp` | A terminal stand-in for the daemon, for testing the MCP path. |
| `ai-specview hook` | Claude Code PreToolUse hook: blocks the built-in question tool only while this session has a spec open. |
| `ai-specview version` | Prints the version: the git tag (`v1.2.3`), a Go pseudo-version for a build between tags, with `+dirty` for uncommitted changes, or `devel` when the build has no version information. |

## Environment

| Variable | Default | Meaning |
|---|---|---|
| `AI_SPECVIEW_ASK_WAIT` | `10m` | How long `ask` blocks before returning `pending`. |
| `AI_SPECVIEW_OPEN` | | `never` stops the bridge from opening the browser when no page is open. |
| `AI_SPECVIEW_SOCKET` | see below | Socket path shared by bridge and daemon. |
| `AI_SPECVIEW_ADDR` | `127.0.0.1:7878` | Daemon HTTP address. |
| `AI_SPECVIEW_DAEMON` | | Command the bridge runs instead of `ai-specview serve --detach` (for example `ai-specview fakeapp`). |

The socket is `$XDG_RUNTIME_DIR/ai-specview.sock` or, on macOS,
`~/Library/Caches/ai-specview/ai-specview.sock`. The detached daemon logs to
`ai-specview.log` in the same cache directory.

After upgrading, run `ai-specview serve --restart` so the running daemon matches
the new bridge.

## MCP tools

| Tool | Purpose |
|---|---|
| `spec_show` | Show a markdown file or a folder of artifacts; returns the section names and the page URL. |
| `focus` | Highlight a spec passage and open code without asking. |
| `ask` | Ask 1–6 questions with spec, doc and code refs; blocks until answered (or `pending`). |
| `ask_wait` | Keep waiting on a `pending` question set. |
| `spec_done` | Mark the spec finished; the page shows a "Spec finished" banner with the summary. |

The full contract is in [docs/01-protocol.md](docs/01-protocol.md).

## Develop

```bash
go test ./...
```

Golden protocol fixtures are in `protocol/fixtures/`, quote-matching fixtures
in `protocol/quotes/`. The browser tests in `internal/daemon/browser_test.go`
drive the question panel in headless Chrome; they are skipped when Chrome is
not installed, or with `go test -short`. `internal/e2e/multi_test.go` runs three
bridge processes against a real daemon, restarts and kills it, and resumes a
session (also skipped with `-short`).

Check the plugin with `claude plugin validate ./plugin` and the marketplace
with `claude plugin validate .`. The pane mod has its own checks:
`claude plugin validate ./plugin-pane` and `claude plugin test ./plugin-pane`.

## Status

The POC (milestones M0–M7 in [docs/05-milestones.md](docs/05-milestones.md)) is
implemented. Differences from the plan:

- The front end is plain JavaScript (about 400 lines) instead of htmx, so
  nothing third-party is vendored.
- The web server, templates and assets live in `internal/daemon` instead of a
  separate `internal/web` package.
- The daemon also answers `ping` and `shutdown` messages, used by
  `serve` for its single-instance check and `--restart`.

The pane mod (`plugin-pane/`, [docs/06-pane-mod.md](docs/06-pane-mod.md)) is a
prototype: it does not draw questions yet.

Not done yet: the change-folder skill steps and the `openspec status`
integration (M8), the handoff command (M9), and the dogfood sessions the
milestones ask for. The status per
milestone is in [docs/05-milestones.md](docs/05-milestones.md).

## License

MIT. See [LICENSE](LICENSE).
