package ipc

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tempSock returns a short socket path under /tmp to avoid the ~104-byte
// macOS Unix socket path limit that t.TempDir() can exceed.
func tempSock(t *testing.T, name string) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "ipc")
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, os.RemoveAll(dir))
	})

	return filepath.Join(dir, name)
}

func TestListener_ListenAndAccept(t *testing.T) {
	sock := tempSock(t, "t.sock")

	ln, err := Listen(sock)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	_, err = os.Stat(sock)
	require.NoError(t, err)

	done := make(chan net.Conn, 1)
	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr == nil {
			done <- conn
		}
	}()

	client, err := net.Dial("unix", sock)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	server := <-done
	t.Cleanup(func() { _ = server.Close() })

	_, err = client.Write([]byte("ping"))
	require.NoError(t, err)

	buf := make([]byte, 4)
	n, err := server.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, "ping", string(buf[:n]))
}

func TestListener_Accept_AfterClose(t *testing.T) {
	sock := tempSock(t, "t.sock")

	ln, err := Listen(sock)
	require.NoError(t, err)

	require.NoError(t, ln.Close())

	// Accept on a closed listener must return an error.
	_, err = ln.Accept()
	assert.Error(t, err)
}

func TestListener_CloseRemovesSocket(t *testing.T) {
	sock := tempSock(t, "t.sock")

	ln, err := Listen(sock)
	require.NoError(t, err)

	err = ln.Close()
	require.NoError(t, err)

	_, err = os.Stat(sock)
	assert.True(t, os.IsNotExist(err))
}

func TestListener_RemovesStaleSocket(t *testing.T) {
	sock := tempSock(t, "t.sock")

	err := os.WriteFile(sock, []byte("stale"), 0o600)
	require.NoError(t, err)

	ln, err := Listen(sock)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	_, err = os.Stat(sock)
	require.NoError(t, err)
}

func TestListener_Addr(t *testing.T) {
	sock := tempSock(t, "t.sock")

	ln, err := Listen(sock)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	assert.Equal(t, sock, ln.Addr())
}

func TestListen_InvalidPath(t *testing.T) {
	// Use a path inside a non-existent directory to force ListenUnix to fail.
	_, err := Listen("/nonexistent-dir-wmux/test.sock")
	assert.Error(t, err)
}

func TestListen_RemoveStaleError(t *testing.T) {
	dir, err := os.MkdirTemp("", "ipc")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	// Place a non-empty subdirectory at the socket path so os.Remove fails (ENOTEMPTY).
	sockPath := filepath.Join(dir, "stale")
	require.NoError(t, os.Mkdir(sockPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sockPath, "f"), []byte("x"), 0o600))

	_, err = Listen(sockPath)
	assert.Error(t, err)
}

func TestListener_Close_AfterClose(t *testing.T) {
	sock := tempSock(t, "t.sock")

	ln, err := Listen(sock)
	require.NoError(t, err)

	require.NoError(t, ln.Close())

	// Second close should return an error from the already-closed listener.
	err = ln.Close()
	assert.Error(t, err)
}
