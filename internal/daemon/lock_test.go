package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLock(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "d.sock")
	release, err := Lock(sock)
	require.NoError(t, err)

	_, err = Lock(sock)
	assert.ErrorIs(t, err, ErrLocked, "a second daemon can't take the lock")

	release()
	release2, err := Lock(sock)
	require.NoError(t, err, "the lock is free again once released")
	release2()
	_, err = os.Stat(sock + ".lock")
	assert.NoError(t, err)
}
