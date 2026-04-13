package client

import (
	"net"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wblech/wmux/internal/platform/auth"
	"github.com/wblech/wmux/internal/platform/protocol"
)

// startMockServer creates a Unix socket server that accepts one connection
// and performs the auth handshake, then returns the socket/token paths.
func startMockServer(t *testing.T) (socketPath, tokenPath string, cleanup func()) {
	t.Helper()
	dir := t.TempDir()
	socketPath = filepath.Join(dir, "daemon.sock")
	tokenPath = filepath.Join(dir, "daemon.token")

	token, err := auth.Generate()
	require.NoError(t, err)
	require.NoError(t, auth.SaveToFile(token, tokenPath))

	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)

	done := make(chan struct{})

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		pConn := protocol.NewConn(conn)
		frame, err := pConn.ReadFrame()
		if err != nil {
			conn.Close()
			return
		}
		if frame.Type == protocol.MsgAuth && auth.Verify(token, frame.Payload) {
			_ = pConn.WriteFrame(protocol.Frame{
				Version: protocol.ProtocolVersion,
				Type:    protocol.MsgOK,
			})
		} else {
			_ = pConn.WriteFrame(protocol.Frame{
				Version: protocol.ProtocolVersion,
				Type:    protocol.MsgError,
				Payload: []byte(`{"error":"auth failed"}`),
			})
		}
		// Keep connection alive until cleanup
		<-done
		conn.Close()
	}()

	return socketPath, tokenPath, func() {
		close(done)
		ln.Close()
	}
}

func TestConnect_Success(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServer(t)
	defer cleanup()

	c, err := Connect(Options{
		SocketPath: socketPath,
		TokenPath:  tokenPath,
	})
	require.NoError(t, err)
	require.NotNil(t, c)
	defer c.Close()
}

func TestConnect_BadSocket(t *testing.T) {
	_, err := Connect(Options{
		SocketPath: "/nonexistent/daemon.sock",
		TokenPath:  "/nonexistent/token",
	})
	assert.Error(t, err)
}

func TestConnect_BadToken(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "daemon.sock")

	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer ln.Close()

	_, err = Connect(Options{
		SocketPath: socketPath,
		TokenPath:  filepath.Join(dir, "nonexistent.token"),
	})
	assert.Error(t, err)
}

func TestClose(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServer(t)
	defer cleanup()

	c, err := Connect(Options{
		SocketPath: socketPath,
		TokenPath:  tokenPath,
	})
	require.NoError(t, err)

	err = c.Close()
	assert.NoError(t, err)
}
