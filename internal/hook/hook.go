// Package hook is `ai-specview hook`, a Claude Code hook for three events:
//
//   - PreToolUse on AskUserQuestion: it blocks the built-in question tool
//     only while this Claude session has a spec open in ai-specview or is in
//     ask mode, and tells Claude to use ai-specview's ask tool instead. In
//     every other case it allows the tool, so other sessions and spec-free
//     work keep the built-in question tool.
//   - PostToolUse on Write, Edit and MultiEdit: it reports a markdown file
//     Claude wrote, so the daemon can make it the working document (07).
//   - PreToolUse on Skill: it reports the skill Claude starts, a signal for
//     spec mode.
//
// It never blocks the last two, and any failure allows the tool.
package hook

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rrgmc/ai-specview/internal/glob"
	"github.com/rrgmc/ai-specview/internal/protocol"
	"github.com/rrgmc/ai-specview/internal/specsrc"
)

// BuiltinAskTool is the built-in question tool's name.
const BuiltinAskTool = "AskUserQuestion"

// Exit codes. Claude Code blocks the tool call on exit code 2 and shows
// stderr to Claude.
const (
	ExitAllow = 0
	ExitBlock = 2
)

// Reason is shown to Claude when the built-in tool is blocked.
const Reason = "A spec is open in ai-specview for this session. Ask this with the ai-specview `ask` tool " +
	"instead of AskUserQuestion: quote the spec verbatim with spec_quote and " +
	"spec_section, and point at the code with code_refs."

// ReasonAskMode is shown instead of Reason when the session is in ask mode
// with no spec open.
const ReasonAskMode = "Ask mode is on in ai-specview for this session. Ask this with the ai-specview `ask` tool " +
	"instead of AskUserQuestion. Refs are optional: add code_refs or doc_refs when the question is about them."

// timeout bounds the daemon query; a slow or missing daemon must never hold
// up Claude.
const timeout = 800 * time.Millisecond

type input struct {
	Event     string `json:"hook_event_name"`
	ToolName  string `json:"tool_name"`
	Cwd       string `json:"cwd"`
	ToolInput struct {
		FilePath string `json:"file_path"`
		Skill    string `json:"skill"`
	} `json:"tool_input"`
}

// postOutput is the PostToolUse answer that adds a line to Claude's context.
type postOutput struct {
	HookSpecificOutput struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

// Run reads the hook event from in and returns the exit code. pids are this
// process's ancestors, which include the Claude Code process.
func Run(in io.Reader, out, errOut io.Writer, socket string, pids []int) int {
	var ev input
	if err := json.NewDecoder(io.LimitReader(in, 4<<20)).Decode(&ev); err != nil {
		return ExitAllow
	}
	switch {
	case ev.Event == "PostToolUse" && (ev.ToolName == "Write" || ev.ToolName == "Edit" || ev.ToolName == "MultiEdit"):
		written(out, socket, pids, &ev)
		return ExitAllow
	case ev.ToolName == "Skill":
		if ev.ToolInput.Skill != "" {
			var res map[string]any
			_ = request(socket, protocol.TypeSkillStarted, &protocol.SkillStarted{PIDs: pids, Name: ev.ToolInput.Skill}, &res)
		}
		return ExitAllow
	case ev.ToolName != BuiltinAskTool:
		return ExitAllow
	}
	res, err := query(socket, pids)
	if err != nil || !res.Active {
		return ExitAllow
	}
	if res.AskMode {
		fmt.Fprintln(errOut, ReasonAskMode)
	} else {
		fmt.Fprintln(errOut, Reason)
	}
	return ExitBlock
}

// written reports a file Claude wrote, when it counts as a document, and
// tells Claude when it became the working document.
func written(out io.Writer, socket string, pids []int, ev *input) {
	path := ev.ToolInput.FilePath
	if path == "" {
		return
	}
	if !filepath.IsAbs(path) && ev.Cwd != "" {
		path = filepath.Join(ev.Cwd, path)
	}
	home, _ := os.UserHomeDir()
	if !Counts(path, home, docPaths(ev.Cwd, os.Getenv("AI_SPECVIEW_DOC_PATHS"))) {
		return
	}
	var res protocol.DocWrittenResult
	if err := request(socket, protocol.TypeDocWritten, &protocol.DocWritten{PIDs: pids, Path: filepath.Clean(path)}, &res); err != nil || !res.Shown {
		return
	}
	var o postOutput
	o.HookSpecificOutput.HookEventName = "PostToolUse"
	o.HookSpecificOutput.AdditionalContext = fmt.Sprintf("ai-specview now shows %s as the working document: "+
		"the user sees it beside the questions, and spec_quote quotes it. Call spec_show to show another file instead.", path)
	_ = json.NewEncoder(out).Encode(&o)
}

// docPaths turns AI_SPECVIEW_DOC_PATHS (comma-separated globs, relative to
// cwd or absolute) into absolute patterns.
func docPaths(cwd, env string) []string {
	var out []string
	for _, p := range strings.Split(env, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !filepath.IsAbs(p) && cwd != "" {
			p = filepath.Join(cwd, p)
		}
		out = append(out, p)
	}
	return out
}

// Counts reports whether a written file can become the working document. A
// markdown file counts unless it is bookkeeping: CLAUDE.md and AGENTS.md,
// changelogs, Claude's memory, and files in dot-folders other than
// .claude/specs. Under ~/.claude only specs/ and plans/ count. Non-empty
// patterns replace these rules: then only a matching file counts.
func Counts(path, home string, patterns []string) bool {
	path = filepath.Clean(path)
	if !specsrc.IsMarkdown(path) {
		return false
	}
	if len(patterns) > 0 {
		return glob.MatchAny(patterns, path)
	}
	switch strings.ToLower(filepath.Base(path)) {
	case "claude.md", "claude.local.md", "agents.md", "changelog.md":
		return false
	}
	if home != "" {
		claude := filepath.Join(home, ".claude") + string(filepath.Separator)
		if strings.HasPrefix(path, claude) {
			rest := filepath.ToSlash(strings.TrimPrefix(path, claude))
			return strings.HasPrefix(rest, "specs/") || strings.HasPrefix(rest, "plans/")
		}
	}
	parts := strings.Split(filepath.ToSlash(filepath.Dir(path)), "/")
	for i, part := range parts {
		if part == "changelog.d" {
			return false
		}
		if strings.HasPrefix(part, ".") && part != "." && part != ".." {
			if part == ".claude" && i+1 < len(parts) && parts[i+1] == "specs" {
				continue
			}
			return false
		}
	}
	return true
}

// query asks the daemon whether a spec is open for one of pids.
func query(socket string, pids []int) (*protocol.ActiveResult, error) {
	var res protocol.ActiveResult
	if err := request(socket, protocol.TypeActive, &protocol.Active{PIDs: pids}, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// request sends one message to the daemon and decodes its result into res.
// An error reply (an older daemon that does not know the message) leaves
// res as it is.
func request(socket, typ string, body, res any) error {
	c, err := net.DialTimeout("unix", socket, timeout)
	if err != nil {
		return err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(timeout))
	env, err := protocol.NewEnvelope(typ, "h1", "", "", body)
	if err != nil {
		return err
	}
	b, err := json.Marshal(env)
	if err != nil {
		return err
	}
	if _, err := c.Write(append(b, '\n')); err != nil {
		return err
	}
	sc := bufio.NewScanner(c)
	sc.Buffer(make([]byte, 64*1024), protocol.MaxLine)
	if !sc.Scan() {
		return fmt.Errorf("no reply from daemon")
	}
	var reply protocol.Envelope
	if err := json.Unmarshal(sc.Bytes(), &reply); err != nil {
		return err
	}
	if reply.Type != protocol.TypeResult {
		return nil
	}
	return reply.Decode(res)
}
