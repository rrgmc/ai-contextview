// Package identity gives a bridge its workspace identity.
package identity

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rrgmc/ai-specview/internal/resolve"
)

// Identity describes one Claude instance.
type Identity struct {
	WS          string
	Cwd         string
	ClaudePID   int
	BridgePID   int
	BridgePPID  int
	ProjectName string
}

// New builds the identity of this bridge process.
func New() (Identity, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return Identity{}, err
	}
	return Identity{
		WS:          "ws_" + UUID(),
		Cwd:         cwd,
		ClaudePID:   ClaudePID(),
		BridgePID:   os.Getpid(),
		BridgePPID:  os.Getppid(),
		ProjectName: ProjectName(cwd),
	}, nil
}

// UUID returns a random UUIDv4.
func UUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// ShortID returns a short random id with a prefix.
func ShortID(prefix string) string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%s%x", prefix, b)
}

// ProjectName is the base name of the git root of cwd, else of cwd.
func ProjectName(cwd string) string {
	if root := resolve.GitRoot(cwd); root != "" {
		return filepath.Base(root)
	}
	return filepath.Base(cwd)
}
