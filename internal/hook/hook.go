// Package hook is `ai-specview hook`, a Claude Code PreToolUse hook. It blocks
// the built-in question tool (AskUserQuestion) only while this Claude session
// has a spec open in ai-specview, and tells Claude to use ai-specview's ask tool
// instead. In every other case it allows the tool, so other sessions and
// spec-free work keep the built-in question tool.
package hook

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/rrgmc/ai-specview/internal/protocol"
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
	ToolName string `json:"tool_name"`
}

// Run reads the PreToolUse event from in and returns the exit code. pids are
// this process's ancestors, which include the Claude Code process.
func Run(in io.Reader, errOut io.Writer, socket string, pids []int) int {
	var ev input
	if err := json.NewDecoder(io.LimitReader(in, 4<<20)).Decode(&ev); err != nil {
		return ExitAllow
	}
	if ev.ToolName != BuiltinAskTool {
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

// query asks the daemon whether a spec is open for one of pids.
func query(socket string, pids []int) (*protocol.ActiveResult, error) {
	c, err := net.DialTimeout("unix", socket, timeout)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(timeout))
	env, err := protocol.NewEnvelope(protocol.TypeActive, "h1", "", "", &protocol.Active{PIDs: pids})
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}
	if _, err := c.Write(append(b, '\n')); err != nil {
		return nil, err
	}
	sc := bufio.NewScanner(c)
	sc.Buffer(make([]byte, 64*1024), protocol.MaxLine)
	if !sc.Scan() {
		return nil, fmt.Errorf("no reply from daemon")
	}
	var reply protocol.Envelope
	if err := json.Unmarshal(sc.Bytes(), &reply); err != nil {
		return nil, err
	}
	if reply.Type != protocol.TypeResult {
		// An older daemon does not know "active": allow.
		return &protocol.ActiveResult{}, nil
	}
	var res protocol.ActiveResult
	if err := reply.Decode(&res); err != nil {
		return nil, err
	}
	return &res, nil
}
