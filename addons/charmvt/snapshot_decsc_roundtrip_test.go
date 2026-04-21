package charmvt

import (
	"testing"

	"github.com/charmbracelet/x/vt"
	"github.com/stretchr/testify/assert"
)

// TestSnapshotRoundTrip_DECSC verifies that charmvt.Snapshot() preserves the
// DECSC saved cursor in the replay byte stream.
//
// Scenario: write "A", then DECSC (ESC 7) to save cursor at col=1, row=0,
// then move the cursor away. Take Snapshot(), replay into a fresh vt.Emulator,
// then append DECRC (ESC 8) as a trailer.  After the trailer, the cursor must
// return to the saved position (col=1, row=0) in both the ground-truth
// emulator and the replay emulator.
//
// Before the fix, Snapshot() does not emit ESC 7, so the replay emulator has
// no saved cursor: DECRC leaves the cursor at (0,0) instead of (1,0).
func TestSnapshotRoundTrip_DECSC(t *testing.T) {
	const cols, rows = 83, 41

	writes := [][]byte{
		[]byte("\x1b[2J\x1b[H"), // clear + home
		[]byte("A"),             // write "A" — cursor is now at col=1, row=0
		[]byte("\x1b7"),         // DECSC: save cursor at (1, 0)
		[]byte("\r\n\r\nB"),     // move cursor down + right (col=1, row=2)
	}
	trailer := []byte("\x1b8") // DECRC: restore cursor to saved position

	// --- source emulator ---
	cfg := defaultConfig()
	src := newEmulator("src", cols, rows, cfg)
	for _, w := range writes {
		src.Process(w)
	}
	snap := src.Snapshot()

	// --- ground truth: raw vt.Emulator with the same writes + trailer ---
	ground := vt.NewEmulator(cols, rows)
	ground.SetED2SavesScrollback(false)
	defer func() { _ = ground.Close() }()
	for _, w := range writes {
		_, _ = ground.Write(w)
	}
	_, _ = ground.Write(trailer)
	groundPos := ground.CursorPosition()

	// --- replay emulator: fresh vt.Emulator fed snapshot + trailer ---
	replay := vt.NewEmulator(cols, rows)
	replay.SetED2SavesScrollback(false)
	defer func() { _ = replay.Close() }()
	_, _ = replay.Write(snap.Replay)
	_, _ = replay.Write(trailer)
	replayPos := replay.CursorPosition()

	assert.Equal(t, groundPos.X, replayPos.X, "cursor X after DECRC must match ground truth")
	assert.Equal(t, groundPos.Y, replayPos.Y, "cursor Y after DECRC must match ground truth")
}
