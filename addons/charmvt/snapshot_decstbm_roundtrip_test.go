package charmvt

import (
	"testing"

	"github.com/charmbracelet/x/vt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSnapshotRoundTrip_DECSTBM verifies that charmvt.Snapshot() preserves the
// active DECSTBM scroll region in the replay byte stream.
//
// Scenario: set scroll region rows 5..20 in a 83x41 terminal, write text
// inside the region, take Snapshot(), replay into a fresh vt.Emulator, and
// then append an LF-burst at row 20 (which must scroll inside the region).
// Before the fix the replay emulator lacks the DECSTBM sequence, so LFs
// scroll past the region boundary, producing cursor divergence.
func TestSnapshotRoundTrip_DECSTBM(t *testing.T) {
	const cols, rows = 83, 41

	// Setup writes: clear, set region rows 5..20, write inside region.
	writes := [][]byte{
		[]byte("\x1b[2J\x1b[H"),   // clear + home
		[]byte("\x1b[5;20r"),      // DECSTBM: scroll region rows 5..20
		[]byte("\x1b[10;1H"),      // cursor inside region (row 10)
		[]byte("inside-region\n"), // write a line inside region
	}

	// --- source emulator ---
	cfg := defaultConfig()
	src := newEmulator("src", cols, rows, cfg)
	for _, w := range writes {
		src.Process(w)
	}
	snap := src.Snapshot()

	// --- ground truth: raw vt.Emulator with the same writes ---
	ground := vt.NewEmulator(cols, rows)
	ground.SetED2SavesScrollback(false)
	defer func() { _ = ground.Close() }()
	for _, w := range writes {
		_, _ = ground.Write(w)
	}

	// --- replay emulator: fresh vt.Emulator fed only the snapshot ---
	replay := vt.NewEmulator(cols, rows)
	replay.SetED2SavesScrollback(false)
	defer func() { _ = replay.Close() }()
	_, _ = replay.Write(snap.Replay)

	// Assert scroll region is preserved in replay (basic).
	groundTop, groundBot, groundDefined := ground.ScrollRegion()
	replayTop, replayBot, replayDefined := replay.ScrollRegion()
	require.True(t, groundDefined, "ground truth must have an active scroll region")
	assert.True(t, replayDefined, "replay must have an active scroll region after Snapshot()")
	assert.Equal(t, groundTop, replayTop, "scroll region top must match")
	assert.Equal(t, groundBot, replayBot, "scroll region bottom must match")

	// Trailer: 5 LFs starting at row 20 — must stay inside region in both
	// emulators. Cursor divergence here is the original symptom.
	trailer := []byte("\x1b[20;1H\n\n\n\n\n")
	for _, w := range writes {
		_, _ = ground.Write(w)
	}
	// Re-create ground with trailer applied on top of original writes.
	groundT := vt.NewEmulator(cols, rows)
	groundT.SetED2SavesScrollback(false)
	defer func() { _ = groundT.Close() }()
	for _, w := range writes {
		_, _ = groundT.Write(w)
	}
	_, _ = groundT.Write(trailer)

	replayT := vt.NewEmulator(cols, rows)
	replayT.SetED2SavesScrollback(false)
	defer func() { _ = replayT.Close() }()
	_, _ = replayT.Write(snap.Replay)
	_, _ = replayT.Write(trailer)

	groundPos := groundT.CursorPosition()
	replayPos := replayT.CursorPosition()
	assert.Equal(t, groundPos.X, replayPos.X, "trailer cursor X must match")
	assert.Equal(t, groundPos.Y, replayPos.Y, "trailer cursor Y must match after scroll region preserved")
}
