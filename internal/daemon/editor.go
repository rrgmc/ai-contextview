package daemon

import (
	"bytes"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// editorStartWait is how long the daemon waits for an editor command to fail
// before it reports success. A URL opener returns at once; an editor's own
// launcher may keep running while the editor is open.
const editorStartWait = 2 * time.Second

// editorCommand returns the command that opens abs at line (0: no line) in
// editor on goos. VS Code, Cursor and Zed open through their URL schemes,
// which the platform's URL opener hands to the installed app. JetBrains IDEs
// have no such scheme for a file path, so GoLand is started with its
// command-line options.
func editorCommand(goos, editor, abs string, line int) ([]string, error) {
	switch editor {
	case "vscode", "cursor", "zed":
		u := editor + "://file"
		if goos == "windows" {
			// Not filepath.ToSlash, which converts only on Windows.
			u += "/" + strings.ReplaceAll(abs, `\`, "/")
		} else {
			u += abs
		}
		if line > 0 {
			u += ":" + strconv.Itoa(line)
		}
		switch goos {
		case "darwin":
			return []string{"open", u}, nil
		case "windows":
			// Not "cmd /c start": cmd would split the URL at "&".
			return []string{"rundll32", "url.dll,FileProtocolHandler", u}, nil
		default:
			return []string{"xdg-open", u}, nil
		}
	case "goland":
		var args []string
		if line > 0 {
			args = []string{"--line", strconv.Itoa(line)}
		}
		args = append(args, abs)
		if goos == "darwin" {
			// -n passes the arguments to a running GoLand too.
			return append([]string{"open", "-na", "GoLand", "--args"}, args...), nil
		}
		return append([]string{"goland"}, args...), nil
	}
	return nil, fmt.Errorf("unknown editor %q", editor)
}

// runEditor starts args and reports an error when the command fails to start
// or exits with an error within editorStartWait.
func runEditor(args []string) error {
	cmd := exec.Command(args[0], args[1:]...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			if msg := strings.TrimSpace(out.String()); msg != "" {
				return fmt.Errorf("%s: %s", args[0], msg)
			}
			return fmt.Errorf("%s: %w", args[0], err)
		}
		return nil
	case <-time.After(editorStartWait):
		return nil
	}
}

// handleOpen opens a file the page shows in the user's editor. The page
// sends kind "doc" with a document key, or kind "code" with the absolute
// path of a code tab; other files are refused.
func (d *Daemon) handleOpen(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxForm)
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	path := r.PostForm.Get("path")
	line, _ := strconv.Atoi(r.PostForm.Get("line"))
	if line < 0 {
		line = 0
	}

	d.mu.Lock()
	ws, _ := d.lookup(r.PathValue("ws"))
	abs, ok := "", false
	if ws != nil {
		switch r.PostForm.Get("kind") {
		case "doc":
			abs, ok = ws.docAbs(path)
		case "code":
			abs, ok = path, ws.codeAllowed[path] && filepath.IsAbs(path)
		}
	}
	run := d.openEditor
	d.mu.Unlock()
	if ws == nil {
		http.Error(w, "unknown workspace", http.StatusNotFound)
		return
	}
	if !ok {
		http.Error(w, "unknown file", http.StatusNotFound)
		return
	}
	args, err := editorCommand(runtime.GOOS, r.PostForm.Get("editor"), abs, line)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if run == nil {
		run = runEditor
	}
	if err := run(args); err != nil {
		d.Logf("open in editor: %v", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
