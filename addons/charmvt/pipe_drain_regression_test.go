package charmvt

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression tests for the pipe drain fix (ADR 0025).
//
// Root cause: vt.Emulator writes DA1/DA2/DSR/CPR responses to an internal
// io.Pipe. Without a reader, Write() deadlocks. The drain goroutine in
// newEmulator() prevents this by reading and discarding responses.
//
// These tests verify the fix holds. They will FAIL if the drain goroutine
// is removed or broken.

// processTimeout is the maximum time Process() should take for any input.
// The drain goroutine makes this near-instant; 100ms is generous.
const processTimeout = 100 * time.Millisecond

// --- Regression: Process() completes for all response-triggering sequences ---

func TestRegression_ProcessDA1Completes(t *testing.T) {
	em := newEmulator("test", 80, 24, defaultConfig())
	defer func() { _ = em.Close() }()

	assertProcessCompletes(t, em, []byte("\x1b[c"), "DA1")
}

func TestRegression_ProcessDA2Completes(t *testing.T) {
	em := newEmulator("test", 80, 24, defaultConfig())
	defer func() { _ = em.Close() }()

	assertProcessCompletes(t, em, []byte("\x1b[>c"), "DA2")
}

func TestRegression_ProcessDSR_CursorPositionCompletes(t *testing.T) {
	em := newEmulator("test", 80, 24, defaultConfig())
	defer func() { _ = em.Close() }()

	assertProcessCompletes(t, em, []byte("\x1b[6n"), "DSR CPR")
}

func TestRegression_ProcessDSR_OperatingStatusCompletes(t *testing.T) {
	em := newEmulator("test", 80, 24, defaultConfig())
	defer func() { _ = em.Close() }()

	assertProcessCompletes(t, em, []byte("\x1b[5n"), "DSR Status")
}

func TestRegression_ProcessDECXCPRCompletes(t *testing.T) {
	em := newEmulator("test", 80, 24, defaultConfig())
	defer func() { _ = em.Close() }()

	assertProcessCompletes(t, em, []byte("\x1b[?6n"), "DECXCPR")
}

// --- Regression: Snapshot works after processing queries ---

func TestRegression_SnapshotAfterDA1(t *testing.T) {
	em := newEmulator("test", 80, 24, defaultConfig())
	defer func() { _ = em.Close() }()

	em.Process([]byte("before query"))
	em.Process([]byte("\x1b[c"))
	em.Process([]byte(" after query"))

	snap := em.Snapshot()
	vp := string(snap.Replay)
	assert.Contains(t, vp, "before query")
	assert.Contains(t, vp, "after query")
}

func TestRegression_SnapshotDuringConcurrentDA1(t *testing.T) {
	em := newEmulator("test", 80, 24, defaultConfig())
	defer func() { _ = em.Close() }()

	em.Process([]byte("initial content\r\n"))

	var wg sync.WaitGroup

	// Goroutine 1: rapid Process with DA1 queries.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 50 {
			em.Process([]byte("\x1b[c"))
			em.Process([]byte("line\r\n"))
		}
	}()

	// Goroutine 2: rapid Snapshot calls.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 50 {
			snap := em.Snapshot()
			_ = snap.Replay // must not panic
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Both completed without deadlock.
	case <-time.After(5 * time.Second):
		t.Fatal("deadlock: concurrent Process(DA1) + Snapshot did not complete in 5s")
	}
}

// --- Regression: Mixed content with queries ---

func TestRegression_MixedContentWithQueriesCompletes(t *testing.T) {
	em := newEmulator("test", 80, 24, defaultConfig())
	defer func() { _ = em.Close() }()

	// Simulates TUI output: normal content interspersed with terminal queries.
	chunks := [][]byte{
		[]byte("\x1b[?1049h"),                         // enter alt screen
		[]byte("\x1b[H\x1b[2J"),                       // clear
		[]byte("\x1b[1;1H\x1b[38;5;82mstatus\x1b[0m"), // styled content
		[]byte("\x1b[c"),                              // DA1 query
		[]byte("\x1b[6n"),                             // cursor position report
		[]byte("\x1b[2;1Hmore content"),               // more content
		[]byte("\x1b[>c"),                             // DA2 query
		[]byte("\x1b[5n"),                             // operating status
		[]byte("\x1b[3;1Hfinal line"),                 // more content
	}

	done := make(chan struct{})
	go func() {
		for _, chunk := range chunks {
			em.Process(chunk)
		}
		close(done)
	}()

	select {
	case <-done:
		snap := em.Snapshot()
		vp := string(snap.Replay)
		assert.Contains(t, vp, "status")
		assert.Contains(t, vp, "more content")
		assert.Contains(t, vp, "final line")
	case <-time.After(2 * time.Second):
		t.Fatal("mixed content with queries blocked")
	}
}

func TestRegression_AllQueryTypesInSingleChunk(t *testing.T) {
	em := newEmulator("test", 80, 24, defaultConfig())
	defer func() { _ = em.Close() }()

	// All query types concatenated in one Write() call.
	payload := []byte("text\x1b[c\x1b[>c\x1b[5n\x1b[6n\x1b[?6nmore text")

	assertProcessCompletes(t, em, payload, "all-queries-single-chunk")

	snap := em.Snapshot()
	vp := string(snap.Replay)
	assert.Contains(t, vp, "text")
	assert.Contains(t, vp, "more text")
}

// --- Regression: Repeated queries don't accumulate backpressure ---

func TestRegression_RepeatedDA1DoesNotDegrade(t *testing.T) {
	em := newEmulator("test", 80, 24, defaultConfig())
	defer func() { _ = em.Close() }()

	// Send 1000 DA1 queries. Without drain, the first one blocks.
	// With drain, all should complete quickly.
	done := make(chan struct{})
	go func() {
		for range 1000 {
			em.Process([]byte("\x1b[c"))
		}
		close(done)
	}()

	select {
	case <-done:
		// All 1000 queries processed without blocking.
	case <-time.After(5 * time.Second):
		t.Fatal("1000 DA1 queries did not complete in 5s — drain may be broken")
	}
}

// --- Regression: High-throughput with interspersed queries ---

func TestRegression_HighThroughputWithQueries(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000
	em := newEmulator("test", 120, 24, cfg)
	defer func() { _ = em.Close() }()

	done := make(chan struct{})
	go func() {
		for i := range 500 {
			em.Process([]byte(fmt.Sprintf("line %04d with some realistic content\r\n", i)))
			if i%50 == 0 {
				em.Process([]byte("\x1b[c"))  // DA1 every 50 lines
				em.Process([]byte("\x1b[6n")) // CPR every 50 lines
			}
		}
		close(done)
	}()

	select {
	case <-done:
		snap := em.Snapshot()
		require.NotNil(t, snap.Replay, "scrollback should be populated after 500 lines")
		assert.Contains(t, string(snap.Replay), "line 0499")
	case <-time.After(5 * time.Second):
		t.Fatal("high-throughput processing with queries blocked")
	}
}

// assertProcessCompletes verifies that Process(data) returns within processTimeout.
func assertProcessCompletes(t *testing.T, em *emulator, data []byte, label string) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		em.Process(data)
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(processTimeout):
		t.Fatalf("Process(%s) did not complete within %v — drain goroutine may be broken", label, processTimeout)
	}
}

// --- Regression: Content integrity after queries ---

func TestRegression_ViewportNotCorruptedByQueries(t *testing.T) {
	em := newEmulator("test", 40, 10, defaultConfig())
	defer func() { _ = em.Close() }()

	// Write a known pattern, then queries, then verify content is intact.
	for i := range 8 {
		em.Process([]byte(fmt.Sprintf("row %d content\r\n", i)))
	}
	em.Process([]byte("\x1b[c\x1b[>c\x1b[6n"))
	em.Process([]byte("\x1b[9;1Hfinal row"))

	snap := em.Snapshot()
	vp := string(snap.Replay)

	for i := range 8 {
		expected := fmt.Sprintf("row %d content", i)
		assert.Contains(t, vp, expected, "row %d missing from viewport", i)
	}
	assert.Contains(t, vp, "final row")
}

func TestRegression_ScrollbackPreservedAcrossQueries(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 500
	em := newEmulator("test", 80, 5, cfg)
	defer func() { _ = em.Close() }()

	// Fill scrollback.
	for i := range 20 {
		em.Process([]byte(fmt.Sprintf("scrollback line %02d\r\n", i)))
	}

	snap1 := em.Snapshot()
	require.NotNil(t, snap1.Replay)

	// Process queries — scrollback should not be affected.
	for range 10 {
		em.Process([]byte("\x1b[c\x1b[6n\x1b[>c"))
	}

	snap2 := em.Snapshot()
	assert.Equal(t, snap1.Replay, snap2.Replay,
		"scrollback should not change from query processing")
}

// --- Regression: Concurrent operations stability ---

func TestRegression_ConcurrentProcessSnapshotResize(t *testing.T) {
	em := newEmulator("test", 80, 24, defaultConfig())
	defer func() { _ = em.Close() }()

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// Writer: content + queries.
	wg.Add(1)
	go func() {
		defer wg.Done()
		i := 0
		for {
			select {
			case <-stop:
				return
			default:
				em.Process([]byte(fmt.Sprintf("line %d\r\n", i)))
				if i%10 == 0 {
					em.Process([]byte("\x1b[c"))
				}
				i++
			}
		}
	}()

	// Reader: snapshots.
	snapshots := atomic.Int64{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				snap := em.Snapshot()
				_ = snap.Replay
				snapshots.Add(1)
			}
		}
	}()

	// Resizer: dimension changes.
	wg.Add(1)
	go func() {
		defer wg.Done()
		cols := 80
		for {
			select {
			case <-stop:
				return
			default:
				cols = 60 + (cols-60+1)%40 // oscillate 60-100
				em.Resize(cols, 24)
				time.Sleep(time.Millisecond)
			}
		}
	}()

	time.Sleep(500 * time.Millisecond)
	close(stop)
	wg.Wait()

	t.Logf("snapshots taken during concurrent test: %d", snapshots.Load())
	assert.Positive(t, snapshots.Load(), "should have taken at least one snapshot")
}
