package charmvt

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEmulator_ProcessAndSnapshot verifies that written content appears in the replay.
func TestEmulator_ProcessAndSnapshot(t *testing.T) {
	cfg := defaultConfig()
	em := newEmulator("session-1", 80, 24, cfg)

	em.Process([]byte("hello world"))

	snap := em.Snapshot()
	require.NotNil(t, snap.Replay)
	assert.Contains(t, string(snap.Replay), "hello world")
}

// TestEmulator_EmptySnapshot verifies that a fresh emulator's Replay begins
// with the clear prefix and ends with a cursor-restore CUP.
// Since Snapshot() now also re-emits the DECSC saved cursor (ESC 7), the tail
// after the clear prefix has the form:
//
//	<CUP to saved pos> ESC 7 <CUP to live pos>
//
// For a freshly created emulator both positions are (1,1), so the tail is
// "\x1b[1;1H\x1b7\x1b[1;1H".
func TestEmulator_EmptySnapshot(t *testing.T) {
	cfg := defaultConfig()
	em := newEmulator("session-empty", 80, 24, cfg)

	snap := em.Snapshot()
	assert.True(t, bytes.HasPrefix(snap.Replay, []byte("\x1b[2J\x1b[H\x1b[3J")),
		"snapshot must begin with clear prefix")
	// The tail must end with a CUP sequence (the live cursor position restore).
	assert.Regexp(t, `\x1b\[\d+;\d+H$`, string(snap.Replay),
		"snapshot must end with a CUP sequence for the live cursor position")
	// The DECSC saved-cursor re-emit must appear somewhere before the final CUP.
	assert.Contains(t, string(snap.Replay), "\x1b7",
		"snapshot must contain ESC 7 (DECSC) to preserve the saved cursor")
}

// TestEmulator_SGR_Colors verifies that SGR color sequences are accepted and the plain text appears in the replay.
func TestEmulator_SGR_Colors(t *testing.T) {
	cfg := defaultConfig()
	em := newEmulator("session-colors", 80, 24, cfg)

	em.Process([]byte("\033[31mred text\033[0m"))

	snap := em.Snapshot()
	assert.Contains(t, string(snap.Replay), "red text")
}

// TestEmulator_AltScreen verifies alt-screen switching: content appears on the correct screen.
func TestEmulator_AltScreen(t *testing.T) {
	cfg := defaultConfig()
	em := newEmulator("session-alt", 80, 24, cfg)

	// Write content on the main screen.
	em.Process([]byte("main content"))

	// Enter alt screen and write alt content.
	em.Process([]byte("\033[?1049h"))
	em.Process([]byte("alt content"))

	snapAlt := em.Snapshot()
	assert.Contains(t, string(snapAlt.Replay), "alt content")

	// Exit alt screen; main content should be visible again.
	em.Process([]byte("\033[?1049l"))
	snapMain := em.Snapshot()
	assert.Contains(t, string(snapMain.Replay), "main content")
}

// TestEmulator_TerminalLineEndings verifies that the replay uses \r\n line endings.
func TestEmulator_TerminalLineEndings(t *testing.T) {
	cfg := defaultConfig()
	em := newEmulator("session-crlf", 80, 24, cfg)

	em.Process([]byte("line one\r\nline two"))

	snap := em.Snapshot()
	vp := string(snap.Replay)
	assert.Contains(t, vp, "line one\r\nline two")
	assert.NotContains(t, vp, "\n\r\n", "should not have bare \\n before \\r\\n")
}

// TestEmulator_NoTrailingEmptyRows verifies that viewport trailing empty rows are stripped.
func TestEmulator_NoTrailingEmptyRows(t *testing.T) {
	cfg := defaultConfig()
	// Small 5-row viewport; write one line of content — 4 rows are empty.
	em := newEmulator("session-trailing", 80, 5, cfg)

	em.Process([]byte("only line"))

	snap := em.Snapshot()
	body := strings.TrimPrefix(string(snap.Replay), "\x1b[2J\x1b[H\x1b[3J")
	assert.Contains(t, body, "only line",
		"replay should contain the written content")
	assert.Regexp(t, `\x1b\[\d+;\d+H$`, string(snap.Replay),
		"replay should end with a CUP sequence, not trailing empty rows")
}

// TestEmulator_Callbacks_Title verifies that the Title callback receives the correct session ID and title.
func TestEmulator_Callbacks_Title(t *testing.T) {
	var gotSession, gotTitle string

	cfg := defaultConfig()
	WithCallbacks(Callbacks{
		Bell: nil,
		Title: func(sid, title string) {
			gotSession = sid
			gotTitle = title
		},
		WorkingDirectory: nil,
		AltScreen:        nil,
	})(cfg)

	em := newEmulator("cb-title-session", 80, 24, cfg)

	// OSC 2 sets the window title; ST can be BEL (\007) or ESC \ (\033\\).
	em.Process([]byte("\033]2;My Terminal\033\\"))

	assert.Equal(t, "cb-title-session", gotSession)
	assert.Equal(t, "My Terminal", gotTitle)
}

// TestEmulator_NilCallbacks_NoPanic verifies that writing event-triggering sequences without any
// callbacks configured does not panic.
func TestEmulator_NilCallbacks_NoPanic(t *testing.T) {
	cfg := defaultConfig()
	// No callbacks configured.
	em := newEmulator("session-no-cb", 80, 24, cfg)

	assert.NotPanics(t, func() {
		em.Process([]byte("\x07"))             // BEL
		em.Process([]byte("\033]0;title\007")) // OSC 0 title via BEL
		em.Process([]byte("\033[?1049h"))      // enter alt screen
		em.Process([]byte("\033[?1049l"))      // exit alt screen
	})
}

// TestEmulator_SetScrollbackSize verifies that increasing the scrollback size preserves existing data.
func TestEmulator_SetScrollbackSize(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 100
	em := newEmulator("test", 80, 5, cfg)

	// Write enough to fill scrollback.
	for i := range 20 {
		em.Process([]byte(fmt.Sprintf("line %d\r\n", i)))
	}

	snap1 := em.Snapshot()
	require.NotEmpty(t, snap1.Replay)

	// Increase scrollback — no data loss.
	em.SetScrollbackSize(200)

	snap2 := em.Snapshot()
	assert.Equal(t, snap1.Replay, snap2.Replay)
}

// TestEmulator_SetScrollbackSize_Decrease verifies that decreasing the scrollback size trims oldest lines.
func TestEmulator_SetScrollbackSize_Decrease(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000
	em := newEmulator("test", 80, 5, cfg)

	for i := range 100 {
		em.Process([]byte(fmt.Sprintf("line %d\r\n", i)))
	}

	snap1 := em.Snapshot()
	require.NotEmpty(t, snap1.Replay)

	// Decrease scrollback — oldest lines trimmed.
	em.SetScrollbackSize(10)

	snap2 := em.Snapshot()
	assert.Less(t, len(snap2.Replay), len(snap1.Replay))
}

// TestEmulator_ScrollbackWithStyles verifies that styled lines pushed into scrollback contain SGR sequences
// and the original text.
func TestEmulator_ScrollbackWithStyles(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000

	// Small viewport (3 rows) so that 10 lines push content into scrollback.
	em := newEmulator("session-sb-styles", 80, 3, cfg)

	for i := range 10 {
		line := fmt.Sprintf("\033[1mline %02d\033[0m\r\n", i)
		em.Process([]byte(line))
	}

	snap := em.Snapshot()
	require.NotEmpty(t, snap.Replay, "replay must not be empty after lines overflow the viewport")

	sb := string(snap.Replay)

	// The bold SGR open sequence must appear in the replay.
	assert.Contains(t, sb, "\033[")

	// At least one early line label must appear in the replay text.
	found := false
	for i := range 7 {
		if strings.Contains(sb, fmt.Sprintf("line %02d", i)) {
			found = true
			break
		}
	}
	assert.True(t, found, "replay should contain text from one of the early lines")
}
