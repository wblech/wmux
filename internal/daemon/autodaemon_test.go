package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWaitForSocket_AlreadyExists(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "test.sock")

	f, err := os.Create(sock)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	err = WaitForSocket(sock, 1*time.Second)
	assert.NoError(t, err)
}

func TestWaitForSocket_Timeout(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "nonexistent.sock")

	err := WaitForSocket(sock, 200*time.Millisecond)
	assert.ErrorIs(t, err, ErrDaemonNotRunning)
}

func TestWaitForSocket_AppearsAfterDelay(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "delayed.sock")

	go func() {
		time.Sleep(100 * time.Millisecond)
		f, err := os.Create(sock)
		if err == nil {
			_ = f.Close()
		}
	}()

	err := WaitForSocket(sock, 2*time.Second)
	assert.NoError(t, err)
}

func TestAutodaemonize_ExecutableError(t *testing.T) {
	orig := executableFunc
	executableFunc = func() (string, error) {
		return "", os.ErrNotExist
	}
	t.Cleanup(func() { executableFunc = orig })

	err := Autodaemonize("/tmp/s.sock", "/tmp/d.pid", "info", 100*time.Millisecond)
	assert.Error(t, err)
}

func TestAutodaemonize_StartError(t *testing.T) {
	orig := executableFunc
	executableFunc = func() (string, error) {
		return "/nonexistent/binary", nil
	}
	t.Cleanup(func() { executableFunc = orig })

	err := Autodaemonize("/tmp/s.sock", "/tmp/d.pid", "info", 100*time.Millisecond)
	assert.Error(t, err)
}

func TestAutodaemonize_SocketTimeout(t *testing.T) {
	// Use "true" as the executable — it exits immediately, socket never appears.
	orig := executableFunc
	executableFunc = func() (string, error) {
		return "/usr/bin/true", nil
	}
	t.Cleanup(func() { executableFunc = orig })

	err := Autodaemonize(filepath.Join(t.TempDir(), "never.sock"), "/tmp/d.pid", "info", 150*time.Millisecond)
	assert.Error(t, err)
}

func TestBuildDaemonArgs(t *testing.T) {
	args := BuildDaemonArgs("/path/to/sock", "/path/to/pid", "info")
	assert.Contains(t, args, "daemon")
	assert.Contains(t, args, "--socket")
	assert.Contains(t, args, "/path/to/sock")
	assert.Contains(t, args, "--pid-file")
	assert.Contains(t, args, "/path/to/pid")
	assert.Contains(t, args, "--log-level")
	assert.Contains(t, args, "info")
}
