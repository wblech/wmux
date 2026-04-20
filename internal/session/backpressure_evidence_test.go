package session

// TestBackpressureEvidence gathers empirical evidence for the
// "terminal freeze at limit" hypothesis:
//
//	When a slow consumer can't drain the wmux Buffer fast enough,
//	Buffer.Paused() returns true, readLoop stops reading from the PTY,
//	the kernel buffer fills, the shell child blocks on write() —
//	freezing both output AND input.
//
// This is an EVIDENCE test, not a correctness test. It does not assert
// pass/fail on the hypothesis — it logs raw measurements so we can
// observe actual behaviour from `go test -v` output.

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wblech/wmux/internal/platform/pty"
)

// samplePoint records one observation of the buffer state.
type samplePoint struct {
	elapsed time.Duration
	bufLen  int
	paused  bool
}

func TestBackpressureEvidence(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	// ---- Session setup -------------------------------------------------------
	// Use a small watermark so backpressure triggers quickly under a slow
	// consumer, without having to wait for 4 MiB to accumulate.
	// High = 512 KiB, Low = 256 KiB.
	const highWM = 512 * 1024
	const lowWM = 256 * 1024

	svc := NewService(&pty.UnixSpawner{})

	opts := defaultCreateOpts()
	opts.Shell = "/bin/sh"
	// `yes` produces ~GB/s of output — perfect for saturating the buffer fast.
	// Fall back to a pure sh loop if `yes` is somehow unavailable.
	opts.Args = []string{"-c",
		"if command -v yes >/dev/null 2>&1; then yes; else " +
			"i=0; while true; do printf 'line of filler text padding here %d\\n' $i; i=$((i+1)); done; fi"}
	opts.HighWatermark = highWM
	opts.LowWatermark = lowWM
	opts.BatchInterval = 5 * time.Millisecond

	const sessionID = "bp-evidence-test"
	_, err := svc.Create(sessionID, opts)
	if err != nil {
		t.Fatalf("svc.Create: %v", err)
	}
	t.Cleanup(func() { _ = svc.Kill(sessionID) })

	// Grab direct reference to the managed session so we can inspect the buffer.
	svc.mu.RLock()
	ms := svc.sessions[sessionID]
	svc.mu.RUnlock()
	if ms == nil {
		t.Fatal("managed session not found after Create")
	}

	// ---- Slow consumer -------------------------------------------------------
	// Simulates a saturated xterm.js main thread: reads output every 100 ms.
	// This deliberately falls behind `yes` production rate.
	var totalBytesConsumed atomic.Int64
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	consumerDone := make(chan struct{})
	go func() {
		defer close(consumerDone)
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
				data, _ := svc.ReadOutput(sessionID)
				totalBytesConsumed.Add(int64(len(data)))
			}
		}
	}()

	// ---- Sampling loop -------------------------------------------------------
	// Every 20 ms for up to 5 seconds, record buffer state.
	var (
		mu               sync.Mutex
		samples          []samplePoint
		timeToFirstPause time.Duration = -1
		maxBufLen        int
		totalPausedNs    int64
		lastPausedAt     time.Time
		wasEverPaused    bool
		lastSamplePaused bool
	)

	start := time.Now()
	samplerDone := make(chan struct{})
	go func() {
		defer close(samplerDone)
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				// Flush any open pause window.
				mu.Lock()
				if lastSamplePaused {
					totalPausedNs += time.Since(lastPausedAt).Nanoseconds()
				}
				mu.Unlock()
				return
			case ts := <-ticker.C:
				bufLen := ms.buffer.Len()
				paused := ms.buffer.Paused()
				elapsed := ts.Sub(start)

				mu.Lock()
				sp := samplePoint{elapsed: elapsed, bufLen: bufLen, paused: paused}
				samples = append(samples, sp)

				if bufLen > maxBufLen {
					maxBufLen = bufLen
				}

				if paused && !wasEverPaused {
					wasEverPaused = true
					timeToFirstPause = elapsed
				}

				// Track contiguous pause windows for total duration.
				if paused && !lastSamplePaused {
					lastPausedAt = ts
				} else if !paused && lastSamplePaused {
					totalPausedNs += ts.Sub(lastPausedAt).Nanoseconds()
				}
				lastSamplePaused = paused
				mu.Unlock()
			}
		}
	}()

	// ---- Wait for Paused() or timeout ----------------------------------------
	// Poll until first pause event or the 5 s window expires.
	pauseDetected := make(chan struct{}, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Millisecond):
				if ms.buffer.Paused() {
					select {
					case pauseDetected <- struct{}{}:
					default:
					}
					return
				}
			}
		}
	}()

	// ---- Input probe after Paused() fires -----------------------------------
	// We want to measure whether WriteInput blocks when the buffer is paused.
	var (
		inputBlockedMs int64 = -1 // ms taken to write input; -1 = not attempted
		inputErr       error
	)

	inputProbeDone := make(chan struct{})
	go func() {
		defer close(inputProbeDone)
		// Wait for pause or timeout.
		select {
		case <-pauseDetected:
		case <-ctx.Done():
			return
		}

		// Buffer is now paused. Try to send a newline as if the user typed.
		inputStart := time.Now()

		writeDone := make(chan error, 1)
		go func() {
			writeDone <- svc.WriteInput(sessionID, []byte("\n"))
		}()

		// Give it 200 ms — if it hasn't returned by then we call it "blocked".
		select {
		case e := <-writeDone:
			inputErr = e
			inputBlockedMs = time.Since(inputStart).Milliseconds()
		case <-time.After(200 * time.Millisecond):
			inputBlockedMs = 200 // timed-out, treat as blocked
		}
	}()

	// Wait for observation window to close.
	<-ctx.Done()
	<-samplerDone
	<-consumerDone
	<-inputProbeDone

	// ---- Final state snapshot ------------------------------------------------
	mu.Lock()
	finalBufLen := ms.buffer.Len()
	finalPaused := ms.buffer.Paused()
	totalPauseDuration := time.Duration(totalPausedNs)
	mu.Unlock()

	totalConsumed := totalBytesConsumed.Load()

	// ---- Emit evidence -------------------------------------------------------
	t.Log("=== BACKPRESSURE EVIDENCE TEST ===")
	t.Logf("  high_watermark      : %d bytes (%d KiB)", highWM, highWM/1024)
	t.Logf("  low_watermark       : %d bytes (%d KiB)", lowWM, lowWM/1024)
	t.Logf("  consumer_interval   : 100ms (simulated slow xterm.js)")
	t.Logf("  observation_window  : 5s")
	t.Log("")
	t.Log("--- Buffer observations ---")
	t.Logf("  max_buffer_len      : %d bytes (%d KiB)", maxBufLen, maxBufLen/1024)
	t.Logf("  final_buffer_len    : %d bytes (%d KiB)", finalBufLen, finalBufLen/1024)
	t.Logf("  final_paused_state  : %v", finalPaused)
	t.Logf("  total_bytes_consumed: %d bytes (%d KiB)", totalConsumed, totalConsumed/1024)
	t.Log("")
	t.Log("--- Pause dynamics ---")
	if wasEverPaused {
		t.Logf("  first_pause_at      : %v after test start", timeToFirstPause.Round(time.Millisecond))
		t.Logf("  total_pause_duration: %v", totalPauseDuration.Round(time.Millisecond))
	} else {
		t.Log("  PAUSED STATE NEVER TRIGGERED during 5s window")
	}
	t.Log("")
	t.Log("--- Input probe (written while paused) ---")
	switch {
	case inputBlockedMs == -1:
		t.Log("  input_probe : NOT ATTEMPTED (paused never triggered)")
	case inputBlockedMs >= 200:
		t.Logf("  input_probe : TIMED OUT (>= 200ms) — write appears blocked")
		if inputErr != nil {
			t.Logf("  input_error : %v", inputErr)
		}
	default:
		t.Logf("  input_probe : returned in %d ms — write succeeded immediately", inputBlockedMs)
		if inputErr != nil {
			t.Logf("  input_error : %v", inputErr)
		}
	}
	t.Log("")

	// Emit a condensed sample timeline (at most 40 lines).
	t.Log("--- Sample timeline (elapsed | buf_len | paused) ---")
	mu.Lock()
	step := 1
	if len(samples) > 40 {
		step = len(samples) / 40
	}
	for i, sp := range samples {
		if i%step == 0 || sp.paused {
			t.Logf("  t=%6s  buf=%8d  paused=%v",
				sp.elapsed.Round(time.Millisecond),
				sp.bufLen,
				sp.paused)
		}
	}
	mu.Unlock()

	t.Log("")
	t.Log("=== END EVIDENCE ===")

	// ---- Hypothesis verdict (logged only, no t.Fail) -------------------------
	switch {
	case wasEverPaused && inputBlockedMs >= 200:
		t.Log("VERDICT: C1_CONFIRMED — Paused() triggered and input write timed out (>= 200ms), consistent with full kernel-buffer freeze")
	case wasEverPaused && inputBlockedMs >= 0 && inputBlockedMs < 200:
		t.Logf("VERDICT: C1_INCONCLUSIVE — Paused() triggered but input write returned in %d ms (PTY write path may not block at this buffer depth)", inputBlockedMs)
	case wasEverPaused && inputBlockedMs == -1:
		t.Log("VERDICT: C1_INCONCLUSIVE — Paused() triggered but input probe was not attempted")
	default:
		t.Log("VERDICT: C1_INCONCLUSIVE — Paused() never triggered in 5s window; watermarks may be too high or output rate insufficient")
	}

	// Fail only on infrastructure errors.
	if ms == nil {
		t.Fatal("infrastructure: managed session was nil")
	}
	// Verify the session was alive and produced output.
	if maxBufLen == 0 && totalConsumed == 0 {
		t.Fatalf("infrastructure: no output produced — shell command may have failed (maxBufLen=%d, consumed=%d)", maxBufLen, totalConsumed)
	}
}
