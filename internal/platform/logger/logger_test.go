package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_JSONOutput(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, slog.LevelInfo)

	log.Info("hello world", "key", "value")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))

	assert.Equal(t, "hello world", entry["msg"])
	assert.Equal(t, "INFO", entry["level"])
	assert.Equal(t, "value", entry["key"])
	assert.NotEmpty(t, entry["time"])
}

func TestNew_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, slog.LevelWarn)

	log.Info("should not appear")
	assert.Empty(t, buf.String(), "info message should be filtered out by warn level")

	log.Warn("should appear")
	assert.NotEmpty(t, buf.String(), "warn message should appear")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	assert.Equal(t, "should appear", entry["msg"])
}

func TestWithSessionID(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, slog.LevelInfo)
	log = WithSessionID(log, "session-abc-123")

	log.Info("event happened")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))

	assert.Equal(t, "session-abc-123", entry["session_id"])
	assert.Equal(t, "event happened", entry["msg"])
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"WARN", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"WARNING", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},
		{"unknown", slog.LevelInfo},
		{"", slog.LevelInfo},
		{"garbage", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, ParseLevel(tt.input))
		})
	}
}
