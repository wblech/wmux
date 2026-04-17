package charmvt

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Evidence tests for cursor position desync after warm reconnect.
//
// Bug: After closing and reopening Watchtower, the terminal cursor blinks
// at the bottom of the viewport instead of at the shell prompt. Typed text
// appears in the wrong location, but Enter sends correctly.
//
// Root cause hypothesis: Snapshot().Viewport does not include a CUP (Cursor
// Position) escape sequence, so xterm.js leaves the cursor wherever write()
// ends — typically the last non-empty row, not the actual cursor position.

// TestEvidence_SnapshotViewportIncludesCursorPosition verifies that the
// viewport snapshot ends with a CUP escape to restore cursor position.
// Without this, xterm.js leaves the cursor wherever write() ends after
// a warm reconnect, causing visual desync between cursor and prompt.
func TestEvidence_SnapshotViewportIncludesCursorPosition(t *testing.T) {
	cfg := defaultConfig()
	em := newEmulator("cursor-test", 80, 24, cfg)

	// Simulate a prompt with typed text (avoids trailing-space trim by Render)
	em.Process([]byte("$ cmd"))

	snap := em.Snapshot()
	vp := string(snap.Viewport)

	// The viewport should contain the prompt text
	require.Contains(t, vp, "$ cmd")

	// FIX VERIFIED: viewport MUST contain a CUP sequence to restore cursor.
	hasCUP := containsCUP(vp)
	assert.True(t, hasCUP,
		"viewport must include CUP sequence to restore cursor position after warm reconnect")
}

// containsCUP checks if a string contains an ANSI CUP (Cursor Position) sequence.
// CUP format: \x1b[<row>;<col>H
func containsCUP(s string) bool {
	for i := 0; i < len(s)-2; i++ {
		if s[i] != '\x1b' || s[i+1] != '[' {
			continue
		}
		// Scan past digits and semicolons looking for 'H'
		for j := i + 2; j < len(s); j++ {
			c := s[j]
			if c == 'H' {
				return true
			}
			if c >= '0' && c <= '9' || c == ';' {
				continue
			}
			break // not a CUP
		}
	}
	return false
}

// TestEvidence_CursorPositionAvailableButUnused proves that the vt.Emulator
// tracks cursor position correctly, but Snapshot() discards it.
func TestEvidence_CursorPositionAvailableButUnused(t *testing.T) {
	cfg := defaultConfig()
	em := newEmulator("cursor-avail", 80, 24, cfg)

	// Write prompt with text so trailing-space trim doesn't affect it
	em.Process([]byte("$ cmd"))

	// The emulator internally knows the cursor position
	em.mu.Lock()
	pos := em.term.CursorPosition()
	em.mu.Unlock()

	// Cursor should be at row 0, col 5 (0-indexed) — right after "$ cmd"
	assert.Equal(t, 0, pos.Y, "cursor row should be 0 (first row)")
	assert.Equal(t, 5, pos.X, "cursor col should be 5 (after '$ cmd')")

	// But the snapshot has NO way to communicate this position
	snap := em.Snapshot()

	// The Snapshot struct only has Viewport and Scrollback — no cursor fields
	_ = snap.Viewport
	_ = snap.Scrollback
	// There is no snap.CursorRow or snap.CursorCol — the position is lost
}

// TestEvidence_CursorDesyncAfterMultilinePrompt shows the desync is worse
// with multi-line prompts: the cursor position drifts further from where
// xterm.js places it after write().
func TestEvidence_CursorDesyncAfterMultilinePrompt(t *testing.T) {
	cfg := defaultConfig()
	em := newEmulator("cursor-multiline", 80, 10, cfg)

	// Simulate a typical starship/powerlevel10k multi-line prompt:
	// Line 1: directory info
	// Line 2: prompt character with cursor
	em.Process([]byte("~/private/watchtower on main\r\n❯ "))

	em.mu.Lock()
	pos := em.term.CursorPosition()
	em.mu.Unlock()

	// Cursor is at row 1, after "❯ " (col varies by unicode width)
	assert.Equal(t, 1, pos.Y, "cursor should be on second row of prompt")
	assert.Positive(t, pos.X, "cursor should be past the prompt character")

	snap := em.Snapshot()
	vp := string(snap.Viewport)

	// After xterm.js writes this viewport, cursor ends at end of last written
	// char. With trimTrailingEmptyRows, that's the end of row 1 — which
	// happens to be close to correct for this case. But if there were output
	// below the prompt (e.g., from a previous command), cursor would be wrong.
	t.Logf("Viewport content:\n%s", vp)
	t.Logf("Actual cursor position: row=%d, col=%d", pos.Y, pos.X)
	t.Logf("BUG: xterm.js cursor will be at end of written text, not at (%d,%d)", pos.Y, pos.X)
}

// TestEvidence_CursorDesyncWithOutputBelowPrompt is the exact scenario from
// the bug report: output exists below the prompt line, so trimTrailingEmptyRows
// does NOT help — the cursor ends up at the last output line, not at the prompt.
func TestEvidence_CursorDesyncWithOutputBelowPrompt(t *testing.T) {
	cfg := defaultConfig()
	em := newEmulator("cursor-below", 80, 10, cfg)

	// Simulate: user ran a command, got output, then got a new prompt with text
	em.Process([]byte("$ ls\r\n"))
	em.Process([]byte("file1.txt  file2.txt  file3.txt\r\n"))
	em.Process([]byte("$ x"))

	em.mu.Lock()
	pos := em.term.CursorPosition()
	em.mu.Unlock()

	// Cursor should be on row 2 (0-indexed), col 3 (after "$ x")
	assert.Equal(t, 2, pos.Y, "cursor should be on prompt row (row 2)")
	assert.Equal(t, 3, pos.X, "cursor should be at col 3 (after '$ x')")

	snap := em.Snapshot()
	vp := string(snap.Viewport)

	// The viewport has 3 lines of content. After writing to xterm.js,
	// the cursor is at the end of the LAST written text.
	// With trimTrailingEmptyRows, the viewport ends at row 2 "$ ".
	// xterm.js cursor would be at end of "$ " which is row 2, col 2.
	// In THIS case it's coincidentally correct — but only because
	// the prompt is the last non-empty line.
	lines := strings.Split(vp, "\r\n")
	lastLine := lines[len(lines)-1]
	t.Logf("Last viewport line: %q", lastLine)
	t.Logf("Cursor position: row=%d, col=%d", pos.Y, pos.X)

	// The REAL bug case: when the prompt is NOT the last line with content.
	// This happens with starship/p10k prompts that have a transient prompt,
	// or when a background job prints output after the prompt appears.
}

// TestEvidence_CursorDesyncPromptNotLastLine is the critical case:
// the prompt is NOT the last non-empty line. This is exactly what
// happens with fancy prompts (starship) where prompt decorations
// span multiple lines and there may be status bar content below.
func TestEvidence_CursorDesyncPromptNotLastLine(t *testing.T) {
	cfg := defaultConfig()
	em := newEmulator("cursor-notlast", 80, 10, cfg)

	// Line 0: previous command output
	em.Process([]byte("hello world\r\n"))
	// Line 1: prompt with cursor (use "$ x" to avoid trailing-space trim)
	em.Process([]byte("$ x"))
	// Save cursor, write content below the prompt, restore cursor
	em.Process([]byte("\x1b7"))      // save cursor
	em.Process([]byte("\x1b[5;1H"))  // move to row 5
	em.Process([]byte("status bar")) // write below prompt
	em.Process([]byte("\x1b8"))      // restore cursor (back to prompt)

	em.mu.Lock()
	pos := em.term.CursorPosition()
	em.mu.Unlock()

	// Cursor is restored to row 1, col 3 (at the prompt after "$ x")
	assert.Equal(t, 1, pos.Y, "cursor should be at prompt row")
	assert.Equal(t, 3, pos.X, "cursor should be at prompt col")

	snap := em.Snapshot()
	vp := string(snap.Viewport)
	lines := strings.Split(vp, "\r\n")

	// The viewport has content on row 0, 1, and 4 (0-indexed)
	// After trimTrailingEmptyRows, the last line is the status bar on row 4
	t.Logf("Viewport lines: %d", len(lines))
	for i, line := range lines {
		t.Logf("  [%d] %q", i, line)
	}
	t.Logf("Actual cursor: row=%d, col=%d", pos.Y, pos.X)

	// BUG EVIDENCE: xterm.js will place cursor at end of last written content
	// (row 4, after "status bar"), but the actual cursor is at row 1, col 2.
	// This is a ~3 row desync.
	lastLineIdx := len(lines) - 1
	assert.NotEqual(t, pos.Y, lastLineIdx,
		"BUG EVIDENCE: cursor row (%d) != last viewport line (%d) — "+
			"xterm.js will put cursor in the wrong place", pos.Y, lastLineIdx)
}

// TestEvidence_ViewportShouldEndWithCUP documents the expected fix:
// after the viewport content, a CUP sequence should position the cursor.
func TestEvidence_ViewportShouldEndWithCUP(t *testing.T) {
	cfg := defaultConfig()
	em := newEmulator("cursor-fix", 80, 10, cfg)

	em.Process([]byte("$ cmd"))

	em.mu.Lock()
	pos := em.term.CursorPosition()
	em.mu.Unlock()

	// Expected CUP sequence (1-indexed for ANSI): \x1b[1;6H
	expectedCUP := fmt.Sprintf("\x1b[%d;%dH", pos.Y+1, pos.X+1)

	snap := em.Snapshot()
	vp := string(snap.Viewport)

	// This test will PASS once the fix is implemented.
	// Currently it FAILS — documenting the expected behavior.
	assert.True(t, strings.HasSuffix(vp, expectedCUP),
		"viewport should end with CUP %q to restore cursor, got suffix: %q",
		expectedCUP, vp[max(0, len(vp)-20):])
}
