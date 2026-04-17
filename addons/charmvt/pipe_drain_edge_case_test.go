package charmvt

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Edge case tests for the pipe drain fix (ADR 0025).
// These test boundary conditions and unusual scenarios.

// --- Close lifecycle ---

func TestEdgeCase_CloseStopsDrainGoroutine(t *testing.T) {
	em := newEmulator("test", 80, 24, defaultConfig())

	// Process a DA1 to verify drain is working.
	assertProcessCompletes(t, em, []byte("\x1b[c"), "pre-close DA1")

	// Close the emulator.
	err := em.Close()
	assert.NoError(t, err)

	// After Close, the drain goroutine should have exited (pipe closed).
	// Verify by checking that term.Close() was effective — subsequent
	// reads would fail, and the goroutine returns.
	// We can't directly observe the goroutine, but we verify no panic.
}

func TestEdgeCase_DoubleCloseNoPanic(t *testing.T) {
	em := newEmulator("test", 80, 24, defaultConfig())

	assert.NoError(t, em.Close())
	// Second close should not panic. vt.Emulator.Close() is idempotent.
	assert.NotPanics(t, func() {
		_ = em.Close()
	})
}

func TestEdgeCase_ProcessAfterClose(t *testing.T) {
	em := newEmulator("test", 80, 24, defaultConfig())
	em.Process([]byte("before close"))
	_ = em.Close()

	// Process after Close: vt.Write returns io.ErrClosedPipe.
	// This should not panic or deadlock.
	assert.NotPanics(t, func() {
		em.Process([]byte("after close"))
	})
}

func TestEdgeCase_SnapshotAfterClose(t *testing.T) {
	em := newEmulator("test", 80, 24, defaultConfig())
	em.Process([]byte("content before close"))
	_ = em.Close()

	// Snapshot after Close: Render() should still work on the existing
	// screen buffer. The pipe is closed but the screen data persists.
	assert.NotPanics(t, func() {
		snap := em.Snapshot()
		_ = snap.Viewport
	})
}

// --- DA1 in unusual positions ---

func TestEdgeCase_DA1AsFirstBytesEver(t *testing.T) {
	em := newEmulator("test", 80, 24, defaultConfig())
	defer em.Close()

	// DA1 as the very first data — no prior state.
	assertProcessCompletes(t, em, []byte("\x1b[c"), "first-bytes-DA1")
}

func TestEdgeCase_DA1AfterPartialEscapeSequence(t *testing.T) {
	em := newEmulator("test", 80, 24, defaultConfig())
	defer em.Close()

	// Send a partial escape sequence, then a DA1.
	// The parser should handle the state transition gracefully.
	em.Process([]byte("\x1b[38;5;")) // incomplete SGR
	assertProcessCompletes(t, em, []byte("\x1b[c"), "DA1-after-partial")
}

func TestEdgeCase_DA1SplitAcrossChunks(t *testing.T) {
	em := newEmulator("test", 80, 24, defaultConfig())
	defer em.Close()

	// DA1 (\x1b[c) split across two Process calls.
	// First chunk: ESC [   Second chunk: c
	em.Process([]byte("\x1b["))
	assertProcessCompletes(t, em, []byte("c"), "split-DA1")
}

func TestEdgeCase_DA1ByteByByte(t *testing.T) {
	em := newEmulator("test", 80, 24, defaultConfig())
	defer em.Close()

	// Each byte of DA1 sent individually.
	em.Process([]byte{0x1b})
	em.Process([]byte{'['})
	assertProcessCompletes(t, em, []byte{'c'}, "byte-by-byte-DA1")
}

// --- UTF-8 interaction ---

func TestEdgeCase_DA1AfterUTF8Content(t *testing.T) {
	em := newEmulator("test", 80, 24, defaultConfig())
	defer em.Close()

	// UTF-8 multi-byte characters followed by DA1.
	em.Process([]byte("こんにちは世界 🌍"))
	assertProcessCompletes(t, em, []byte("\x1b[c"), "DA1-after-utf8")

	snap := em.Snapshot()
	assert.Contains(t, string(snap.Viewport), "こんにちは世界")
}

func TestEdgeCase_DA1InterleavedWithUTF8(t *testing.T) {
	em := newEmulator("test", 80, 24, defaultConfig())
	defer em.Close()

	// Single chunk: UTF-8 + DA1 + more UTF-8.
	payload := []byte("日本語\x1b[cрусский")
	assertProcessCompletes(t, em, payload, "utf8-DA1-utf8")

	snap := em.Snapshot()
	vp := string(snap.Viewport)
	assert.Contains(t, vp, "日本語")
	assert.Contains(t, vp, "русский")
}

// --- Alt screen interaction ---

func TestEdgeCase_DA1InAltScreen(t *testing.T) {
	em := newEmulator("test", 80, 24, defaultConfig())
	defer em.Close()

	em.Process([]byte("main content"))
	em.Process([]byte("\x1b[?1049h")) // enter alt screen
	em.Process([]byte("alt content"))
	assertProcessCompletes(t, em, []byte("\x1b[c"), "DA1-in-alt-screen")

	snap := em.Snapshot()
	assert.Contains(t, string(snap.Viewport), "alt content")
	assert.NotContains(t, string(snap.Viewport), "main content")
}

func TestEdgeCase_DA1DuringAltScreenTransition(t *testing.T) {
	em := newEmulator("test", 80, 24, defaultConfig())
	defer em.Close()

	// Enter alt screen, DA1, exit alt screen — all in one chunk.
	payload := []byte("\x1b[?1049halt text\x1b[c\x1b[?1049l")
	assertProcessCompletes(t, em, payload, "DA1-during-alt-transition")
}

// --- Concurrent edge cases ---

func TestEdgeCase_ConcurrentCloseAndProcess(t *testing.T) {
	em := newEmulator("test", 80, 24, defaultConfig())

	var wg sync.WaitGroup

	// Goroutine 1: Process DA1 in a loop.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 100 {
			em.Process([]byte("\x1b[c"))
		}
	}()

	// Goroutine 2: Close after a short delay.
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(5 * time.Millisecond)
		_ = em.Close()
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// No deadlock, no panic.
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent Close + Process deadlocked")
	}
}

func TestEdgeCase_ConcurrentCloseAndSnapshot(t *testing.T) {
	em := newEmulator("test", 80, 24, defaultConfig())
	em.Process([]byte("content"))

	var wg sync.WaitGroup

	// Goroutine 1: Snapshot in a loop.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 100 {
			_ = em.Snapshot()
		}
	}()

	// Goroutine 2: Close after a short delay.
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(5 * time.Millisecond)
		_ = em.Close()
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// No deadlock, no panic.
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent Close + Snapshot deadlocked")
	}
}

// --- Rapid fire edge case ---

func TestEdgeCase_RapidAlternatingQueriesAndContent(t *testing.T) {
	em := newEmulator("test", 80, 24, defaultConfig())
	defer em.Close()

	// Alternate between query and content bytes rapidly.
	queries := [][]byte{
		{0x1b, '[', 'c'},       // DA1
		{0x1b, '[', '>', 'c'},  // DA2
		{0x1b, '[', '6', 'n'},  // CPR
		{0x1b, '[', '5', 'n'},  // Status
		{0x1b, '[', '?', '6', 'n'}, // DECXCPR
	}

	done := make(chan struct{})
	go func() {
		for i := range 200 {
			q := queries[i%len(queries)]
			em.Process(q)
			em.Process([]byte("x"))
		}
		close(done)
	}()

	select {
	case <-done:
		// All 200 query+content pairs processed.
	case <-time.After(5 * time.Second):
		t.Fatal("rapid alternating queries and content deadlocked")
	}
}

// --- Zero-size and boundary dimensions ---

func TestEdgeCase_SmallTerminalDA1(t *testing.T) {
	em := newEmulator("test", 1, 1, defaultConfig())
	defer em.Close()

	assertProcessCompletes(t, em, []byte("\x1b[c"), "1x1-terminal-DA1")
}

func TestEdgeCase_LargeTerminalDA1(t *testing.T) {
	em := newEmulator("test", 500, 200, defaultConfig())
	defer em.Close()

	assertProcessCompletes(t, em, []byte("\x1b[c"), "500x200-terminal-DA1")

	snap := em.Snapshot()
	assert.NotNil(t, snap.Viewport)
}
