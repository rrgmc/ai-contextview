// Package identity gives a bridge its workspace identity.
package identity

import (
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

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

// AncestorPIDs returns the parent, grandparent, … of this process (at most
// max), read with one `ps` call.
func AncestorPIDs(max int) []int {
	out, err := exec.Command("ps", "-A", "-o", "pid=,ppid=").Output()
	parent := map[int]int{}
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			f := strings.Fields(line)
			if len(f) != 2 {
				continue
			}
			pid, err1 := strconv.Atoi(f[0])
			ppid, err2 := strconv.Atoi(f[1])
			if err1 == nil && err2 == nil {
				parent[pid] = ppid
			}
		}
	}
	var pids []int
	for pid := os.Getppid(); pid > 1 && len(pids) < max; {
		pids = append(pids, pid)
		next, ok := parent[pid]
		if !ok || next == pid {
			break
		}
		pid = next
	}
	return pids
}

// ClaudePID walks up from the parent process looking for Claude Code. It is
// informational only; it falls back to the parent PID.
func ClaudePID() int {
	ppid := os.Getppid()
	pid := ppid
	for i := 0; i < 8 && pid > 1; i++ {
		out, err := exec.Command("ps", "-o", "ppid=,comm=,args=", "-p", strconv.Itoa(pid)).Output()
		if err != nil {
			break
		}
		fields := strings.Fields(string(out))
		if len(fields) < 2 {
			break
		}
		comm := filepath.Base(fields[1])
		args := strings.Join(fields[2:], " ")
		if comm == "claude" || (comm == "node" && strings.Contains(args, "claude")) {
			return pid
		}
		next, err := strconv.Atoi(fields[0])
		if err != nil {
			break
		}
		pid = next
	}
	return ppid
}
