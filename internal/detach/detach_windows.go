// Package detach starts processes that outlive their parent's session.
package detach

import "syscall"

const detachedProcess = 0x00000008 // DETACHED_PROCESS

// SysProcAttr starts the child without a console and in its own process
// group, so closing the parent's console or a Ctrl+C there does not stop it.
func SysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess,
		HideWindow:    true,
	}
}
