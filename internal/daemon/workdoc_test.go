package daemon

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rrgmc/ai-specview/internal/protocol"
)

const testPID = 515151

// workSetup starts a workspace owned by testPID in a project with three
// markdown files.
func workSetup(t *testing.T) (*harness, *bridge, string) {
	h := newHarness(t)
	cwd := t.TempDir()
	for name, body := range map[string]string{
		"docs/specs/SW-1.md": "# SW-1\n\n## Requirements\n\nR1.\n\n## Design\n\nD.\n",
		"docs/other.md":      "# Other\n\nText.\n",
		"README.md":          "# Readme\n\nHello.\n",
	} {
		p := filepath.Join(cwd, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	b := h.bridge("ws_A", cwd)
	h.d.mu.Lock()
	h.d.workspaces["ws_A"].ClaudePID = testPID
	h.d.mu.Unlock()
	return h, b, cwd
}

func written(h *harness, path string) *protocol.DocWrittenResult {
	return h.d.docWritten(&protocol.DocWritten{PIDs: []int{999, testPID, 1}, Path: path})
}

func workingDoc(h *harness) string {
	h.d.mu.Lock()
	defer h.d.mu.Unlock()
	w := h.d.workspaces["ws_A"]
	if w.Root == nil {
		return ""
	}
	return w.Root.PrimaryAbs()
}

func TestAskModeFirstWrittenDocStays(t *testing.T) {
	h, b, cwd := workSetup(t)
	readme := filepath.Join(cwd, "README.md")
	spec := filepath.Join(cwd, "docs/specs/SW-1.md")

	// Not in ask mode: ignored.
	assert.False(t, written(h, readme).Shown)
	assert.Empty(t, workingDoc(h))

	b.req(protocol.TypeAskMode, &protocol.AskMode{On: true})
	assert.True(t, written(h, spec).Shown)
	assert.Equal(t, spec, workingDoc(h))
	// Another write does not switch.
	assert.False(t, written(h, readme).Shown)
	assert.Equal(t, spec, workingDoc(h))
	// Another session's write is not ours.
	assert.False(t, h.d.docWritten(&protocol.DocWritten{PIDs: []int{4}, Path: readme}).Shown)

	// A question that quotes the other written file switches to it.
	b.req(protocol.TypeAsk, &protocol.Ask{QSID: "qs_1", Questions: []protocol.AskQuestion{{
		ID: "q1", Text: "Ok?", Options: []protocol.Option{{Label: "Yes"}, {Label: "No"}},
		Docs: []protocol.ResolvedDocRef{{Path: readme, Quote: "Hello.", Kind: protocol.KindOther}},
	}}})
	assert.Equal(t, readme, workingDoc(h))
}

func TestExplicitSpecShowWins(t *testing.T) {
	h, b, cwd := workSetup(t)
	other := filepath.Join(cwd, "docs/other.md")
	b.req(protocol.TypeAskMode, &protocol.AskMode{On: true})
	b.req(protocol.TypeSpecShow, &protocol.SpecShow{RootAbs: other})
	assert.False(t, written(h, filepath.Join(cwd, "docs/specs/SW-1.md")).Shown)
	assert.Equal(t, other, workingDoc(h))

	// A replay of a picked document (auto) does not count as explicit.
	b.req(protocol.TypeSpecShow, &protocol.SpecShow{RootAbs: other, Auto: true})
	h.d.mu.Lock()
	assert.False(t, h.d.workspaces["ws_A"].work.explicit)
	h.d.mu.Unlock()
}

func TestSpecModeWithPaths(t *testing.T) {
	h, b, cwd := workSetup(t)
	b.req(protocol.TypeAskMode, &protocol.AskMode{On: true, Spec: true, Paths: []string{filepath.Join(cwd, "docs/specs/**")}})
	assert.False(t, written(h, filepath.Join(cwd, "README.md")).Shown)
	assert.True(t, written(h, filepath.Join(cwd, "docs/specs/SW-1.md")).Shown)
}

func TestSpecModeScoring(t *testing.T) {
	h, b, cwd := workSetup(t)
	b.req(protocol.TypeAskMode, &protocol.AskMode{On: true, Spec: true})
	readme := filepath.Join(cwd, "README.md")
	spec := filepath.Join(cwd, "docs/specs/SW-1.md")
	// README: no signal. Written twice, it still scores 1.
	assert.False(t, written(h, readme).Shown)
	assert.False(t, written(h, readme).Shown)
	// The spec: a spec folder and spec headings, 2.
	assert.True(t, written(h, spec).Shown)

	// A spec skill alone reaches the threshold.
	h2, b2, cwd2 := workSetup(t)
	b2.req(protocol.TypeAskMode, &protocol.AskMode{On: true, Spec: true})
	h2.d.skillStarted(&protocol.SkillStarted{PIDs: []int{testPID}, Name: "harness-core:harness-elaborate-spec"})
	assert.True(t, written(h2, filepath.Join(cwd2, "docs/other.md")).Shown)

	// Turning ask mode off resets the pick.
	b.req(protocol.TypeAskMode, &protocol.AskMode{On: false})
	h.d.mu.Lock()
	assert.Empty(t, h.d.workspaces["ws_A"].work.doc)
	assert.False(t, h.d.workspaces["ws_A"].work.spec)
	h.d.mu.Unlock()
}

func TestPickedDocIsNotAnOpenSpec(t *testing.T) {
	h, b, cwd := workSetup(t)
	pids := []int{999, testPID, 1}
	b.req(protocol.TypeAskMode, &protocol.AskMode{On: true})
	require.True(t, written(h, filepath.Join(cwd, "docs/specs/SW-1.md")).Shown)
	assert.True(t, h.d.active(pids, "").Active, "ask mode is on")
	assert.True(t, h.d.active(pids, "").AskMode)

	// Ask mode off: the picked document is cleared and blocks nothing.
	b.req(protocol.TypeAskMode, &protocol.AskMode{On: false})
	assert.Empty(t, workingDoc(h))
	assert.False(t, h.d.active(pids, "").Active)

	// Claude's own spec_show is an open spec again.
	b.req(protocol.TypeSpecShow, &protocol.SpecShow{RootAbs: filepath.Join(cwd, "docs/other.md")})
	assert.True(t, h.d.active(pids, "").Active)
	assert.False(t, h.d.active(pids, "").AskMode)
}

func TestAskModeOffKeepsExplicitSpec(t *testing.T) {
	h, b, cwd := workSetup(t)
	other := filepath.Join(cwd, "docs/other.md")
	b.req(protocol.TypeAskMode, &protocol.AskMode{On: true})
	b.req(protocol.TypeSpecShow, &protocol.SpecShow{RootAbs: other})
	b.req(protocol.TypeAskMode, &protocol.AskMode{On: false})
	assert.Equal(t, other, workingDoc(h))
}

func TestSessionCwdFromHook(t *testing.T) {
	h, b, cwd := workSetup(t)
	wt := filepath.Join(cwd, ".claude", "worktrees", "x")
	b.req(protocol.TypeAskMode, &protocol.AskMode{On: true})
	spec := filepath.Join(cwd, "docs/specs/SW-1.md")
	sub := h.d.hub.Subscribe("ws_A")
	t.Cleanup(func() { h.d.hub.Unsubscribe("ws_A", sub) })
	res := h.d.docWritten(&protocol.DocWritten{PIDs: []int{testPID}, Path: spec, Cwd: wt})
	assert.True(t, res.Shown)
	h.d.mu.Lock()
	w := h.d.workspaces["ws_A"]
	assert.Equal(t, wt, w.SessionCwd)
	assert.Equal(t, wt, w.projectDir())
	h.d.mu.Unlock()

	// The bridge gets session_cwd once, and a focus event reaches the page.
	var types []string
	_ = b.c.SetReadDeadline(time.Now().Add(2 * time.Second))
	for len(types) < 2 && b.sc.Scan() {
		var env protocol.Envelope
		require.NoError(t, json.Unmarshal(b.sc.Bytes(), &env))
		types = append(types, env.Type)
	}
	assert.Contains(t, types, protocol.TypeSessionCwd)
	assert.Contains(t, types, protocol.TypeDocShown)

	// The open page is told to load the picked document.
	var focus []byte
	for len(sub.ch) > 0 {
		if ev := <-sub.ch; ev.Name == "focus" {
			focus = ev.Data
		}
	}
	assert.Contains(t, string(focus), `"path":"SW-1.md"`)
}

func TestSessionCwdBeforeHello(t *testing.T) {
	h := newHarness(t)
	cwd := t.TempDir()
	wt := filepath.Join(cwd, ".claude", "worktrees", "x")
	// The hook reports the folder before the session's bridge connected.
	h.d.sessionCwd(&protocol.SessionCwd{PIDs: []int{999, testPID, 1}, Cwd: wt})

	c, err := net.Dial("unix", h.sock)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	sc := bufio.NewScanner(c)
	sc.Buffer(make([]byte, 64*1024), protocol.MaxLine)
	b := &bridge{t: t, c: c, sc: sc, ws: "ws_A"}
	reply := b.req(protocol.TypeHello, &protocol.Hello{Protocol: 1, WS: "ws_A", Cwd: cwd, ClaudePID: testPID, ProjectName: "p"})
	require.Equal(t, protocol.TypeHelloAck, reply.Type)

	// The bridge's cwd request gets the folder.
	var res protocol.CwdResult
	env := b.req(protocol.TypeCwd, &protocol.Cwd{})
	require.NoError(t, env.Decode(&res))
	assert.Equal(t, wt, res.Cwd)

	// The folder was used once.
	h.d.mu.Lock()
	assert.Empty(t, h.d.pendingCwd)
	h.d.mu.Unlock()
}

func TestCwdWithoutSessionFolder(t *testing.T) {
	_, b, _ := workSetup(t)
	var res protocol.CwdResult
	env := b.req(protocol.TypeCwd, &protocol.Cwd{})
	require.NoError(t, env.Decode(&res))
	assert.Empty(t, res.Cwd)
}
