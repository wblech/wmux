package client

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wblech/wmux/internal/platform/protocol"
)

func TestClient_UntilExit(t *testing.T) {
	ec := 0
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgWait: func(payload []byte) protocol.Frame {
			var req struct {
				SessionID string `json:"session_id"`
				Mode      string `json:"mode"`
				Timeout   int64  `json:"timeout"`
			}
			require.NoError(t, json.Unmarshal(payload, &req))
			assert.Equal(t, "s1", req.SessionID)
			assert.Equal(t, "exit", req.Mode)
			assert.Equal(t, int64(0), req.Timeout)

			return okFrame(WaitResult{
				SessionID: "s1",
				Mode:      "exit",
				ExitCode:  &ec,
				Matched:   false,
				TimedOut:  false,
			})
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	result, err := c.UntilExit("s1")
	require.NoError(t, err)
	assert.Equal(t, "exit", result.Mode)
	assert.Equal(t, "s1", result.SessionID)
	require.NotNil(t, result.ExitCode)
	assert.Equal(t, 0, *result.ExitCode)
	assert.False(t, result.TimedOut)
}

func TestClient_UntilExit_WithTimeout(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgWait: func(payload []byte) protocol.Frame {
			var req struct {
				Timeout int64 `json:"timeout"`
			}
			require.NoError(t, json.Unmarshal(payload, &req))
			assert.Equal(t, int64(5000), req.Timeout)

			return okFrame(WaitResult{
				SessionID: "s1",
				Mode:      "exit",
				ExitCode:  nil,
				Matched:   false,
				TimedOut:  true,
			})
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	result, err := c.UntilExit("s1", WithTimeout(5*time.Second))
	require.NoError(t, err)
	assert.True(t, result.TimedOut)
}

func TestClient_UntilIdle(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgWait: func(payload []byte) protocol.Frame {
			var req struct {
				SessionID string `json:"session_id"`
				Mode      string `json:"mode"`
				IdleFor   int64  `json:"idle_for"`
			}
			require.NoError(t, json.Unmarshal(payload, &req))
			assert.Equal(t, "s1", req.SessionID)
			assert.Equal(t, "idle", req.Mode)
			assert.Equal(t, int64(2000), req.IdleFor)

			return okFrame(WaitResult{
				SessionID: "s1",
				Mode:      "idle",
				ExitCode:  nil,
				Matched:   false,
				TimedOut:  false,
			})
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	result, err := c.UntilIdle("s1", 2*time.Second)
	require.NoError(t, err)
	assert.Equal(t, "idle", result.Mode)
	assert.False(t, result.TimedOut)
}

func TestClient_UntilMatch(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgWait: func(payload []byte) protocol.Frame {
			var req struct {
				SessionID string `json:"session_id"`
				Mode      string `json:"mode"`
				Pattern   string `json:"pattern"`
			}
			require.NoError(t, json.Unmarshal(payload, &req))
			assert.Equal(t, "s1", req.SessionID)
			assert.Equal(t, "match", req.Mode)
			assert.Equal(t, "\\$ $", req.Pattern)

			return okFrame(WaitResult{
				SessionID: "s1",
				Mode:      "match",
				ExitCode:  nil,
				Matched:   true,
				TimedOut:  false,
			})
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	result, err := c.UntilMatch("s1", "\\$ $")
	require.NoError(t, err)
	assert.Equal(t, "match", result.Mode)
	assert.True(t, result.Matched)
}

func TestClient_UntilExit_Error(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgWait: func(_ []byte) protocol.Frame {
			return errFrame("session not found")
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	_, err = c.UntilExit("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
}
