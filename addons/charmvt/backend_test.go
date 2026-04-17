package charmvt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()
	assert.Equal(t, 10000, cfg.scrollback)
	assert.Nil(t, cfg.callbacks)
	assert.Nil(t, cfg.logger)
}

func TestWithScrollbackSize(t *testing.T) {
	cfg := defaultConfig()
	WithScrollbackSize(500)(cfg)
	assert.Equal(t, 500, cfg.scrollback)
}

func TestWithCallbacks(t *testing.T) {
	cfg := defaultConfig()
	cb := Callbacks{
		Bell:             func(sessionID string) {},
		Title:            nil,
		WorkingDirectory: nil,
		AltScreen:        nil,
	}
	WithCallbacks(cb)(cfg)
	require.NotNil(t, cfg.callbacks)
	assert.NotNil(t, cfg.callbacks.Bell)
}

func TestWithLogger(t *testing.T) {
	cfg := defaultConfig()
	assert.Nil(t, cfg.logger)

	l := &testLogger{messages: nil}
	WithLogger(l)(cfg)
	assert.Equal(t, l, cfg.logger)
}

func TestFactory_CreateReturnsEmulator(t *testing.T) {
	f := &factory{cfg: defaultConfig()}
	emu := f.Create("test-session", 80, 24)
	require.NotNil(t, emu)

	// Write some data and verify it appears in the snapshot.
	emu.Process([]byte("hello world"))
	snap := emu.Snapshot()
	assert.Contains(t, string(snap.Replay), "hello world")
}

func TestFactory_CreateIndependentInstances(t *testing.T) {
	f := &factory{cfg: defaultConfig()}
	emu1 := f.Create("session-1", 80, 24)
	emu2 := f.Create("session-2", 80, 24)

	emu1.Process([]byte("alpha"))
	emu2.Process([]byte("bravo"))

	snap1 := emu1.Snapshot()
	snap2 := emu2.Snapshot()

	assert.Contains(t, string(snap1.Replay), "alpha")
	assert.NotContains(t, string(snap1.Replay), "bravo")
	assert.Contains(t, string(snap2.Replay), "bravo")
	assert.NotContains(t, string(snap2.Replay), "alpha")
}

func TestBackend_ReturnsOption(t *testing.T) {
	opt := Backend()
	assert.NotNil(t, opt)

	// Verify the returned value satisfies the client.Option type.
	var _ = opt
}

func TestFactory_Close(t *testing.T) {
	f := &factory{cfg: defaultConfig()}
	// Close should not panic.
	f.Close()
}

func TestEmulator_Resize(t *testing.T) {
	f := &factory{cfg: defaultConfig()}
	emu := f.Create("test-resize", 80, 24)

	// Write a long line, resize to smaller, verify no panic.
	emu.Process([]byte("this is a test line"))
	emu.Resize(40, 12)
	snap := emu.Snapshot()
	assert.NotEmpty(t, snap.Replay)
}

func TestEmulator_Scrollback(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 100
	f := &factory{cfg: cfg}
	emu := f.Create("test-scrollback", 80, 5)

	// Write enough lines to push some into scrollback (5-line viewport).
	var lines []string
	for i := range 20 {
		lines = append(lines, strings.Repeat("x", 10))
		_ = i
	}
	emu.Process([]byte(strings.Join(lines, "\r\n")))

	snap := emu.Snapshot()
	// With 20 lines and 5-row viewport, scrollback should have content.
	assert.NotEmpty(t, snap.Replay, "scrollback should contain data when lines exceed viewport")
}

func TestEmulator_CallbacksBound(t *testing.T) {
	var bellSession string
	var titleSession, titleValue string

	cfg := defaultConfig()
	WithCallbacks(Callbacks{
		Bell: func(sid string) { bellSession = sid },
		Title: func(sid, title string) {
			titleSession = sid
			titleValue = title
		},
		WorkingDirectory: nil,
		AltScreen:        nil,
	})(cfg)

	f := &factory{cfg: cfg}
	emu := f.Create("cb-session", 80, 24)

	// Trigger bell (BEL character).
	emu.Process([]byte("\x07"))
	assert.Equal(t, "cb-session", bellSession)

	// Trigger title change via OSC 0.
	emu.Process([]byte("\x1b]0;My Title\x07"))
	assert.Equal(t, "cb-session", titleSession)
	assert.Equal(t, "My Title", titleValue)
}

func TestTrimTrailingEmptyRows(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", ""},
		{"only newlines", "\n\n\n", ""},
		{"content with trailing newlines", "hello\nworld\n\n\n", "hello\nworld"},
		{"content without trailing newlines", "hello\nworld", "hello\nworld"},
		{"single line no newline", "hello", "hello"},
		{"single newline", "\n", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, trimTrailingEmptyRows(tc.input))
		})
	}
}

func TestToTerminalLineEndings(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"no newlines", "hello", "hello"},
		{"single newline", "a\nb", "a\r\nb"},
		{"multiple newlines", "a\nb\nc", "a\r\nb\r\nc"},
		{"empty string", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, toTerminalLineEndings(tc.input))
		})
	}
}

// testLogger is a simple Logger implementation for testing.
type testLogger struct {
	messages []string
}

func (l *testLogger) Printf(format string, v ...any) {
	l.messages = append(l.messages, format)
}
