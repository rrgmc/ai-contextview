package tools

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// expandPath expands a path the way Claude Code's file tools do, so a path
// the model writes with Write also works here. A leading "~" or "~/" is the
// user's home folder (also "~\" on Windows). On Windows, a Git Bash path such
// as "/c/Users/me" is "C:\Users\me". Any other path is returned unchanged.
func expandPath(p string) string {
	windows := runtime.GOOS == "windows"
	if p == "~" || strings.HasPrefix(p, "~/") || (windows && strings.HasPrefix(p, `~\`)) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, filepath.FromSlash(p[1:]))
		}
		return p
	}
	if windows && len(p) >= 3 && p[0] == '/' && isASCIILetter(p[1]) && p[2] == '/' {
		return strings.ToUpper(p[1:2]) + `:\` + filepath.FromSlash(p[3:])
	}
	return p
}

func isASCIILetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
