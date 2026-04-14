package client

import (
	"encoding/json"
	"io"
	"net"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wblech/wmux/internal/platform/auth"
	"github.com/wblech/wmux/internal/platform/protocol"
)

func TestClient_RecordStart(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgRecord: func(payload []byte) protocol.Frame {
			var req struct {
				SessionID string `json:"session_id"`
				Action    string `json:"action"`
			}
			require.NoError(t, json.Unmarshal(payload, &req))
			assert.Equal(t, "s1", req.SessionID)
			assert.Equal(t, "start", req.Action)
			return okFrame(RecordResult{
				SessionID: "s1",
				Recording: true,
				Path:      "/tmp/s1/recording.cast",
			})
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	result, err := c.RecordStart("s1")
	require.NoError(t, err)
	assert.True(t, result.Recording)
	assert.Equal(t, "s1", result.SessionID)
	assert.NotEmpty(t, result.Path)
}

func TestClient_RecordStop(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgRecord: func(payload []byte) protocol.Frame {
			var req struct {
				Action string `json:"action"`
			}
			require.NoError(t, json.Unmarshal(payload, &req))
			assert.Equal(t, "stop", req.Action)
			return okFrame(RecordResult{
				SessionID: "s1",
				Recording: false,
				Path:      "",
			})
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	result, err := c.RecordStop("s1")
	require.NoError(t, err)
	assert.False(t, result.Recording)
}

func TestClient_RecordStart_Error(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgRecord: func(_ []byte) protocol.Frame {
			return errFrame("session not found")
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	_, err = c.RecordStart("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
}

// startStreamingMockServer creates a mock server that can send multiple frames per request.
// The handler returns a slice of frames to send.
func startStreamingMockServer(t *testing.T, handler func(protocol.MessageType, []byte) []protocol.Frame) (socketPath, tokenPath string, cleanup func()) {
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
		if frame.Type != protocol.MsgAuth || len(frame.Payload) != 1+auth.TokenSize || !auth.Verify(token, frame.Payload[1:]) {
			_ = pConn.WriteFrame(errFrame("auth failed"))
			_ = conn.Close()
			return
		}
		_ = pConn.WriteFrame(protocol.Frame{
			Version: protocol.ProtocolVersion,
			Type:    protocol.MsgOK,
			Payload: nil,
		})

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

			frames := handler(frame.Type, frame.Payload)
			for _, f := range frames {
				_ = pConn.WriteFrame(f)
			}
		}
	}()

	return socketPath, tokenPath, func() {
		close(done)
		_ = ln.Close()
	}
}

func TestClient_History(t *testing.T) {
	historyData := []byte("hello world scrollback data")

	socketPath, tokenPath, cleanup := startStreamingMockServer(t, func(msgType protocol.MessageType, _ []byte) []protocol.Frame {
		if msgType == protocol.MsgHistory {
			return []protocol.Frame{
				{Version: protocol.ProtocolVersion, Type: protocol.MsgHistory, Payload: historyData},
				{Version: protocol.ProtocolVersion, Type: protocol.MsgHistoryEnd, Payload: nil},
			}
		}
		return []protocol.Frame{errFrame("unhandled")}
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	reader, err := c.History("s1", "text", 0)
	require.NoError(t, err)
	defer reader.Close() //nolint:errcheck

	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, historyData, data)
}

func TestClient_History_Empty(t *testing.T) {
	socketPath, tokenPath, cleanup := startStreamingMockServer(t, func(msgType protocol.MessageType, _ []byte) []protocol.Frame {
		if msgType == protocol.MsgHistory {
			return []protocol.Frame{
				{Version: protocol.ProtocolVersion, Type: protocol.MsgHistoryEnd, Payload: nil},
			}
		}
		return []protocol.Frame{errFrame("unhandled")}
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	reader, err := c.History("s1", "ansi", 0)
	require.NoError(t, err)
	defer reader.Close() //nolint:errcheck

	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Empty(t, data)
}

func TestClient_History_Error(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgHistory: func(_ []byte) protocol.Frame {
			return errFrame("session not found")
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	_, err = c.History("nonexistent", "ansi", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
}
