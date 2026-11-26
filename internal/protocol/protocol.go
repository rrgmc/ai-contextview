// Package protocol defines the socket protocol between the bridge
// (ai-specview mcp) and the viewer daemon (ai-specview serve), and the shared
// types used by the MCP tools. See docs/01-protocol.md.
package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
)

// Version is the envelope version (`v`) and Protocol the message-set version
// sent in hello.
const (
	Version  = 1
	Protocol = 1
)

// MaxLine is the largest NDJSON line either side accepts.
const MaxLine = 4 << 20

// Message types.
const (
	TypeHello     = "hello"
	TypeHelloAck  = "hello_ack"
	TypeSpecShow  = "spec_show"
	TypeFocus     = "focus"
	TypeAsk       = "ask"
	TypeAskCancel = "ask_cancel"
	TypeBye       = "bye"
	TypeDone      = "done"
	TypeResult    = "result"
	TypeError     = "error"
	TypeAskShown  = "ask_shown"
	TypeAskResult = "ask_result"
	// TypeViewers asks how many pages show the workspace; the reply is a
	// result with ViewersResult.
	TypeViewers = "viewers"
	// TypeAskMode turns ask mode on or off for the workspace; the reply is
	// result {}.
	TypeAskMode = "ask_mode"
	// TypeDocWritten is sent by `ai-specview hook` after Claude writes a
	// markdown file; the reply is a result with DocWrittenResult.
	TypeDocWritten = "doc_written"
	// TypeSkillStarted is sent by `ai-specview hook` when Claude starts a
	// skill; the reply is result {}.
	TypeSkillStarted = "skill_started"
	// TypeDocShown is sent by the daemon to the bridge, unasked, when it
	// made a written file the working document.
	TypeDocShown = "doc_shown"
	// TypeSessionCwd carries the Claude session's working folder: from
	// `ai-specview hook` to the daemon (with pids), and from the daemon to
	// the bridge, unasked, when it changed. In a desktop-app worktree it is
	// the worktree, while the bridge was started in the main checkout.
	TypeSessionCwd = "session_cwd"
	// TypePing and TypeShutdown are daemon-management messages used by
	// `ai-specview serve` itself (single-instance check and --restart).
	TypePing     = "ping"
	TypeShutdown = "shutdown"
	// TypeActive is sent by `ai-specview hook`: is a spec open in ai-specview for
	// one of these Claude Code processes?
	TypeActive = "active"
)

// Error codes.
const (
	CodeBadRequest   = "bad_request"
	CodeFileNotFound = "file_not_found"
	CodeNotMarkdown  = "not_markdown"
	CodeEmptyFolder  = "empty_folder"
	CodeUnknownQS    = "unknown_qs"
	CodeBusy         = "busy"
	CodeInternal     = "internal"
)

var errorCodes = map[string]bool{
	CodeBadRequest: true, CodeFileNotFound: true, CodeNotMarkdown: true,
	CodeEmptyFolder: true, CodeUnknownQS: true, CodeBusy: true, CodeInternal: true,
}

// Ask result statuses.
const (
	StatusAnswered  = "answered"
	StatusInChat    = "in_chat"
	StatusDismissed = "dismissed"
	StatusPending   = "pending"
)

// Envelope wraps every socket message.
type Envelope struct {
	V    int             `json:"v"`
	Type string          `json:"type"`
	ID   string          `json:"id,omitempty"`
	Re   string          `json:"re,omitempty"`
	WS   string          `json:"ws,omitempty"`
	Body json.RawMessage `json:"body,omitempty"`
}

// Validate checks the envelope fields that do not depend on the type.
func (e *Envelope) Validate() error {
	if e.V != Version {
		return fmt.Errorf("unsupported envelope version %d", e.V)
	}
	if e.Type == "" {
		return errors.New("missing type")
	}
	return nil
}

// Decode unmarshals the body into v.
func (e *Envelope) Decode(v any) error {
	if len(e.Body) == 0 {
		return json.Unmarshal([]byte("{}"), v)
	}
	return json.Unmarshal(e.Body, v)
}

// NewEnvelope builds an envelope with a marshalled body.
func NewEnvelope(typ, id, re, ws string, body any) (Envelope, error) {
	env := Envelope{V: Version, Type: typ, ID: id, Re: re, WS: ws}
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return env, err
		}
		env.Body = b
	}
	return env, nil
}

// Validator is implemented by every message body.
type Validator interface{ Validate() error }

// ---------------------------------------------------------------- shared types

// ArtifactKind classifies a markdown artifact.
type ArtifactKind string

const (
	KindSpec         ArtifactKind = "spec"
	KindADR          ArtifactKind = "adr"
	KindResearch     ArtifactKind = "research"
	KindOpenQuestion ArtifactKind = "open_question"
	KindOther        ArtifactKind = "other"
)

// Valid reports whether k is a known kind.
func (k ArtifactKind) Valid() bool {
	switch k {
	case KindSpec, KindADR, KindResearch, KindOpenQuestion, KindOther:
		return true
	}
	return false
}

// Match is the quality of a quote match.
type Match string

const (
	MatchExact      Match = "exact"
	MatchNormalized Match = "normalized"
	MatchFuzzy      Match = "fuzzy"
	MatchHeading    Match = "heading"
	MatchNone       Match = "none"
)

// CodeRef points into code, as given by Claude.
type CodeRef struct {
	Path   string `json:"path" jsonschema:"file path, relative to the project directory or absolute"`
	Symbol string `json:"symbol,omitempty" jsonschema:"Go symbol: pkg.Func, pkg.Type, pkg.(*Type).Method, pkg.Type.Method or pkg.Var"`
	Lines  string `json:"lines,omitempty" jsonschema:"line or line range: N or N-M"`
}

// ResolvedRef is a code ref resolved to an absolute path and line range.
type ResolvedRef struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Symbol    string `json:"symbol,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Source    string `json:"source"`
}

// Validate checks a resolved code ref.
func (r *ResolvedRef) Validate() error {
	if !filepath.IsAbs(r.Path) {
		return fmt.Errorf("code ref path %q is not absolute", r.Path)
	}
	if r.StartLine < 1 || r.EndLine < r.StartLine {
		return fmt.Errorf("code ref %s: bad line range %d-%d", r.Path, r.StartLine, r.EndLine)
	}
	if r.Source != "symbol" && r.Source != "lines" {
		return fmt.Errorf("code ref %s: bad source %q", r.Path, r.Source)
	}
	return nil
}

// DocRef points into a markdown document, as given by Claude.
type DocRef struct {
	Path    string `json:"path,omitempty" jsonschema:"document path: relative to the spec_show folder, else to the project dir, or absolute; omit for the primary spec file"`
	Quote   string `json:"quote,omitempty" jsonschema:"verbatim passage copied from a markdown file (3-300 characters); give quote or lines"`
	Lines   string `json:"lines,omitempty" jsonschema:"line or line range N or N-M; the only choice for a file that is not markdown"`
	Section string `json:"section,omitempty" jsonschema:"heading of the section containing the quote"`
}

// ResolvedDocRef is a doc ref with an absolute path and artifact kind.
type ResolvedDocRef struct {
	Path      string       `json:"path"`
	Quote     string       `json:"quote,omitempty"`
	StartLine int          `json:"start_line,omitempty"`
	EndLine   int          `json:"end_line,omitempty"`
	Section   string       `json:"section,omitempty"`
	Kind      ArtifactKind `json:"kind"`
}

// Validate checks a resolved doc ref.
func (r *ResolvedDocRef) Validate() error {
	if !filepath.IsAbs(r.Path) {
		return fmt.Errorf("doc ref path %q is not absolute", r.Path)
	}
	if (r.Quote == "") == (r.StartLine == 0) {
		return errors.New("doc ref needs exactly one of quote or start_line")
	}
	if r.StartLine < 0 || (r.StartLine > 0 && r.EndLine < r.StartLine) {
		return fmt.Errorf("doc ref %s: bad lines %d-%d", r.Path, r.StartLine, r.EndLine)
	}
	if !r.Kind.Valid() {
		return fmt.Errorf("doc ref %s: bad kind %q", r.Path, r.Kind)
	}
	return nil
}

// Artifact is one markdown file of the shown root.
type Artifact struct {
	Path     string       `json:"path"`
	Kind     ArtifactKind `json:"kind"`
	Sections []string     `json:"sections"`
}

// ChangedFile names a file the caller just created or edited.
type ChangedFile struct {
	Path     string   `json:"path" jsonschema:"file path relative to the folder"`
	Sections []string `json:"sections,omitempty" jsonschema:"headings changed in that file"`
}

// Option is one answer option of a question.
type Option struct {
	Label       string `json:"label" jsonschema:"short option label"`
	Description string `json:"description,omitempty" jsonschema:"optional longer explanation"`
	Recommended bool   `json:"recommended,omitempty" jsonschema:"true for the option you recommend (at most one per question)"`
}

// Answer is the user's answer to one question. Deferred means the user
// chose "Decide later": Selected is empty and Notes may say why.
type Answer struct {
	ID       string   `json:"id"`
	Selected []string `json:"selected"`
	Other    *string  `json:"other"`
	Notes    *string  `json:"notes"`
	Deferred bool     `json:"deferred,omitempty"`
}

// ----------------------------------------------------------- bridge → daemon

// Hello opens a workspace.
type Hello struct {
	Protocol      int    `json:"protocol"`
	WS            string `json:"ws"`
	Cwd           string `json:"cwd"`
	ClaudePID     int    `json:"claude_pid"`
	BridgePID     int    `json:"bridge_pid"`
	BridgeVersion string `json:"bridge_version"`
	TTY           string `json:"tty,omitempty"`
	ProjectName   string `json:"project_name"`
	// BridgePPID is the bridge's parent: the Claude Code process that
	// started it. `ai-specview hook` matches it against its own ancestors.
	BridgePPID int `json:"bridge_ppid,omitempty"`
}

func (h *Hello) Validate() error {
	if h.Protocol < 1 {
		return errors.New("hello: missing protocol")
	}
	if h.WS == "" {
		return errors.New("hello: missing ws")
	}
	if !filepath.IsAbs(h.Cwd) {
		return errors.New("hello: cwd must be absolute")
	}
	return nil
}

// SpecShow declares the shown root.
type SpecShow struct {
	RootAbs         string        `json:"root_abs"`
	IsDir           bool          `json:"is_dir"`
	Primary         string        `json:"primary,omitempty"`
	ChangedSections []string      `json:"changed_sections,omitempty"`
	Changed         []ChangedFile `json:"changed,omitempty"`
	// Auto marks a replay of a working document the daemon picked from a
	// write (doc_shown), so it does not count as Claude's own spec_show.
	Auto bool `json:"auto,omitempty"`
}

func (s *SpecShow) Validate() error {
	if !filepath.IsAbs(s.RootAbs) {
		return errors.New("spec_show: root_abs must be absolute")
	}
	if !s.IsDir && (s.Primary != "" || len(s.Changed) > 0) {
		return errors.New("spec_show: primary and changed are only allowed for folders")
	}
	return nil
}

// Focus highlights refs without asking.
type Focus struct {
	Docs []ResolvedDocRef `json:"docs"`
	Code []ResolvedRef    `json:"code"`
}

func (f *Focus) Validate() error {
	if len(f.Docs) == 0 && len(f.Code) == 0 {
		return errors.New("focus: no refs")
	}
	for i := range f.Docs {
		if err := f.Docs[i].Validate(); err != nil {
			return err
		}
	}
	for i := range f.Code {
		if err := f.Code[i].Validate(); err != nil {
			return err
		}
	}
	return nil
}

// AskQuestion is one question of an ask, with refs already resolved.
type AskQuestion struct {
	ID          string           `json:"id"`
	Text        string           `json:"text"`
	Options     []Option         `json:"options"`
	MultiSelect bool             `json:"multi_select,omitempty"`
	AllowOther  bool             `json:"allow_other,omitempty"`
	Docs        []ResolvedDocRef `json:"docs"`
	Code        []ResolvedRef    `json:"code"`
}

// Ask shows a question set.
type Ask struct {
	QSID      string        `json:"qs_id"`
	Title     string        `json:"title,omitempty"`
	Questions []AskQuestion `json:"questions"`
}

func (a *Ask) Validate() error {
	if a.QSID == "" {
		return errors.New("ask: missing qs_id")
	}
	if len(a.Questions) < 1 || len(a.Questions) > 6 {
		return fmt.Errorf("ask: %d questions, want 1-6", len(a.Questions))
	}
	seen := map[string]bool{}
	for _, q := range a.Questions {
		if q.ID == "" || q.Text == "" {
			return errors.New("ask: question without id or text")
		}
		if seen[q.ID] {
			return fmt.Errorf("ask: duplicate question id %q", q.ID)
		}
		seen[q.ID] = true
		if len(q.Options) < 2 || len(q.Options) > 6 {
			return fmt.Errorf("ask: question %s has %d options, want 2-6", q.ID, len(q.Options))
		}
		// A question may have no refs: in ask mode the bridge allows it, and
		// it checks the rule itself otherwise.
		for i := range q.Docs {
			if err := q.Docs[i].Validate(); err != nil {
				return err
			}
		}
		for i := range q.Code {
			if err := q.Code[i].Validate(); err != nil {
				return err
			}
		}
	}
	return nil
}

// AskCancel withdraws a question set.
type AskCancel struct {
	QSID   string `json:"qs_id"`
	Reason string `json:"reason"`
}

func (a *AskCancel) Validate() error {
	if a.QSID == "" {
		return errors.New("ask_cancel: missing qs_id")
	}
	return nil
}

// MaxSummary is the longest spec_done summary, in bytes.
const MaxSummary = 8000

// Done tells the viewer that Claude finished the spec: no more questions are
// coming. A later ask clears it.
type Done struct {
	Summary string `json:"summary,omitempty"`
}

func (d *Done) Validate() error {
	if len(d.Summary) > MaxSummary {
		return fmt.Errorf("done: summary is %d bytes, at most %d", len(d.Summary), MaxSummary)
	}
	return nil
}

// AskMode turns ask mode on or off. In ask mode every question of the
// session goes through ai-specview, with or without a spec, and the hook
// blocks the built-in question tool.
type AskMode struct {
	On bool `json:"on"`
	// Spec turns on spec mode: ask mode that expects a spec and searches
	// harder for it (07).
	Spec bool `json:"spec,omitempty"`
	// Paths are absolute glob patterns; in spec mode, the first written
	// file that matches becomes the working document.
	Paths []string `json:"paths,omitempty"`
}

func (a *AskMode) Validate() error { return nil }

// DocWritten tells the daemon that the Claude session of one of PIDs wrote
// Path, an absolute markdown file that the hook's file rules let through.
type DocWritten struct {
	PIDs []int  `json:"pids"`
	Path string `json:"path"`
	// Cwd is the session's working folder, from the hook's input.
	Cwd string `json:"cwd,omitempty"`
}

func (d *DocWritten) Validate() error {
	if !filepath.IsAbs(d.Path) {
		return fmt.Errorf("doc_written: path %q is not absolute", d.Path)
	}
	return nil
}

// DocWrittenResult answers doc_written. Shown is true when the file just
// became the working document.
type DocWrittenResult struct {
	Shown bool   `json:"shown"`
	Path  string `json:"path,omitempty"`
}

func (r *DocWrittenResult) Validate() error { return nil }

// SkillStarted tells the daemon that the Claude session of one of PIDs
// started the skill Name.
type SkillStarted struct {
	PIDs []int  `json:"pids"`
	Name string `json:"name"`
	// Cwd is the session's working folder, from the hook's input.
	Cwd string `json:"cwd,omitempty"`
}

func (s *SkillStarted) Validate() error {
	if s.Name == "" {
		return errors.New("skill_started: missing name")
	}
	return nil
}

// SessionCwd is the Claude session's working folder. PIDs are set only on
// the hook's message.
type SessionCwd struct {
	PIDs []int  `json:"pids,omitempty"`
	Cwd  string `json:"cwd"`
}

func (s *SessionCwd) Validate() error {
	if !filepath.IsAbs(s.Cwd) {
		return fmt.Errorf("session_cwd: cwd %q is not absolute", s.Cwd)
	}
	return nil
}

// DocShown tells the bridge that the daemon made RootAbs the working
// document, as if spec_show had shown it.
type DocShown struct {
	RootAbs string `json:"root_abs"`
}

func (d *DocShown) Validate() error {
	if !filepath.IsAbs(d.RootAbs) {
		return fmt.Errorf("doc_shown: root_abs %q is not absolute", d.RootAbs)
	}
	return nil
}

// Bye closes the workspace.
type Bye struct{}

func (Bye) Validate() error { return nil }

// ----------------------------------------------------------- daemon → bridge

// HelloAck answers hello.
type HelloAck struct {
	DaemonVersion string `json:"daemon_version"`
	URL           string `json:"url"`
	AdoptedFrom   string `json:"adopted_from,omitempty"`
}

func (h *HelloAck) Validate() error {
	if h.DaemonVersion == "" || h.URL == "" {
		return errors.New("hello_ack: missing daemon_version or url")
	}
	return nil
}

// SpecShowResult answers spec_show.
type SpecShowResult struct {
	Primary   string     `json:"primary"`
	Sections  []string   `json:"sections"`
	Artifacts []Artifact `json:"artifacts"`
}

func (r *SpecShowResult) Validate() error {
	if r.Primary == "" {
		return errors.New("spec_show result: missing primary")
	}
	for _, a := range r.Artifacts {
		if !a.Kind.Valid() {
			return fmt.Errorf("spec_show result: bad kind %q", a.Kind)
		}
	}
	return nil
}

// DocMatch is the match quality of one doc ref.
type DocMatch struct {
	Path  string `json:"path"`
	Match Match  `json:"match"`
}

// FocusResult answers focus.
type FocusResult struct {
	Docs []DocMatch `json:"docs"`
}

func (r *FocusResult) Validate() error { return nil }

// QuestionMatch is the match quality of one doc ref of one question.
type QuestionMatch struct {
	ID    string `json:"id"`
	Doc   int    `json:"doc"`
	Match Match  `json:"match"`
}

// AskShown tells the bridge the question set is on screen.
type AskShown struct {
	QSID    string          `json:"qs_id"`
	Viewers int             `json:"viewers"`
	Matches []QuestionMatch `json:"matches"`
}

func (a *AskShown) Validate() error {
	if a.QSID == "" {
		return errors.New("ask_shown: missing qs_id")
	}
	return nil
}

// AskResult carries the user's action on a question set. Deferred lists the
// questions the user chose to decide later (answered only). KeepOpen is
// kept for older viewers.
type AskResult struct {
	QSID     string   `json:"qs_id"`
	Status   string   `json:"status"`
	Answers  []Answer `json:"answers,omitempty"`
	Deferred []string `json:"deferred,omitempty"`
	KeepOpen []string `json:"keep_open,omitempty"`
}

func (a *AskResult) Validate() error {
	if a.QSID == "" {
		return errors.New("ask_result: missing qs_id")
	}
	switch a.Status {
	case StatusAnswered:
		if len(a.Answers) == 0 {
			return errors.New("ask_result: answered without answers")
		}
	case StatusInChat, StatusDismissed:
	default:
		return fmt.Errorf("ask_result: bad status %q", a.Status)
	}
	return nil
}

// ErrorBody is the body of an error reply.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *ErrorBody) Validate() error {
	if !errorCodes[e.Code] {
		return fmt.Errorf("error: unknown code %q", e.Code)
	}
	if e.Message == "" {
		return errors.New("error: missing message")
	}
	return nil
}

func (e *ErrorBody) Error() string { return e.Code + ": " + e.Message }

// Active asks whether a spec is open for one of these processes (the
// hook's ancestor process ids).
type Active struct {
	PIDs []int `json:"pids"`
	// Cwd is the session's working folder, from the hook's input.
	Cwd string `json:"cwd,omitempty"`
}

// ActiveResult answers active.
type ActiveResult struct {
	Active bool   `json:"active"`
	WS     string `json:"ws,omitempty"`
	URL    string `json:"url,omitempty"`
	// AskMode is true when the session is active because of ask mode, with
	// no spec open.
	AskMode bool `json:"ask_mode,omitempty"`
}

// Viewers asks how many browser pages show the bridge's workspace. Its body
// is empty.
type Viewers struct{}

func (v *Viewers) Validate() error { return nil }

// ViewersResult answers viewers.
type ViewersResult struct {
	Viewers int `json:"viewers"`
}

func (r *ViewersResult) Validate() error {
	if r.Viewers < 0 {
		return errors.New("viewers result: negative viewers")
	}
	return nil
}

// PingResult answers ping.
type PingResult struct {
	DaemonVersion string `json:"daemon_version"`
	URL           string `json:"url"`
	PID           int    `json:"pid"`
}
