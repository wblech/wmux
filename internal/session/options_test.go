package session

import (
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wblech/wmux/internal/platform/pty"
)

func TestWithMaxSessions(t *testing.T) {
	svc := NewService(&pty.UnixSpawner{}, WithMaxSessions(5))
	assert.Equal(t, 5, svc.maxSessions)
}

func TestWithMaxSessions_Zero(t *testing.T) {
	svc := NewService(&pty.UnixSpawner{}, WithMaxSessions(0))
	assert.Equal(t, 0, svc.maxSessions)
}

// TestService_OnDataReadyFiresAfterBufferWrite is the regression gate for
// ADR-0031 (event-driven broadcast wakeup). The callback registered via
// WithOnDataReady must fire with the correct session ID after the batcher
// flushes PTY output into the buffer. This is what the daemon broadcaster
// hooks to wake up immediately instead of polling on a 16 ms ticker.
func TestService_OnDataReadyFiresAfterBufferWrite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	var (
		fired atomic.Int32
		gotID atomic.Value
	)
	gotID.Store("")

	svc := NewService(&pty.UnixSpawner{}, WithOnDataReady(func(id string) {
		fired.Add(1)
		gotID.Store(id)
	}))

	const sessID = "ondataready-1"
	opts := CreateOptions{
		Shell:         "/bin/sh",
		Args:          []string{"-c", "printf hello; sleep 0.5"},
		Cols:          80,
		Rows:          24,
		BatchInterval: 5 * time.Millisecond,
	}
	_, err := svc.Create(sessID, opts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = svc.Kill(sessID) })

	// The shell prints "hello" and exits; the read loop pushes that to the
	// batcher, which flushes into the buffer and invokes onDataReady.
	require.Eventually(t, func() bool {
		return fired.Load() > 0
	}, 5*time.Second, 5*time.Millisecond,
		"OnDataReady never fired after PTY output")

	assert.Equal(t, sessID, gotID.Load(),
		"OnDataReady fired with wrong session ID")
}

// TestService_OnDataReadyDefaultsNil ensures the callback is opt-in. A
// Service constructed without WithOnDataReady must not panic when the
// batcher flushes — this is the contract that keeps existing tests and
// non-daemon callers (CLI helpers, test fixtures) unaffected.
func TestService_OnDataReadyDefaultsNil(t *testing.T) {
	svc := NewService(&pty.UnixSpawner{})
	assert.Nil(t, svc.onDataReady,
		"onDataReady must default to nil so the batcher's nil-check skips it")
}
