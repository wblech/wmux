package event

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestType_String(t *testing.T) {
	tests := []struct {
		et   Type
		want string
	}{
		{SessionCreated, "session.created"},
		{SessionAttached, "session.attached"},
		{SessionDetached, "session.detached"},
		{SessionExited, "session.exited"},
		{Type(99), "unknown"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.et.String())
	}
}

func TestType_String_Phase2(t *testing.T) {
	tests := []struct {
		et   Type
		want string
	}{
		{SessionIdle, "session.idle"},
		{SessionKilled, "session.killed"},
		{Resize, "resize"},
		{CwdChanged, "cwd.changed"},
		{Notification, "notification"},
		{OutputFlood, "output.flood"},
		{RecordingLimitReached, "recording.limit_reached"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.et.String())
		})
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
