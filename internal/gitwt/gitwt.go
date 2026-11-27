// Package gitwt finds the git worktrees of a repository by reading its .git
// files, without running git.
package gitwt

import (
	"os"
	"path/filepath"
	"strings"
)

// Root returns the root folder of the worktree that holds dir, and its git
// folder, or "" when dir is in no git repository.
func Root(dir string) (root, gitDir string) {
	dir = filepath.Clean(dir)
	for {
		p := filepath.Join(dir, ".git")
		if fi, err := os.Stat(p); err == nil {
			if fi.IsDir() {
				return dir, p
			}
			if g := readGitFile(p); g != "" {
				return dir, g
			}
			return "", ""
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ""
		}
		dir = parent
	}
}

// Worktrees returns the root folders of every worktree of the repository
// that holds dir: the main one first, then the linked ones that still
// exist. It returns nil when dir is in no git repository.
func Worktrees(dir string) []string {
	_, gitDir := Root(dir)
	if gitDir == "" {
		return nil
	}
	common := gitDir
	if b, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
		c := strings.TrimSpace(string(b))
		if !filepath.IsAbs(c) {
			c = filepath.Join(gitDir, c)
		}
		common = filepath.Clean(c)
	}
	var out []string
	seen := map[string]bool{}
	add := func(root string) {
		root = filepath.Clean(root)
		if seen[root] {
			return
		}
		if fi, err := os.Stat(root); err == nil && fi.IsDir() {
			seen[root] = true
			out = append(out, root)
		}
	}
	if filepath.Base(common) == ".git" {
		add(filepath.Dir(common))
	}
	entries, _ := os.ReadDir(filepath.Join(common, "worktrees"))
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(common, "worktrees", e.Name(), "gitdir"))
		if err != nil {
			continue
		}
		// gitdir names the worktree's .git file.
		if g := strings.TrimSpace(string(b)); filepath.IsAbs(g) {
			add(filepath.Dir(g))
		}
	}
	return out
}

// readGitFile reads a worktree's .git file ("gitdir: <path>") and returns
// the absolute git folder it names.
func readGitFile(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	g, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir:")
	if !ok {
		return ""
	}
	g = strings.TrimSpace(g)
	if !filepath.IsAbs(g) {
		g = filepath.Join(filepath.Dir(p), g)
	}
	return filepath.Clean(g)
}
