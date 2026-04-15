package client

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wblech/wmux/internal/platform/protocol"
)

func TestClient_Exec(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgExec: func(payload []byte) protocol.Frame {
			var req struct {
				SessionID string `json:"session_id"`
				Input     string `json:"input"`
				Newline   bool   `json:"newline"`
			}
			require.NoError(t, json.Unmarshal(payload, &req))
			assert.Equal(t, "s1", req.SessionID)
			assert.Equal(t, "ls -la", req.Input)
			assert.True(t, req.Newline)
			return okFrame(nil)
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	err = c.Exec("s1", "ls -la")
	require.NoError(t, err)
}

func TestClient_Exec_WithNoNewline(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgExec: func(payload []byte) protocol.Frame {
			var req struct {
				Newline bool `json:"newline"`
			}
			require.NoError(t, json.Unmarshal(payload, &req))
			assert.False(t, req.Newline)
			return okFrame(nil)
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	err = c.Exec("s1", "ls", WithNewline(false))
	require.NoError(t, err)
}

func TestClient_Exec_Error(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgExec: func(_ []byte) protocol.Frame {
			return errFrame("session not found")
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	err = c.Exec("nonexistent", "ls")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
}

func TestClient_ExecSync(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgExecSync: func(payload []byte) protocol.Frame {
			var req struct {
				SessionIDs []string `json:"session_ids"`
				Input      string   `json:"input"`
				Newline    bool     `json:"newline"`
			}
			require.NoError(t, json.Unmarshal(payload, &req))
			assert.Equal(t, []string{"s1", "s2"}, req.SessionIDs)
			assert.Equal(t, "git pull", req.Input)
			assert.True(t, req.Newline)

			resp := struct {
				Results []ExecResult `json:"results"`
			}{
				Results: []ExecResult{
					{SessionID: "s1", OK: true, Error: ""},
					{SessionID: "s2", OK: true, Error: ""},
				},
			}
			data, _ := json.Marshal(resp)
			return protocol.Frame{Version: protocol.ProtocolVersion, Type: protocol.MsgOK, Payload: data}
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	results, err := c.ExecSync("git pull", "s1", "s2")
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.True(t, results[0].OK)
	assert.True(t, results[1].OK)
}

func TestClient_ExecPrefix(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgExecSync: func(payload []byte) protocol.Frame {
			var req struct {
				Prefix string `json:"prefix"`
				Input  string `json:"input"`
			}
			require.NoError(t, json.Unmarshal(payload, &req))
			assert.Equal(t, "proj-a", req.Prefix)
			assert.Equal(t, "ls", req.Input)

			resp := struct {
				Results []ExecResult `json:"results"`
			}{
				Results: []ExecResult{
					{SessionID: "proj-a/s1", OK: true, Error: ""},
				},
			}
			data, _ := json.Marshal(resp)
			return protocol.Frame{Version: protocol.ProtocolVersion, Type: protocol.MsgOK, Payload: data}
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	results, err := c.ExecPrefix("proj-a", "ls")
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.True(t, results[0].OK)
}

func TestClient_ExecSync_Error(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgExecSync: func(_ []byte) protocol.Frame {
			return errFrame("no matching sessions")
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	_, err = c.ExecSync("cmd", "s1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no matching sessions")
}
