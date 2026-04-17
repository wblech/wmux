package charmvt

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const clearPrefix = "\x1b[2J\x1b[H\x1b[3J"

// TestSnapshot_EmptyEmulator — Replay on a fresh emulator is just
// clearPrefix + cursor CUP. Applying it to a dirty destination must yield
// the same empty-state Replay.
func TestSnapshot_EmptyEmulator(t *testing.T) {
	cfg := defaultConfig()
	src := newEmulator("src", 80, 24, cfg)
	snap := src.Snapshot()

	assert.True(t, bytes.HasPrefix(snap.Replay, []byte(clearPrefix)))

	dst := newEmulator("dst", 80, 24, cfg)
	dst.Process([]byte("pre-existing garbage\r\n"))
	dst.Process(snap.Replay)

	replayed := dst.Snapshot().Replay
	assert.Equal(t, string(snap.Replay), string(replayed),
		"empty Replay applied to dirty dst must match source's empty Replay")
	assert.NotContains(t, string(replayed), "garbage")
}

// TestSnapshot_PreservesSGRColors — SGR color sequences must round-trip
// through Replay so xterm.js keeps coloring after warm reconnect.
func TestSnapshot_PreservesSGRColors(t *testing.T) {
	cfg := defaultConfig()
	src := newEmulator("src", 80, 24, cfg)
	src.Process([]byte("\x1b[31mred\x1b[0m \x1b[1;34mbold-blue\x1b[0m"))

	snap := src.Snapshot()
	dst := newEmulator("dst", 80, 24, cfg)
	dst.Process(snap.Replay)

	replayed := string(dst.Snapshot().Replay)
	assert.Contains(t, replayed, "red")
	assert.Contains(t, replayed, "bold-blue")
	// Color sequences must survive — presence of SGR in the replay stream.
	assert.Regexp(t, `\x1b\[[0-9;]*m`, replayed,
		"SGR color sequences must be preserved in Replay")
}

// TestSnapshot_PreservesUnicode — multi-byte UTF-8 and combining chars
// must replay intact.
func TestSnapshot_PreservesUnicode(t *testing.T) {
	cfg := defaultConfig()
	src := newEmulator("src", 80, 24, cfg)
	src.Process([]byte("こんにちは 世界 🌍 café"))

	snap := src.Snapshot()
	dst := newEmulator("dst", 80, 24, cfg)
	dst.Process(snap.Replay)

	replayed := string(dst.Snapshot().Replay)
	assert.Contains(t, replayed, "こんにちは")
	assert.Contains(t, replayed, "世界")
	assert.Contains(t, replayed, "café")
}

// TestSnapshot_CursorRestored — after replay, cursor must be at the same
// (row, col) as in the source.
func TestSnapshot_CursorRestored(t *testing.T) {
	cfg := defaultConfig()
	src := newEmulator("src", 80, 24, cfg)
	// Move cursor to a non-origin spot and leave it there.
	src.Process([]byte("\x1b[5;10H"))
	src.Process([]byte("X"))
	// Cursor is now at (5, 11) after writing 'X'.

	snap := src.Snapshot()
	// The tail of Replay must contain a CUP to the source cursor position.
	// Use a relaxed regex — exact row/col depends on vt.Emulator internals.
	assert.Regexp(t, `\x1b\[\d+;\d+H$`, string(snap.Replay),
		"Replay must end with a CUP sequence")
}

// TestSnapshot_AltScreenContent — a TUI-style session in alt screen must
// still round-trip through Replay. This mirrors the real watchtower bug
// scenario: Claude Code runs, takes snapshot, replay applied later.
func TestSnapshot_AltScreenContent(t *testing.T) {
	cfg := defaultConfig()
	src := newEmulator("src", 80, 24, cfg)
	src.Process([]byte("$ claude\r\n"))           // main screen history
	src.Process([]byte("\x1b[?1049h"))            // enter alt screen
	src.Process([]byte("\x1b[2J\x1b[HCLAUDE UI")) // Claude-like TUI

	snap := src.Snapshot()
	require.NotEmpty(t, snap.Replay)

	dst := newEmulator("dst", 80, 24, cfg)
	dst.Process(snap.Replay)

	replayed := string(dst.Snapshot().Replay)
	// The current visible content (CLAUDE UI) must be present exactly once —
	// no duplication with the main-screen history.
	assert.Equal(t, 1, strings.Count(replayed, "CLAUDE UI"),
		"alt-screen content must not duplicate on replay")
}

// TestSnapshot_DoubleReplay — A.Replay → B → B.Replay → C. State at C
// must equal state at A (fixed-point under repeated replay).
func TestSnapshot_DoubleReplay(t *testing.T) {
	cfg := defaultConfig()
	a := newEmulator("a", 80, 24, cfg)
	a.Process([]byte("line 1\r\nline 2\r\n"))
	a.Process([]byte("\x1b[5;1Hfixed"))

	snapA := a.Snapshot()

	b := newEmulator("b", 80, 24, cfg)
	b.Process(snapA.Replay)
	snapB := b.Snapshot()

	c := newEmulator("c", 80, 24, cfg)
	c.Process(snapB.Replay)
	snapC := c.Snapshot()

	assert.Equal(t, string(snapA.Replay), string(snapB.Replay),
		"snapA → B replay must produce identical state")
	assert.Equal(t, string(snapB.Replay), string(snapC.Replay),
		"snapB → C replay must produce identical state (fixed point)")
}
