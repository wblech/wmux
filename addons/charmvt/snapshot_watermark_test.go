package charmvt

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==========================================================================
// Edge-case catalogue for ED2-aware scrollback discrimination ("watermark").
//
// Each test defines the CORRECT behavior. All tests that depend on ED2
// discrimination will be RED until the fix is implemented.
//
// Terminology:
//   - "natural scrollback" = lines that scrolled off the viewport because
//     new content pushed them up (shell output overflow).
//   - "ED2 junk" = lines pushed into scrollback by \e[2J (Erase Display 2),
//     which copies the entire viewport into scrollback before clearing it.
//     These lines are about to be redrawn, so serializing them creates
//     duplicate content in the Replay.
// ==========================================================================

// ---------------------------------------------------------------------------
// Helper: count occurrences of a substring in the Replay.
// ---------------------------------------------------------------------------
func countInReplay(snap []byte, substr string) int {
	return strings.Count(string(snap), substr)
}

// ---------------------------------------------------------------------------
// Helper: fill N lines into a small-viewport emulator to force natural
// scroll-off. Uses unique line markers for assertability.
// ---------------------------------------------------------------------------
func fillLines(em *emulator, prefix string, n int) {
	for i := range n {
		em.Process([]byte(fmt.Sprintf("%s-%03d\r\n", prefix, i)))
	}
}

func newSinceLastClearEmulator(id string, cols, rows int, scrollback int) *emulator {
	cfg := defaultConfig()
	cfg.scrollback = scrollback
	cfg.scrollbackMode = SnapshotScrollbackSinceLastClear
	return newEmulator(id, cols, rows, cfg)
}

// ---------------------------------------------------------------------------
// EC1: Pure shell — no ED2 ever. All scrollback is natural.
//
// Expected: every line that scrolled off is present in the Replay.
// This must PASS with or without the fix (baseline sanity).
// ---------------------------------------------------------------------------
func TestWatermark_EC1_PureShell_AllScrollbackPreserved(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000
	em := newEmulator("ec1", 80, 5, cfg)

	fillLines(em, "SHELL", 20)

	snap := em.Snapshot()
	replay := string(snap.Replay)

	// The earliest lines should be in scrollback (they scrolled off the 5-row viewport).
	assert.Contains(t, replay, "SHELL-000", "first line must be in scrollback")
	assert.Contains(t, replay, "SHELL-005", "middle line must be in scrollback")

	// The latest lines should be in the viewport.
	assert.Contains(t, replay, "SHELL-019", "last line must be in viewport")
}

// ---------------------------------------------------------------------------
// EC2: Pure TUI — only ED2 redraws, no prior shell scrollback.
//
// TUI draws its UI, then does ED2 + identical redraw (simulating SIGWINCH).
// The Replay must contain the UI exactly ONCE (from viewport), not twice.
// ---------------------------------------------------------------------------
func TestWatermark_EC2_PureTUI_NoDuplication(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000
	em := newEmulator("ec2", 80, 24, cfg)

	// Initial TUI paint.
	em.Process([]byte("\x1b[H"))
	em.Process([]byte("TUI-BANNER\r\n"))
	em.Process([]byte("TUI-STATUS\r\n"))

	// TUI redraws on SIGWINCH: ED2 + same content.
	em.Process([]byte("\x1b[2J\x1b[H"))
	em.Process([]byte("TUI-BANNER\r\n"))
	em.Process([]byte("TUI-STATUS\r\n"))

	snap := em.Snapshot()

	assert.Equal(t, 1, countInReplay(snap.Replay, "TUI-BANNER"),
		"TUI banner must appear exactly once after a redraw")
	assert.Equal(t, 1, countInReplay(snap.Replay, "TUI-STATUS"),
		"TUI status must appear exactly once after a redraw")
}

// ---------------------------------------------------------------------------
// EC3: Shell → TUI. Natural scrollback from shell, then TUI does ED2.
//
// SinceLastClear mode: pre-ED2 shell output excluded, TUI viewport preserved.
// ---------------------------------------------------------------------------
func TestWatermark_EC3_ShellThenTUI_StaleScrollbackExcluded(t *testing.T) {
	em := newSinceLastClearEmulator("ec3", 80, 5, 1000)

	fillLines(em, "CMD", 20)
	em.Process([]byte("\x1b[2J\x1b[H"))
	em.Process([]byte("APP-BANNER\r\n"))
	em.Process([]byte("APP-UI\r\n"))

	snap := em.Snapshot()
	replay := string(snap.Replay)

	assert.NotContains(t, replay, "CMD-000",
		"pre-TUI shell output must not appear in snapshot after ED2")
	assert.NotContains(t, replay, "CMD-010",
		"pre-TUI shell mid-range must not appear in snapshot after ED2")
	assert.Contains(t, replay, "APP-BANNER", "TUI banner must be in viewport")
	assert.Equal(t, 1, countInReplay(snap.Replay, "APP-BANNER"),
		"TUI banner must appear exactly once")
}

// ---------------------------------------------------------------------------
// EC4: TUI → shell. TUI runs first (with ED2 redraws), then user exits
// to shell and produces output that scrolls off.
//
// Expected: post-TUI shell scrollback IS preserved in the Replay.
// This is the case that breaks a simple counter-based watermark.
// ---------------------------------------------------------------------------
func TestWatermark_EC4_TUIThenShell_PostTUIScrollbackPreserved(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000
	em := newEmulator("ec4", 80, 5, cfg)

	// TUI phase: paint + redraw.
	em.Process([]byte("\x1b[H"))
	em.Process([]byte("TUI-HEADER\r\n"))
	em.Process([]byte("TUI-BODY\r\n"))
	em.Process([]byte("\x1b[2J\x1b[H"))
	em.Process([]byte("TUI-HEADER\r\n"))
	em.Process([]byte("TUI-BODY\r\n"))

	// User exits TUI, shell prompt appears.
	// Shell output scrolls lines off viewport.
	fillLines(em, "POST", 20)

	snap := em.Snapshot()
	replay := string(snap.Replay)

	// Post-TUI shell scrollback MUST be present.
	assert.Contains(t, replay, "POST-000",
		"post-TUI shell output must be in scrollback")
	assert.Contains(t, replay, "POST-010",
		"post-TUI shell mid-range output must be in scrollback")

	// TUI content may appear ONCE via natural scroll-off (POST lines pushed
	// TUI-HEADER off the viewport into scrollback). That's correct behavior.
	// The bug was DUPLICATION (2+ copies from ED2 push + viewport).
	assert.LessOrEqual(t, countInReplay(snap.Replay, "TUI-HEADER"), 1,
		"TUI content must not be duplicated — at most once from natural scroll-off")
}

// ---------------------------------------------------------------------------
// EC5: Shell → TUI → shell. The critical interleaving scenario.
//
// Scrollback layout:  [shell1-natural] [ED2-junk] [shell2-natural]
// Expected: both natural segments preserved, junk excluded.
// ---------------------------------------------------------------------------
func TestWatermark_EC5_ShellTUIShell_BothNaturalSegmentsPreserved(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000
	em := newEmulator("ec5", 80, 5, cfg)

	// Phase 1: shell output.
	fillLines(em, "BEFORE", 15)

	// Phase 2: TUI with ED2 redraws.
	em.Process([]byte("\x1b[2J\x1b[H"))
	em.Process([]byte("TUI-CONTENT-A\r\n"))
	em.Process([]byte("TUI-CONTENT-B\r\n"))

	// Second redraw.
	em.Process([]byte("\x1b[2J\x1b[H"))
	em.Process([]byte("TUI-CONTENT-A\r\n"))
	em.Process([]byte("TUI-CONTENT-B\r\n"))

	// Phase 3: user exits TUI, back to shell.
	fillLines(em, "AFTER", 15)

	snap := em.Snapshot()
	replay := string(snap.Replay)

	// Pre-TUI shell scrollback must be preserved.
	assert.Contains(t, replay, "BEFORE-000",
		"pre-TUI shell scrollback must survive")
	assert.Contains(t, replay, "BEFORE-010",
		"pre-TUI shell mid-range must survive")

	// Post-TUI shell scrollback must be preserved.
	assert.Contains(t, replay, "AFTER-000",
		"post-TUI shell scrollback must survive")
	assert.Contains(t, replay, "AFTER-010",
		"post-TUI shell mid-range must survive")

	// TUI content may appear ONCE from natural scroll-off (AFTER lines pushed
	// TUI content off the viewport). The bug was ED2-induced DUPLICATION.
	tuiScrollbackCount := countInReplay(snap.Replay, "TUI-CONTENT-A")
	assert.LessOrEqual(t, tuiScrollbackCount, 1,
		"TUI content must not be duplicated — at most once from natural scroll-off")
}

// ---------------------------------------------------------------------------
// EC6: Multiple consecutive ED2 redraws — the TUI redraws N times.
//
// Expected: no junk accumulates. Replay size stays stable.
// ---------------------------------------------------------------------------
func TestWatermark_EC6_MultipleED2Redraws_NoAccumulation(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000
	em := newEmulator("ec6", 80, 24, cfg)

	// Initial paint.
	em.Process([]byte("\x1b[H"))
	em.Process([]byte("STABLE-BANNER\r\n"))
	em.Process([]byte("STABLE-BODY\r\n"))

	baseSnap := em.Snapshot()
	baseSize := len(baseSnap.Replay)

	// 10 consecutive ED2 redraws.
	for range 10 {
		em.Process([]byte("\x1b[2J\x1b[H"))
		em.Process([]byte("STABLE-BANNER\r\n"))
		em.Process([]byte("STABLE-BODY\r\n"))
	}

	snap := em.Snapshot()

	assert.Equal(t, 1, countInReplay(snap.Replay, "STABLE-BANNER"),
		"banner must appear exactly once after 10 redraws")

	// Size should be roughly stable (within 20% of baseline).
	assert.InDelta(t, baseSize, len(snap.Replay), float64(baseSize)*0.2,
		"Replay size must not grow with repeated redraws (base=%d, after=%d)",
		baseSize, len(snap.Replay))
}

// ---------------------------------------------------------------------------
// EC7: ED2 + content in a single Process chunk.
//
// Some programs send ED2 and the redraw in the same write(2) call.
// The emulator receives them as a single Process() invocation.
// ---------------------------------------------------------------------------
func TestWatermark_EC7_MixedChunk_ED2AndContentInOneCall(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000
	em := newEmulator("ec7", 80, 24, cfg)

	// Initial content.
	em.Process([]byte("\x1b[H"))
	em.Process([]byte("INITIAL-PAINT\r\n"))

	// Single chunk: ED2 + cursor home + redraw.
	em.Process([]byte("\x1b[2J\x1b[HREDRAW-PAINT\r\n"))

	snap := em.Snapshot()

	assert.Equal(t, 1, countInReplay(snap.Replay, "REDRAW-PAINT"),
		"redrawn content must appear exactly once")
	assert.Equal(t, 0, countInReplay(snap.Replay, "INITIAL-PAINT"),
		"initial content pushed by ED2 must not appear in Replay")
}

// ---------------------------------------------------------------------------
// EC8: Shell `clear` command → ED2 → more shell output.
//
// SinceLastClear mode: pre-clear output excluded, post-clear output preserved.
// ---------------------------------------------------------------------------
func TestWatermark_EC8_ShellClear_PostClearOutputPreserved(t *testing.T) {
	em := newSinceLastClearEmulator("ec8", 80, 5, 1000)

	fillLines(em, "OLD", 10)
	em.Process([]byte("\x1b[2J\x1b[H"))
	fillLines(em, "NEW", 20)

	snap := em.Snapshot()
	replay := string(snap.Replay)

	assert.NotContains(t, replay, "OLD-000",
		"pre-clear shell scrollback must be excluded after ED2")
	assert.Contains(t, replay, "NEW-000",
		"post-clear shell output must be in scrollback")
	assert.Contains(t, replay, "NEW-010",
		"post-clear shell mid-range must be in scrollback")
}

// ---------------------------------------------------------------------------
// EC9: ED3 (\e[3J) explicitly clears scrollback.
//
// After ED3, all prior scrollback (natural or junk) is gone.
// New natural scroll-off after ED3 must be tracked fresh.
// ---------------------------------------------------------------------------
func TestWatermark_EC9_ED3_ClearsAllScrollback(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000
	em := newEmulator("ec9", 80, 5, cfg)

	// Build scrollback.
	fillLines(em, "BEFORE-ED3", 20)

	// ED3: clear scrollback.
	em.Process([]byte("\x1b[3J"))

	// New output after ED3.
	fillLines(em, "AFTER-ED3", 20)

	snap := em.Snapshot()
	replay := string(snap.Replay)

	// Old scrollback is gone.
	assert.NotContains(t, replay, "BEFORE-ED3",
		"pre-ED3 scrollback must be cleared")

	// New scrollback is tracked.
	assert.Contains(t, replay, "AFTER-ED3-000",
		"post-ED3 natural scroll-off must be preserved")
}

// ---------------------------------------------------------------------------
// EC10: Scrollback buffer rollover — buffer full, oldest lines dropped.
//
// With a small scrollback limit, old lines are evicted as new ones arrive.
// The watermark tracking must handle index shifts gracefully.
// ---------------------------------------------------------------------------
func TestWatermark_EC10_ScrollbackRollover(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 20 // Very small buffer.
	em := newEmulator("ec10", 80, 5, cfg)

	// Overflow the scrollback buffer.
	fillLines(em, "WAVE1", 30)

	// At this point, only ~20 lines survive in scrollback.
	// Earliest lines are evicted.
	snap1 := em.Snapshot()
	replay1 := string(snap1.Replay)
	assert.NotContains(t, replay1, "WAVE1-000",
		"evicted lines must not be in scrollback")
	assert.Contains(t, replay1, "WAVE1-025",
		"recent lines must survive in scrollback")

	// Now TUI does ED2 + redraw.
	em.Process([]byte("\x1b[2J\x1b[H"))
	em.Process([]byte("TUI-RO\r\n"))

	// More shell output to push lines.
	fillLines(em, "WAVE2", 15)

	snap2 := em.Snapshot()
	replay2 := string(snap2.Replay)

	// TUI content may appear once from natural scroll-off (WAVE2 pushed it).
	assert.LessOrEqual(t, countInReplay(snap2.Replay, "TUI-RO"), 1,
		"TUI content must not be duplicated after rollover")

	// Recent natural lines must be present.
	assert.Contains(t, replay2, "WAVE2-010",
		"post-ED2 natural lines must survive rollover")
}

// ---------------------------------------------------------------------------
// EC11: Empty scrollback — fresh session, nothing scrolled off.
//
// Snapshot should just be clear + viewport + CUP. No scrollback section.
// Must PASS with or without the fix (baseline sanity).
// ---------------------------------------------------------------------------
func TestWatermark_EC11_EmptyScrollback(t *testing.T) {
	cfg := defaultConfig()
	em := newEmulator("ec11", 80, 24, cfg)

	em.Process([]byte("just viewport"))

	snap := em.Snapshot()
	replay := string(snap.Replay)

	assert.Contains(t, replay, "just viewport")
	assert.Equal(t, 1, countInReplay(snap.Replay, "just viewport"),
		"content in viewport only must appear exactly once")
}

// ---------------------------------------------------------------------------
// EC12: Snapshot idempotency — multiple snapshots without Process calls.
//
// Taking a snapshot must NOT alter the emulator state.
// ---------------------------------------------------------------------------
func TestWatermark_EC12_SnapshotIdempotency(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000
	em := newEmulator("ec12", 80, 5, cfg)

	fillLines(em, "DATA", 20)

	snap1 := em.Snapshot()
	snap2 := em.Snapshot()
	snap3 := em.Snapshot()

	assert.Equal(t, snap1.Replay, snap2.Replay, "consecutive snapshots must be identical")
	assert.Equal(t, snap2.Replay, snap3.Replay, "consecutive snapshots must be identical")
}

// ---------------------------------------------------------------------------
// EC13: Snapshot idempotency AFTER ED2 — snapshots between redraws.
//
// After a TUI redraw, taking multiple snapshots must not accumulate junk.
// ---------------------------------------------------------------------------
func TestWatermark_EC13_SnapshotIdempotencyAfterED2(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000
	em := newEmulator("ec13", 80, 24, cfg)

	em.Process([]byte("\x1b[HBANNER\r\n"))
	em.Process([]byte("\x1b[2J\x1b[H"))
	em.Process([]byte("BANNER\r\n"))

	snap1 := em.Snapshot()
	snap2 := em.Snapshot()

	assert.Equal(t, snap1.Replay, snap2.Replay, "snapshots after ED2 must be stable")
	assert.Equal(t, 1, countInReplay(snap1.Replay, "BANNER"),
		"banner must appear exactly once")
}

// ---------------------------------------------------------------------------
// EC14: Resize between snapshots — SIGWINCH triggers Resize(), which may
// cause a TUI to redraw with ED2.
//
// SinceLastClear mode: pre-resize+ED2 scrollback excluded, TUI UI preserved.
// ---------------------------------------------------------------------------
func TestWatermark_EC14_ResizeBetweenSnapshots(t *testing.T) {
	em := newSinceLastClearEmulator("ec14", 80, 5, 1000)

	fillLines(em, "PRE-RESIZE", 15)
	em.Resize(120, 10)
	em.Process([]byte("\x1b[2J\x1b[H"))
	em.Process([]byte("RESIZED-UI\r\n"))

	snap := em.Snapshot()
	replay := string(snap.Replay)

	assert.NotContains(t, replay, "PRE-RESIZE-000",
		"shell scrollback from before resize+ED2 must be excluded")
	assert.Equal(t, 1, countInReplay(snap.Replay, "RESIZED-UI"),
		"resized TUI content must appear exactly once")
}

// ---------------------------------------------------------------------------
// EC15: Rapid alternation — shell → TUI → shell → TUI → shell.
//
// Multiple transitions. Each natural segment must be preserved,
// each ED2 junk segment excluded.
// ---------------------------------------------------------------------------
func TestWatermark_EC15_RapidAlternation(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 5000
	em := newEmulator("ec15", 80, 5, cfg)

	// Round 1: shell.
	fillLines(em, "S1", 10)

	// Round 1: TUI.
	em.Process([]byte("\x1b[2J\x1b[H"))
	em.Process([]byte("TUI1-BANNER\r\n"))

	// Round 2: shell.
	fillLines(em, "S2", 10)

	// Round 2: TUI.
	em.Process([]byte("\x1b[2J\x1b[H"))
	em.Process([]byte("TUI2-BANNER\r\n"))

	// Round 3: shell.
	fillLines(em, "S3", 10)

	snap := em.Snapshot()
	replay := string(snap.Replay)

	// All natural shell segments preserved.
	assert.Contains(t, replay, "S1-000", "shell round 1 must survive")
	assert.Contains(t, replay, "S2-000", "shell round 2 must survive")
	assert.Contains(t, replay, "S3-000", "shell round 3 must survive")

	// TUI content may appear once per round from natural scroll-off
	// (shell lines pushed TUI off the viewport). No duplication.
	assert.LessOrEqual(t, countInReplay(snap.Replay, "TUI1-BANNER"), 1,
		"TUI round 1 must not be duplicated")
	assert.LessOrEqual(t, countInReplay(snap.Replay, "TUI2-BANNER"), 1,
		"TUI round 2 must not be duplicated")
}

// ---------------------------------------------------------------------------
// EC16: Natural scroll-off happening DURING a TUI session (between ED2s).
//
// Some TUIs produce enough output to cause scroll-off between redraws.
// For example: a TUI prints 30 lines on a 5-row viewport, then does ED2.
// The scroll-off from the 30 lines is natural; the ED2 push is junk.
// ---------------------------------------------------------------------------
func TestWatermark_EC16_NaturalScrollOffBetweenED2s(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000
	em := newEmulator("ec16", 80, 5, cfg)

	// TUI initial paint.
	em.Process([]byte("\x1b[2J\x1b[H"))
	em.Process([]byte("TUI-V1\r\n"))

	// TUI outputs a long log/result that causes natural scroll-off.
	fillLines(em, "LOG", 20)

	// TUI redraws (new ED2).
	em.Process([]byte("\x1b[2J\x1b[H"))
	em.Process([]byte("TUI-V2\r\n"))

	snap := em.Snapshot()
	replay := string(snap.Replay)

	// The log lines that naturally scrolled off should be preserved.
	assert.Contains(t, replay, "LOG-000",
		"natural scroll-off between ED2s must be preserved")
	assert.Contains(t, replay, "LOG-015",
		"natural scroll-off mid-range must be preserved")

	// Current TUI version in viewport.
	assert.Contains(t, replay, "TUI-V2", "current TUI state must be in viewport")

	// Old TUI version may appear once from natural scroll-off (LOG lines
	// pushed it off the viewport). The bug was ED2-induced duplication.
	assert.LessOrEqual(t, countInReplay(snap.Replay, "TUI-V1"), 1,
		"old TUI version must not be duplicated")
}

// ---------------------------------------------------------------------------
// EC17: Alt-screen programs are unaffected.
//
// Programs using the alternate screen (\e[?1049h) don't interact with
// main-screen scrollback. ED2 inside alt-screen should not affect
// main-screen watermark tracking.
// ---------------------------------------------------------------------------
func TestWatermark_EC17_AltScreenUnaffected(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000
	em := newEmulator("ec17", 80, 5, cfg)

	// Shell work.
	fillLines(em, "MAIN", 15)

	// Enter alt-screen (e.g., vim).
	em.Process([]byte("\x1b[?1049h"))
	em.Process([]byte("\x1b[2J\x1b[H")) // ED2 on alt-screen.
	em.Process([]byte("VIM-CONTENT\r\n"))

	// Exit alt-screen.
	em.Process([]byte("\x1b[?1049l"))

	snap := em.Snapshot()
	replay := string(snap.Replay)

	// Main-screen scrollback must be intact.
	assert.Contains(t, replay, "MAIN-000",
		"main-screen scrollback must survive alt-screen ED2")
	assert.Contains(t, replay, "MAIN-010",
		"main-screen scrollback mid-range must survive")

	// Alt-screen content should NOT appear (it's gone after exit).
	assert.NotContains(t, replay, "VIM-CONTENT",
		"alt-screen content must not leak into main-screen Replay")
}

// ---------------------------------------------------------------------------
// EC18: Replay size stability — many redraws must not inflate the Replay.
//
// This is the original bug signal: Replay nearly doubled (1934→3670)
// between consecutive reconnects.
// ---------------------------------------------------------------------------
func TestWatermark_EC18_ReplaySizeStability(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000
	em := newEmulator("ec18", 80, 24, cfg)

	// Fill viewport with rich content (simulates Claude Code UI).
	em.Process([]byte("\x1b[H"))
	for i := range 20 {
		em.Process([]byte(fmt.Sprintf("\x1b[38;2;%d;119;87mUI-ROW-%02d some content here\x1b[0m\r\n", 100+i*5, i)))
	}

	baseSnap := em.Snapshot()
	baseSize := len(baseSnap.Replay)

	// Simulate 5 SIGWINCH-triggered redraws (each: ED2 + same content).
	for range 5 {
		em.Process([]byte("\x1b[2J\x1b[H"))
		for i := range 20 {
			em.Process([]byte(fmt.Sprintf("\x1b[38;2;%d;119;87mUI-ROW-%02d some content here\x1b[0m\r\n", 100+i*5, i)))
		}
	}

	snap := em.Snapshot()

	// Size must stay within 20% of baseline.
	assert.InDelta(t, baseSize, len(snap.Replay), float64(baseSize)*0.2,
		"Replay must not inflate after 5 redraws (base=%d, after=%d)",
		baseSize, len(snap.Replay))
}

// ---------------------------------------------------------------------------
// EC19: Concurrent snapshots and process calls.
//
// Watermark tracking must be safe under concurrent access (the emulator
// already uses a mutex — this test validates it holds under load).
// ---------------------------------------------------------------------------
func TestWatermark_EC19_ConcurrentSafety(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000
	em := newEmulator("ec19", 80, 5, cfg)

	done := make(chan struct{})

	// Writer goroutine: alternates between shell output and ED2 redraws.
	go func() {
		defer close(done)
		for i := range 100 {
			if i%10 == 0 {
				em.Process([]byte("\x1b[2J\x1b[H"))
				em.Process([]byte("TUI-CYCLE\r\n"))
			} else {
				em.Process([]byte(fmt.Sprintf("line-%04d\r\n", i)))
			}
		}
	}()

	// Reader goroutine: takes snapshots concurrently.
	snapshots := make([]int, 0, 50)
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				snap := em.Snapshot()
				snapshots = append(snapshots, len(snap.Replay))
			}
		}
	}()

	<-done

	// Basic sanity: no panics, all snapshots non-empty.
	for i, size := range snapshots {
		require.Greater(t, size, 0, "snapshot %d must be non-empty", i)
	}
}

// ---------------------------------------------------------------------------
// EC20: ED2 at the very start of a session (before any content).
//
// Some programs send ED2 as their first action. With no content in the
// viewport, the push should be harmless (empty lines).
// ---------------------------------------------------------------------------
func TestWatermark_EC20_ED2AsFirstAction(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000
	em := newEmulator("ec20", 80, 24, cfg)

	// ED2 before any content.
	em.Process([]byte("\x1b[2J\x1b[H"))

	// Now draw content.
	em.Process([]byte("FIRST-CONTENT\r\n"))

	snap := em.Snapshot()

	assert.Equal(t, 1, countInReplay(snap.Replay, "FIRST-CONTENT"),
		"content after initial ED2 must appear exactly once")
}

// ---------------------------------------------------------------------------
// EC21: Partial ED2 sequence split across Process calls.
//
// PTY writes can split escape sequences across chunks. E.g., the bytes
// for \x1b[2J might arrive as [\x1b[2] then [J] in separate Process calls.
// The vt.Emulator handles sequence reassembly, but our ED2 detection
// in the data bytes must account for this.
// ---------------------------------------------------------------------------
func TestWatermark_EC21_SplitED2Sequence(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000
	em := newEmulator("ec21", 80, 24, cfg)

	// Initial content.
	em.Process([]byte("\x1b[H"))
	em.Process([]byte("BEFORE-SPLIT\r\n"))

	// ED2 arrives split across two Process calls.
	em.Process([]byte("\x1b[2"))
	em.Process([]byte("J"))

	// Redraw.
	em.Process([]byte("\x1b[H"))
	em.Process([]byte("AFTER-SPLIT\r\n"))

	snap := em.Snapshot()

	// This is a known hard edge case. The fix may or may not handle it
	// perfectly — document the actual behavior.
	// At minimum, AFTER-SPLIT must appear.
	assert.Contains(t, string(snap.Replay), "AFTER-SPLIT",
		"content after split ED2 must be present")

	// Ideally BEFORE-SPLIT should not be duplicated, but this depends
	// on whether the implementation can detect split sequences.
	// We mark the ideal behavior:
	beforeCount := countInReplay(snap.Replay, "BEFORE-SPLIT")
	t.Logf("BEFORE-SPLIT count: %d (ideal: 0, acceptable: 0-1)", beforeCount)
}

// ---------------------------------------------------------------------------
// EC22: ED2 with parameter — \e[2J specifically. Not \e[0J, \e[1J, or \e[J.
//
// Only ED mode 2 pushes viewport to scrollback. Other ED modes must not
// trigger watermark logic.
// ---------------------------------------------------------------------------
func TestWatermark_EC22_OnlyED2TriggersWatermark(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000
	em := newEmulator("ec22", 80, 5, cfg)

	// Fill viewport + scrollback with natural content.
	fillLines(em, "NATURAL", 20)

	// ED mode 0 (clear from cursor to end of screen) — should NOT trigger watermark.
	em.Process([]byte("\x1b[0J"))

	// More natural output.
	fillLines(em, "EXTRA", 10)

	snap := em.Snapshot()
	replay := string(snap.Replay)

	// All natural content should be preserved (ED0 doesn't push to scrollback).
	assert.Contains(t, replay, "NATURAL-000", "ED0 must not affect scrollback tracking")
	assert.Contains(t, replay, "EXTRA-000", "content after ED0 must be preserved")
}

// ---------------------------------------------------------------------------
// EC23: All mode (default) — pre-TUI scrollback preserved even after ED2.
// ---------------------------------------------------------------------------
func TestWatermark_EC23_AllMode_PreservesEverything(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000
	em := newEmulator("ec23", 80, 5, cfg)

	fillLines(em, "SHELL", 20)
	em.Process([]byte("\x1b[2J\x1b[H"))
	em.Process([]byte("TUI-BANNER\r\n"))

	snap := em.Snapshot()
	replay := string(snap.Replay)

	assert.Contains(t, replay, "SHELL-000",
		"All mode must preserve pre-TUI scrollback")
	assert.Contains(t, replay, "SHELL-010",
		"All mode must preserve pre-TUI scrollback mid-range")
	assert.Contains(t, replay, "TUI-BANNER",
		"All mode must include TUI viewport")
}

// ---------------------------------------------------------------------------
// EC24: SinceLastClear mode — ED2 on alt-screen must NOT update the baseline.
// ---------------------------------------------------------------------------
func TestWatermark_EC24_SinceLastClear_AltScreenIgnored(t *testing.T) {
	em := newSinceLastClearEmulator("ec24", 80, 5, 1000)

	fillLines(em, "MAIN", 15)

	em.Process([]byte("\x1b[?1049h"))
	em.Process([]byte("\x1b[2J\x1b[H"))
	em.Process([]byte("VIM\r\n"))
	em.Process([]byte("\x1b[?1049l"))

	snap := em.Snapshot()
	replay := string(snap.Replay)

	assert.Contains(t, replay, "MAIN-000",
		"main-screen scrollback must survive alt-screen ED2 in SinceLastClear mode")
	assert.Contains(t, replay, "MAIN-010",
		"main-screen scrollback mid-range must survive")
}

// ---------------------------------------------------------------------------
// EC5_SinceLastClear_ShellTUIShell: SinceLastClear variant with ED2 redraws.
//
// Pre-TUI shell scrollback excluded (last ED2 baseline).
// Post-TUI shell scrollback preserved (after baseline).
// ---------------------------------------------------------------------------
func TestWatermark_EC5_SinceLastClear_ShellTUIShell(t *testing.T) {
	em := newSinceLastClearEmulator("ec5-slc", 80, 5, 1000)

	fillLines(em, "BEFORE", 15)

	em.Process([]byte("\x1b[2J\x1b[H"))
	em.Process([]byte("TUI-CONTENT-A\r\n"))
	em.Process([]byte("TUI-CONTENT-B\r\n"))

	em.Process([]byte("\x1b[2J\x1b[H"))
	em.Process([]byte("TUI-CONTENT-A\r\n"))
	em.Process([]byte("TUI-CONTENT-B\r\n"))

	fillLines(em, "AFTER", 15)

	snap := em.Snapshot()
	replay := string(snap.Replay)

	// Pre-TUI shell scrollback excluded (last ED2 baseline).
	assert.NotContains(t, replay, "BEFORE-000",
		"pre-TUI shell scrollback excluded in SinceLastClear")

	// Post-TUI shell scrollback preserved (after baseline).
	assert.Contains(t, replay, "AFTER-000",
		"post-TUI shell scrollback must survive")
	assert.Contains(t, replay, "AFTER-010",
		"post-TUI shell mid-range must survive")
}

// ---------------------------------------------------------------------------
// EC16_SinceLastClear_ScrollOffBetweenED2s: SinceLastClear with scroll-off.
//
// Natural scroll-off between ED2 redraws is preserved.
// Lines scrolled off before last ED2 are excluded.
// ---------------------------------------------------------------------------
func TestWatermark_EC16_SinceLastClear_ScrollOffBetweenED2s(t *testing.T) {
	em := newSinceLastClearEmulator("ec16-slc", 80, 5, 1000)

	em.Process([]byte("\x1b[2J\x1b[H"))
	em.Process([]byte("TUI-V1\r\n"))

	// TUI outputs long result causing natural scroll-off.
	fillLines(em, "LOG", 20)

	// TUI redraws (new ED2). Baseline moves to current scrollbackLen.
	em.Process([]byte("\x1b[2J\x1b[H"))
	em.Process([]byte("TUI-V2\r\n"))

	snap := em.Snapshot()
	replay := string(snap.Replay)

	// LOG lines are before the last ED2 baseline — excluded.
	assert.NotContains(t, replay, "LOG-000",
		"scroll-off before last ED2 excluded in SinceLastClear")

	// Current TUI viewport present.
	assert.Contains(t, replay, "TUI-V2",
		"current TUI state must be in viewport")
}
