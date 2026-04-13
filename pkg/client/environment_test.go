package client

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wblech/wmux/internal/platform/protocol"
)

func TestClient_ForwardEnv(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgEnvForward: func(payload []byte) protocol.Frame {
			var req struct {
				SessionID string            `json:"session_id"`
				Env       map[string]string `json:"env"`
			}
			if err := json.Unmarshal(payload, &req); err != nil {
				return errFrame("bad payload")
			}
			if req.SessionID != "s1" {
				return errFrame("unexpected session")
			}
			if req.Env["SSH_AUTH_SOCK"] != "/tmp/ssh-xxx" {
				return errFrame("unexpected env")
			}
			return okFrame(nil)
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	err = c.ForwardEnv("s1", map[string]string{"SSH_AUTH_SOCK": "/tmp/ssh-xxx"})
	assert.NoError(t, err)
}

func TestClient_ForwardEnv_EmptyMap(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgEnvForward: func(_ []byte) protocol.Frame {
			return okFrame(nil)
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	err = c.ForwardEnv("s1", map[string]string{})
	assert.NoError(t, err)
}

func TestClient_ForwardEnv_Error(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgEnvForward: func(_ []byte) protocol.Frame {
			return errFrame("session not found")
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	err = c.ForwardEnv("no-such", map[string]string{"K": "V"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
}
