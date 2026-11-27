package e2e

// TestMultiSession is the M7 scripted test with real processes: the ai-contextview
// binary is built, three `ai-contextview mcp` bridges run as separate processes in
// three projects (as three Claude sessions would start them), and the
// bridges start the daemon themselves. While all three have a question
// pending, the daemon is restarted and then killed. Each question must come
// back on its own page, and each answer must reach its own session. Last,
// one session exits and a new bridge in the same directory takes its
// workspace over (`claude --resume`).
//
// It is skipped with `go test -short`.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rrgmc/ai-contextview/internal/protocol"
	"github.com/rrgmc/ai-contextview/internal/testenv"
)

// multiEnv is the environment shared by the bridges and the daemon: its own
// socket, port, home (for the log file) and no browser.
type multiEnv struct {
	t    *testing.T
	dir  string
	bin  string
	sock string
	addr string
	home string
}

func (e *multiEnv) environ() []string {
	return append(os.Environ(),
		"AI_CONTEXTVIEW_SOCKET="+e.sock,
		"AI_CONTEXTVIEW_ADDR="+e.addr,
		"AI_CONTEXTVIEW_OPEN=never",
		"AI_CONTEXTVIEW_ASK_WAIT=30s",
		"HOME="+e.home,
		"XDG_CACHE_HOME="+filepath.Join(e.home, ".cache"),
		"LOCALAPPDATA="+filepath.Join(e.home, "AppData", "Local"),
	)
}

func (e *multiEnv) base() string { return "http://" + e.addr }

func newMultiEnv(t *testing.T) *multiEnv {
	if testing.Short() {
		t.Skip("multi-process test skipped with -short")
	}
	dir := testenv.ShortTempDir(t, "svm")

	wd, err := os.Getwd()
	require.NoError(t, err)
	root := wd
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		require.NotEqual(t, parent, root, "go.mod not found")
		root = parent
	}
	bin := filepath.Join(dir, "ai-contextview")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, "./cmd/ai-contextview")
	build.Dir = root
	out, err := build.CombinedOutput()
	require.NoError(t, err, string(out))

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	e := &multiEnv{t: t, dir: dir, bin: bin, sock: filepath.Join(dir, "s.sock"), addr: addr, home: filepath.Join(dir, "home")}
	require.NoError(t, os.MkdirAll(e.home, 0o755))
	t.Cleanup(func() { _, _ = e.run("serve", "--stop") })
	// On failure, show the daemon and bridge logs (this runs before the
	// directory is removed).
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		logs, _ := filepath.Glob(filepath.Join(dir, "*.log"))
		logs = append(logs, filepath.Join(e.home, "Library", "Caches", "ai-contextview", "ai-contextview.log"),
			filepath.Join(e.home, ".cache", "ai-contextview", "ai-contextview.log"),
			filepath.Join(e.home, "AppData", "Local", "ai-contextview", "ai-contextview.log"))
		for _, l := range logs {
			if b, err := os.ReadFile(l); err == nil {
				t.Logf("--- %s\n%s", filepath.Base(l), b)
			}
		}
	})
	return e
}

// run runs the ai-contextview binary with the shared environment.
func (e *multiEnv) run(args ...string) (string, error) {
	cmd := exec.Command(e.bin, args...)
	cmd.Env = e.environ()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// ping returns the running daemon's pid, or 0.
func (e *multiEnv) ping() int {
	c, err := net.DialTimeout("unix", e.sock, time.Second)
	if err != nil {
		return 0
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	env, _ := protocol.NewEnvelope(protocol.TypePing, "p", "", "", map[string]any{})
	b, _ := json.Marshal(env)
	if _, err := c.Write(append(b, '\n')); err != nil {
		return 0
	}
	sc := bufio.NewScanner(c)
	if !sc.Scan() {
		return 0
	}
	var reply protocol.Envelope
	var res protocol.PingResult
	if json.Unmarshal(sc.Bytes(), &reply) != nil || reply.Decode(&res) != nil {
		return 0
	}
	return res.PID
}

// project is one Claude session: a project directory and its bridge process.
type project struct {
	name    string
	cwd     string
	session *mcp.ClientSession
	ws      string
}

func (e *multiEnv) project(name string) *project {
	cwd := filepath.Join(e.home, "src", name)
	require.NoError(e.t, os.MkdirAll(cwd, 0o755))
	spec := fmt.Sprintf("# %s spec\n\n## Retry policy\n\nProject %s retries use exponential backoff capped at 30s.\n", name, name)
	require.NoError(e.t, os.WriteFile(filepath.Join(cwd, "spec.md"), []byte(spec), 0o644))
	require.NoError(e.t, os.WriteFile(filepath.Join(cwd, "code.go"), []byte("package p\n\nfunc A() {}\n"), 0o644))
	p := &project{name: name, cwd: cwd}
	e.start(p)
	return p
}

// start runs a bridge process for p, as Claude Code does.
func (e *multiEnv) start(p *project) {
	cmd := exec.Command(e.bin, "mcp")
	cmd.Dir = p.cwd
	cmd.Env = e.environ()
	logf, err := os.OpenFile(filepath.Join(e.dir, "bridge-"+p.name+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	require.NoError(e.t, err)
	e.t.Cleanup(func() { _ = logf.Close() })
	cmd.Stderr = logf
	c := mcp.NewClient(&mcp.Implementation{Name: "claude-" + p.name, Version: "1"}, nil)
	s, err := c.Connect(context.Background(), &mcp.CommandTransport{Command: cmd}, nil)
	require.NoError(e.t, err)
	p.session = s
	e.t.Cleanup(func() { _ = s.Close() })
}

func call(t *testing.T, s *mcp.ClientSession, name string, args any) string {
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	require.False(t, res.IsError, b.String())
	return b.String()
}

// show calls spec_show and records the workspace id from the page URL.
func (e *multiEnv) show(p *project) {
	out := call(e.t, p.session, "spec_show", map[string]any{"path": "spec.md"})
	i := strings.Index(out, e.base()+"/w/")
	require.GreaterOrEqual(e.t, i, 0, out)
	rest := out[i+len(e.base()+"/w/"):]
	p.ws = rest[:strings.IndexAny(rest, "\"\\ \n")]
}

// askEvent returns the data of the workspace's SSE ask event ("" if the
// page can't be read).
func (e *multiEnv) askEvent(ws string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, e.base()+"/w/"+ws+"/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 4<<20)
	inAsk := false
	for sc.Scan() {
		line := sc.Text()
		if line == "event: ask" {
			inAsk = true
		} else if inAsk && strings.HasPrefix(line, "data: ") {
			return strings.TrimPrefix(line, "data: ")
		}
	}
	return ""
}

// pending waits until the workspace shows a pending question set and returns
// its id and the ask event.
func (e *multiEnv) pending(stage, ws string, early func() string) (string, string) {
	var ev string
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		require.Empty(e.t, early(), "%s: the ask returned before it was answered", stage)
		ev = e.askEvent(ws)
		var d struct {
			QSID string `json:"qs_id"`
		}
		if json.Unmarshal([]byte(ev), &d) == nil && d.QSID != "" {
			return d.QSID, ev
		}
	}
	require.FailNow(e.t, "no pending question", "%s: workspace %s, last ask event: %.300s", stage, ws, ev)
	return "", ""
}

func (e *multiEnv) answer(ws, qs, choice string) {
	req, _ := http.NewRequest(http.MethodPost, e.base()+"/w/"+ws+"/qs/"+qs+"/answer",
		strings.NewReader(url.Values{"sel.q1": {choice}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", e.base())
	resp, err := http.DefaultClient.Do(req)
	require.NoError(e.t, err)
	_ = resp.Body.Close()
	require.Equal(e.t, http.StatusNoContent, resp.StatusCode, "answer %s/%s", ws, qs)
}

func TestMultiSession(t *testing.T) {
	e := newMultiEnv(t)
	names := []string{"alpha", "beta", "gamma"}
	var ps []*project
	for _, n := range names {
		ps = append(ps, e.project(n))
	}

	// The first spec_show starts the daemon; all three get their own page.
	for _, p := range ps {
		e.show(p)
	}
	firstPID := e.ping()
	require.NotZero(t, firstPID, "a bridge started the daemon")
	assert.NotEqual(t, ps[0].ws, ps[1].ws)
	assert.NotEqual(t, ps[1].ws, ps[2].ws)

	// All three ask at once; each ask blocks in its own session.
	type result struct {
		out protocol.AskOutput
		raw string
	}
	results := make([]result, len(ps))
	var mu sync.Mutex // guards results until wg.Wait
	var wg sync.WaitGroup
	for i, p := range ps {
		wg.Add(1)
		go func(i int, p *project) {
			defer wg.Done()
			res, err := p.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "ask", Arguments: map[string]any{
				"title": "Question for " + p.name,
				"questions": []any{map[string]any{
					"id": "q1", "text": "Which cap for " + p.name + "?",
					"options":    []any{map[string]any{"label": "cap-" + p.name}, map[string]any{"label": "no cap"}},
					"spec_quote": "Project " + p.name + " retries use exponential backoff", "spec_section": "Retry policy",
					"code_refs": []any{map[string]any{"path": "code.go", "lines": "3"}},
				}},
			}})
			raw := ""
			if err != nil {
				raw = "error: " + err.Error()
			} else {
				for _, c := range res.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						raw += tc.Text
					}
				}
			}
			mu.Lock()
			defer mu.Unlock()
			results[i].raw = raw
			_ = json.Unmarshal([]byte(raw), &results[i].out)
		}(i, p)
	}

	// No cross-talk: each page shows only its own question.
	checkPages := func(stage string) map[string]string {
		ids := map[string]string{}
		for i, p := range ps {
			qs, ev := e.pending(stage, p.ws, func() string {
				mu.Lock()
				defer mu.Unlock()
				return results[i].raw
			})
			ids[p.name] = qs
			assert.Contains(t, ev, "Which cap for "+p.name, "%s: %s shows its question", stage, p.name)
			for _, other := range ps {
				if other != p {
					assert.NotContains(t, ev, other.name, "%s: %s shows nothing of %s", stage, p.name, other.name)
				}
			}
		}
		return ids
	}
	before := checkPages("start")

	// Restart the daemon while all three wait. The bridges reconnect and
	// show their questions again, with the same ids.
	out, err := e.run("serve", "--restart")
	require.NoError(t, err, out)
	require.Eventually(t, func() bool { pid := e.ping(); return pid != 0 && pid != firstPID }, 15*time.Second, 100*time.Millisecond)
	assert.Equal(t, before, checkPages("after restart"), "the same question sets are pending")

	// Kill it hard: a bridge starts a new one.
	pid := e.ping()
	proc, err := os.FindProcess(pid)
	require.NoError(t, err)
	require.NoError(t, proc.Kill())
	require.Eventually(t, func() bool { n := e.ping(); return n != 0 && n != pid }, 20*time.Second, 100*time.Millisecond, "a bridge restarted the daemon")
	assert.Equal(t, before, checkPages("after kill"), "the same question sets are pending")

	// Each answer reaches its own session.
	for _, p := range ps {
		e.answer(p.ws, before[p.name], "cap-"+p.name)
	}
	wg.Wait()
	for i, p := range ps {
		r := results[i]
		require.Equal(t, protocol.StatusAnswered, r.out.Status, "%s: %s", p.name, r.raw)
		require.Len(t, r.out.Answers, 1)
		assert.Equal(t, []string{"cap-" + p.name}, r.out.Answers[0].Selected, p.name)
	}

	// claude --resume in alpha: the session exits, a new bridge starts in the
	// same directory and takes the workspace over.
	alpha := ps[0]
	oldWS := alpha.ws
	require.NoError(t, alpha.session.Close())
	require.Eventually(t, func() bool {
		resp, err := http.Get(e.base() + "/")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		var b strings.Builder
		_, _ = bufio.NewReader(resp.Body).WriteTo(&b)
		// Archived items carry the "archived" class.
		return strings.Contains(b.String(), `<li class="ws archived">
    <a href="/w/`+oldWS+`">`)
	}, 5*time.Second, 100*time.Millisecond, "the ended session is archived")
	e.start(alpha)
	e.show(alpha)
	assert.NotEqual(t, oldWS, alpha.ws, "the resumed session is a new bridge")

	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := noRedirect.Get(e.base() + "/w/" + oldWS)
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusFound, resp.StatusCode, "the old page redirects")
	assert.Equal(t, "/w/"+alpha.ws, resp.Header.Get("Location"))

	// The adopted workspace keeps alpha's history, and nothing of the others.
	hist := historyEvent(t, e, alpha.ws)
	assert.Contains(t, hist, "cap-alpha", "history survives the resume")
	assert.NotContains(t, hist, "beta")
	assert.NotContains(t, hist, "gamma")
}

// historyEvent returns the data of the workspace's SSE history event.
func historyEvent(t *testing.T, e *multiEnv, ws string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, e.base()+"/w/"+ws+"/events", nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 4<<20)
	in := false
	for sc.Scan() {
		line := sc.Text()
		if line == "event: history" {
			in = true
		} else if in && strings.HasPrefix(line, "data: ") {
			return strings.TrimPrefix(line, "data: ")
		}
	}
	return ""
}
