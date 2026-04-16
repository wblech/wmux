package charmvt

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEmulator_ProcessAndSnapshot verifies that written content appears in the viewport.
func TestEmulator_ProcessAndSnapshot(t *testing.T) {
	cfg := defaultConfig()
	em := newEmulator("session-1", 80, 24, cfg)

	em.Process([]byte("hello world"))

	snap := em.Snapshot()
	require.NotNil(t, snap.Viewport)
	assert.Contains(t, string(snap.Viewport), "hello world")
}

// TestEmulator_EmptySnapshot verifies that a fresh emulator has no trailing empty rows
// and nil Scrollback.
func TestEmulator_EmptySnapshot(t *testing.T) {
	cfg := defaultConfig()
	em := newEmulator("session-empty", 80, 24, cfg)

	snap := em.Snapshot()
	assert.NotNil(t, snap.Viewport)
	assert.NotContains(t, string(snap.Viewport), "\n",
		"empty viewport should have no newlines (trailing empty rows stripped)")
	assert.Nil(t, snap.Scrollback)
}

// TestEmulator_SGR_Colors verifies that SGR color sequences are accepted and the plain text appears in the viewport.
func TestEmulator_SGR_Colors(t *testing.T) {
	cfg := defaultConfig()
	em := newEmulator("session-colors", 80, 24, cfg)

	em.Process([]byte("\033[31mred text\033[0m"))

	snap := em.Snapshot()
	assert.Contains(t, string(snap.Viewport), "red text")
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
	assert.Contains(t, string(snapAlt.Viewport), "alt content")

	// Exit alt screen; main content should be visible again.
	em.Process([]byte("\033[?1049l"))
	snapMain := em.Snapshot()
	assert.Contains(t, string(snapMain.Viewport), "main content")
}

// TestEmulator_TerminalLineEndings verifies that the viewport uses \r\n line endings.
func TestEmulator_TerminalLineEndings(t *testing.T) {
	cfg := defaultConfig()
	em := newEmulator("session-crlf", 80, 24, cfg)

	em.Process([]byte("line one\r\nline two"))

	snap := em.Snapshot()
	vp := string(snap.Viewport)
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
	vp := string(snap.Viewport)
	assert.True(t, strings.HasPrefix(vp, "only line"),
		"viewport should start with content")
	assert.False(t, strings.HasSuffix(vp, "\r\n"),
		"viewport should not end with empty rows")
}

// TestEmulator_Callbacks_Title verifies that the Title callback receives the correct session ID and title.
func TestEmulator_Callbacks_Title(t *testing.T) {
	var gotSession, gotTitle string

	cfg := defaultConfig()
	WithCallbacks(Callbacks{
		Title: func(sid, title string) {
			gotSession = sid
			gotTitle = title
		},
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
		em.Process([]byte("\x07"))              // BEL
		em.Process([]byte("\033]0;title\007"))  // OSC 0 title via BEL
		em.Process([]byte("\033[?1049h"))       // enter alt screen
		em.Process([]byte("\033[?1049l"))       // exit alt screen
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
	require.NotNil(t, snap1.Scrollback)

	// Increase scrollback — no data loss.
	em.SetScrollbackSize(200)

	snap2 := em.Snapshot()
	assert.Equal(t, snap1.Scrollback, snap2.Scrollback)
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
	require.NotNil(t, snap1.Scrollback)

	// Decrease scrollback — oldest lines trimmed.
	em.SetScrollbackSize(10)

	snap2 := em.Snapshot()
	// Should have less scrollback than before.
	assert.Less(t, len(snap2.Scrollback), len(snap1.Scrollback))
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
	require.NotNil(t, snap.Scrollback, "scrollback must not be nil after lines overflow the viewport")

	sb := string(snap.Scrollback)

	// The bold SGR open sequence must appear in the scrollback output.
	assert.Contains(t, sb, "\033[")

	// At least one early line label must appear in scrollback text.
	found := false
	for i := range 7 {
		if strings.Contains(sb, fmt.Sprintf("line %02d", i)) {
			found = true
			break
		}
	}
	assert.True(t, found, "scrollback should contain text from one of the early lines")
}
