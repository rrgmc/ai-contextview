package daemon

// Browser tests: they drive the workspace page in headless Chrome, so the
// question panel's script (auto-advance, review step, Decide later, the
// finished state) is tested as the user sees it. They are skipped with
// `go test -short` and when no Chrome is installed. CHROME_PATH picks the
// browser binary.

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rrgmc/ai-contextview/internal/protocol"
)

func chromePath() string {
	if p := os.Getenv("CHROME_PATH"); p != "" {
		return p
	}
	paths := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
	}
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
		if d := os.Getenv(env); d != "" {
			paths = append(paths, filepath.Join(d, "Google", "Chrome", "Application", "chrome.exe"))
		}
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "headless-shell"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// newBrowser returns a headless Chrome tab that is closed when the test ends.
// Call it after newHarness: cleanups run last-in first-out, so the browser
// closes its event stream before the test HTTP server shuts down.
func newBrowser(t *testing.T) context.Context {
	t.Helper()
	if testing.Short() {
		t.Skip("browser test skipped with -short")
	}
	path := chromePath()
	if path == "" {
		t.Skip("no Chrome found; set CHROME_PATH to run the browser tests")
	}
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(path),
		chromedp.WindowSize(1400, 900),
	)
	actx, cancelA := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancelC := chromedp.NewContext(actx)
	ctx, cancelT := context.WithTimeout(ctx, 30*time.Second)
	t.Cleanup(func() { cancelT(); cancelC(); cancelA() })
	return ctx
}

// twoQuestions starts a workspace with a spec and a pending two-question set.
func twoQuestions(t *testing.T) (*harness, *bridge) {
	h := newHarness(t)
	cwd, specPath := writeSpec(t)
	b := h.bridge("ws_A", cwd)
	b.req(protocol.TypeSpecShow, &protocol.SpecShow{RootAbs: specPath})
	ask := askBody(cwd, specPath, "qs_1")
	q2 := ask.Questions[0]
	q2.ID, q2.Text = "q2", "Cap value?"
	q2.Options = []protocol.Option{{Label: "10s"}, {Label: "30s", Recommended: true}}
	ask.Questions = append(ask.Questions, q2)
	b.req(protocol.TypeAsk, ask)
	return h, b
}

// result reads bridge messages until the ask_result.
func (b *bridge) result() protocol.AskResult {
	for {
		env := b.read()
		if env.Type != protocol.TypeAskResult {
			continue
		}
		var r protocol.AskResult
		require.NoError(b.t, env.Decode(&r))
		return r
	}
}

// run runs browser steps and fails the test on an error.
func run(t *testing.T, ctx context.Context, steps ...chromedp.Action[chromedp.Void]) {
	t.Helper()
	require.NoError(t, chromedp.Do(ctx, steps...))
}

// text returns the visible text of the first element matching sel.
func text(t *testing.T, ctx context.Context, sel string) string {
	t.Helper()
	s, err := chromedp.Run(ctx, chromedp.Text(sel))
	require.NoError(t, err)
	return s
}

// eval evaluates a JavaScript expression in the page.
func eval[T any](t *testing.T, ctx context.Context, expr string) T {
	t.Helper()
	v, err := chromedp.Run(ctx, chromedp.Evaluate[T](expr))
	require.NoError(t, err)
	return v
}

const (
	q1Section = `section.q[data-qid="q1"]`
	q2Section = `section.q[data-qid="q2"]`
)

func TestBrowserAnswerFlow(t *testing.T) {
	h, b := twoQuestions(t)
	ctx := newBrowser(t)

	run(t, ctx,
		chromedp.Navigate(h.http.URL+"/w/ws_A"),
		chromedp.WaitVisible(q1Section+` .qtext`),
		// The quote is highlighted and the recommended option is marked.
		chromedp.WaitVisible(`#doc mark.q.active`),
		chromedp.WaitVisible(q1Section+` label.opt.recommended .rec`),
	)
	assert.Equal(t, "Send answers (0/2)", text(t, ctx, `button[data-action="answer"]`))

	// Clicking the option text answers and moves on to Q2.
	run(t, ctx,
		chromedp.Click(q1Section+` label.opt:nth-of-type(2) .opt-body b`),
		chromedp.WaitVisible(q2Section+` .qtext`),
	)
	assert.True(t, eval[bool](t, ctx, `document.querySelector('.qs-next-btn').hidden`), "no Next while the last question is open")

	// Clicking the radio circle only selects: Q2 stays, the review waits for
	// Next.
	run(t, ctx,
		chromedp.Click(q2Section+` input[value="10s"]`),
		chromedp.Sleep(600*time.Millisecond),
	)
	assert.Equal(t, "Review ›", text(t, ctx, `.qs-next-btn`))
	assert.False(t, eval[bool](t, ctx, `document.querySelector('`+q2Section+`').hidden`), "Q2 is still shown")
	assert.True(t, eval[bool](t, ctx, `document.querySelector('.review').hidden`), "no review yet")
	assert.True(t, eval[bool](t, ctx, `document.querySelector('`+q2Section+` input[value="10s"]').checked`))

	// Next on the last question opens the review.
	run(t, ctx,
		chromedp.Click(`.qs-next-btn`),
		chromedp.WaitVisible(`.review .review-list`),
	)
	review := text(t, ctx, `.review`)
	assert.Contains(t, review, "→ No")
	assert.Contains(t, review, "→ 10s")

	// A review item goes back to its question; Next twice comes back to the
	// review.
	run(t, ctx,
		chromedp.Click(`.review-item[data-step="0"]`),
		chromedp.WaitVisible(q1Section+` .qtext`),
		chromedp.Click(`.qs-next-btn`),
		chromedp.WaitVisible(q2Section+` .qtext`),
		chromedp.Click(`.qs-next-btn`),
		chromedp.WaitVisible(`.review .review-list`),
		chromedp.Click(`button[data-action="answer"]`),
	)
	r := b.result()
	assert.Equal(t, protocol.StatusAnswered, r.Status)
	require.Len(t, r.Answers, 2)
	assert.Equal(t, []string{"No"}, r.Answers[0].Selected)
	assert.Equal(t, []string{"10s"}, r.Answers[1].Selected)
}

func TestBrowserDecideLater(t *testing.T) {
	h, b := twoQuestions(t)
	ctx := newBrowser(t)

	run(t, ctx,
		chromedp.Navigate(h.http.URL+"/w/ws_A"),
		chromedp.WaitVisible(q1Section+` .qtext`),
		// Select with the radio circle, then Decide later clears it.
		chromedp.Click(q1Section+` input[value="Yes"]`),
		chromedp.Click(q1Section+` label.later .opt-body b`),
		chromedp.WaitVisible(q2Section+` .qtext`),
	)
	assert.False(t, eval[bool](t, ctx, `document.querySelector('`+q1Section+` input[value="Yes"]').checked`))

	// Number key 2 picks the second option of Q2 and opens the review.
	run(t, ctx,
		chromedp.KeyEvent("2"),
		chromedp.WaitVisible(`.review .review-later`),
		chromedp.Click(`button[data-action="answer"]`),
	)
	r := b.result()
	assert.Equal(t, protocol.StatusAnswered, r.Status)
	assert.Equal(t, []string{"q1"}, r.Deferred)
	require.Len(t, r.Answers, 2)
	assert.True(t, r.Answers[0].Deferred)
	assert.Equal(t, []string{"30s"}, r.Answers[1].Selected)
}

func TestBrowserSendIncomplete(t *testing.T) {
	h, _ := twoQuestions(t)
	ctx := newBrowser(t)

	run(t, ctx,
		chromedp.Navigate(h.http.URL+"/w/ws_A"),
		chromedp.WaitVisible(q1Section+` .qtext`),
		chromedp.Click(`button[data-action="answer"]`),
		chromedp.WaitVisible(`.qs-hint`),
	)
	assert.Equal(t, "Still open: Q1, Q2", text(t, ctx, `.qs-hint`))
	h.d.mu.Lock()
	pending := h.d.workspaces["ws_A"].Pending
	h.d.mu.Unlock()
	assert.NotNil(t, pending, "nothing was sent")
}

func TestBrowserDone(t *testing.T) {
	h, b := twoQuestions(t)
	ctx := newBrowser(t)

	run(t, ctx,
		chromedp.Navigate(h.http.URL+"/w/ws_A"),
		chromedp.WaitVisible(`#doc mark.q.active`),
	)
	b.req(protocol.TypeDone, &protocol.Done{Summary: "**Decided**\n\n- cap 30s\n\n**Still open**\n\n- none\n"})
	run(t, ctx, chromedp.WaitVisible(`.done-banner`))
	banner := text(t, ctx, `.done-banner`)
	assert.Contains(t, banner, "Spec finished")
	assert.Contains(t, banner, "cap 30s")
	assert.Equal(t, 0, eval[int](t, ctx, `document.querySelectorAll('#doc mark.q.active').length`), "no passage stays highlighted")
	assert.Equal(t, 0, eval[int](t, ctx, `document.querySelectorAll('#code-tabs button').length`), "the code pane is cleared")
}

func TestBrowserNoDocument(t *testing.T) {
	h := newHarness(t)
	cwd, specPath := writeSpec(t)
	b := h.bridge("ws_A", cwd)
	b.req(protocol.TypeSpecShow, &protocol.SpecShow{RootAbs: specPath})
	ask := askBody(cwd, specPath, "qs_1")
	ask.Questions = append(ask.Questions, protocol.AskQuestion{
		ID: "q2", Text: "Ship it?", Options: []protocol.Option{{Label: "Yes"}, {Label: "No"}},
	})
	b.req(protocol.TypeAsk, ask)
	ctx := newBrowser(t)

	run(t, ctx,
		chromedp.Navigate(h.http.URL+"/w/ws_A"),
		chromedp.WaitVisible(q1Section+` .qtext`),
	)
	// The first question has a document: no pill, no note.
	assert.False(t, eval[bool](t, ctx, `!!document.querySelector('`+q1Section+` .chip-nodoc')`))
	assert.True(t, eval[bool](t, ctx, `document.getElementById("doc-note").hidden`))
	run(t, ctx,
		chromedp.Click(`.step[data-step="1"]`),
		chromedp.WaitVisible(q2Section+` .chip-nodoc`),
	)
	assert.Equal(t, "No document", text(t, ctx, q2Section+` .chip-nodoc`))
	assert.False(t, eval[bool](t, ctx, `document.getElementById("doc-note").hidden`))
}

func TestBrowserHidesEmptyCodePane(t *testing.T) {
	h := newHarness(t)
	cwd, specPath := writeSpec(t)
	b := h.bridge("ws_A", cwd)
	b.req(protocol.TypeSpecShow, &protocol.SpecShow{RootAbs: specPath})
	ask := askBody(cwd, specPath, "qs_1")
	ask.Questions = append(ask.Questions, protocol.AskQuestion{
		ID: "q2", Text: "Ship it?", Options: []protocol.Option{{Label: "Yes"}, {Label: "No"}},
	})
	b.req(protocol.TypeAsk, ask)
	ctx := newBrowser(t)
	codeShown := `getComputedStyle(document.querySelector(".code-pane")).display !== "none"`

	// The first question has code: the pane shows.
	run(t, ctx,
		chromedp.Navigate(h.http.URL+"/w/ws_A"),
		chromedp.WaitVisible(`.code-pane .line.hl`),
	)
	assert.True(t, eval[bool](t, ctx, codeShown))

	// The second has none: with the questions above the code, the pane hides.
	run(t, ctx,
		chromedp.Click(`.step[data-step="1"]`),
		chromedp.WaitVisible(q2Section+` .qtext`),
	)
	assert.False(t, eval[bool](t, ctx, codeShown))

	// With the questions below, the code pane sits beside the document and stays.
	run(t, ctx, chromedp.Click(`#dock-toggle`))
	assert.True(t, eval[bool](t, ctx, codeShown))
	run(t, ctx, chromedp.Click(`#dock-toggle`))

	// Back on the first question, the pane shows again.
	run(t, ctx,
		chromedp.Click(`.step[data-step="0"]`),
		chromedp.WaitVisible(`.code-pane .line.hl`),
	)
	assert.True(t, eval[bool](t, ctx, codeShown))
}

// The doc pane header shows the document's line count while it has room.
func TestBrowserDocLines(t *testing.T) {
	h, _ := twoQuestions(t)
	ctx := newBrowser(t)
	want := fmt.Sprintf("%d lines", lineCount([]byte(spec)))

	run(t, ctx,
		chromedp.EmulateViewport(1600, 900),
		chromedp.Navigate(h.http.URL+"/w/ws_A"),
		chromedp.WaitVisible(`#doc mark.q.active`),
		chromedp.WaitVisible(`#doc-lines`),
	)
	assert.Equal(t, want, text(t, ctx, `#doc-lines`))

	// A phone-width window has no room for it.
	run(t, ctx,
		chromedp.EmulateViewport(400, 900),
		chromedp.Sleep(300*time.Millisecond),
	)
	assert.True(t, eval[bool](t, ctx, `document.querySelector('#doc-lines').hidden`))

	// Wide again, it comes back.
	run(t, ctx,
		chromedp.EmulateViewport(1600, 900),
		chromedp.WaitVisible(`#doc-lines`),
	)
	assert.Equal(t, want, text(t, ctx, `#doc-lines`))
}

// The page reports that it is used, so an unseen question can be told apart.
func TestBrowserReportsUse(t *testing.T) {
	h, _ := twoQuestions(t)
	ctx := newBrowser(t)
	run(t, ctx,
		chromedp.Navigate(h.http.URL+"/w/ws_A"),
		chromedp.WaitVisible(q1Section+` .qtext`),
	)
	total, hidden := h.d.hub.Counts("ws_A")
	assert.Equal(t, 1, total)
	assert.Equal(t, 0, hidden)
	assert.False(t, h.d.viewers("ws_A").Seen)

	run(t, ctx, chromedp.Click(q1Section+` .qtext`))
	assert.Eventually(t, func() bool { return h.d.viewers("ws_A").Seen }, 3*time.Second, 50*time.Millisecond)
}

// The zoom button shows only the document; clicking again or Esc restores,
// and a new question set restores it too.
func TestBrowserZoomDoc(t *testing.T) {
	h := newHarness(t)
	cwd, specPath := writeSpec(t)
	b := h.bridge("ws_A", cwd)
	b.req(protocol.TypeSpecShow, &protocol.SpecShow{RootAbs: specPath})
	b.req(protocol.TypeAsk, askBody(cwd, specPath, "qs_1"))
	ctx := newBrowser(t)
	panelShown := func() bool {
		return eval[bool](t, ctx, `getComputedStyle(document.querySelector('#panel')).display !== 'none'`)
	}
	docWidth := func() float64 {
		return eval[float64](t, ctx, `document.querySelector('.doc-pane').getBoundingClientRect().width`)
	}
	run(t, ctx,
		chromedp.EmulateViewport(1400, 900),
		chromedp.Navigate(h.http.URL+"/w/ws_A"),
		chromedp.WaitVisible(q1Section+` .qtext`),
	)
	require.True(t, panelShown())
	assert.Less(t, docWidth(), 1000.0)

	run(t, ctx, chromedp.Click(`#doc-zoom`))
	assert.False(t, panelShown(), "zoomed: the questions are hidden")
	assert.Greater(t, docWidth(), 1350.0, "the document takes the whole width")
	assert.Equal(t, "true", eval[string](t, ctx, `document.querySelector('#doc-zoom').getAttribute('aria-pressed')`))

	run(t, ctx, chromedp.Click(`#doc-zoom`))
	assert.True(t, panelShown(), "clicking again restores")

	run(t, ctx, chromedp.Click(`#doc-zoom`), chromedp.KeyEvent("\x1b"))
	assert.True(t, panelShown(), "Esc restores")

	// A new question set restores the layout.
	run(t, ctx, chromedp.Click(`#doc-zoom`))
	require.False(t, panelShown())
	resp := h.do(http.MethodPost, "/w/ws_A/qs/qs_1/answer", url.Values{"sel.q1": {"No"}}, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	b.read()
	b.req(protocol.TypeAsk, askBody(cwd, specPath, "qs_2"))
	assert.Eventually(t, panelShown, 3*time.Second, 50*time.Millisecond)
}
