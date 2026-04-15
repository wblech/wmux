package client

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wblech/wmux/internal/platform/protocol"
)

func TestConnect_SendsSubscribeFrame(t *testing.T) {
	subscribeCalled := make(chan []byte, 1)

	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgEvent: func(payload []byte) protocol.Frame {
			subscribeCalled <- payload
			return protocol.Frame{
				Version: protocol.ProtocolVersion,
				Type:    protocol.MsgOK,
				Payload: nil,
			}
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	select {
	case <-subscribeCalled:
		// Subscribe frame was sent during connect — success.
	case <-time.After(2 * time.Second):
		t.Fatal("connect() did not send MsgEvent subscribe frame")
	}
}

func TestConnect_SubscribeError_FailsConnect(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgEvent: func(_ []byte) protocol.Frame {
			payload, _ := json.Marshal(map[string]string{"error": "events not enabled"})
			return protocol.Frame{
				Version: protocol.ProtocolVersion,
				Type:    protocol.MsgError,
				Payload: payload,
			}
		},
	})
	defer cleanup()

	_, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subscribe")
}
