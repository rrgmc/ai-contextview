package daemon

import (
	"net/http"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rrgmc/ai-specview/internal/protocol"
)

func TestEditorCommand(t *testing.T) {
	for _, tc := range []struct {
		goos, editor, abs string
		line              int
		want              []string
	}{
		{"darwin", "vscode", "/p/a.go", 12, []string{"open", "vscode://file/p/a.go:12"}},
		{"darwin", "cursor", "/p/a.go", 0, []string{"open", "cursor://file/p/a.go"}},
		{"linux", "zed", "/p/a.go", 3, []string{"xdg-open", "zed://file/p/a.go:3"}},
		{"windows", "vscode", `C:\p\a.go`, 3, []string{"rundll32", "url.dll,FileProtocolHandler", "vscode://file/C:/p/a.go:3"}},
		{"darwin", "goland", "/p/a.go", 7, []string{"open", "-na", "GoLand", "--args", "--line", "7", "/p/a.go"}},
		{"linux", "goland", "/p/a.go", 0, []string{"goland", "/p/a.go"}},
	} {
		got, err := editorCommand(tc.goos, tc.editor, tc.abs, tc.line)
		require.NoError(t, err, "%s %s", tc.goos, tc.editor)
		assert.Equal(t, tc.want, got, "%s %s", tc.goos, tc.editor)
	}
	_, err := editorCommand("darwin", "notepad", "/p/a.go", 1)
	assert.Error(t, err)
}

func TestOpenInEditor(t *testing.T) {
	h := newHarness(t)
	cwd, specPath := writeSpec(t)
	b := h.bridge("ws_A", cwd)
	b.req(protocol.TypeSpecShow, &protocol.SpecShow{RootAbs: specPath})
	b.req(protocol.TypeAsk, askBody(cwd, specPath, "qs_1"))

	var ran [][]string
	h.d.mu.Lock()
	h.d.openEditor = func(args []string) error {
		ran = append(ran, args)
		return nil
	}
	h.d.mu.Unlock()

	code := filepath.Join(cwd, "code.go")
	open := func(form url.Values, hdr map[string]string) int {
		return h.do(http.MethodPost, "/w/ws_A/open", form, hdr).StatusCode
	}
	assert.Equal(t, http.StatusNoContent, open(url.Values{"kind": {"doc"}, "path": {"spec.md"}, "line": {"1"}, "editor": {"goland"}}, nil))
	assert.Equal(t, http.StatusNoContent, open(url.Values{"kind": {"code"}, "path": {code}, "line": {"3"}, "editor": {"vscode"}}, nil))
	require.Len(t, ran, 2)
	assert.Contains(t, ran[0], specPath)
	assert.Contains(t, ran[1][len(ran[1])-1], code+":3")

	// Only files the page shows; a known editor; same origin.
	assert.Equal(t, http.StatusNotFound, open(url.Values{"kind": {"code"}, "path": {"/etc/passwd"}, "editor": {"vscode"}}, nil))
	assert.Equal(t, http.StatusNotFound, open(url.Values{"kind": {"doc"}, "path": {"../secret.md"}, "editor": {"vscode"}}, nil))
	assert.Equal(t, http.StatusBadRequest, open(url.Values{"kind": {"code"}, "path": {code}, "editor": {"notepad"}}, nil))
	assert.Equal(t, http.StatusForbidden, open(url.Values{"kind": {"code"}, "path": {code}, "editor": {"vscode"}}, map[string]string{"Origin": "http://evil.example"}))
	assert.Len(t, ran, 2)
}
