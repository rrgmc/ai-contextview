package daemon

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// ErrLocked means another daemon holds the instance lock.
var ErrLocked = errors.New("another ai-contextview daemon is running or starting")

// Lock takes the single-instance lock for the socket at sockPath: an
// exclusive LockFileEx on sockPath+".lock", held until release is called or
// the process exits (Windows drops it when the handle closes). Take it before
// listening: only the holder may remove a stale socket file.
func Lock(sockPath string) (release func(), err error) {
	f, err := os.OpenFile(sockPath+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	ol := new(windows.Overlapped)
	err = windows.LockFileEx(windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol)
	if err != nil {
		_ = f.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, ErrLocked
		}
		return nil, err
	}
	return func() { _ = f.Close() }, nil
}
