package daemon

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rrgmc/ai-specview/internal/glob"
	"github.com/rrgmc/ai-specview/internal/protocol"
	"github.com/rrgmc/ai-specview/internal/specsrc"
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
		if !w.AskMode || w.work.explicit {
			continue
		}
		wd := &w.work
		if wd.edits == nil {
			wd.edits = map[string]int{}
		}
		wd.edits[path]++
		if wd.doc != "" {
			// The working document stays until a question asks about
			// another candidate (switchToAsked) or Claude calls spec_show.
			continue
		}
		if !wd.wants(path) {
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
	if wd.specSkill {
		n += 2
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
	w.send(protocol.TypeDocShown, "", &protocol.DocShown{RootAbs: path})
	return true
}

// skillStarted records a skill the session started; in spec mode, a spec
// skill raises the score of the files it writes.
func (d *Daemon) skillStarted(req *protocol.SkillStarted) {
	if !specSkillName.MatchString(req.Name) {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, w := range d.sessionWorkspaces(req.PIDs) {
		w.work.specSkill = true
	}
}
