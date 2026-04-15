package client

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

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

// startMockServer creates a Unix socket server that accepts control + stream
// connections and performs the auth handshake for both.
func startMockServer(t *testing.T) (socketPath, tokenPath string, cleanup func()) {
	t.Helper()
	socketPath, tokenPath, _, cleanup = startMockServerWithHandlers(t, nil)
	return socketPath, tokenPath, cleanup
}

func TestNew_Success(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServer(t)
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	require.NotNil(t, c)
	defer c.Close() //nolint:errcheck
}

func TestNew_BadSocket(t *testing.T) {
	_, err := New(WithSocket("/nonexistent/daemon.sock"), WithTokenPath("/nonexistent/token"), WithAutoStart(false))
	require.Error(t, err)
}

func TestNew_BadToken(t *testing.T) {
	dir := shortTempDir(t)
	socketPath := filepath.Join(dir, "d.sock")

	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer ln.Close() //nolint:errcheck

	_, err = New(WithSocket(socketPath), WithTokenPath(filepath.Join(dir, "nonexistent.token")), WithAutoStart(false))
	require.Error(t, err)
}

func TestClose(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServer(t)
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
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

type mockServer struct {
	streamConn *protocol.Conn
	ready      chan struct{} // closed when both connections are accepted
}

// startMockServerWithHandlers creates a mock server that dispatches requests by message type.
func startMockServerWithHandlers(
	t *testing.T,
	handlers map[protocol.MessageType]handlerFunc,
) (socketPath, tokenPath string, mock *mockServer, cleanup func()) {
	t.Helper()

	if handlers == nil {
		handlers = make(map[protocol.MessageType]handlerFunc)
	}
	// Default: auto-ack event subscription from connect().
	if _, ok := handlers[protocol.MsgEvent]; !ok {
		handlers[protocol.MsgEvent] = func(_ []byte) protocol.Frame {
			return protocol.Frame{
				Version: protocol.ProtocolVersion,
				Type:    protocol.MsgOK,
				Payload: nil,
			}
		}
	}

	dir := shortTempDir(t)
	socketPath = filepath.Join(dir, "d.sock")
	tokenPath = filepath.Join(dir, "d.token")

	token, err := auth.Generate()
	require.NoError(t, err)
	require.NoError(t, auth.SaveToFile(token, tokenPath))

	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)

	done := make(chan struct{})
	ready := make(chan struct{})
	mock = &mockServer{streamConn: nil, ready: ready}

	go func() {
		// Accept control connection.
		conn, err := ln.Accept()
		if err != nil {
			close(ready)
			return
		}
		pConn := protocol.NewConn(conn)

		frame, err := pConn.ReadFrame()
		if err != nil {
			_ = conn.Close()
			close(ready)
			return
		}
		if frame.Type != protocol.MsgAuth || len(frame.Payload) < 1+auth.TokenSize ||
			!auth.Verify(token, frame.Payload[1:1+auth.TokenSize]) {
			_ = pConn.WriteFrame(errFrame("auth failed"))
			_ = conn.Close()
			close(ready)
			return
		}

		clientID := "mock-client-1"
		_ = pConn.WriteFrame(protocol.Frame{
			Version: protocol.ProtocolVersion,
			Type:    protocol.MsgOK,
			Payload: []byte(clientID),
		})

		// Accept stream connection.
		streamRaw, err := ln.Accept()
		if err != nil {
			_ = conn.Close()
			close(ready)
			return
		}
		streamPConn := protocol.NewConn(streamRaw)

		sFrame, err := streamPConn.ReadFrame()
		if err != nil {
			_ = streamRaw.Close()
			_ = conn.Close()
			close(ready)
			return
		}
		if sFrame.Type == protocol.MsgAuth {
			_ = streamPConn.WriteFrame(protocol.Frame{
				Version: protocol.ProtocolVersion,
				Type:    protocol.MsgOK,
				Payload: nil,
			})
		}
		mock.streamConn = streamPConn
		close(ready)

		// Dispatch loop for control channel.
		for {
			select {
			case <-done:
				_ = conn.Close()
				_ = streamRaw.Close()
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

	return socketPath, tokenPath, mock, func() {
		close(done)
		_ = ln.Close()
	}
}

func TestClient_Create(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgCreate: func(_ []byte) protocol.Frame {
			return okFrame(SessionInfo{ID: "s1", State: "alive", Pid: 42, Cols: 80, Rows: 24, Shell: "/bin/zsh"})
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
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
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgList: func(_ []byte) protocol.Frame {
			return okFrame([]SessionInfo{
				{ID: "s1", State: "alive", Pid: 0, Cols: 0, Rows: 0, Shell: ""},
				{ID: "s2", State: "detached", Pid: 0, Cols: 0, Rows: 0, Shell: ""},
			})
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	sessions, err := c.List()
	require.NoError(t, err)
	assert.Len(t, sessions, 2)
	assert.Equal(t, "s1", sessions[0].ID)
}

func TestClient_Info(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgInfo: func(_ []byte) protocol.Frame {
			return okFrame(SessionInfo{ID: "s1", State: "alive", Pid: 100, Cols: 120, Rows: 40, Shell: "/bin/bash"})
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	info, err := c.Info("s1")
	require.NoError(t, err)
	assert.Equal(t, "s1", info.ID)
	assert.Equal(t, 100, info.Pid)
	assert.Equal(t, 120, info.Cols)
}

func TestClient_Attach(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
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

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
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
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
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

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	result, err := c.Attach("s1")
	require.NoError(t, err)
	assert.Equal(t, "s1", result.Session.ID)
	assert.Nil(t, result.Snapshot.Scrollback)
	assert.Nil(t, result.Snapshot.Viewport)
}

func TestClient_Detach(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgDetach: func(_ []byte) protocol.Frame {
			return okFrame(nil)
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	err = c.Detach("s1")
	assert.NoError(t, err)
}

func TestClient_Kill(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgKill: func(_ []byte) protocol.Frame {
			return okFrame(nil)
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	err = c.Kill("s1")
	assert.NoError(t, err)
}

func TestClient_Write(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
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

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	err = c.Write("s1", []byte("hello"))
	assert.NoError(t, err)
}

func TestClient_Resize(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
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

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	err = c.Resize("s1", 120, 40)
	assert.NoError(t, err)
}

func TestClient_ErrorResponse(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgKill: func(_ []byte) protocol.Frame {
			return errFrame("session not found")
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	err = c.Kill("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
}

func TestClient_OnData(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServer(t)
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
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

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
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

func TestNew_AuthRejected(t *testing.T) {
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
		// Expect [channelByte][token].
		if frame.Type == protocol.MsgAuth && len(frame.Payload) == 1+auth.TokenSize && auth.Verify(token, frame.Payload[1:]) {
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

	_, err = New(WithSocket(socketPath), WithTokenPath(badTokenPath), WithAutoStart(false))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auth failed")
}

func TestClient_Create_Error(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgCreate: func(_ []byte) protocol.Frame {
			return errFrame("session already exists")
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
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
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgAttach: func(_ []byte) protocol.Frame {
			return errFrame("session not found")
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	_, err = c.Attach("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
}

func TestClient_Detach_Error(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgDetach: func(_ []byte) protocol.Frame {
			return errFrame("not attached")
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	err = c.Detach("s1")
	require.Error(t, err)
}

func TestClient_Write_Error(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgInput: func(_ []byte) protocol.Frame {
			return errFrame("session not found")
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	err = c.Write("nonexistent", []byte("data"))
	require.Error(t, err)
}

func TestClient_Resize_Error(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgResize: func(_ []byte) protocol.Frame {
			return errFrame("session not found")
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	err = c.Resize("nonexistent", 80, 24)
	require.Error(t, err)
}

func TestClient_List_Error(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgList: func(_ []byte) protocol.Frame {
			return errFrame("internal error")
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	_, err = c.List()
	require.Error(t, err)
}

func TestClient_SendRequest_ClosedConn(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgList: func(_ []byte) protocol.Frame {
			return okFrame(nil)
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
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
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgKill: func(_ []byte) protocol.Frame {
			return protocol.Frame{
				Version: protocol.ProtocolVersion,
				Type:    protocol.MsgError,
				Payload: []byte("not-json"),
			}
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	err = c.Kill("s1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unparsable")
}

func TestClient_List_BadPayload(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgList: func(_ []byte) protocol.Frame {
			return protocol.Frame{
				Version: protocol.ProtocolVersion,
				Type:    protocol.MsgOK,
				Payload: []byte("not-json"),
			}
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	_, err = c.List()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestClient_Attach_BadPayload(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgAttach: func(_ []byte) protocol.Frame {
			return protocol.Frame{
				Version: protocol.ProtocolVersion,
				Type:    protocol.MsgOK,
				Payload: []byte("not-json"),
			}
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	_, err = c.Attach("s1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestClient_Info_BadPayload(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgInfo: func(_ []byte) protocol.Frame {
			return protocol.Frame{
				Version: protocol.ProtocolVersion,
				Type:    protocol.MsgOK,
				Payload: []byte("not-json"),
			}
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	_, err = c.Info("s1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestClient_Info_Error(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgInfo: func(_ []byte) protocol.Frame {
			return errFrame("session not found")
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	_, err = c.Info("nonexistent")
	require.Error(t, err)
}

func TestClient_EventDelivery(t *testing.T) {
	dir := shortTempDir(t)
	socketPath := filepath.Join(dir, "d.sock")
	tokenPath := filepath.Join(dir, "d.token")

	token, err := auth.Generate()
	require.NoError(t, err)
	require.NoError(t, auth.SaveToFile(token, tokenPath))

	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)

	done := make(chan struct{})
	var ctrlConn *protocol.Conn

	go func() {
		// Accept control.
		raw, err := ln.Accept()
		if err != nil {
			return
		}
		pConn := protocol.NewConn(raw)
		frame, _ := pConn.ReadFrame()
		if frame.Type == protocol.MsgAuth && auth.Verify(token, frame.Payload[1:1+auth.TokenSize]) {
			_ = pConn.WriteFrame(protocol.Frame{
				Version: protocol.ProtocolVersion,
				Type:    protocol.MsgOK,
				Payload: []byte("evt-client"),
			})
		}
		ctrlConn = pConn

		// Accept stream.
		sRaw, err := ln.Accept()
		if err != nil {
			return
		}
		sPConn := protocol.NewConn(sRaw)
		sFrame, _ := sPConn.ReadFrame()
		if sFrame.Type == protocol.MsgAuth {
			_ = sPConn.WriteFrame(protocol.Frame{
				Version: protocol.ProtocolVersion,
				Type:    protocol.MsgOK,
				Payload: nil,
			})
		}

		<-done
		_ = raw.Close()
		_ = sRaw.Close()
	}()

	defer func() {
		close(done)
		_ = ln.Close()
	}()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	received := make(chan Event, 1)
	c.OnEvent(func(evt Event) {
		received <- evt
	})

	// Server pushes an event on the control channel.
	evtPayload, _ := json.Marshal(Event{
		Type:      "session.created",
		SessionID: "s1",
		Data:      nil,
	})
	require.NotNil(t, ctrlConn)
	err = ctrlConn.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgEvent,
		Payload: evtPayload,
	})
	require.NoError(t, err)

	select {
	case evt := <-received:
		assert.Equal(t, "session.created", evt.Type)
		assert.Equal(t, "s1", evt.SessionID)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event callback")
	}
}

func TestClient_EventDoesNotCorruptRPC(t *testing.T) {
	dir := shortTempDir(t)
	socketPath := filepath.Join(dir, "d.sock")
	tokenPath := filepath.Join(dir, "d.token")

	token, err := auth.Generate()
	require.NoError(t, err)
	require.NoError(t, auth.SaveToFile(token, tokenPath))

	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)

	done := make(chan struct{})

	go func() {
		// Accept control.
		raw, err := ln.Accept()
		if err != nil {
			return
		}
		pConn := protocol.NewConn(raw)
		frame, _ := pConn.ReadFrame()
		if frame.Type == protocol.MsgAuth {
			_ = pConn.WriteFrame(protocol.Frame{
				Version: protocol.ProtocolVersion,
				Type:    protocol.MsgOK,
				Payload: []byte("rpc-client"),
			})
		}

		// Accept stream.
		sRaw, err := ln.Accept()
		if err != nil {
			return
		}
		sPConn := protocol.NewConn(sRaw)
		sFrame, _ := sPConn.ReadFrame()
		if sFrame.Type == protocol.MsgAuth {
			_ = sPConn.WriteFrame(protocol.Frame{
				Version: protocol.ProtocolVersion,
				Type:    protocol.MsgOK,
				Payload: nil,
			})
		}

		// Dispatch loop: for each incoming request, push an event THEN the response.
		for {
			select {
			case <-done:
				_ = raw.Close()
				_ = sRaw.Close()
				return
			default:
			}

			frame, err := pConn.ReadFrame()
			if err != nil {
				return
			}

			// Push a sneaky event before the RPC response.
			evtPayload, _ := json.Marshal(Event{
				Type:      "session.created",
				SessionID: "sneaky",
				Data:      nil,
			})
			_ = pConn.WriteFrame(protocol.Frame{
				Version: protocol.ProtocolVersion,
				Type:    protocol.MsgEvent,
				Payload: evtPayload,
			})

			// Now send the actual RPC response.
			if frame.Type == protocol.MsgList {
				resp, _ := json.Marshal([]SessionInfo{{ID: "s1", State: "alive", Pid: 0, Cols: 0, Rows: 0, Shell: ""}})
				_ = pConn.WriteFrame(protocol.Frame{
					Version: protocol.ProtocolVersion,
					Type:    protocol.MsgOK,
					Payload: resp,
				})
			}
		}
	}()

	defer func() {
		close(done)
		_ = ln.Close()
	}()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	eventCount := 0
	c.OnEvent(func(_ Event) {
		eventCount++
	})

	// Make an RPC — the server will push an event before responding.
	sessions, err := c.List()
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, "s1", sessions[0].ID)

	// Give the event goroutine a moment to dispatch.
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 1, eventCount, "event should have been delivered despite interleaving")
}

func TestClient_StreamDataDelivery(t *testing.T) {
	socketPath, tokenPath, mock, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgAttach: func(_ []byte) protocol.Frame {
			return okFrame(struct {
				ID    string `json:"id"`
				State string `json:"state"`
			}{ID: "s1", State: "attached"})
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	<-mock.ready // wait for mock server to accept both connections

	received := make(chan struct {
		sessionID string
		data      []byte
	}, 1)

	c.OnData(func(sessionID string, data []byte) {
		received <- struct {
			sessionID string
			data      []byte
		}{sessionID: sessionID, data: data}
	})

	// Send MsgData on the stream channel from the mock server.
	dataPayload := []byte{2, 's', '1'}
	dataPayload = append(dataPayload, []byte("hello world")...)
	err = mock.streamConn.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgData,
		Payload: dataPayload,
	})
	require.NoError(t, err)

	select {
	case msg := <-received:
		assert.Equal(t, "s1", msg.sessionID)
		assert.Equal(t, []byte("hello world"), msg.data)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for data callback")
	}
}
