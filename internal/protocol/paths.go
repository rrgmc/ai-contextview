package protocol

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// SocketPath returns the daemon socket path: $AI_SPECVIEW_SOCKET, else
// $XDG_RUNTIME_DIR/ai-specview.sock, else ~/Library/Caches/ai-specview/ai-specview.sock
// on macOS, else a per-user file in the temp directory.
func SocketPath() string {
	if p := os.Getenv("AI_SPECVIEW_SOCKET"); p != "" {
		return p
	}
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return filepath.Join(d, "ai-specview.sock")
	}
	return filepath.Join(StateDir(), "ai-specview.sock")
}

// StateDir is where the daemon keeps its socket (when no runtime dir is set)
// and its log file.
func StateDir() string {
	if runtime.GOOS == "darwin" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, "Library", "Caches", "ai-specview")
		}
	}
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "ai-specview")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("ai-specview-%d", os.Getuid()))
}
