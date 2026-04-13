package client

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wblech/wmux/internal/platform/protocol"
)

func TestClient_MetaSet(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgMetaSet: func(payload []byte) protocol.Frame {
			var req struct {
				SessionID string `json:"session_id"`
				Key       string `json:"key"`
				Value     string `json:"value"`
			}
			if err := json.Unmarshal(payload, &req); err != nil {
				return errFrame("bad payload")
			}
			if req.SessionID != "s1" || req.Key != "app" || req.Value != "watchtower" {
				return errFrame("unexpected values")
			}
			return okFrame(nil)
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	err = c.MetaSet("s1", "app", "watchtower")
	assert.NoError(t, err)
}

func TestClient_MetaSet_Error(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgMetaSet: func(_ []byte) protocol.Frame {
			return errFrame("session not found")
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	err = c.MetaSet("no-such", "k", "v")
	assert.Error(t, err)
}

func TestClient_MetaGet(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgMetaGet: func(payload []byte) protocol.Frame {
			var req struct {
				Key string `json:"key"`
			}
			_ = json.Unmarshal(payload, &req)
			return okFrame(map[string]string{"value": "watchtower"})
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	val, err := c.MetaGet("s1", "app")
	require.NoError(t, err)
	assert.Equal(t, "watchtower", val)
}

func TestClient_MetaGet_Error(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgMetaGet: func(_ []byte) protocol.Frame {
			return errFrame("session not found")
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	_, err = c.MetaGet("no-such", "k")
	assert.Error(t, err)
}

func TestClient_MetaGetAll(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgMetaGet: func(payload []byte) protocol.Frame {
			var req struct {
				Key string `json:"key"`
			}
			_ = json.Unmarshal(payload, &req)
			if req.Key != "" {
				return errFrame("expected empty key for get all")
			}
			return okFrame(map[string]any{
				"metadata": map[string]string{"app": "watchtower", "env": "prod"},
			})
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	meta, err := c.MetaGetAll("s1")
	require.NoError(t, err)
	assert.Equal(t, "watchtower", meta["app"])
	assert.Equal(t, "prod", meta["env"])
}

func TestClient_MetaGetAll_Error(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgMetaGet: func(_ []byte) protocol.Frame {
			return errFrame("session not found")
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	_, err = c.MetaGetAll("no-such")
	assert.Error(t, err)
}
