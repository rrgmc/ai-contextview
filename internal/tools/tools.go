// Package tools implements the MCP tools of `ai-specview mcp`: spec_show, focus,
// ask and ask_wait.
package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rrgmc/ai-specview/internal/identity"
	"github.com/rrgmc/ai-specview/internal/protocol"
	"github.com/rrgmc/ai-specview/internal/resolve"
	"github.com/rrgmc/ai-specview/internal/specsrc"
	"github.com/rrgmc/ai-specview/internal/validate"
)

// Daemon is the bridge's view of the daemon connection (appconn.Client).
type Daemon interface {
	Request(ctx context.Context, typ string, body any) (protocol.Envelope, error)
	WaitResult(ctx context.Context, qs string, d time.Duration) (*protocol.AskResult, error)
	HasResult(qs string) bool
	URL() string
}

// Bridge holds the per-instance state of the tool handlers.
type Bridge struct {
	Cwd      string
	Daemon   Daemon
	Resolver resolve.Resolver
	// AskWait is how long ask and ask_wait block before returning pending.
	AskWait time.Duration
	// Open opens a URL in the browser; nil never opens.
	Open func(url string)
	// OpenIn is reported in the spec_show result: OpenPane asks Claude to open
	// the viewer in the built-in browser pane (see OpenInFromEnv).
	OpenIn string
	// ReopenDelay is how long ask waits, in pane mode, for a page to connect
	// before it opens the system browser instead. Zero means DefaultReopenDelay.
	ReopenDelay time.Duration
	Logf        func(format string, args ...any)

	mu      sync.Mutex
	root    *specsrc.Root
	pending map[string]bool
}

// Messages returned to Claude.
const (
	MsgInChat   = "The user will answer in the chat. Wait for their message."
	MsgPending  = "Still waiting for the user. Call ask_wait with this id to keep waiting."
	MsgNoViewer = "No viewer page is open, so the user cannot see these questions. " +
		"If you have the built-in browser tools, open url in the built-in browser pane now (preview_start with url) and tell the user in one line; " +
		"otherwise the system browser opens shortly. Then call ask_wait with this id."
	MsgDismissed = "The user skipped these questions. Continue without these decisions; ask in chat only if one is essential."
	MsgDeferred  = "The user chose to decide some questions later (see deferred). Add each to the spec's open questions with its refs, " +
		"continue without the decision, and ask them again before finishing."
)

// DefaultReopenDelay is the ReopenDelay used when it is zero.
const DefaultReopenDelay = 10 * time.Second

// OpenPane asks Claude to open the viewer in the built-in browser pane of the
// Claude desktop app. It is the default; AI_SPECVIEW_OPEN=browser or never
// turns it off.
const OpenPane = "pane"

// OpenInFromEnv returns OpenPane unless AI_SPECVIEW_OPEN is set to another
// value than pane (browser or never).
func OpenInFromEnv() string {
	switch os.Getenv("AI_SPECVIEW_OPEN") {
	case "", OpenPane:
		return OpenPane
	}
	return ""
}

// OpenBrowser opens a URL with the platform opener unless AI_SPECVIEW_OPEN=never.
// It also opens with the pane default: the bridge calls it only when no page
// is open, so it is the fallback when there is no pane.
func OpenBrowser(url string) {
	if os.Getenv("AI_SPECVIEW_OPEN") == "never" {
		return
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		// Not "cmd /c start": cmd would split the URL at "&".
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func (b *Bridge) logf(format string, args ...any) {
	if b.Logf != nil {
		b.Logf(format, args...)
	}
}

func (b *Bridge) absPath(p string) string {
	p = expandPath(p)
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(b.Cwd, p)
}

// Register adds the tools to an MCP server.
func Register(s *mcp.Server, b *Bridge) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "spec_show",
		Description: "Show a spec in the ai-specview viewer (a browser page beside the chat). " +
			"Call it as soon as the spec path is known and again after every edit, with the headings you changed. " +
			"The path is a markdown file or a folder of markdown artifacts. Returns the section names and the viewer url. " +
			"If the result has open: \"pane\", open the url in the built-in browser pane when that tool is available.",
	}, b.SpecShow)
	mcp.AddTool(s, &mcp.Tool{
		Name: "focus",
		Description: "Highlight a spec passage and open code in the viewer, without asking anything. " +
			"Quotes must be copied verbatim from the file.",
	}, b.Focus)
	mcp.AddTool(s, &mcp.Tool{
		Name: "ask",
		Description: "Ask the user 1-6 clarifying questions in the ai-specview viewer and wait for the answers. " +
			"Use this for every clarifying question instead of asking in chat. Each question needs refs: " +
			"spec_quote (copied verbatim from the spec, with spec_section), doc_refs into other artifacts, " +
			"and/or code_refs (Go: symbol like pkg.(*Type).Method; other files: lines N-M). " +
			"Results: answered (apply the answers), in_chat (wait for the user's chat message), " +
			"pending (call ask_wait with question_set_id; a pending result with url means no viewer page is open: open url in the built-in browser pane first), " +
			"dismissed (continue without it). " +
			"An answered result may list deferred questions (the user chose Decide later): record them as open questions and ask again before finishing. " +
			"Mark the option you recommend with recommended: true.",
	}, b.Ask)
	mcp.AddTool(s, &mcp.Tool{
		Name: "spec_done",
		Description: "Tell the viewer the spec is finished and no more questions are coming. " +
			"Call it once at the end of a spec session, with a summary as markdown with a **Decided** heading and a **Still open** heading, each followed by a bullet list with one item per decision or open point (never a run-on paragraph). " +
			"The page shows it as a finished banner. Asking again later reopens the session.",
	}, b.SpecDone)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "ask_wait",
		Description: "Keep waiting for a pending question set returned by ask. Same result shape as ask.",
	}, b.AskWaitTool)
}

func (b *Bridge) currentRoot() *specsrc.Root {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.root
}

// SpecShow implements the spec_show tool.
func (b *Bridge) SpecShow(ctx context.Context, _ *mcp.CallToolRequest, in protocol.SpecShowInput) (*mcp.CallToolResult, protocol.SpecShowOutput, error) {
	var out protocol.SpecShowOutput
	if strings.TrimSpace(in.Path) == "" {
		return nil, out, errors.New("path is required")
	}
	abs := b.absPath(in.Path)
	root, err := specsrc.LoadRoot(abs, expandPath(in.Primary))
	if err != nil {
		return nil, out, err
	}
	if !root.IsDir && len(in.Changed) > 0 {
		return nil, out, errors.New("changed is only allowed when path is a folder; use changed_sections")
	}
	b.mu.Lock()
	b.root = root
	b.mu.Unlock()

	req := &protocol.SpecShow{
		RootAbs: abs, IsDir: root.IsDir, ChangedSections: in.ChangedSections, Changed: in.Changed,
	}
	if root.IsDir {
		req.Primary = root.Primary
	}
	reply, err := b.Daemon.Request(ctx, protocol.TypeSpecShow, req)
	if err != nil {
		return nil, out, err
	}
	var res protocol.SpecShowResult
	if err := reply.Decode(&res); err != nil {
		return nil, out, err
	}
	out = protocol.SpecShowOutput{OK: true, URL: b.Daemon.URL(), Open: b.OpenIn, Primary: res.Primary, Sections: res.Sections, Artifacts: res.Artifacts}
	if out.Sections == nil {
		out.Sections = []string{}
	}
	return nil, out, nil
}

// resolveDocs folds spec_quote into the doc refs, resolves their paths and
// pre-checks the quotes. Problems are appended to errs.
func (b *Bridge) resolveDocs(label, specQuote, specSection string, refs []protocol.DocRef, errs *[]string) []protocol.ResolvedDocRef {
	all := refs
	if specQuote != "" {
		all = append([]protocol.DocRef{{Quote: specQuote, Section: specSection}}, refs...)
	}
	if len(all) == 0 {
		return []protocol.ResolvedDocRef{}
	}
	root := b.currentRoot()
	if root == nil {
		*errs = append(*errs, label+": no spec is shown yet; call spec_show first")
		return nil
	}
	out := make([]protocol.ResolvedDocRef, 0, len(all))
	for _, d := range all {
		abs := root.PrimaryAbs()
		if d.Path != "" {
			switch p := expandPath(d.Path); {
			case filepath.IsAbs(p):
				abs = filepath.Clean(p)
			case root.IsDir:
				abs = filepath.Join(root.Dir, d.Path)
			default:
				abs = filepath.Join(b.Cwd, d.Path)
			}
		}
		name := root.Rel(abs)
		if !specsrc.IsMarkdown(abs) {
			*errs = append(*errs, fmt.Sprintf("%s: doc ref %s is not a markdown file", label, d.Path))
			continue
		}
		if root.IsDir && strings.HasPrefix(name, "../") {
			*errs = append(*errs, fmt.Sprintf("%s: doc ref %s is outside the shown folder %s", label, d.Path, root.Dir))
			continue
		}
		src, err := specsrc.ReadPrepared(abs)
		if err != nil {
			*errs = append(*errs, fmt.Sprintf("%s: doc ref %s does not exist", label, d.Path))
			continue
		}
		if m := specsrc.Match(src, d.Quote, ""); !m.Found() {
			closest := specsrc.ClosestHeadings(src, d.Quote, 3)
			*errs = append(*errs, fmt.Sprintf("%s: quote %q not found in %s (copy it verbatim from the file); closest sections: %s",
				label, d.Quote, name, strings.Join(closest, ", ")))
			continue
		}
		kind := specsrc.InferKind(name, "")
		if a := root.Artifact(name); a != nil {
			kind = a.Kind
		}
		out = append(out, protocol.ResolvedDocRef{Path: abs, Quote: d.Quote, Section: d.Section, Kind: kind})
	}
	return out
}

func (b *Bridge) resolveCode(ctx context.Context, label string, refs []protocol.CodeRef, errs *[]string) []protocol.ResolvedRef {
	out := make([]protocol.ResolvedRef, 0, len(refs))
	for _, r := range refs {
		res, err := b.Resolver.Resolve(ctx, b.Cwd, r)
		if err != nil {
			*errs = append(*errs, fmt.Sprintf("%s: code ref %s: %v", label, r.Path, err))
			continue
		}
		out = append(out, res)
	}
	return out
}

func errList(errs []string) error {
	if len(errs) == 0 {
		return nil
	}
	return errors.New("fix these and call again:\n- " + strings.Join(errs, "\n- "))
}

// Focus implements the focus tool.
func (b *Bridge) Focus(ctx context.Context, _ *mcp.CallToolRequest, in protocol.FocusInput) (*mcp.CallToolResult, protocol.FocusOutput, error) {
	var out protocol.FocusOutput
	if err := validate.Focus(&in); err != nil {
		return nil, out, err
	}
	var errs []string
	docs := b.resolveDocs("focus", in.SpecQuote, in.SpecSection, in.DocRefs, &errs)
	code := b.resolveCode(ctx, "focus", in.CodeRefs, &errs)
	if err := errList(errs); err != nil {
		return nil, out, err
	}
	reply, err := b.Daemon.Request(ctx, protocol.TypeFocus, &protocol.Focus{Docs: docs, Code: code})
	if err != nil {
		return nil, out, err
	}
	var res protocol.FocusResult
	if err := reply.Decode(&res); err != nil {
		return nil, out, err
	}
	out = protocol.FocusOutput{OK: true, Docs: res.Docs, Code: code}
	if out.Docs == nil {
		out.Docs = []protocol.DocMatch{}
	}
	if in.SpecQuote != "" && len(res.Docs) > 0 {
		out.SpecMatch = res.Docs[0].Match
	}
	return nil, out, nil
}

// Ask implements the ask tool.
func (b *Bridge) Ask(ctx context.Context, _ *mcp.CallToolRequest, in protocol.AskInput) (*mcp.CallToolResult, protocol.AskOutput, error) {
	var out protocol.AskOutput
	if err := validate.Ask(&in); err != nil {
		return nil, out, err
	}
	normalizeRecommended(&in)
	ask := &protocol.Ask{QSID: identity.ShortID("qs_"), Title: in.Title}
	var errs []string
	for _, q := range in.Questions {
		label := "question " + q.ID
		ask.Questions = append(ask.Questions, protocol.AskQuestion{
			ID: q.ID, Text: q.Text, Options: q.Options, MultiSelect: q.MultiSelect, AllowOther: q.AllowOther,
			Docs: b.resolveDocs(label, q.SpecQuote, q.SpecSection, q.DocRefs, &errs),
			Code: b.resolveCode(ctx, label, q.CodeRefs, &errs),
		})
	}
	if err := errList(errs); err != nil {
		return nil, out, err
	}

	reply, err := b.Daemon.Request(ctx, protocol.TypeAsk, ask)
	if err != nil {
		return nil, out, err
	}
	var shown protocol.AskShown
	if err := reply.Decode(&shown); err != nil {
		return nil, out, err
	}
	for _, m := range shown.Matches {
		if m.Match != protocol.MatchNone {
			continue
		}
		b.cancel(ask.QSID, "quote_not_found")
		for _, q := range ask.Questions {
			if q.ID == m.ID && m.Doc < len(q.Docs) {
				d := q.Docs[m.Doc]
				src, _ := specsrc.ReadPrepared(d.Path)
				return nil, out, fmt.Errorf("question %s: quote %q not found in %s; closest sections: %s",
					q.ID, d.Quote, d.Path, strings.Join(specsrc.ClosestHeadings(src, d.Quote, 3), ", "))
			}
		}
		return nil, out, fmt.Errorf("question %s: quote not found", m.ID)
	}
	if u := b.Daemon.URL(); shown.Viewers == 0 && u != "" {
		if b.OpenIn == OpenPane {
			// No page shows the questions, for example because the user closed
			// the pane. Claude can reopen the pane only once ask returns, so
			// return now and open the system browser only if no page connects.
			b.mu.Lock()
			if b.pending == nil {
				b.pending = map[string]bool{}
			}
			b.pending[ask.QSID] = true
			b.mu.Unlock()
			if b.Open != nil {
				go b.openLater(ask.QSID, u)
			}
			return nil, protocol.AskOutput{Status: protocol.StatusPending, QuestionSetID: ask.QSID, URL: u, Message: MsgNoViewer}, nil
		}
		if b.Open != nil {
			b.Open(u)
		}
	}
	return b.wait(ctx, ask.QSID)
}

// openLater opens the system browser for a question set after ReopenDelay,
// unless a page shows the workspace by then or the set has a result.
func (b *Bridge) openLater(qs, url string) {
	d := b.ReopenDelay
	if d <= 0 {
		d = DefaultReopenDelay
	}
	time.Sleep(d)
	if b.Daemon.HasResult(qs) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	reply, err := b.Daemon.Request(ctx, protocol.TypeViewers, &protocol.Viewers{})
	if err == nil {
		var res protocol.ViewersResult
		if err = reply.Decode(&res); err == nil && res.Viewers > 0 {
			return
		}
	}
	if err != nil {
		b.logf("viewers %s: %v", qs, err)
	}
	b.Open(url)
}

// SpecDone implements the spec_done tool.
func (b *Bridge) SpecDone(ctx context.Context, _ *mcp.CallToolRequest, in protocol.SpecDoneInput) (*mcp.CallToolResult, protocol.SpecDoneOutput, error) {
	req := &protocol.Done{Summary: strings.TrimSpace(in.Summary)}
	if err := req.Validate(); err != nil {
		return nil, protocol.SpecDoneOutput{}, err
	}
	if _, err := b.Daemon.Request(ctx, protocol.TypeDone, req); err != nil {
		return nil, protocol.SpecDoneOutput{}, err
	}
	return nil, protocol.SpecDoneOutput{OK: true, URL: b.Daemon.URL()}, nil
}

// recommendedSuffix is the built-in question tool's convention for marking
// the recommended option; it is turned into the recommended flag.
const recommendedSuffix = "(recommended)"

func normalizeRecommended(in *protocol.AskInput) {
	for qi := range in.Questions {
		opts := in.Questions[qi].Options
		for oi := range opts {
			l := strings.TrimSpace(opts[oi].Label)
			if strings.HasSuffix(strings.ToLower(l), recommendedSuffix) {
				opts[oi].Label = strings.TrimSpace(l[:len(l)-len(recommendedSuffix)])
				opts[oi].Recommended = true
			}
		}
	}
}

// AskWaitTool implements the ask_wait tool.
func (b *Bridge) AskWaitTool(ctx context.Context, _ *mcp.CallToolRequest, in protocol.AskWaitInput) (*mcp.CallToolResult, protocol.AskOutput, error) {
	b.mu.Lock()
	known := b.pending[in.QuestionSetID]
	b.mu.Unlock()
	if !known && !b.Daemon.HasResult(in.QuestionSetID) {
		return nil, protocol.AskOutput{}, fmt.Errorf("unknown question_set_id %q", in.QuestionSetID)
	}
	return b.wait(ctx, in.QuestionSetID)
}

func (b *Bridge) wait(ctx context.Context, qs string) (*mcp.CallToolResult, protocol.AskOutput, error) {
	res, err := b.Daemon.WaitResult(ctx, qs, b.AskWait)
	if err != nil {
		if ctx.Err() != nil {
			b.cancel(qs, "client_cancelled")
		}
		return nil, protocol.AskOutput{}, err
	}
	b.mu.Lock()
	if b.pending == nil {
		b.pending = map[string]bool{}
	}
	if res == nil {
		b.pending[qs] = true
	} else {
		delete(b.pending, qs)
	}
	b.mu.Unlock()
	if res == nil {
		return nil, protocol.AskOutput{Status: protocol.StatusPending, QuestionSetID: qs, Message: MsgPending}, nil
	}
	out := protocol.AskOutput{Status: res.Status, Answers: res.Answers, Deferred: res.Deferred, KeepOpen: res.KeepOpen}
	switch res.Status {
	case protocol.StatusAnswered:
		if len(res.Deferred) > 0 {
			out.Message = MsgDeferred
		}
	case protocol.StatusInChat:
		out.Message = MsgInChat
	case protocol.StatusDismissed:
		out.Message = MsgDismissed
	}
	return nil, out, nil
}

func (b *Bridge) cancel(qs, reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := b.Daemon.Request(ctx, protocol.TypeAskCancel, &protocol.AskCancel{QSID: qs, Reason: reason}); err != nil {
		b.logf("ask_cancel %s: %v", qs, err)
	}
}
