package charmvt

import (
	"errors"
	"io"
	"testing"
	"time"

	"github.com/charmbracelet/x/vt"
)

// Evidence tests: vt.Write() blocks when the emulator's internal io.Pipe is
// not drained. The vt.Emulator uses an io.Pipe (pr/pw) for terminal responses
// (DA1, DA2, DSR, CPR). io.Pipe is synchronous — Write blocks until Read
// consumes the data. If nobody reads, Write blocks forever.
//
// This is the root cause of the "TUI app blocks all terminals" bug:
//
//   Claude Code sends DA1 query (\x1b[c, 3 bytes)
//     → vt parser dispatches CSI 'c' handler
//       → handler calls io.WriteString(e.pw, DA1 response)
//         → pw.Write BLOCKS (nobody reads e.pr)
//           → emulator.Process() never returns
//             → mutex held forever → Snapshot() blocks → daemon freezes

// TestEvidence_DA1_Blocks proves that vt.Write() blocks indefinitely when
// processing a DA1 (Primary Device Attributes) query without a pipe reader.
// DA1 = \x1b[c — exactly 3 bytes, matching the investigation finding.
func TestEvidence_DA1_Blocks(t *testing.T) {
	em := vt.NewEmulator(80, 24)
	defer func() { _ = em.Close() }()

	done := make(chan struct{})
	go func() {
		_, _ = em.Write([]byte("\x1b[c")) // DA1: Primary Device Attributes
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Write(\x1b[c) returned, but should block — nobody draining the response pipe")
	case <-time.After(200 * time.Millisecond):
		// Expected: Write blocked because nobody reads from the pipe.
	}
}

// TestEvidence_DA2_Blocks proves the same deadlock for DA2 (Secondary Device
// Attributes, \x1b[>c).
func TestEvidence_DA2_Blocks(t *testing.T) {
	em := vt.NewEmulator(80, 24)
	defer func() { _ = em.Close() }()

	done := make(chan struct{})
	go func() {
		_, _ = em.Write([]byte("\x1b[>c")) // DA2: Secondary Device Attributes
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Write(\x1b[>c) returned, but should block — nobody draining the response pipe")
	case <-time.After(200 * time.Millisecond):
		// Expected: blocked.
	}
}

// TestEvidence_DSR_CursorPosition_Blocks proves the deadlock for DSR CPR
// (Device Status Report — Cursor Position Report, \x1b[6n).
func TestEvidence_DSR_CursorPosition_Blocks(t *testing.T) {
	em := vt.NewEmulator(80, 24)
	defer func() { _ = em.Close() }()

	done := make(chan struct{})
	go func() {
		_, _ = em.Write([]byte("\x1b[6n")) // DSR: Cursor Position Report
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Write(\x1b[6n) returned, but should block — nobody draining the response pipe")
	case <-time.After(200 * time.Millisecond):
		// Expected: blocked.
	}
}

// TestEvidence_DSR_OperatingStatus_Blocks proves the deadlock for DSR
// (Operating Status Report, \x1b[5n).
func TestEvidence_DSR_OperatingStatus_Blocks(t *testing.T) {
	em := vt.NewEmulator(80, 24)
	defer func() { _ = em.Close() }()

	done := make(chan struct{})
	go func() {
		_, _ = em.Write([]byte("\x1b[5n")) // DSR: Operating Status
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Write(\x1b[5n) returned, but should block — nobody draining the response pipe")
	case <-time.After(200 * time.Millisecond):
		// Expected: blocked.
	}
}

// TestEvidence_DECXCPR_Blocks proves the deadlock for DECXCPR
// (Extended Cursor Position Report, \x1b[?6n).
func TestEvidence_DECXCPR_Blocks(t *testing.T) {
	em := vt.NewEmulator(80, 24)
	defer func() { _ = em.Close() }()

	done := make(chan struct{})
	go func() {
		_, _ = em.Write([]byte("\x1b[?6n")) // DECXCPR: Extended CPR
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Write(\x1b[?6n) returned, but should block — nobody draining the response pipe")
	case <-time.After(200 * time.Millisecond):
		// Expected: blocked.
	}
}

// TestEvidence_DrainedPipe_Unblocks proves that when the pipe IS drained,
// Write() completes normally for all response-triggering sequences.
func TestEvidence_DrainedPipe_Unblocks(t *testing.T) {
	sequences := []struct {
		name string
		seq  []byte
	}{
		{"DA1", []byte("\x1b[c")},
		{"DA2", []byte("\x1b[>c")},
		{"DSR_CPR", []byte("\x1b[6n")},
		{"DSR_Status", []byte("\x1b[5n")},
		{"DECXCPR", []byte("\x1b[?6n")},
	}

	for _, tc := range sequences {
		t.Run(tc.name, func(t *testing.T) {
			em := vt.NewEmulator(80, 24)
			defer func() { _ = em.Close() }()

			// Drain the response pipe — this is what charmvt should do.
			go func() {
				buf := make([]byte, 1024)
				for {
					_, err := em.Read(buf)
					if err != nil {
						return
					}
				}
			}()

			done := make(chan struct{})
			go func() {
				_, _ = em.Write(tc.seq)
				close(done)
			}()

			select {
			case <-done:
				// Expected: Write completes because the pipe is being drained.
			case <-time.After(200 * time.Millisecond):
				t.Fatalf("Write(%q) blocked even with a pipe reader — unexpected", tc.seq)
			}
		})
	}
}

// TestEvidence_MixedContent_BlocksMidStream proves that normal content
// followed by a DA1 query blocks mid-stream. This simulates what happens
// when a TUI app's output contains a DA1 query among normal rendering:
// the emulator processes the normal bytes fine, then hangs on the DA1.
func TestEvidence_MixedContent_BlocksMidStream(t *testing.T) {
	em := vt.NewEmulator(80, 24)
	defer func() { _ = em.Close() }()

	// Normal TUI content followed by DA1 query — a single Write call.
	payload := []byte("hello world\x1b[31mred text\x1b[0m\x1b[c")

	done := make(chan struct{})
	go func() {
		_, _ = em.Write(payload)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Write with embedded DA1 returned, but should block")
	case <-time.After(200 * time.Millisecond):
		// Expected: blocks when it reaches the \x1b[c at the end.
	}
}

// NOTE: The charmvt-layer evidence tests (TestEvidence_CharmVT_Process_Blocks
// and TestEvidence_CharmVT_Snapshot_Blocked_By_Stuck_Process) were removed
// after the drain goroutine fix. They proved the bug existed; the regression
// tests in pipe_drain_regression_test.go now verify the fix holds.

// TestEvidence_PipeResponse_Content verifies that the DA1 response written
// to the pipe is a valid ANSI DA1 response. This confirms the pipe mechanism
// is correct — the issue is purely that nobody reads it.
func TestEvidence_PipeResponse_Content(t *testing.T) {
	em := vt.NewEmulator(80, 24)
	defer func() { _ = em.Close() }()

	// Read the response in a goroutine.
	responseCh := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 256)
		n, err := em.Read(buf)
		if err != nil && !errors.Is(err, io.EOF) {
			return
		}
		responseCh <- buf[:n]
	}()

	// Write DA1 query.
	_, err := em.Write([]byte("\x1b[c"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	select {
	case resp := <-responseCh:
		// DA1 response should be CSI ? ... c
		s := string(resp)
		t.Logf("DA1 response: %q", s)
		if len(resp) == 0 {
			t.Fatal("empty DA1 response")
		}
		// Response should start with ESC[ and end with 'c'
		if resp[0] != 0x1b || resp[1] != '[' {
			t.Errorf("DA1 response should start with ESC[, got %q", s)
		}
		if resp[len(resp)-1] != 'c' {
			t.Errorf("DA1 response should end with 'c', got %q", s)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("no DA1 response received")
	}
}
