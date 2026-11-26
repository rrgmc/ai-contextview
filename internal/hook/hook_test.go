package hook

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rrgmc/ai-specview/internal/daemon"
	"github.com/rrgmc/ai-specview/internal/identity"
	"github.com/rrgmc/ai-specview/internal/protocol"
	"github.com/rrgmc/ai-specview/internal/testenv"
)

const askEvent = `{"session_id":"s","cwd":"/x","hook_event_name":"PreToolUse","tool_name":"AskUserQuestion","tool_input":{}}`

// claudePID stands in for the Claude Code process that started the bridge.
const claudePID = 424242

type env struct {
	t    *testing.T
	sock string
	conn net.Conn
	sc   *bufio.Scanner
	n    int
}

func startDaemon(t *testing.T) *env {
	dir := testenv.ShortTempDir(t, "svh")
	sock := filepath.Join(dir, "d.sock")
	d := daemon.New("test")
	t.Cleanup(d.RequestShutdown)
	d.BaseURL = "http://127.0.0.1:1"
	ln, err := daemon.ListenSocket(sock)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() { _ = d.ServeSocket(ln) }()
	return &env{t: t, sock: sock}
}

// bridge connects as a bridge started by claudePID.
func (e *env) bridge(cwd string) {
	c, err := net.Dial("unix", e.sock)
	require.NoError(e.t, err)
	e.t.Cleanup(func() { _ = c.Close() })
	e.conn, e.sc = c, bufio.NewScanner(c)
	e.req(protocol.TypeHello, &protocol.Hello{Protocol: 1, WS: "ws_h", Cwd: cwd, BridgePPID: claudePID})
}

func (e *env) req(typ string, body any) protocol.Envelope {
	e.n++
	id := "m" + string(rune('0'+e.n))
	env, err := protocol.NewEnvelope(typ, id, "", "ws_h", body)
	require.NoError(e.t, err)
	b, _ := json.Marshal(env)
	_, err = e.conn.Write(append(b, '\n'))
	require.NoError(e.t, err)
	_ = e.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for e.sc.Scan() {
		var r protocol.Envelope
		require.NoError(e.t, json.Unmarshal(e.sc.Bytes(), &r))
		if r.Re == id {
			return r
		}
	}
	e.t.Fatalf("no reply to %s", typ)
	return protocol.Envelope{}
}

func run(sock string, event string, pids []int) (int, string) {
	var errOut bytes.Buffer
	code := Run(strings.NewReader(event), &errOut, sock, pids)
	return code, errOut.String()
}

func TestBlocksOnlyWhileSpecOpen(t *testing.T) {
	e := startDaemon(t)
	cwd := t.TempDir()
	spec := filepath.Join(cwd, "spec.md")
	require.NoError(t, os.WriteFile(spec, []byte("# Spec\n\nText.\n"), 0o644))
	ancestors := []int{1111, claudePID, 1}

	// Connected, but no spec shown yet: allow.
	e.bridge(cwd)
	code, _ := run(e.sock, askEvent, ancestors)
	assert.Equal(t, ExitAllow, code)

	// Spec shown: block, with the reason for Claude.
	e.req(protocol.TypeSpecShow, &protocol.SpecShow{RootAbs: spec})
	code, msg := run(e.sock, askEvent, ancestors)
	assert.Equal(t, ExitBlock, code)
	assert.Contains(t, msg, "ai-specview `ask` tool")

	// Another Claude session (different ancestors): allow.
	code, _ = run(e.sock, askEvent, []int{2222, 3333})
	assert.Equal(t, ExitAllow, code)

	// Other tools are never blocked.
	code, _ = run(e.sock, `{"tool_name":"Bash"}`, ancestors)
	assert.Equal(t, ExitAllow, code)

	// Spec finished: allow again.
	e.req(protocol.TypeDone, &protocol.Done{})
	code, _ = run(e.sock, askEvent, ancestors)
	assert.Equal(t, ExitAllow, code)
}

func TestAllowsWithoutDaemonOrInput(t *testing.T) {
	code, msg := run("/tmp/does-not-exist-specview.sock", askEvent, []int{claudePID})
	assert.Equal(t, ExitAllow, code)
	assert.Empty(t, msg)
	code, _ = run("/tmp/does-not-exist-specview.sock", "not json", nil)
	assert.Equal(t, ExitAllow, code)
}

func TestAncestorPIDs(t *testing.T) {
	pids := identity.AncestorPIDs(16)
	require.NotEmpty(t, pids)
	assert.Equal(t, os.Getppid(), pids[0])
}

func TestBlocksInAskMode(t *testing.T) {
	e := startDaemon(t)
	cwd := t.TempDir()
	spec := filepath.Join(cwd, "spec.md")
	require.NoError(t, os.WriteFile(spec, []byte("# Spec\n\nText.\n"), 0o644))
	ancestors := []int{1111, claudePID, 1}
	e.bridge(cwd)

	// Ask mode with no spec: block, with the ask-mode reason.
	e.req(protocol.TypeAskMode, &protocol.AskMode{On: true})
	code, msg := run(e.sock, askEvent, ancestors)
	assert.Equal(t, ExitBlock, code)
	assert.Equal(t, ReasonAskMode+"\n", msg)

	// A spec open in ask mode: the spec reason.
	e.req(protocol.TypeSpecShow, &protocol.SpecShow{RootAbs: spec})
	code, msg = run(e.sock, askEvent, ancestors)
	assert.Equal(t, ExitBlock, code)
	assert.Equal(t, Reason+"\n", msg)

	// spec_done does not end ask mode.
	e.req(protocol.TypeDone, &protocol.Done{})
	code, msg = run(e.sock, askEvent, ancestors)
	assert.Equal(t, ExitBlock, code)
	assert.Equal(t, ReasonAskMode+"\n", msg)

	// Ask mode off: allow.
	e.req(protocol.TypeAskMode, &protocol.AskMode{On: false})
	code, _ = run(e.sock, askEvent, ancestors)
	assert.Equal(t, ExitAllow, code)
}
