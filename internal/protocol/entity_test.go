package protocol

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMessageType_String(t *testing.T) {
	cases := []struct {
		mt   MessageType
		want string
	}{
		{MsgData, "data"},
		{MsgCreate, "create"},
		{MsgAttach, "attach"},
		{MsgDetach, "detach"},
		{MsgKill, "kill"},
		{MsgResize, "resize"},
		{MsgList, "list"},
		{MsgInfo, "info"},
		{MsgInput, "input"},
		{MsgEvent, "event"},
		{MsgHeartbeat, "heartbeat"},
		{MsgHeartbeatAck, "heartbeat_ack"},
		{MsgError, "error"},
		{MsgOK, "ok"},
		{MsgShutdown, "shutdown"},
	}

	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.mt.String())
		})
	}
}

func TestFrame_HeaderSize(t *testing.T) {
	assert.Equal(t, 6, HeaderSize)
}

func TestVersion(t *testing.T) {
	assert.Equal(t, byte(1), ProtocolVersion)
}
