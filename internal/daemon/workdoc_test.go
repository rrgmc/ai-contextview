package daemon

import (
	"os"
	"path/filepath"
	"testing"

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
