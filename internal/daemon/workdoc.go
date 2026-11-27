package daemon

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rrgmc/ai-contextview/internal/glob"
	"github.com/rrgmc/ai-contextview/internal/protocol"
	"github.com/rrgmc/ai-contextview/internal/specsrc"
)

// workDoc picks the session's working document from the markdown files
// Claude writes, in ask mode and spec mode (docs/07-layers.md, "Finding the
// working document"). The hook has already dropped bookkeeping files.
type workDoc struct {
	// explicit is set by Claude's own spec_show, which always wins.
	explicit bool
	// spec is spec mode: ask mode that expects a spec.
	spec bool
	// paths are spec mode's absolute glob patterns.
	paths []string
	// specSkill is set once a skill whose name looks like a spec skill
	// started in the session.
	specSkill bool
	// doc is the working document the daemon picked, absolute.
	doc string
	// provisional is set while doc was shown with spec_show provisional:
	// the first written file that wants() accepts replaces it.
	provisional bool
	// edits counts the writes of each candidate file, by absolute path.
	edits map[string]int
}

// scoreThreshold is the score a candidate needs in spec mode without paths.
const scoreThreshold = 2

var (
	specSkillName = regexp.MustCompile(`(?i)spec|rfc|design`)
	specFolder    = map[string]bool{"specs": true, "spec": true, "rfc": true, "rfcs": true, "design": true, "designs": true, "adr": true, "adrs": true}
	specHeading   = regexp.MustCompile(`(?im)^#{1,6}\s+(requirements|goals?|non-goals|design|acceptance criteria|open questions|motivation|proposal|decisions?)\b`)
	specKind      = regexp.MustCompile(`(?m)^kind:\s*spec\s*$`)
)

// sessionWorkspaces returns the live workspaces whose Claude process is one
// of pids (the hook's ancestors); d.mu is held.
func (d *Daemon) sessionWorkspaces(pids []int) []*Workspace {
	set := map[int]bool{}
	for _, p := range pids {
		if p > 1 {
			set[p] = true
		}
	}
	var out []*Workspace
	for _, w := range d.workspaces {
		if !w.Live {
			continue
		}
		if (w.BridgePPID > 1 && set[w.BridgePPID]) || (w.ClaudePID > 1 && set[w.ClaudePID]) {
			out = append(out, w)
		}
	}
	return out
}

// docWritten handles a markdown file the session wrote.
func (d *Daemon) docWritten(req *protocol.DocWritten) *protocol.DocWrittenResult {
	d.mu.Lock()
	defer d.mu.Unlock()
	path := filepath.Clean(req.Path)
	for _, w := range d.sessionWorkspaces(req.PIDs) {
		d.setSessionCwd(w, req.Cwd)
		if !w.AskMode || w.work.explicit {
			continue
		}
		wd := &w.work
		if wd.edits == nil {
			wd.edits = map[string]int{}
		}
		wd.edits[path]++
		if wd.doc != "" && !wd.provisional {
			// The working document stays until a question asks about
			// another candidate (switchToAsked) or Claude calls spec_show.
			continue
		}
		if path == wd.doc || !wd.wants(path) {
			continue
		}
		if d.showWorking(w, path) {
			return &protocol.DocWrittenResult{Shown: true, Path: path}
		}
	}
	return &protocol.DocWrittenResult{}
}

// wants reports whether a written file becomes the working document while
// there is none.
func (wd *workDoc) wants(path string) bool {
	switch {
	case !wd.spec:
		return true
	case len(wd.paths) > 0:
		return glob.MatchAny(wd.paths, path)
	default:
		return wd.score(path) >= scoreThreshold
	}
}

// score rates how much a written file looks like the spec, in spec mode
// without paths. The weights are a first guess, tuned in use.
func (wd *workDoc) score(path string) int {
	n := 0
	// A spec skill counts once: alone it does not pick a file, so a README
	// or notes file written before the spec does not take its place.
	if wd.specSkill {
		n++
	}
	if inSpecFolder(path) {
		n++
	}
	if src, err := os.ReadFile(path); err == nil {
		if specKind.Match(src) || len(specHeading.FindAll(src, -1)) >= 2 {
			n++
		}
	}
	if wd.edits[path] >= 2 {
		n++
	}
	return n
}

func inSpecFolder(path string) bool {
	if home, err := os.UserHomeDir(); err == nil {
		for _, dir := range []string{"specs", "plans"} {
			if strings.HasPrefix(path, filepath.Join(home, ".claude", dir)+string(filepath.Separator)) {
				return true
			}
		}
	}
	for _, part := range strings.Split(filepath.ToSlash(filepath.Dir(path)), "/") {
		if specFolder[strings.ToLower(part)] {
			return true
		}
	}
	return false
}

// switchToAsked makes a written file the working document when a question
// of req quotes it; d.mu is held.
func (d *Daemon) switchToAsked(w *Workspace, req *protocol.Ask) {
	wd := &w.work
	if !w.AskMode || wd.explicit {
		return
	}
	for _, q := range req.Questions {
		for _, doc := range q.Docs {
			p := filepath.Clean(doc.Path)
			if wd.edits[p] > 0 && p != wd.doc && specsrc.IsMarkdown(p) {
				d.showWorking(w, p)
				return
			}
		}
	}
}

// showWorking shows path as the working document and tells the bridge, so
// its spec_quote and replay follow; d.mu is held.
func (d *Daemon) showWorking(w *Workspace, path string) bool {
	if _, err := d.specShowLocked(w, &protocol.SpecShow{RootAbs: path}); err != nil {
		return false
	}
	w.work.doc = path
	w.RootAuto = true
	w.send(protocol.TypeDocShown, "", &protocol.DocShown{RootAbs: path})
	// An open page shows only the document it already has; tell it to load
	// the new one.
	d.hub.Publish(w.ID, newEvent("focus", map[string]any{"doc": map[string]string{"path": w.CurrentDoc}}))
	return true
}

// skillStarted records a skill the session started; in spec mode, a spec
// skill raises the score of the files it writes.
func (d *Daemon) skillStarted(req *protocol.SkillStarted) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, w := range d.sessionWorkspaces(req.PIDs) {
		d.setSessionCwd(w, req.Cwd)
		if specSkillName.MatchString(req.Name) {
			w.work.specSkill = true
		}
	}
}

// sessionCwd records the session's working folder from the hook. Before the
// session's bridge connected there is no workspace yet: the folder waits in
// pendingCwd for its hello.
func (d *Daemon) sessionCwd(req *protocol.SessionCwd) {
	d.mu.Lock()
	defer d.mu.Unlock()
	ws := d.sessionWorkspaces(req.PIDs)
	for _, w := range ws {
		d.setSessionCwd(w, req.Cwd)
	}
	if len(ws) == 0 {
		d.keepPendingCwd(req.PIDs, req.Cwd)
	}
}

// pendingCwd is a session folder waiting for its bridge's hello.
type pendingCwd struct {
	cwd string
	at  time.Time
}

// pendingCwdTTL bounds how long a reported folder waits for a bridge.
const pendingCwdTTL = 10 * time.Minute

// keepPendingCwd keeps cwd under each of the hook's ancestor processes;
// d.mu is held.
func (d *Daemon) keepPendingCwd(pids []int, cwd string) {
	if !filepath.IsAbs(cwd) {
		return
	}
	now := time.Now()
	for p, e := range d.pendingCwd {
		if now.Sub(e.at) > pendingCwdTTL {
			delete(d.pendingCwd, p)
		}
	}
	for _, p := range pids {
		if p > 1 {
			d.pendingCwd[p] = pendingCwd{cwd: filepath.Clean(cwd), at: now}
		}
	}
}

// takePendingCwd returns the folder kept for the bridge's Claude process
// (or the bridge's parent), and forgets that report; d.mu is held.
func (d *Daemon) takePendingCwd(pids ...int) string {
	for _, p := range pids {
		if p <= 1 {
			continue
		}
		e, ok := d.pendingCwd[p]
		if !ok {
			continue
		}
		// The other ancestors kept the same report: forget them too.
		for q, f := range d.pendingCwd {
			if f == e {
				delete(d.pendingCwd, q)
			}
		}
		if time.Since(e.at) <= pendingCwdTTL {
			return e.cwd
		}
	}
	return ""
}

// cwd answers the bridge's cwd request.
func (d *Daemon) cwd(id string) *protocol.CwdResult {
	d.mu.Lock()
	defer d.mu.Unlock()
	if w := d.workspaces[id]; w != nil {
		return &protocol.CwdResult{Cwd: w.SessionCwd}
	}
	return &protocol.CwdResult{}
}

// setSessionCwd keeps the session's working folder and tells the bridge
// when it changed, so relative paths resolve against it; d.mu is held.
func (d *Daemon) setSessionCwd(w *Workspace, cwd string) {
	if cwd == "" || !filepath.IsAbs(cwd) {
		return
	}
	cwd = filepath.Clean(cwd)
	if cwd == w.SessionCwd {
		return
	}
	w.SessionCwd = cwd
	w.send(protocol.TypeSessionCwd, "", &protocol.SessionCwd{Cwd: cwd})
}
