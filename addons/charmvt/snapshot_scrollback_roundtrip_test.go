package charmvt

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/vt"
	"github.com/stretchr/testify/assert"
)

// viewportRows returns the visible viewport rows of a vt.Emulator as a slice of
// trimmed strings, one per row. Rows are trimmed of trailing spaces.
func viewportRows(em *vt.Emulator, cols, rows int) []string {
	rendered := em.Render()
	// Render() returns rows joined by \n, possibly with trailing \n on empty rows.
	lines := strings.Split(rendered, "\n")
	result := make([]string, rows)
	for y := 0; y < rows; y++ {
		if y < len(lines) {
			result[y] = strings.TrimRight(lines[y], " \t")
		}
	}
	return result
}

// TestSnapshotRoundTrip_PrimaryScrollback_Go reproduces the off-by-one in
// charmvt.Snapshot() scrollback serialization.
//
// Scenario: write 60 unique lines into a 83x41 primary-screen terminal.
// This pushes 19 lines into scrollback and leaves 41 lines in the viewport.
// Take Snapshot(), replay into a fresh vt.Emulator, and assert that the
// visible viewport rows match the original emulator's viewport.
//
// Before the fix this test MUST FAIL with row[0]="line-20" in ground truth
// but row[0]="" (or off-by-one content) in replay.
func TestSnapshotRoundTrip_PrimaryScrollback_Go(t *testing.T) {
	const cols, rows = 83, 41

	// --- source emulator (charmvt addon) ---
	cfg := defaultConfig()
	src := newEmulator("src", cols, rows, cfg)
	src.Process([]byte("\x1b[2J\x1b[H"))
	for i := 1; i <= 60; i++ {
		src.Process([]byte(fmt.Sprintf("line-%02d\n", i)))
	}
	snap := src.Snapshot()

	// --- ground truth: raw vt.Emulator with the same writes ---
	ground := vt.NewEmulator(cols, rows)
	ground.SetED2SavesScrollback(false)
	defer func() { _ = ground.Close() }()
	_, _ = ground.Write([]byte("\x1b[2J\x1b[H"))
	for i := 1; i <= 60; i++ {
		_, _ = fmt.Fprintf(ground, "line-%02d\n", i)
	}

	// --- replay emulator: fresh vt.Emulator fed the snapshot ---
	replay := vt.NewEmulator(cols, rows)
	replay.SetED2SavesScrollback(false)
	defer func() { _ = replay.Close() }()
	_, _ = replay.Write(snap.Replay)

	// Compare visible viewport rows.
	groundVP := viewportRows(ground, cols, rows)
	replayVP := viewportRows(replay, cols, rows)

	// Compare cursor position.
	groundPos := ground.CursorPosition()
	replayPos := replay.CursorPosition()
	assert.Equal(t, groundPos.X, replayPos.X, "cursor X must match")
	assert.Equal(t, groundPos.Y, replayPos.Y, "cursor Y must match")

	// Compare every viewport row.
	for y := 0; y < rows; y++ {
		assert.Equal(t, groundVP[y], replayVP[y],
			"viewport row %d mismatch: ground=%q replay=%q", y, groundVP[y], replayVP[y])
	}
}
