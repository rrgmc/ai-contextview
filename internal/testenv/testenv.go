// Package testenv holds helpers shared by tests.
package testenv

import (
	"os"
	"runtime"
	"testing"
)

// ShortTempDir makes a temporary directory with a short path, removed when
// the test ends. Unix socket paths are limited to about 100 bytes, and
// macOS's per-user $TMPDIR is long, so Unix uses /tmp; Windows has no /tmp
// and its temp directory is short enough.
func ShortTempDir(t testing.TB, prefix string) string {
	t.Helper()
	root := "/tmp"
	if runtime.GOOS == "windows" {
		root = ""
	}
	dir, err := os.MkdirTemp(root, prefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
