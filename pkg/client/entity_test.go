package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOptions_Defaults(t *testing.T) {
	opts := Options{}
	assert.Empty(t, opts.SocketPath)
	assert.Empty(t, opts.TokenPath)
}

func TestCreateParams_Zero(t *testing.T) {
	p := CreateParams{}
	assert.Empty(t, p.Shell)
	assert.Equal(t, 0, p.Cols)
	assert.Equal(t, 0, p.Rows)
}

func TestSnapshot_Empty(t *testing.T) {
	s := Snapshot{}
	assert.Nil(t, s.Scrollback)
	assert.Nil(t, s.Viewport)
}

func TestSessionInfo_Fields(t *testing.T) {
	info := SessionInfo{
		ID:    "test",
		State: "alive",
		Pid:   1234,
		Cols:  80,
		Rows:  24,
		Shell: "/bin/zsh",
	}
	assert.Equal(t, "test", info.ID)
	assert.Equal(t, 1234, info.Pid)
}

func TestAttachResult_WithSnapshot(t *testing.T) {
	result := AttachResult{
		Session: SessionInfo{ID: "s1"},
		Snapshot: Snapshot{
			Scrollback: []byte("history"),
			Viewport:   []byte("screen"),
		},
	}
	assert.Equal(t, "s1", result.Session.ID)
	assert.Equal(t, []byte("history"), result.Snapshot.Scrollback)
	assert.Equal(t, []byte("screen"), result.Snapshot.Viewport)
}

func TestEvent_Fields(t *testing.T) {
	evt := Event{
		Type:      "session.created",
		SessionID: "s1",
		Data:      map[string]any{"shell": "/bin/zsh"},
	}
	assert.Equal(t, "session.created", evt.Type)
	assert.Equal(t, "/bin/zsh", evt.Data["shell"])
}
