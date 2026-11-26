package daemon

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rrgmc/ai-specview/internal/protocol"
	"github.com/rrgmc/ai-specview/internal/render"
	"github.com/rrgmc/ai-specview/internal/specsrc"
)

const (
	changedTTL    = 5 * time.Minute
	adoptWindow   = 2 * time.Hour
	maxArchived   = 10
	maxCodeTabs   = 10
	statusReplace = "replaced"
	statusCancel  = "cancelled"
	statusGone    = "disconnected"
)

// CodeTab is one open code file.
type CodeTab struct {
	Path  string
	Start int
	End   int
	Lines int
	Stale bool
	mtime time.Time
	size  int64
}

type fileState struct {
	mtime   time.Time
	size    int64
	hashes  map[string]string
	changed map[string]time.Time
	// start is the file as first seen in this session (nil for a file
	// created during the session); answers is the file when the user last
	// replied to a question set. They are the bases of the Changes tab.
	start      []byte
	answers    []byte
	hasAnswers bool
}

type docMark struct {
	Rel, Ref, Quote, Section string
}

type questionSet struct {
	Ask   protocol.Ask
	Shown time.Time
}

// HistoryEntry is a closed question set.
type HistoryEntry struct {
	QS       *questionSet
	Status   string
	Answers  map[string]protocol.Answer
	KeepOpen []string
	Deferred []string
	At       time.Time
}

// Workspace is the daemon state of one Claude instance.
type Workspace struct {
	ID            string
	Project       string
	Cwd           string
	ClaudePID     int
	BridgePID     int
	BridgePPID    int
	BridgeVersion string
	Started       time.Time
	Live          bool
	ArchivedAt    time.Time

	conn *Conn

	Root       *specsrc.Root
	files      map[string]*fileState
	extraDocs  map[string]bool
	CurrentDoc string

	Code        []*CodeTab
	codeAllowed map[string]bool

	focus   []docMark
	Pending *questionSet
	History []*HistoryEntry
	// Done is set by spec_done: the spec is finished. A later ask clears it.
	Done *DoneState
	// AskMode is set by ask_mode: every question of the session goes
	// through ai-specview, with or without a spec.
	AskMode bool
	// AnswersAt is when the user last replied to a question set (send, back
	// to chat or skip); the Changes tab compares against the files then.
	AnswersAt time.Time
}

// DoneState is the finished-spec message.
type DoneState struct {
	Summary string
	At      time.Time
}

func newWorkspace(h *protocol.Hello) *Workspace {
	return &Workspace{
		ID: h.WS, Project: h.ProjectName, Cwd: h.Cwd, ClaudePID: h.ClaudePID, BridgePID: h.BridgePID, BridgePPID: h.BridgePPID,
		BridgeVersion: h.BridgeVersion, Started: time.Now(), Live: true,
		files: map[string]*fileState{}, extraDocs: map[string]bool{}, codeAllowed: map[string]bool{},
	}
}

// adopt moves the state of an archived workspace into w.
func (w *Workspace) adopt(old *Workspace) {
	w.Root, w.files, w.extraDocs, w.CurrentDoc = old.Root, old.files, old.extraDocs, old.CurrentDoc
	w.Code, w.codeAllowed, w.focus, w.History = old.Code, old.codeAllowed, old.focus, old.History
	w.Done = old.Done
	w.AskMode = old.AskMode
	w.AnswersAt = old.AnswersAt
}

// Change bases.
const (
	baseAnswers = "answers"
	baseStart   = "start"
)

// snapshotAnswers records every doc as it is now, as the "since your last
// answers" base.
func (w *Workspace) snapshotAnswers() {
	if w.Root == nil {
		return
	}
	for rel, fs := range w.files {
		if src, err := specsrc.ReadPrepared(filepath.Join(w.Root.Dir, filepath.FromSlash(rel))); err == nil {
			fs.answers, fs.hasAnswers = src, true
		}
	}
	w.AnswersAt = time.Now()
}

// defaultBase is "answers" once the user has replied to a question set,
// else "start".
func (w *Workspace) defaultBase() string {
	if w.AnswersAt.IsZero() {
		return baseStart
	}
	return baseAnswers
}

// baseFor returns the old version of a doc for a base. A doc created after
// the base was taken compares against nothing (all added).
func (w *Workspace) baseFor(rel, base string) []byte {
	fs := w.files[rel]
	if fs == nil {
		return nil
	}
	if base == baseAnswers && fs.hasAnswers {
		return fs.answers
	}
	if base == baseAnswers && !w.AnswersAt.IsZero() {
		return nil
	}
	return fs.start
}

// docAbs returns the absolute path for a doc key, if it may be shown.
func (w *Workspace) docAbs(rel string) (string, bool) {
	if w.Root == nil {
		return "", false
	}
	if w.Root.Artifact(rel) == nil && !w.extraDocs[rel] {
		return "", false
	}
	return filepath.Join(w.Root.Dir, filepath.FromSlash(rel)), true
}

// docKey turns an absolute doc path into its key (relative to the root dir).
func (w *Workspace) docKey(abs string) string {
	if w.Root == nil {
		return abs
	}
	rel := w.Root.Rel(abs)
	if w.Root.Artifact(rel) == nil {
		w.extraDocs[rel] = true
	}
	return rel
}

// changedIDs returns the active changed section ids of a doc.
func (w *Workspace) changedIDs(rel string) map[string]bool {
	out := map[string]bool{}
	if fs := w.files[rel]; fs != nil {
		for id, at := range fs.changed {
			if time.Since(at) < changedTTL {
				out[id] = true
			}
		}
	}
	return out
}

// loadFile (re)reads a doc and updates its section hashes. A file seen for
// the first time is a baseline unless markNew is set. It reports whether any
// section changed.
func (w *Workspace) loadFile(rel string, markNew bool) bool {
	abs := filepath.Join(w.Root.Dir, filepath.FromSlash(rel))
	st, err := os.Stat(abs)
	if err != nil {
		return false
	}
	src, err := specsrc.ReadPrepared(abs)
	if err != nil {
		return false
	}
	secs := specsrc.Sections(src)
	hashes := render.SectionHashes(src, secs)
	fs := w.files[rel]
	if fs == nil {
		fs = &fileState{changed: map[string]time.Time{}}
		w.files[rel] = fs
		fs.mtime, fs.size, fs.hashes = st.ModTime(), st.Size(), hashes
		if !markNew {
			fs.start = src
		}
		if markNew {
			now := time.Now()
			for id := range hashes {
				fs.changed[id] = now
			}
			return true
		}
		return false
	}
	fs.mtime, fs.size = st.ModTime(), st.Size()
	diff := render.SectionChanges(fs.hashes, hashes)
	fs.hashes = hashes
	if len(diff) == 0 {
		return false
	}
	// "changed" lasts until the next reload that changes something, or 5 min.
	now := time.Now()
	fs.changed = map[string]time.Time{}
	for _, id := range diff {
		fs.changed[id] = now
	}
	return true
}

// hint marks headings as changed (spec_show changed_sections / changed).
func (w *Workspace) hint(rel string, sections []string) bool {
	fs := w.files[rel]
	if fs == nil || len(sections) == 0 {
		return false
	}
	src, err := specsrc.ReadPrepared(filepath.Join(w.Root.Dir, filepath.FromSlash(rel)))
	if err != nil {
		return false
	}
	ids := render.SectionIDs(specsrc.Sections(src), sections)
	now := time.Now()
	for _, id := range ids {
		fs.changed[id] = now
	}
	return len(ids) > 0
}

// fileChanged reports whether a doc's mtime or size differ from the last load.
func (w *Workspace) fileChanged(rel string) bool {
	fs := w.files[rel]
	st, err := os.Stat(filepath.Join(w.Root.Dir, filepath.FromSlash(rel)))
	if err != nil {
		return false
	}
	return fs == nil || !fs.mtime.Equal(st.ModTime()) || fs.size != st.Size()
}

// marksFor returns the highlight marks of a doc: the focus refs and the
// refs of the pending question set.
func (w *Workspace) marksFor(rel string, src []byte) []render.Mark {
	var all []docMark
	all = append(all, w.focus...)
	if w.Pending != nil {
		all = append(all, w.pendingMarks()...)
	}
	var out []render.Mark
	for _, m := range all {
		if m.Rel != rel {
			continue
		}
		r := specsrc.Match(src, m.Quote, m.Section)
		switch {
		case r.Found():
			out = append(out, render.Mark{Start: r.Start, End: r.End, Ref: m.Ref})
		case r.Quality == protocol.MatchHeading:
			out = append(out, render.Mark{Start: r.Start, End: r.End, Ref: m.Ref, Heading: true})
		}
	}
	return out
}

func markRef(qid string, j int) string { return qid + "-" + itoa(j) }

func (w *Workspace) pendingMarks() []docMark {
	var out []docMark
	for _, q := range w.Pending.Ask.Questions {
		for j, d := range q.Docs {
			out = append(out, docMark{Rel: w.docKey(d.Path), Ref: markRef(q.ID, j), Quote: d.Quote, Section: d.Section})
		}
	}
	return out
}

// addCode opens or updates a code tab.
func (w *Workspace) addCode(r protocol.ResolvedRef) {
	w.codeAllowed[r.Path] = true
	for _, t := range w.Code {
		if t.Path == r.Path {
			t.Start, t.End, t.Stale = r.StartLine, r.EndLine, false
			return
		}
	}
	t := &CodeTab{Path: r.Path, Start: r.StartLine, End: r.EndLine}
	if st, err := os.Stat(r.Path); err == nil {
		t.mtime, t.size = st.ModTime(), st.Size()
	}
	if src, err := os.ReadFile(r.Path); err == nil {
		t.Lines = render.LineCount(src)
	}
	w.Code = append(w.Code, t)
	if len(w.Code) > maxCodeTabs {
		w.Code = w.Code[len(w.Code)-maxCodeTabs:]
	}
}

// openQuestions counts open_question artifacts.
func (w *Workspace) openQuestions() int {
	if w.Root == nil {
		return 0
	}
	n := 0
	for _, a := range w.Root.Artifacts {
		if a.Kind == protocol.KindOpenQuestion {
			n++
		}
	}
	return n
}

// watchDirs lists the directories this workspace needs watched.
func (w *Workspace) watchDirs() []string {
	set := map[string]bool{}
	if w.Root != nil {
		set[w.Root.Dir] = true
		if w.Root.IsDir {
			_ = filepath.WalkDir(w.Root.Dir, func(p string, d os.DirEntry, err error) error {
				if err != nil || !d.IsDir() {
					return nil
				}
				if p != w.Root.Dir && strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				rel, _ := filepath.Rel(w.Root.Dir, p)
				if strings.Count(filepath.ToSlash(rel), "/") >= 2 {
					return filepath.SkipDir
				}
				set[p] = true
				return nil
			})
		}
		for rel := range w.extraDocs {
			set[filepath.Dir(filepath.Join(w.Root.Dir, rel))] = true
		}
	}
	for _, t := range w.Code {
		set[filepath.Dir(t.Path)] = true
	}
	out := make([]string, 0, len(set))
	for d := range set {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		p--
		b[p] = '-'
	}
	return string(b[p:])
}
