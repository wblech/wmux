package event

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEventType_String(t *testing.T) {
	tests := []struct {
		et   EventType
		want string
	}{
		{SessionCreated, "session.created"},
		{SessionAttached, "session.attached"},
		{SessionDetached, "session.detached"},
		{SessionExited, "session.exited"},
		{EventType(99), "unknown"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.et.String())
	}
}

func TestEvent_Fields(t *testing.T) {
	e := Event{
		Type:      SessionCreated,
		SessionID: "s1",
		Payload:   map[string]any{"shell": "/bin/zsh"},
	}

	assert.Equal(t, SessionCreated, e.Type)
	assert.Equal(t, "s1", e.SessionID)
	assert.Equal(t, "/bin/zsh", e.Payload["shell"])
}
