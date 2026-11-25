package tools

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	got, err := os.UserHomeDir()
	require.NoError(t, err)
	require.Equal(t, home, got)

	assert.Equal(t, home, expandPath("~"))
	assert.Equal(t, filepath.Join(home, ".claude", "specs", "app", "x.md"), expandPath("~/.claude/specs/app/x.md"))
	assert.Equal(t, "docs/specs/x.md", expandPath("docs/specs/x.md"))
	assert.Equal(t, "~user/x.md", expandPath("~user/x.md"))

	if runtime.GOOS == "windows" {
		assert.Equal(t, filepath.Join(home, "x.md"), expandPath(`~\x.md`))
		assert.Equal(t, `C:\Users\me\x.md`, expandPath("/c/Users/me/x.md"))
	} else {
		assert.Equal(t, `~\x.md`, expandPath(`~\x.md`))
		assert.Equal(t, "/c/Users/me/x.md", expandPath("/c/Users/me/x.md"))
	}
}

func TestAbsPathExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	b := &Bridge{Cwd: t.TempDir()}
	assert.Equal(t, filepath.Join(home, ".claude", "specs", "app", "x.md"), b.absPath("~/.claude/specs/app/x.md"))
	assert.Equal(t, filepath.Join(b.Cwd, "docs", "x.md"), b.absPath("docs/x.md"))
}
