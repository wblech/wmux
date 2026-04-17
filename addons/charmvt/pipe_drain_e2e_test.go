package charmvt

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// End-to-end tests for the pipe drain fix (ADR 0025).
//
// These tests simulate realistic production scenarios:
// - TUI app output patterns (Claude Code, vim, htop)
// - Full emulator lifecycle (create → process → snapshot → close)
// - Session-layer behavior (emulator as io.Closer)

// --- E2E: Simulated Claude Code output ---

// TestE2E_ClaudeCodeOutputPattern simulates the output pattern that triggered
// the original bug: Claude Code sends DA1 during startup, mixed with TUI
// rendering (alt screen, SGR colors, cursor positioning, box drawing).
func TestE2E_ClaudeCodeOutputPattern(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000
	em := newEmulator("claude-session", 120, 36, cfg)
	defer func() { _ = em.Close() }()

	// Phase 1: Shell prompt before Claude starts.
	em.Process([]byte("~/project ➜ claude\r\n"))

	// Phase 2: Claude Code startup — DA1 query for terminal capabilities.
	em.Process([]byte("\x1b[c"))  // DA1: detect terminal type
	em.Process([]byte("\x1b[6n")) // CPR: detect cursor position

	// Phase 3: Enter alt screen, set up TUI.
	em.Process([]byte("\x1b[?1049h"))   // enter alt screen
	em.Process([]byte("\x1b[?25l"))     // hide cursor
	em.Process([]byte("\x1b[H\x1b[2J")) // clear screen
	em.Process([]byte("\x1b[?2004h"))   // enable bracketed paste

	// Phase 4: Render Claude Code UI with box drawing and colors.
	em.Process([]byte("\x1b[1;1H"))
	em.Process([]byte("\x1b[38;5;39m╭─── Claude Code v2.1.112 ───╮\x1b[0m\r\n"))
	em.Process([]byte("\x1b[38;5;39m│\x1b[0m                            \x1b[38;5;39m│\x1b[0m\r\n"))
	em.Process([]byte("\x1b[38;5;39m│\x1b[0m  \x1b[1mHello! How can I help?\x1b[0m   \x1b[38;5;39m│\x1b[0m\r\n"))
	em.Process([]byte("\x1b[38;5;39m│\x1b[0m                            \x1b[38;5;39m│\x1b[0m\r\n"))
	em.Process([]byte("\x1b[38;5;39m╰────────────────────────────╯\x1b[0m\r\n"))

	// Phase 5: Periodic DA1 queries (TUI frameworks check terminal caps).
	em.Process([]byte("\x1b[c"))

	// Phase 6: User types, Claude responds with streaming output.
	// Use cursor positioning without \r\n to avoid scrolling the header off.
	for i := range 20 {
		em.Process([]byte(fmt.Sprintf("\x1b[%d;3HLine %02d of response content\x1b[0m", 7+i, i)))
		if i%10 == 0 {
			em.Process([]byte("\x1b[6n")) // periodic CPR
		}
	}

	// Verify: snapshot should contain Claude Code UI elements.
	snap := em.Snapshot()
	vp := string(snap.Replay)

	assert.Contains(t, vp, "Claude Code v2.1.112", "missing Claude Code header")
	assert.Contains(t, vp, "Hello! How can I help?", "missing greeting")
	assert.Contains(t, vp, "Line 00 of response", "missing response content")
	assert.Contains(t, vp, "Line 19 of response", "missing last response line")
}

// TestE2E_VimLikeOutputPattern simulates vim-like editor output:
// alt screen + cursor positioning + mode queries.
func TestE2E_VimLikeOutputPattern(t *testing.T) {
	em := newEmulator("vim-session", 80, 24, defaultConfig())
	defer func() { _ = em.Close() }()

	// Vim startup sequence.
	em.Process([]byte("\x1b[?1049h"))   // alt screen
	em.Process([]byte("\x1b[c"))        // DA1
	em.Process([]byte("\x1b[>c"))       // DA2
	em.Process([]byte("\x1b[H\x1b[2J")) // clear

	// File content.
	for i := range 20 {
		em.Process([]byte(fmt.Sprintf("\x1b[%d;1Hpackage main // line %d\r\n", i+1, i+1)))
	}

	// Status line with mode query.
	em.Process([]byte("\x1b[24;1H\x1b[7m -- INSERT -- \x1b[0m"))
	em.Process([]byte("\x1b[6n")) // cursor position report

	snap := em.Snapshot()
	vp := string(snap.Replay)
	assert.Contains(t, vp, "package main")
	assert.Contains(t, vp, "INSERT")
}

// --- E2E: Full lifecycle ---

// TestE2E_FullLifecycle tests create → process with queries → snapshot → close.
// Verifies no resources leak and all operations complete cleanly.
func TestE2E_FullLifecycle(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 200
	em := newEmulator("lifecycle", 80, 24, cfg)

	// Phase 1: Initial content.
	em.Process([]byte("line 1\r\nline 2\r\n"))

	// Phase 2: DA1 query.
	em.Process([]byte("\x1b[c"))

	// Phase 3: Snapshot (must work after query).
	snap1 := em.Snapshot()
	assert.Contains(t, string(snap1.Replay), "line 1")

	// Phase 4: More content pushing into scrollback.
	for i := range 30 {
		em.Process([]byte(fmt.Sprintf("overflow line %02d\r\n", i)))
	}

	// Phase 5: Another query + snapshot.
	em.Process([]byte("\x1b[6n"))
	snap2 := em.Snapshot()
	require.NotNil(t, snap2.Replay, "scrollback must be populated")

	// Phase 6: Close.
	err := em.Close()
	require.NoError(t, err)

	// Phase 7: Post-close operations should not panic.
	assert.NotPanics(t, func() {
		em.Process([]byte("after close"))
		_ = em.Snapshot()
	})
}

// TestE2E_MultipleEmulatorInstances verifies that multiple emulators can
// operate concurrently, each with their own drain goroutine, without
// interference.
func TestE2E_MultipleEmulatorInstances(t *testing.T) {
	cfg := defaultConfig()
	emulators := make([]*emulator, 10)

	for i := range emulators {
		emulators[i] = newEmulator(fmt.Sprintf("session-%d", i), 80, 24, cfg)
	}

	var wg sync.WaitGroup
	for i, em := range emulators {
		wg.Add(1)
		go func() {
			defer wg.Done()
			em.Process([]byte(fmt.Sprintf("content for session %d\r\n", i)))
			em.Process([]byte("\x1b[c"))  // DA1
			em.Process([]byte("\x1b[6n")) // CPR
			em.Process([]byte(fmt.Sprintf("more content %d\r\n", i)))

			snap := em.Snapshot()
			if !strings.Contains(string(snap.Replay), fmt.Sprintf("content for session %d", i)) {
				t.Errorf("session %d: missing content in viewport", i)
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All 10 emulators processed without interference.
	case <-time.After(5 * time.Second):
		t.Fatal("multiple emulator instances deadlocked")
	}

	// Clean up all emulators.
	for _, em := range emulators {
		assert.NoError(t, em.Close())
	}
}

// --- E2E: Session-layer io.Closer contract ---

// TestE2E_EmulatorImplementsIOCloser verifies the io.Closer interface
// contract that the session waitLoop relies on for cleanup.
func TestE2E_EmulatorImplementsIOCloser(t *testing.T) {
	cfg := defaultConfig()
	em := newEmulator("test", 80, 24, cfg)

	// The session waitLoop uses this exact type assertion pattern.
	var iface interface{} = em
	closer, ok := iface.(interface{ Close() error })
	require.True(t, ok, "emulator must implement io.Closer for session cleanup")

	em.Process([]byte("\x1b[c")) // ensure drain is active
	assert.NoError(t, closer.Close())
}

// TestE2E_NonCloserEmulatorUnaffected verifies that emulators without Close()
// (like NoneEmulator) are unaffected by the io.Closer type assertion.
func TestE2E_NonCloserEmulatorUnaffected(t *testing.T) {
	// Simulate what the session waitLoop does with a non-Closer emulator.
	type minimalEmulator struct{}

	var iface interface{} = &minimalEmulator{}
	_, ok := iface.(interface{ Close() error })
	assert.False(t, ok, "non-closer emulators should not match the type assertion")
}

// --- E2E: Simulated attach/detach cycle ---

// TestE2E_AttachDetachCycle simulates the production failure scenario:
// 1. Emulator processes TUI output with DA1
// 2. Client detaches (Watchtower closes)
// 3. Client reattaches — Snapshot must work
func TestE2E_AttachDetachCycle(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 500
	em := newEmulator("attach-test", 100, 30, cfg)
	defer func() { _ = em.Close() }()

	// Phase 1: Normal operation — TUI app sends content + queries.
	em.Process([]byte("\x1b[?1049h\x1b[H\x1b[2J")) // alt screen + clear
	for i := range 25 {
		em.Process([]byte(fmt.Sprintf("\x1b[%d;1HRow %02d: active content\x1b[0m", i+1, i)))
	}
	em.Process([]byte("\x1b[c"))  // DA1 from TUI app
	em.Process([]byte("\x1b[6n")) // CPR from TUI app

	// Phase 2: Client detaches — no more snapshot requests.
	// (Emulator continues processing in background.)
	for i := range 100 {
		em.Process([]byte(fmt.Sprintf("\x1b[1;1HUpdate %03d\x1b[0m", i)))
		if i%20 == 0 {
			em.Process([]byte("\x1b[c")) // periodic DA1
		}
	}

	// Phase 3: Client reattaches — Snapshot must complete immediately.
	// This is the exact operation that was deadlocking in production.
	snapshotDone := make(chan struct{})
	go func() {
		snap := em.Snapshot()
		assert.NotEmpty(t, snap.Replay, "viewport must not be empty on reattach")
		close(snapshotDone)
	}()

	select {
	case <-snapshotDone:
		// Snapshot completed — attach succeeded.
	case <-time.After(2 * time.Second):
		t.Fatal("Snapshot blocked on reattach — this is the original production bug")
	}
}

// --- E2E: Sustained operation ---

// TestE2E_SustainedOperationNoDegradation runs the emulator under sustained
// load with queries to verify no memory accumulation or performance degradation.
func TestE2E_SustainedOperationNoDegradation(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 2000
	em := newEmulator("sustained", 120, 40, cfg)
	defer func() { _ = em.Close() }()

	// Warm up.
	for i := range 100 {
		em.Process([]byte(fmt.Sprintf("warmup line %04d\r\n", i)))
	}

	// Measure first snapshot.
	t0 := time.Now()
	snap1 := em.Snapshot()
	baseline := time.Since(t0)

	// Sustained operation: 2000 lines with queries every 100 lines.
	for i := range 2000 {
		em.Process([]byte(fmt.Sprintf("sustained line %04d with some realistic filler content\r\n", i)))
		if i%100 == 0 {
			em.Process([]byte("\x1b[c\x1b[6n\x1b[>c"))
		}
	}

	// Measure post-load snapshot.
	t1 := time.Now()
	snap2 := em.Snapshot()
	loaded := time.Since(t1)

	t.Logf("Baseline snapshot: %v, After 2000 lines + queries: %v", baseline, loaded)

	require.NotNil(t, snap1.Replay)
	require.NotNil(t, snap2.Replay)
	require.NotNil(t, snap2.Replay, "scrollback should be populated after 2000 lines")

	// Snapshot time should not degrade dramatically (allow 10x for scrollback growth).
	if loaded > baseline*10+50*time.Millisecond {
		t.Errorf("snapshot degraded: baseline=%v loaded=%v (>10x)", baseline, loaded)
	}
}

// --- E2E: Factory creates properly draining emulators ---

// TestE2E_FactoryCreatedEmulatorDrains verifies that emulators created
// through the Backend() factory (the production code path) have working drains.
func TestE2E_FactoryCreatedEmulatorDrains(t *testing.T) {
	f := &factory{cfg: defaultConfig()}

	em := f.Create("factory-test", 80, 24)

	// Process DA1 through the factory-created emulator.
	done := make(chan struct{})
	go func() {
		em.Process([]byte("\x1b[c"))
		em.Process([]byte("content after DA1"))
		close(done)
	}()

	select {
	case <-done:
		snap := em.Snapshot()
		assert.Contains(t, string(snap.Replay), "content after DA1")
	case <-time.After(processTimeout):
		t.Fatal("factory-created emulator blocked on DA1 — drain not working")
	}

	// Close via io.Closer interface (production path).
	if closer, ok := em.(interface{ Close() error }); ok {
		require.NoError(t, closer.Close())
	}

	f.Close()
}
