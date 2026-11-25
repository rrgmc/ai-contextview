package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rrgmc/ai-specview/internal/appconn"
	"github.com/rrgmc/ai-specview/internal/fakeapp"
	"github.com/rrgmc/ai-specview/internal/protocol"
	"github.com/rrgmc/ai-specview/internal/resolve"
	"github.com/rrgmc/ai-specview/internal/testenv"
)

const specMD = `# Retry spec

## Retry policy

All retries use **exponential backoff** capped at 30s.

## Errors

Errors are wrapped.
`

type env struct {
	cwd    string
	fake   *fakeapp.Server
	client *appconn.Client
	bridge *Bridge
	opened []string
}

// shortSocket returns a socket path short enough for macOS (104 bytes).
func shortSocket(t *testing.T) string {
	dir := testenv.ShortTempDir(t, "sv")
	return filepath.Join(dir, "s.sock")
}

func setup(t *testing.T, onAsk func(string, *protocol.Ask) *protocol.AskResult) *env {
	cwd := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(cwd, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "docs/spec.md"), []byte(specMD), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "main.txt"), []byte("a\nb\nc\nd\n"), 0o644))

	sock := shortSocket(t)
	fake := &fakeapp.Server{Socket: sock, OnAsk: onAsk}
	require.NoError(t, fake.Listen())
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = fake.Serve(ctx) }()

	client := appconn.New(appconn.Options{
		Socket: sock,
		Hello:  protocol.Hello{WS: "ws_test", Cwd: cwd, ProjectName: "p"},
		Logf:   t.Logf,
	})
	t.Cleanup(client.Close)
	e := &env{cwd: cwd, fake: fake, client: client}
	e.bridge = &Bridge{
		Cwd: cwd, Daemon: client, Resolver: resolve.New(), AskWait: 2 * time.Second,
		Open: func(u string) { e.opened = append(e.opened, u) },
	}
	return e
}

func (e *env) show(t *testing.T) {
	_, out, err := e.bridge.SpecShow(context.Background(), nil, protocol.SpecShowInput{Path: "docs/spec.md"})
	require.NoError(t, err)
	assert.True(t, out.OK)
	assert.Equal(t, []string{"Retry spec", "Retry policy", "Errors"}, out.Sections)
	assert.Contains(t, out.URL, "/w/ws_test")
}

func TestSpecShowOpenIn(t *testing.T) {
	e := setup(t, nil)
	_, out, err := e.bridge.SpecShow(context.Background(), nil, protocol.SpecShowInput{Path: "docs/spec.md"})
	require.NoError(t, err)
	assert.Empty(t, out.Open)

	e.bridge.OpenIn = OpenPane
	_, out, err = e.bridge.SpecShow(context.Background(), nil, protocol.SpecShowInput{Path: "docs/spec.md"})
	require.NoError(t, err)
	assert.Equal(t, OpenPane, out.Open)
}

func TestOpenInFromEnv(t *testing.T) {
	for value, want := range map[string]string{"": OpenPane, "pane": OpenPane, "browser": "", "never": ""} {
		t.Setenv("AI_SPECVIEW_OPEN", value)
		assert.Equal(t, want, OpenInFromEnv(), "AI_SPECVIEW_OPEN=%q", value)
	}
}

func question(quote string) protocol.AskInput {
	return protocol.AskInput{Title: "T", Questions: []protocol.QuestionInput{{
		ID: "q1", Text: "Reset?", Options: []protocol.Option{{Label: "Yes"}, {Label: "No"}},
		SpecQuote: quote, SpecSection: "Retry policy",
		CodeRefs: []protocol.CodeRef{{Path: "main.txt", Lines: "2-3"}},
	}}}
}

func TestAskAnswered(t *testing.T) {
	e := setup(t, func(_ string, a *protocol.Ask) *protocol.AskResult {
		return &protocol.AskResult{QSID: a.QSID, Status: protocol.StatusAnswered,
			Answers: []protocol.Answer{{ID: "q1", Selected: []string{"No"}}}}
	})
	e.show(t)
	_, out, err := e.bridge.Ask(context.Background(), nil, question("retries use exponential backoff capped at 30s"))
	require.NoError(t, err)
	assert.Equal(t, protocol.StatusAnswered, out.Status)
	require.Len(t, out.Answers, 1)
	assert.Equal(t, []string{"No"}, out.Answers[0].Selected)
	assert.Len(t, e.opened, 1, "viewers 0 opens the page")

	// The daemon received resolved refs.
	var ask protocol.Ask
	for _, m := range e.fake.Received() {
		if m.Type == protocol.TypeAsk {
			require.NoError(t, m.Decode(&ask))
		}
	}
	require.Len(t, ask.Questions, 1)
	assert.Equal(t, filepath.Join(e.cwd, "docs/spec.md"), ask.Questions[0].Docs[0].Path)
	assert.Equal(t, 2, ask.Questions[0].Code[0].StartLine)
}

func TestAskQuoteNotFound(t *testing.T) {
	e := setup(t, nil)
	e.show(t)
	_, _, err := e.bridge.Ask(context.Background(), nil, question("we honour whatever the server says about delays"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in spec.md")
	assert.Contains(t, err.Error(), "closest sections")
}

func TestAskWithoutSpecShow(t *testing.T) {
	e := setup(t, nil)
	_, _, err := e.bridge.Ask(context.Background(), nil, question("retries use exponential backoff"))
	assert.ErrorContains(t, err, "call spec_show first")
}

func TestAskPendingThenWait(t *testing.T) {
	e := setup(t, nil)
	e.bridge.AskWait = 200 * time.Millisecond
	e.show(t)
	_, out, err := e.bridge.Ask(context.Background(), nil, question("capped at 30s"))
	require.NoError(t, err)
	require.Equal(t, protocol.StatusPending, out.Status)
	qs := out.QuestionSetID
	require.NotEmpty(t, qs)

	// The answer arrives while nobody waits; ask_wait returns it buffered.
	require.NoError(t, e.fake.Answer("ws_test", &protocol.AskResult{QSID: qs, Status: protocol.StatusInChat, KeepOpen: []string{"q1"}}))
	require.Eventually(t, func() bool { return e.client.HasResult(qs) }, 2*time.Second, 10*time.Millisecond)
	_, out, err = e.bridge.AskWaitTool(context.Background(), nil, protocol.AskWaitInput{QuestionSetID: qs})
	require.NoError(t, err)
	assert.Equal(t, protocol.StatusInChat, out.Status)
	assert.Equal(t, []string{"q1"}, out.KeepOpen)
	assert.Equal(t, MsgInChat, out.Message)

	_, _, err = e.bridge.AskWaitTool(context.Background(), nil, protocol.AskWaitInput{QuestionSetID: "qs_nope"})
	assert.Error(t, err)
}

func TestAskCancelled(t *testing.T) {
	e := setup(t, nil)
	e.show(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, _, err := e.bridge.Ask(ctx, nil, question("capped at 30s"))
	require.Error(t, err)
	require.Eventually(t, func() bool {
		for _, m := range e.fake.Received() {
			if m.Type == protocol.TypeAskCancel {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)
}

func TestAskValidation(t *testing.T) {
	e := setup(t, nil)
	e.show(t)
	in := protocol.AskInput{Questions: []protocol.QuestionInput{{ID: "q1", Text: "x", Options: []protocol.Option{{Label: "a"}}}}}
	_, _, err := e.bridge.Ask(context.Background(), nil, in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "options")
	assert.Contains(t, err.Error(), "needs at least one of")
}

func TestReconnectRedeliversPendingAsk(t *testing.T) {
	e := setup(t, nil)
	e.bridge.AskWait = 100 * time.Millisecond
	e.show(t)
	_, out, err := e.bridge.Ask(context.Background(), nil, question("capped at 30s"))
	require.NoError(t, err)
	require.Equal(t, protocol.StatusPending, out.Status)

	// Kill the fake daemon and start a new one on the same socket.
	sock := e.fake.Socket
	e.fake.Close()
	fake2 := &fakeapp.Server{Socket: sock}
	require.NoError(t, fake2.Listen())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = fake2.Serve(ctx) }()

	require.Eventually(t, func() bool {
		var types []string
		for _, m := range fake2.Received() {
			types = append(types, m.Type)
		}
		return assert.ObjectsAreEqual([]string{protocol.TypeHello, protocol.TypeSpecShow, protocol.TypeAsk}, types)
	}, 5*time.Second, 20*time.Millisecond)

	require.NoError(t, fake2.Answer("ws_test", &protocol.AskResult{QSID: out.QuestionSetID, Status: protocol.StatusDismissed}))
	e.bridge.AskWait = 2 * time.Second
	_, out, err = e.bridge.AskWaitTool(context.Background(), nil, protocol.AskWaitInput{QuestionSetID: out.QuestionSetID})
	require.NoError(t, err)
	assert.Equal(t, protocol.StatusDismissed, out.Status)
}

func TestRecommendedOption(t *testing.T) {
	var got *protocol.Ask
	e := setup(t, func(_ string, a *protocol.Ask) *protocol.AskResult {
		got = a
		return &protocol.AskResult{QSID: a.QSID, Status: protocol.StatusDismissed}
	})
	e.show(t)
	in := question("capped at 30s")
	in.Questions[0].Options = []protocol.Option{{Label: "Yes (Recommended)"}, {Label: "No"}}
	_, _, err := e.bridge.Ask(context.Background(), nil, in)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, protocol.Option{Label: "Yes", Recommended: true}, got.Questions[0].Options[0])
	assert.False(t, got.Questions[0].Options[1].Recommended)

	in = question("capped at 30s")
	in.Questions[0].Options = []protocol.Option{{Label: "Yes", Recommended: true}, {Label: "No", Recommended: true}}
	_, _, err = e.bridge.Ask(context.Background(), nil, in)
	assert.ErrorContains(t, err, "at most 1")
}

func TestSpecDone(t *testing.T) {
	e := setup(t, nil)
	e.show(t)
	_, out, err := e.bridge.SpecDone(context.Background(), nil, protocol.SpecDoneInput{Summary: "  done  "})
	require.NoError(t, err)
	assert.True(t, out.OK)
	var got protocol.Done
	for _, m := range e.fake.Received() {
		if m.Type == protocol.TypeDone {
			require.NoError(t, m.Decode(&got))
		}
	}
	assert.Equal(t, "done", got.Summary)
}

func TestDeferredMessage(t *testing.T) {
	e := setup(t, func(_ string, a *protocol.Ask) *protocol.AskResult {
		return &protocol.AskResult{QSID: a.QSID, Status: protocol.StatusAnswered,
			Answers:  []protocol.Answer{{ID: "q1", Selected: []string{}, Deferred: true}},
			Deferred: []string{"q1"}}
	})
	e.show(t)
	_, out, err := e.bridge.Ask(context.Background(), nil, question("capped at 30s"))
	require.NoError(t, err)
	assert.Equal(t, protocol.StatusAnswered, out.Status)
	assert.Equal(t, []string{"q1"}, out.Deferred)
	assert.Equal(t, MsgDeferred, out.Message)
}
