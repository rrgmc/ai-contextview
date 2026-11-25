package identity

import (
	"os"
	"strings"
	"syscall"
	"unsafe"
)

type proc struct {
	ppid int
	exe  string
}

// processes reads the process table with one Toolhelp snapshot.
func processes() map[int]proc {
	snap, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer syscall.CloseHandle(snap)
	m := map[int]proc{}
	var e syscall.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = syscall.Process32First(snap, &e); err == nil; err = syscall.Process32Next(snap, &e) {
		m[int(e.ProcessID)] = proc{ppid: int(e.ParentProcessID), exe: syscall.UTF16ToString(e.ExeFile[:])}
	}
	return m
}

// AncestorPIDs returns the parent, grandparent, … of this process (at most
// max), read from one process snapshot.
func AncestorPIDs(max int) []int {
	ps := processes()
	var pids []int
	for pid := os.Getppid(); pid > 0 && len(pids) < max; {
		pids = append(pids, pid)
		p, ok := ps[pid]
		if !ok || p.ppid == pid {
			break
		}
		pid = p.ppid
	}
	return pids
}

// ClaudePID walks up from the parent process looking for Claude Code. It is
// informational only; it falls back to the parent PID.
func ClaudePID() int {
	ps := processes()
	ppid := os.Getppid()
	pid := ppid
	for i := 0; i < 8 && pid > 0; i++ {
		p, ok := ps[pid]
		if !ok {
			break
		}
		if strings.EqualFold(p.exe, "claude.exe") {
			return pid
		}
		pid = p.ppid
	}
	return ppid
}
