//go:build !windows

package identity

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

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
