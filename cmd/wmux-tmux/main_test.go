package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTranslateKeys(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Enter", "\n"},
		{"C-c", "\x03"},
		{"C-d", "\x04"},
		{"C-z", "\x1a"},
		{"C-l", "\x0c"},
		{"hello Enter", "hello \n"},
		{"ls -la Enter", "ls -la \n"},
		{"Tab", "\t"},
		{"Space", " "},
		{"Escape", "\x1b"},
		{"BSpace", "\x7f"},
		{"no special keys", "no special keys"},
		{"C-c C-d", "\x03 \x04"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := translateKeys(tc.input)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestTranslateKeys_NoChange(t *testing.T) {
	input := "git commit -m 'hello world'"
	got := translateKeys(input)
	assert.Equal(t, input, got)
}

func TestPrintUsage_DoesNotPanic(t *testing.T) {
	assert.NotPanics(t, func() {
		printUsage()
	})
}

func TestNewSession_MissingName(t *testing.T) {
	code := cmdNewSession([]string{"-d"})
	assert.Equal(t, 1, code)
}

func TestNewSession_MissingSArg(t *testing.T) {
	code := cmdNewSession([]string{"-s"})
	assert.Equal(t, 1, code)
}

func TestSendKeys_MissingTarget(t *testing.T) {
	code := cmdSendKeys([]string{"hello"})
	assert.Equal(t, 1, code)
}

func TestSendKeys_EmptyKeys(t *testing.T) {
	// Empty keys after -t should return 0 (no-op).
	code := cmdSendKeys([]string{"-t", "test"})
	assert.Equal(t, 0, code)
}

func TestCapturePane_MissingTarget(t *testing.T) {
	code := cmdCapturePane([]string{"-p"})
	assert.Equal(t, 1, code)
}

func TestCapturePane_MissingPrintFlag(t *testing.T) {
	code := cmdCapturePane([]string{"-t", "test"})
	assert.Equal(t, 1, code)
}

func TestKillSession_MissingTarget(t *testing.T) {
	code := cmdKillSession([]string{})
	assert.Equal(t, 1, code)
}

func TestHasSession_MissingTarget(t *testing.T) {
	code := cmdHasSession([]string{})
	assert.Equal(t, 1, code)
}
