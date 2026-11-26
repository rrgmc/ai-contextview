# ai-specview — implementation plan

ai-specview shows **the spec and the code side by side** while Claude elaborates a
spec. When a spec-writing skill creates or edits the spec, ai-specview displays it
right away. When the skill asks a question, ai-specview highlights the part of the
spec being asked about, opens the code it refers to at the right line, and shows
the question in a docked, non-modal panel.

The viewer is a **local web page** served by a Go daemon; the MCP bridge and the
daemon are one Go binary, `ai-specview`.

## Documents

| # | File | What it covers |
|---|------|----------------|
| 0 | [00-overview.md](00-overview.md) | Goals, non-goals, architecture, key decisions, future change-folder workflow |
| 1 | [01-protocol.md](01-protocol.md) | **The contract**: MCP tool schemas and the bridge↔daemon socket protocol |
| 2 | [02-bridge-go.md](02-bridge-go.md) | `ai-specview mcp`: the per-Claude-instance stdio MCP server |
| 3 | [03-app-web.md](03-app-web.md) | `ai-specview serve`: the Go web viewer daemon (UI + state) |
| 4 | [04-skill-integration.md](04-skill-integration.md) | Wiring it into Claude Code: MCP config, skill rules, hooks |
| 5 | [05-milestones.md](05-milestones.md) | Build order, acceptance criteria, testing, risks |
| 6 | [06-pane-mod.md](06-pane-mod.md) | The pane: the Claude Code mod and the `spec-elaborate-mod` skill that show the spec, code and questions beside the chat |
| 7 | [07-layers.md](07-layers.md) | Proposed: questions with context first, then a working document, then the spec workflow; the document panel and how the user selects a layer |

Read 00 → 01 first. 02 and 03 can then be built in parallel against 01.

## Repository layout (target)

```
ai-specview/
├── docs/                 # this plan
├── go.mod                # one module: github.com/rrgmc/ai-specview
├── cmd/ai-specview/         # subcommands: mcp, serve, fakeapp, hook, version
├── internal/             # protocol, specsrc, resolve, daemon, render, hook, …
├── protocol/             # golden message fixtures + quote fixtures
├── .claude-plugin/       # marketplace.json: this repo is a plugin marketplace
├── plugin/               # the Claude Code plugin: MCP server, skills, hook, pane mod
└── examples/             # manual setup: .mcp.json, settings, CLAUDE.md snippet
```
