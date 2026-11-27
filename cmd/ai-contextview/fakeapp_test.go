package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rrgmc/ai-contextview/internal/daemon"
	"github.com/rrgmc/ai-contextview/internal/testenv"
)

// fakeSocket makes a file at the socket path that stands for a running
// daemon's socket, so a test can check that fakeapp left it alone.
func fakeSocket(t *testing.T) string {
	t.Helper()
	sock := filepath.Join(testenv.ShortTempDir(t, "acv-fa"), "s.sock")
	require.NoError(t, os.WriteFile(sock, nil, 0o600))
	return sock
}

func TestFakeappHelpDoesNotStart(t *testing.T) {
	for _, arg := range []string{"-h", "--help"} {
		t.Run(arg, func(t *testing.T) {
			sock := fakeSocket(t)
			var stdout, stderr bytes.Buffer
			err := runFakeapp([]string{arg}, sock, strings.NewReader(""), &stdout, &stderr)
			require.NoError(t, err)
			assert.Contains(t, stderr.String(), "usage: ai-contextview fakeapp")
			assert.Empty(t, stdout.String())
			assert.FileExists(t, sock)
			assert.NoFileExists(t, sock+".lock")
		})
	}
}

func TestFakeappRejectsArguments(t *testing.T) {
	for _, args := range [][]string{{"--bogus"}, {"extra"}} {
		t.Run(args[0], func(t *testing.T) {
			sock := fakeSocket(t)
			var stdout, stderr bytes.Buffer
			err := runFakeapp(args, sock, strings.NewReader(""), &stdout, &stderr)
			require.Error(t, err)
			assert.FileExists(t, sock)
			assert.NoFileExists(t, sock+".lock")
		})
	}
}

func TestFakeappRefusesWhileDaemonHoldsLock(t *testing.T) {
	sock := fakeSocket(t)
	release, err := daemon.Lock(sock)
	require.NoError(t, err)
	t.Cleanup(release)

	var stdout, stderr bytes.Buffer
	err = runFakeapp(nil, sock, strings.NewReader(""), &stdout, &stderr)
	require.ErrorIs(t, err, errFakeappLocked)
	assert.FileExists(t, sock, "fakeapp must not remove a running daemon's socket")
	assert.Empty(t, stdout.String())
}
