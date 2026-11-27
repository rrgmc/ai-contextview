package protocol

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// SocketPath returns the daemon socket path: $AI_CONTEXTVIEW_SOCKET, else
// $XDG_RUNTIME_DIR/ai-contextview.sock, else ~/Library/Caches/ai-contextview/ai-contextview.sock
// on macOS, else a per-user file in the temp directory.
func SocketPath() string {
	if p := os.Getenv("AI_CONTEXTVIEW_SOCKET"); p != "" {
		return p
	}
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return filepath.Join(d, "ai-contextview.sock")
	}
	return filepath.Join(StateDir(), "ai-contextview.sock")
}

// StateDir is where the daemon keeps its socket (when no runtime dir is set)
// and its log file.
func StateDir() string {
	if runtime.GOOS == "darwin" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, "Library", "Caches", "ai-contextview")
		}
	}
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "ai-contextview")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("ai-contextview-%d", os.Getuid()))
}
