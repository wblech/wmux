package client

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wblech/wmux/internal/platform/protocol"
)

func TestClient_List_WithPrefix(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgList: func(payload []byte) protocol.Frame {
			var req struct {
				Prefix string `json:"prefix"`
			}
			require.NoError(t, json.Unmarshal(payload, &req))
			assert.Equal(t, "proj-a", req.Prefix)

			sessions := []SessionInfo{
				{ID: "proj-a/s1", State: "alive", Pid: 100, Cols: 80, Rows: 24, Shell: "/bin/bash"},
			}
			return okFrame(sessions)
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close()

	sessions, err := c.List(WithListPrefix("proj-a"))
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, "proj-a/s1", sessions[0].ID)
}

func TestClient_List_NoOptions(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgList: func(_ []byte) protocol.Frame {
			sessions := []SessionInfo{
				{ID: "s1", State: "alive", Pid: 1, Cols: 80, Rows: 24, Shell: "/bin/sh"},
				{ID: "s2", State: "detached", Pid: 2, Cols: 80, Rows: 24, Shell: "/bin/sh"},
			}
			return okFrame(sessions)
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close()

	sessions, err := c.List()
	require.NoError(t, err)
	assert.Len(t, sessions, 2)
}

func TestClient_KillPrefix(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgKillPrefix: func(payload []byte) protocol.Frame {
			var req struct {
				Prefix string `json:"prefix"`
			}
			require.NoError(t, json.Unmarshal(payload, &req))
			assert.Equal(t, "proj-a", req.Prefix)

			return okFrame(KillPrefixResult{
				Killed: []string{"proj-a/s1", "proj-a/s2"},
				Errors: nil,
			})
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close()

	result, err := c.KillPrefix("proj-a")
	require.NoError(t, err)
	assert.Len(t, result.Killed, 2)
	assert.Empty(t, result.Errors)
}

func TestClient_KillPrefix_Error(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgKillPrefix: func(_ []byte) protocol.Frame {
			return errFrame("no sessions match prefix")
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close()

	_, err = c.KillPrefix("nonexistent")
	assert.Error(t, err)
}
