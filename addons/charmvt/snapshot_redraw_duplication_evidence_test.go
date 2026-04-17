package charmvt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEvidence_RedrawDuplicatesInSnapshot proves the root cause of the
// "2 banners visible" bug in watchtower.
//
// Scenario (mirrors Claude Code on workspace switch):
//  1. A TUI draws its UI (no alt-screen) — cells fill the visible viewport.
//  2. A SIGWINCH / resize later, the TUI redraws itself with \e[2J\e[H + UI.
//
// The upstream vt.Emulator handles \e[2J by PUSHING the current viewport
// cells into scrollback (xterm-style preservation). This is correct terminal
// semantics, documented in the upstream test "ED 2 saves to scrollback".
//
// Our Snapshot emits:  clear-prefix + scrollback + viewport + cursor.
// So after the redraw, Snapshot contains:
//   - scrollback lines = the OLD TUI rendering (just pushed by ED2)
//   - viewport        = the NEW TUI rendering (current)
//
// -> The TUI's UI is serialized TWICE in one Replay stream.
//
// Writing that Replay into a destination terminal reproduces the
// "duplicated banner" the user reported.
func TestEvidence_RedrawDuplicatesInSnapshot(t *testing.T) {
	cfg := defaultConfig()
	em := newEmulator("evidence-redraw", 80, 24, cfg)

	// 1. Initial TUI render: banner + some UI rows.
	em.Process([]byte("\x1b[H"))
	em.Process([]byte("BANNER ROW 1\r\n"))
	em.Process([]byte("BANNER ROW 2\r\n"))
	em.Process([]byte("UI-LINE\r\n"))

	// 2. Snapshot #1 — baseline, should contain banner/UI exactly once.
	snap1 := em.Snapshot()
	count1 := strings.Count(string(snap1.Replay), "BANNER ROW 1")
	require.Equal(t, 1, count1, "baseline snapshot must contain the banner exactly once")

	// 3. TUI redraws itself on SIGWINCH: \e[2J\e[H + same banner/UI.
	em.Process([]byte("\x1b[2J\x1b[H"))
	em.Process([]byte("BANNER ROW 1\r\n"))
	em.Process([]byte("BANNER ROW 2\r\n"))
	em.Process([]byte("UI-LINE\r\n"))

	// 4. Snapshot #2 — this is what a workspace-switch reconnect sees.
	//    The upstream vt.Emulator saved the pre-ED2 viewport to scrollback
	//    during step 3, so Snapshot's scrollback now contains the OLD
	//    rendering and the viewport contains the NEW rendering.
	snap2 := em.Snapshot()
	count2 := strings.Count(string(snap2.Replay), "BANNER ROW 1")

	// EVIDENCE: the banner appears TWICE in a single Replay stream.
	// A correct Replay should show it exactly once (the current state).
	assert.Equal(t, 1, count2,
		"after a TUI redraw the Snapshot Replay must contain the banner exactly once; "+
			"found %d occurrences — the \\e[2J push-to-scrollback behavior "+
			"of the underlying vt.Emulator is being serialized additively, "+
			"which is the root cause of the watchtower duplicated-banner bug",
		count2)
}

// TestEvidence_ReplaySizeGrowsAfterRedraw — secondary signal: the Replay
// stream roughly doubles after a redraw, matching the 1934 → 3670 byte
// jump observed in the watchtower debug log.
func TestEvidence_ReplaySizeGrowsAfterRedraw(t *testing.T) {
	cfg := defaultConfig()
	em := newEmulator("evidence-growth", 80, 24, cfg)

	em.Process([]byte("\x1b[H"))
	for i := 0; i < 10; i++ {
		em.Process([]byte("content line with ansi \x1b[38;2;215;119;87mcolor\x1b[0m text\r\n"))
	}

	before := len(em.Snapshot().Replay)

	// TUI redraws the same content.
	em.Process([]byte("\x1b[2J\x1b[H"))
	for i := 0; i < 10; i++ {
		em.Process([]byte("content line with ansi \x1b[38;2;215;119;87mcolor\x1b[0m text\r\n"))
	}

	after := len(em.Snapshot().Replay)

	// EVIDENCE: the Replay should stay roughly the same size (same viewport
	// content), not grow. Growth proves the old viewport was serialized AS
	// scrollback in addition to the new viewport.
	assert.InDelta(t, before, after, float64(before)*0.2,
		"Replay size nearly doubled after an identical redraw "+
			"(before=%d after=%d) — evidence that scrollback accumulated "+
			"content from the ED2 push",
		before, after)
}
