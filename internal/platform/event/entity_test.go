package event

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestType_String_ShellReady(t *testing.T) {
	assert.Equal(t, "shell.ready", ShellReady.String())
}

func TestType_MarshalJSON(t *testing.T) {
	tests := []struct {
		et   Type
		want string
	}{
		{SessionCreated, `"session.created"`},
		{SessionExited, `"session.exited"`},
		{ShellReady, `"shell.ready"`},
		{Type(99), `"unknown"`},
	}
	for _, tt := range tests {
		data, err := json.Marshal(tt.et)
		require.NoError(t, err)
		assert.Equal(t, tt.want, string(data))
	}
}

func TestType_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		input string
		want  Type
	}{
		{`"session.created"`, SessionCreated},
		{`"session.exited"`, SessionExited},
		{`"shell.ready"`, ShellReady},
		{`"recording.limit_reached"`, RecordingLimitReached},
	}
	for _, tt := range tests {
		var got Type
		err := json.Unmarshal([]byte(tt.input), &got)
		require.NoError(t, err)
		assert.Equal(t, tt.want, got)
	}
}

func TestType_UnmarshalJSON_Errors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"unknown type", `"bogus.event"`},
		{"not a string", `42`},
		{"empty string", `""`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Type
			err := json.Unmarshal([]byte(tt.input), &got)
			assert.Error(t, err)
		})
	}
}

func TestEvent_JSON_RoundTrip(t *testing.T) {
	original := Event{
		Type:      SessionExited,
		SessionID: "s42",
		Payload:   map[string]any{"exit_code": float64(0)},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded Event
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, original.Type, decoded.Type)
	assert.Equal(t, original.SessionID, decoded.SessionID)
	assert.Equal(t, original.Payload["exit_code"], decoded.Payload["exit_code"])
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

func TestType_String_CommandPromptShown(t *testing.T) {
	if got := CommandPromptShown.String(); got != "command.prompt_shown" {
		t.Errorf("String() = %q, want %q", got, "command.prompt_shown")
	}
}

func TestType_String_CommandStarted(t *testing.T) {
	if got := CommandStarted.String(); got != "command.started" {
		t.Errorf("String() = %q, want %q", got, "command.started")
	}
}

func TestType_String_CommandFinished(t *testing.T) {
	if got := CommandFinished.String(); got != "command.finished" {
		t.Errorf("String() = %q, want %q", got, "command.finished")
	}
}

func TestType_RoundtripJSON_CommandTypes(t *testing.T) {
	cases := []struct {
		name string
		typ  Type
	}{
		{"prompt_shown", CommandPromptShown},
		{"started", CommandStarted},
		{"finished", CommandFinished},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, err := c.typ.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON: %v", err)
			}
			var got Type
			if err := got.UnmarshalJSON(data); err != nil {
				t.Fatalf("UnmarshalJSON: %v", err)
			}
			if got != c.typ {
				t.Errorf("roundtrip: got %v, want %v", got, c.typ)
			}
		})
	}
}
