package charmvt

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Regression tests for: "Attach blocks indefinitely when session runs a TUI app"
//
// These tests verify that Snapshot() completes promptly even under continuous
// Process() calls. Prior to the fix, renderScrollback acquired the SafeEmulator
// mutex per cell (up to 1M times), causing 394x slowdown under contention.
// The fix uses a single operation-level mutex instead.

// TestRegression_SnapshotUnderContinuousWrites verifies that Snapshot()
// completes promptly even when Process() runs concurrently at high
// throughput. Regression test for per-cell mutex starvation fix.
func TestRegression_SnapshotUnderContinuousWrites(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 5000

	em := newEmulator("contention-regression", 200, 24, cfg)

	var setup strings.Builder
	for i := range 5200 {
		setup.WriteString(fmt.Sprintf("scrollback line %04d with some content for realism\r\n", i))
	}
	em.Process([]byte(setup.String()))

	baseStart := time.Now()
	baseline := em.Snapshot()
	baseElapsed := time.Since(baseStart)
	if baseline.Scrollback == nil {
		t.Fatal("precondition failed: scrollback must be populated")
	}
	t.Logf("Baseline Snapshot (no contention): %v", baseElapsed)

	stop := make(chan struct{})
	writes := atomic.Int64{}
	go func() {
		chunk := []byte("\033[1;1H\033[38;5;82mstatus line content\033[0m\033[2;1Hmore output here\r\n")
		for {
			select {
			case <-stop:
				return
			default:
				em.Process(chunk)
				writes.Add(1)
			}
		}
	}()

	done := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		_ = em.Snapshot()
		done <- time.Since(start)
	}()

	select {
	case elapsed := <-done:
		close(stop)
		t.Logf("Snapshot under contention: %v (writes during test: %d)", elapsed, writes.Load())

		// With operation-level mutex, Snapshot waits at most for one
		// Process() call to finish, then runs uninterrupted. Should
		// complete well under 1 second even with 5000×200 scrollback.
		if elapsed > 1*time.Second {
			t.Errorf("Snapshot too slow under contention: %v — possible regression "+
				"to per-cell locking", elapsed)
		}

	case <-time.After(5 * time.Second):
		close(stop)
		t.Fatalf("Snapshot did not complete within 5s — regression: "+
			"mutex starvation (writes: %d)", writes.Load())
	}
}

// TestRegression_SnapshotBaselineWithoutContention verifies Snapshot is fast
// without concurrent writers (sanity check).
func TestRegression_SnapshotBaselineWithoutContention(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000
	em := newEmulator("baseline-regression", 120, 24, cfg)

	var setup strings.Builder
	for i := range 600 {
		setup.WriteString(fmt.Sprintf("scrollback line %04d\r\n", i))
	}
	em.Process([]byte(setup.String()))

	start := time.Now()
	snap := em.Snapshot()
	elapsed := time.Since(start)

	if snap.Scrollback == nil {
		t.Fatal("scrollback must not be nil")
	}

	t.Logf("Snapshot without contention: %v", elapsed)

	if elapsed > 200*time.Millisecond {
		t.Errorf("Snapshot too slow even without contention: %v", elapsed)
	}
}
