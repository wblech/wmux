package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSessionHistory_Fields(t *testing.T) {
	h := SessionHistory{
		Scrollback: []byte("log"),
		SessionID:  "s1",
		Shell:      "/bin/zsh",
		Cwd:        "/home/user",
		Cols:       80,
		Rows:       24,
	}
	assert.Equal(t, "s1", h.SessionID)
	assert.Equal(t, "/bin/zsh", h.Shell)
	assert.Equal(t, 80, h.Cols)
}

func TestCreateParams_Zero(t *testing.T) {
	p := CreateParams{
		Shell: "",
		Args:  nil,
		Cols:  0,
		Rows:  0,
		Cwd:   "",
		Env:   nil,
	}
	assert.Empty(t, p.Shell)
	assert.Equal(t, 0, p.Cols)
	assert.Equal(t, 0, p.Rows)
}

func TestSnapshot_Empty(t *testing.T) {
	s := Snapshot{
		Scrollback: nil,
		Viewport:   nil,
	}
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
		Session: SessionInfo{
			ID:    "s1",
			State: "",
			Pid:   0,
			Cols:  0,
			Rows:  0,
			Shell: "",
		},
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
