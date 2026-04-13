package client

import (
	"encoding/json"
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

// handlerFunc processes a request payload and returns a response frame.
type handlerFunc func(payload []byte) protocol.Frame

func okFrame(v any) protocol.Frame {
	var payload []byte
	if v != nil {
		payload, _ = json.Marshal(v)
	}
	return protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgOK,
		Payload: payload,
	}
}

func errFrame(msg string) protocol.Frame {
	payload, _ := json.Marshal(map[string]string{"error": msg})
	return protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgError,
		Payload: payload,
	}
}

// startMockServerWithHandlers creates a mock server that dispatches requests by message type.
func startMockServerWithHandlers(t *testing.T, handlers map[protocol.MessageType]handlerFunc) (socketPath, tokenPath string, cleanup func()) {
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

		// Auth handshake
		frame, err := pConn.ReadFrame()
		if err != nil {
			conn.Close()
			return
		}
		if frame.Type != protocol.MsgAuth || !auth.Verify(token, frame.Payload) {
			_ = pConn.WriteFrame(errFrame("auth failed"))
			conn.Close()
			return
		}
		_ = pConn.WriteFrame(protocol.Frame{
			Version: protocol.ProtocolVersion,
			Type:    protocol.MsgOK,
		})

		// Dispatch loop
		for {
			select {
			case <-done:
				conn.Close()
				return
			default:
			}

			frame, err := pConn.ReadFrame()
			if err != nil {
				return
			}

			handler, ok := handlers[frame.Type]
			if !ok {
				_ = pConn.WriteFrame(errFrame("unhandled message type"))
				continue
			}

			resp := handler(frame.Payload)
			_ = pConn.WriteFrame(resp)
		}
	}()

	return socketPath, tokenPath, func() {
		close(done)
		ln.Close()
	}
}

func TestClient_Create(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgCreate: func(payload []byte) protocol.Frame {
			return okFrame(SessionInfo{ID: "s1", State: "alive", Pid: 42, Cols: 80, Rows: 24, Shell: "/bin/zsh"})
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close()

	info, err := c.Create("s1", CreateParams{Shell: "/bin/zsh", Cols: 80, Rows: 24})
	require.NoError(t, err)
	assert.Equal(t, "s1", info.ID)
	assert.Equal(t, "alive", info.State)
	assert.Equal(t, 42, info.Pid)
}

func TestClient_List(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgList: func(payload []byte) protocol.Frame {
			return okFrame([]SessionInfo{
				{ID: "s1", State: "alive"},
				{ID: "s2", State: "detached"},
			})
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close()

	sessions, err := c.List()
	require.NoError(t, err)
	assert.Len(t, sessions, 2)
	assert.Equal(t, "s1", sessions[0].ID)
}

func TestClient_Info(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgInfo: func(payload []byte) protocol.Frame {
			return okFrame(SessionInfo{ID: "s1", State: "alive", Pid: 100, Cols: 120, Rows: 40, Shell: "/bin/bash"})
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close()

	info, err := c.Info("s1")
	require.NoError(t, err)
	assert.Equal(t, "s1", info.ID)
	assert.Equal(t, 100, info.Pid)
	assert.Equal(t, 120, info.Cols)
}

func TestClient_Attach(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgAttach: func(payload []byte) protocol.Frame {
			resp := struct {
				ID       string `json:"id"`
				State    string `json:"state"`
				Pid      int    `json:"pid"`
				Cols     int    `json:"cols"`
				Rows     int    `json:"rows"`
				Shell    string `json:"shell"`
				Snapshot *struct {
					Scrollback []byte `json:"scrollback"`
					Viewport   []byte `json:"viewport"`
				} `json:"snapshot,omitempty"`
			}{
				ID: "s1", State: "attached", Pid: 42, Cols: 80, Rows: 24, Shell: "/bin/zsh",
				Snapshot: &struct {
					Scrollback []byte `json:"scrollback"`
					Viewport   []byte `json:"viewport"`
				}{
					Scrollback: []byte("scrollback-data"),
					Viewport:   []byte("viewport-data"),
				},
			}
			return okFrame(resp)
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close()

	result, err := c.Attach("s1")
	require.NoError(t, err)
	assert.Equal(t, "s1", result.Session.ID)
	assert.Equal(t, "attached", result.Session.State)
	assert.Equal(t, []byte("scrollback-data"), result.Snapshot.Scrollback)
	assert.Equal(t, []byte("viewport-data"), result.Snapshot.Viewport)
}

func TestClient_Attach_NoSnapshot(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgAttach: func(payload []byte) protocol.Frame {
			return okFrame(struct {
				ID    string `json:"id"`
				State string `json:"state"`
				Pid   int    `json:"pid"`
				Cols  int    `json:"cols"`
				Rows  int    `json:"rows"`
				Shell string `json:"shell"`
			}{ID: "s1", State: "attached", Pid: 42, Cols: 80, Rows: 24, Shell: "/bin/zsh"})
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close()

	result, err := c.Attach("s1")
	require.NoError(t, err)
	assert.Equal(t, "s1", result.Session.ID)
	assert.Nil(t, result.Snapshot.Scrollback)
	assert.Nil(t, result.Snapshot.Viewport)
}

func TestClient_Detach(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgDetach: func(payload []byte) protocol.Frame {
			return okFrame(nil)
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close()

	err = c.Detach("s1")
	assert.NoError(t, err)
}

func TestClient_Kill(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgKill: func(payload []byte) protocol.Frame {
			return okFrame(nil)
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close()

	err = c.Kill("s1")
	assert.NoError(t, err)
}

func TestClient_Write(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgInput: func(payload []byte) protocol.Frame {
			// Verify binary encoding: [len:1][session_id][data]
			idLen := int(payload[0])
			sessionID := string(payload[1 : 1+idLen])
			data := payload[1+idLen:]
			if sessionID != "s1" || string(data) != "hello" {
				return errFrame("unexpected input payload")
			}
			return okFrame(nil)
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close()

	err = c.Write("s1", []byte("hello"))
	assert.NoError(t, err)
}

func TestClient_Resize(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgResize: func(payload []byte) protocol.Frame {
			var req struct {
				SessionID string `json:"session_id"`
				Cols      int    `json:"cols"`
				Rows      int    `json:"rows"`
			}
			if err := json.Unmarshal(payload, &req); err != nil {
				return errFrame("bad payload")
			}
			if req.SessionID != "s1" || req.Cols != 120 || req.Rows != 40 {
				return errFrame("unexpected resize params")
			}
			return okFrame(nil)
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close()

	err = c.Resize("s1", 120, 40)
	assert.NoError(t, err)
}

func TestClient_ErrorResponse(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgKill: func(payload []byte) protocol.Frame {
			return errFrame("session not found")
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close()

	err = c.Kill("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
}
