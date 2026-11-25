//go:build !windows

package daemon

import (
	"errors"
	"os"
	"syscall"
)

// ErrLocked means another daemon holds the instance lock.
var ErrLocked = errors.New("another ai-specview daemon is running or starting")

// Lock takes the single-instance lock for the socket at sockPath: an
// exclusive flock on sockPath+".lock", held until release is called or the
// process exits (the kernel drops it even on kill -9). Take it before
// listening: only the holder may remove a stale socket file, so two daemons
// started at the same moment can't both listen.
func Lock(sockPath string) (release func(), err error) {
	f, err := os.OpenFile(sockPath+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrLocked
		}
		return nil, err
	}
	return func() { _ = f.Close() }, nil
}
