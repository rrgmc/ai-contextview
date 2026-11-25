//go:build !windows

// Package detach starts processes that outlive their parent's session.
package detach

import "syscall"

// SysProcAttr puts the child in a new session, so it survives the terminal
// or Claude session that started it.
func SysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
