package gitwt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRepo lays out a main checkout with two linked worktrees, as git does,
// and a third worktree whose folder was deleted.
func fakeRepo(t *testing.T) (main, wt1, wt2 string) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	main = filepath.Join(base, "repo")
	wt1 = filepath.Join(base, "repo-a")
	wt2 = filepath.Join(main, ".claude", "worktrees", "b")
	gone := filepath.Join(base, "repo-gone")
	require.NoError(t, os.MkdirAll(filepath.Join(main, ".git"), 0o755))
	for name, root := range map[string]string{"a": wt1, "b": wt2, "gone": gone} {
		meta := filepath.Join(main, ".git", "worktrees", name)
		require.NoError(t, os.MkdirAll(meta, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(meta, "gitdir"), []byte(filepath.Join(root, ".git")+"\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(meta, "commondir"), []byte("../..\n"), 0o644))
		if name == "gone" {
			continue
		}
		require.NoError(t, os.MkdirAll(root, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: "+meta+"\n"), 0o644))
	}
	return main, wt1, wt2
}

func TestWorktrees(t *testing.T) {
	main, wt1, wt2 := fakeRepo(t)
	want := []string{main, wt1, wt2}

	// From the main checkout, a linked worktree, or a folder inside one.
	assert.ElementsMatch(t, want, Worktrees(main))
	assert.Equal(t, main, Worktrees(main)[0], "the main checkout comes first")
	assert.ElementsMatch(t, want, Worktrees(wt1))
	sub := filepath.Join(wt2, "docs", "specs")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	assert.ElementsMatch(t, want, Worktrees(sub))

	root, _ := Root(sub)
	assert.Equal(t, wt2, root, "a nested worktree is its own root")

	// Outside a repository.
	assert.Nil(t, Worktrees(t.TempDir()))
}
