package daemon

// TestScreenshots renders the README screenshots from the sample spec and
// code in testdata/screenshots. It runs only when AI_CONTEXTVIEW_SCREENSHOTS
// names the output directory; a relative path is taken from the module root:
//
//	AI_CONTEXTVIEW_SCREENSHOTS=docs/images go test -run TestScreenshots ./internal/daemon/

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/require"

	"github.com/rrgmc/ai-contextview/internal/protocol"
)

func TestScreenshots(t *testing.T) {
	out := os.Getenv("AI_CONTEXTVIEW_SCREENSHOTS")
	if out == "" {
		t.Skip("set AI_CONTEXTVIEW_SCREENSHOTS to the output directory to render the screenshots")
	}
	if !filepath.IsAbs(out) {
		out = filepath.Join(moduleRoot(t), out)
	}
	require.NoError(t, os.MkdirAll(out, 0o755))

	h := newHarness(t)
	cwd := sampleProject(t)
	specPath := filepath.Join(cwd, "spec.md")
	codePath := filepath.Join(cwd, "orders", "retry.go")
	b := h.bridge("ws_orders", cwd)
	b.req(protocol.TypeSpecShow, &protocol.SpecShow{RootAbs: specPath})
	b.req(protocol.TypeAsk, sampleAsk(specPath, codePath))

	shoot := func(ctx context.Context, name string) {
		t.Helper()
		png, err := chromedp.Run(ctx, chromedp.CaptureScreenshot())
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(out, name), png, 0o644))
	}

	// The first question, with its spec passage and code highlighted.
	ctx := newBrowser(t)
	run(t, ctx,
		chromedp.EmulateViewport(1440, 900, chromedp.EmulateScale(2)),
		chromedp.Navigate(h.http.URL+"/w/ws_orders"),
		chromedp.WaitVisible(q1Section+` .qtext`),
		chromedp.WaitVisible(`#doc mark.q.active`),
		chromedp.Sleep(500*time.Millisecond),
	)
	shoot(ctx, "question.png")

	// The same page in the dark theme.
	run(t, ctx,
		chromedp.Click(`#theme-toggle`),
		chromedp.Sleep(300*time.Millisecond),
	)
	eval[bool](t, ctx, `document.activeElement.blur(), true`)
	shoot(ctx, "question-dark.png")
	run(t, ctx,
		chromedp.Click(`#theme-toggle`),
		chromedp.Sleep(300*time.Millisecond),
	)

	// The user answers; Claude applies the answers to the spec, and the
	// Changes view shows what changed since.
	run(t, ctx,
		chromedp.Click(q1Section+` label.opt:nth-of-type(1) .opt-body b`),
		chromedp.WaitVisible(q2Section+` .qtext`),
		chromedp.Click(q2Section+` label.opt:nth-of-type(1) .opt-body b`),
		chromedp.WaitVisible(`.review .review-list`),
		chromedp.Click(`button[data-action="answer"]`),
	)
	_ = b.result()
	data, err := os.ReadFile(specPath)
	require.NoError(t, err)
	edited := strings.NewReplacer(
		"capped at 30s.", "capped at 30s. A successful delivery to a partner resets the\nbackoff for that partner's other pending events.",
		"endpoint answers with a 5xx status or 429.", "endpoint answers with a 5xx status or 429. For 429 the next retry\nwaits for the `Retry-After` header when it is longer than the backoff.",
		"- Should a successful delivery reset the backoff for the partner?\n", "",
	).Replace(string(data))
	require.NoError(t, os.WriteFile(specPath, []byte(edited), 0o644))
	h.d.refresh("ws_orders")
	b.req(protocol.TypeFocus, &protocol.Focus{
		Code: []protocol.ResolvedRef{{Path: codePath, StartLine: 36, EndLine: 46, Symbol: "classify", Kind: "func", Source: "symbol"}},
	})
	run(t, ctx,
		chromedp.Click(`.dtab[data-view="changes"]`),
		chromedp.WaitVisible(`#changes .chg-add`),
		chromedp.Sleep(500*time.Millisecond),
	)
	shoot(ctx, "changes.png")
}

// sampleProject copies testdata/screenshots into a temporary project folder
// named orders-service, so the page shows a readable project name.
func sampleProject(t *testing.T) string {
	t.Helper()
	cwd := filepath.Join(t.TempDir(), "orders-service")
	src := filepath.Join("testdata", "screenshots")
	require.NoError(t, filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		dst := filepath.Join(cwd, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	}))
	return cwd
}

// sampleAsk is a two-question set about the sample spec.
func sampleAsk(specPath, codePath string) *protocol.Ask {
	return &protocol.Ask{QSID: "qs_retry", Title: "Retry policy", Questions: []protocol.AskQuestion{
		{
			ID:   "q1",
			Text: "Should a successful delivery reset the backoff for the partner's other pending events?",
			Options: []protocol.Option{
				{Label: "Yes, per partner", Description: "A partner that recovers gets its backlog quickly.", Recommended: true},
				{Label: "No, per event", Description: "Each event keeps its own backoff, as Backoff does today."},
			},
			AllowOther: true,
			Docs:       []protocol.ResolvedDocRef{{Path: specPath, Quote: "each later retry doubles the wait, capped at 30s", Section: "Retry policy", Kind: "spec"}},
			Code:       []protocol.ResolvedRef{{Path: codePath, StartLine: 24, EndLine: 31, Symbol: "RetryPolicy.Backoff", Kind: "method", Source: "symbol"}},
		},
		{
			ID:   "q2",
			Text: "Should a 429 answer honor the Retry-After header?",
			Options: []protocol.Option{
				{Label: "Yes, when longer", Description: "Wait for Retry-After when it is longer than the backoff.", Recommended: true},
				{Label: "No", Description: "Treat 429 like a 5xx status."},
			},
			AllowOther: true,
			Docs:       []protocol.ResolvedDocRef{{Path: specPath, Quote: "endpoint answers with a 5xx status or 429", Section: "Retry policy", Kind: "spec"}},
			Code:       []protocol.ResolvedRef{{Path: codePath, StartLine: 36, EndLine: 46, Symbol: "classify", Kind: "func", Source: "symbol"}},
		},
	}}
}

// moduleRoot walks up from the working directory to the folder with go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "no go.mod above the working directory")
		dir = parent
	}
}
