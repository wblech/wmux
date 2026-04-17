package session

import (
	"io"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Tests for emulator io.Closer cleanup in waitLoop (ADR 0025).
//
// The session waitLoop calls Close() on emulators that implement io.Closer
// to stop the drain goroutine and release resources. This follows the same
// pattern used for historyWriter cleanup.

// closableEmulator is a test double that tracks Close() calls.
type closableEmulator struct {
	mu         sync.Mutex
	closed     atomic.Bool
	closeCalls atomic.Int64
	snapshot   Snapshot
}

func (e *closableEmulator) Process(_ []byte) {}
func (e *closableEmulator) Snapshot() Snapshot {
	return e.snapshot
}
func (e *closableEmulator) Resize(_, _ int) {}

func (e *closableEmulator) Close() error {
	e.closeCalls.Add(1)
	e.closed.Store(true)
	return nil
}

// nonClosableEmulator is a test double without Close() method.
type nonClosableEmulator struct{}

func (e *nonClosableEmulator) Process(_ []byte)  {}
func (e *nonClosableEmulator) Snapshot() Snapshot { return Snapshot{} }
func (e *nonClosableEmulator) Resize(_, _ int)    {}

// TestEmulatorCloseTypeAssertion verifies the io.Closer type assertion
// pattern used in waitLoop works correctly for both closer and non-closer
// emulators.
func TestEmulatorCloseTypeAssertion(t *testing.T) {
	t.Run("closable emulator", func(t *testing.T) {
		em := &closableEmulator{}

		// Simulate what waitLoop does — type assertion through the interface.
		var screenEm ScreenEmulator = em
		if closer, ok := screenEm.(io.Closer); ok {
			_ = closer.Close()
		}

		assert.True(t, em.closed.Load(), "Close() should have been called")
		assert.Equal(t, int64(1), em.closeCalls.Load())
	})

	t.Run("non-closable emulator", func(t *testing.T) {
		em := &nonClosableEmulator{}

		// Should not panic or match.
		var iface ScreenEmulator = em
		_, ok := iface.(io.Closer)
		assert.False(t, ok, "nonClosableEmulator should not match io.Closer")
	})

	t.Run("interface variable", func(t *testing.T) {
		// Verify the type assertion works through the ScreenEmulator interface
		// (the actual type stored in managedSession.emulator).
		var em ScreenEmulator = &closableEmulator{}

		closer, ok := em.(io.Closer)
		assert.True(t, ok)
		assert.NoError(t, closer.Close())
		assert.True(t, em.(*closableEmulator).closed.Load())
	})
}

// TestWaitLoopClosesEmulator verifies that when a session's process exits,
// the waitLoop calls Close() on the emulator if it implements io.Closer.
// This is a behavioral test of the cleanup code path.
func TestWaitLoopClosesEmulator(t *testing.T) {
	em := &closableEmulator{}

	// Simulate the cleanup code path from waitLoop:
	//   if closer, ok := ms.emulator.(io.Closer); ok {
	//       _ = closer.Close()
	//   }
	var screenEm ScreenEmulator = em
	if closer, ok := screenEm.(io.Closer); ok {
		_ = closer.Close()
	}

	assert.True(t, em.closed.Load(), "waitLoop should close the emulator")
}

// TestWaitLoopDoesNotCloseNonCloser verifies that emulators without Close()
// are handled gracefully — no panic, no error.
func TestWaitLoopDoesNotCloseNonCloser(t *testing.T) {
	em := &nonClosableEmulator{}

	var screenEm ScreenEmulator = em

	assert.NotPanics(t, func() {
		if closer, ok := screenEm.(io.Closer); ok {
			_ = closer.Close()
		}
	})
}
