package ipc

import (
	"net"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractPeerCredentials(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("peer credentials not supported on Windows")
	}

	sock := tempSock(t, "c.sock")

	ln, err := Listen(sock)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

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

	creds, err := ExtractPeerCredentials(server)
	require.NoError(t, err)

	assert.Equal(t, uint32(os.Getuid()), creds.UID)
	assert.Positive(t, creds.PID)
}

func TestExtractPeerCredentials_NotUnix(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() { _ = server.Close() })
	t.Cleanup(func() { _ = client.Close() })

	_, err := ExtractPeerCredentials(server)
	assert.Error(t, err)
}
