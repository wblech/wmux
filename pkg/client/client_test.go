package client

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wblech/wmux/internal/platform/auth"
	"github.com/wblech/wmux/internal/platform/protocol"
)

// shortTempDir creates a short temp directory to avoid Unix socket path limits.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "wc")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// startMockServer creates a Unix socket server that accepts one connection
// and performs the auth handshake, then returns the socket/token paths.
func startMockServer(t *testing.T) (socketPath, tokenPath string, cleanup func()) {
	t.Helper()
	dir := shortTempDir(t)
	socketPath = filepath.Join(dir, "d.sock")
	tokenPath = filepath.Join(dir, "d.token")

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
			_ = conn.Close()
			return
		}
		if frame.Type == protocol.MsgAuth && auth.Verify(token, frame.Payload) {
			_ = pConn.WriteFrame(protocol.Frame{
				Version: protocol.ProtocolVersion,
				Type:    protocol.MsgOK,
				Payload: nil,
			})
		} else {
			_ = pConn.WriteFrame(protocol.Frame{
				Version: protocol.ProtocolVersion,
				Type:    protocol.MsgError,
				Payload: []byte(`{"error":"auth failed"}`),
			})
		}
		<-done
		_ = conn.Close()
	}()

	return socketPath, tokenPath, func() {
		close(done)
		_ = ln.Close()
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
	defer c.Close() //nolint:errcheck
}

func TestConnect_BadSocket(t *testing.T) {
	_, err := Connect(Options{
		SocketPath: "/nonexistent/daemon.sock",
		TokenPath:  "/nonexistent/token",
	})
	require.Error(t, err)
}

func TestConnect_BadToken(t *testing.T) {
	dir := shortTempDir(t)
	socketPath := filepath.Join(dir, "d.sock")

	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer ln.Close() //nolint:errcheck

	_, err = Connect(Options{
		SocketPath: socketPath,
		TokenPath:  filepath.Join(dir, "nonexistent.token"),
	})
	require.Error(t, err)
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
	dir := shortTempDir(t)
	socketPath = filepath.Join(dir, "d.sock")
	tokenPath = filepath.Join(dir, "d.token")

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
			_ = conn.Close()
			return
		}
		if frame.Type != protocol.MsgAuth || !auth.Verify(token, frame.Payload) {
			_ = pConn.WriteFrame(errFrame("auth failed"))
			_ = conn.Close()
			return
		}
		_ = pConn.WriteFrame(protocol.Frame{
			Version: protocol.ProtocolVersion,
			Type:    protocol.MsgOK,
			Payload: nil,
		})

		// Dispatch loop
		for {
			select {
			case <-done:
				_ = conn.Close()
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
		_ = ln.Close()
	}
}

func TestClient_Create(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgCreate: func(_ []byte) protocol.Frame {
			return okFrame(SessionInfo{ID: "s1", State: "alive", Pid: 42, Cols: 80, Rows: 24, Shell: "/bin/zsh"})
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	info, err := c.Create("s1", CreateParams{
		Shell: "/bin/zsh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.NoError(t, err)
	assert.Equal(t, "s1", info.ID)
	assert.Equal(t, "alive", info.State)
	assert.Equal(t, 42, info.Pid)
}

func TestClient_List(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgList: func(_ []byte) protocol.Frame {
			return okFrame([]SessionInfo{
				{ID: "s1", State: "alive", Pid: 0, Cols: 0, Rows: 0, Shell: ""},
				{ID: "s2", State: "detached", Pid: 0, Cols: 0, Rows: 0, Shell: ""},
			})
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	sessions, err := c.List()
	require.NoError(t, err)
	assert.Len(t, sessions, 2)
	assert.Equal(t, "s1", sessions[0].ID)
}

func TestClient_Info(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgInfo: func(_ []byte) protocol.Frame {
			return okFrame(SessionInfo{ID: "s1", State: "alive", Pid: 100, Cols: 120, Rows: 40, Shell: "/bin/bash"})
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	info, err := c.Info("s1")
	require.NoError(t, err)
	assert.Equal(t, "s1", info.ID)
	assert.Equal(t, 100, info.Pid)
	assert.Equal(t, 120, info.Cols)
}

func TestClient_Attach(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgAttach: func(_ []byte) protocol.Frame {
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
	defer c.Close() //nolint:errcheck

	result, err := c.Attach("s1")
	require.NoError(t, err)
	assert.Equal(t, "s1", result.Session.ID)
	assert.Equal(t, "attached", result.Session.State)
	assert.Equal(t, []byte("scrollback-data"), result.Snapshot.Scrollback)
	assert.Equal(t, []byte("viewport-data"), result.Snapshot.Viewport)
}

func TestClient_Attach_NoSnapshot(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgAttach: func(_ []byte) protocol.Frame {
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
	defer c.Close() //nolint:errcheck

	result, err := c.Attach("s1")
	require.NoError(t, err)
	assert.Equal(t, "s1", result.Session.ID)
	assert.Nil(t, result.Snapshot.Scrollback)
	assert.Nil(t, result.Snapshot.Viewport)
}

func TestClient_Detach(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgDetach: func(_ []byte) protocol.Frame {
			return okFrame(nil)
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	err = c.Detach("s1")
	assert.NoError(t, err)
}

func TestClient_Kill(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgKill: func(_ []byte) protocol.Frame {
			return okFrame(nil)
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

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
	defer c.Close() //nolint:errcheck

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
	defer c.Close() //nolint:errcheck

	err = c.Resize("s1", 120, 40)
	assert.NoError(t, err)
}

func TestClient_ErrorResponse(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgKill: func(_ []byte) protocol.Frame {
			return errFrame("session not found")
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	err = c.Kill("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
}

func TestClient_OnData(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServer(t)
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	var called bool
	c.OnData(func(_ string, _ []byte) {
		called = true
	})
	assert.NotNil(t, c.dataHandler)
	c.dataHandler("test", []byte("data"))
	assert.True(t, called)
}

func TestClient_OnEvent(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServer(t)
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	var called bool
	c.OnEvent(func(_ Event) {
		called = true
	})
	assert.NotNil(t, c.evtHandler)
	c.evtHandler(Event{Type: "test", SessionID: "s1", Data: nil})
	assert.True(t, called)
}

func TestConnect_AuthRejected(t *testing.T) {
	dir := shortTempDir(t)
	socketPath := filepath.Join(dir, "d.sock")
	tokenPath := filepath.Join(dir, "d.token")
	badTokenPath := filepath.Join(dir, "bad.token")

	// Generate the real token and a different bad token
	token, err := auth.Generate()
	require.NoError(t, err)
	require.NoError(t, auth.SaveToFile(token, tokenPath))

	badToken, err := auth.Generate()
	require.NoError(t, err)
	require.NoError(t, auth.SaveToFile(badToken, badTokenPath))

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
			_ = conn.Close()
			return
		}
		if frame.Type == protocol.MsgAuth && auth.Verify(token, frame.Payload) {
			_ = pConn.WriteFrame(protocol.Frame{
				Version: protocol.ProtocolVersion,
				Type:    protocol.MsgOK,
				Payload: nil,
			})
		} else {
			_ = pConn.WriteFrame(protocol.Frame{
				Version: protocol.ProtocolVersion,
				Type:    protocol.MsgError,
				Payload: []byte(`{"error":"auth failed"}`),
			})
		}
		<-done
		_ = conn.Close()
	}()
	defer func() {
		close(done)
		_ = ln.Close()
	}()

	_, err = Connect(Options{
		SocketPath: socketPath,
		TokenPath:  badTokenPath,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auth failed")
}

func TestClient_Create_Error(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgCreate: func(_ []byte) protocol.Frame {
			return errFrame("session already exists")
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	_, err = c.Create("s1", CreateParams{
		Shell: "/bin/zsh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session already exists")
}

func TestClient_Attach_Error(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgAttach: func(_ []byte) protocol.Frame {
			return errFrame("session not found")
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	_, err = c.Attach("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
}

func TestClient_Detach_Error(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgDetach: func(_ []byte) protocol.Frame {
			return errFrame("not attached")
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	err = c.Detach("s1")
	require.Error(t, err)
}

func TestClient_Write_Error(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgInput: func(_ []byte) protocol.Frame {
			return errFrame("session not found")
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	err = c.Write("nonexistent", []byte("data"))
	require.Error(t, err)
}

func TestClient_Resize_Error(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgResize: func(_ []byte) protocol.Frame {
			return errFrame("session not found")
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	err = c.Resize("nonexistent", 80, 24)
	require.Error(t, err)
}

func TestClient_List_Error(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgList: func(_ []byte) protocol.Frame {
			return errFrame("internal error")
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	_, err = c.List()
	require.Error(t, err)
}

func TestClient_SendRequest_ClosedConn(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgList: func(_ []byte) protocol.Frame {
			return okFrame(nil)
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)

	// Close the connection, then try to use it
	require.NoError(t, c.Close())

	_, err = c.List()
	require.Error(t, err)

	_, err = c.Create("x", CreateParams{Shell: "/bin/sh", Args: nil, Cols: 80, Rows: 24, Cwd: "", Env: nil})
	require.Error(t, err)

	_, err = c.Attach("x")
	require.Error(t, err)

	err = c.Detach("x")
	require.Error(t, err)

	err = c.Kill("x")
	require.Error(t, err)

	err = c.Write("x", []byte("data"))
	require.Error(t, err)

	err = c.Resize("x", 80, 24)
	require.Error(t, err)

	_, err = c.Info("x")
	require.Error(t, err)
}

func TestClient_ParseError_BadPayload(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgKill: func(_ []byte) protocol.Frame {
			return protocol.Frame{
				Version: protocol.ProtocolVersion,
				Type:    protocol.MsgError,
				Payload: []byte("not-json"),
			}
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	err = c.Kill("s1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unparsable")
}

func TestClient_List_BadPayload(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgList: func(_ []byte) protocol.Frame {
			return protocol.Frame{
				Version: protocol.ProtocolVersion,
				Type:    protocol.MsgOK,
				Payload: []byte("not-json"),
			}
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	_, err = c.List()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestClient_Attach_BadPayload(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgAttach: func(_ []byte) protocol.Frame {
			return protocol.Frame{
				Version: protocol.ProtocolVersion,
				Type:    protocol.MsgOK,
				Payload: []byte("not-json"),
			}
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	_, err = c.Attach("s1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestClient_Info_BadPayload(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgInfo: func(_ []byte) protocol.Frame {
			return protocol.Frame{
				Version: protocol.ProtocolVersion,
				Type:    protocol.MsgOK,
				Payload: []byte("not-json"),
			}
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	_, err = c.Info("s1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestClient_Info_Error(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgInfo: func(_ []byte) protocol.Frame {
			return errFrame("session not found")
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	_, err = c.Info("nonexistent")
	require.Error(t, err)
}
