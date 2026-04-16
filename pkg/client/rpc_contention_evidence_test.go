package client

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wblech/wmux/internal/platform/protocol"
)

// Regression tests for: "Attach blocks indefinitely when session runs a TUI app"
//
// These tests verify that a slow daemon response causes ErrRequestTimeout
// instead of blocking all client RPCs indefinitely. Prior to the fix,
// sendRequest held rpcMu with no timeout, causing head-of-line blocking.
//
// Design note: the mock server processes requests sequentially (one at a time),
// which mirrors real daemon behaviour. The Attach handler sleeps longer than
// rpcTimeout to simulate a blocked daemon handler. After the client times out,
// a drain goroutine waits for the stale response; once consumed, the next RPC
// (List) can proceed normally.

// TestRegression_SlowRPCTimesOutInsteadOfBlocking verifies that a slow
// daemon response causes ErrRequestTimeout instead of blocking all RPCs.
func TestRegression_SlowRPCTimesOutInsteadOfBlocking(t *testing.T) {
	const rpcTimeout = 500 * time.Millisecond
	// Attach handler sleeps longer than rpcTimeout so the client times out,
	// but short enough that the subsequent List completes within the test limit.
	const attachDelay = 800 * time.Millisecond

	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		// MsgAttach: simulate slow Snapshot — takes longer than rpcTimeout.
		protocol.MsgAttach: func(_ []byte) protocol.Frame {
			time.Sleep(attachDelay)
			return okFrame(map[string]any{
				"id": "s1", "state": "alive", "pid": 1, "cols": 80, "rows": 24, "shell": "/bin/zsh",
			})
		},
		// MsgList: fast response.
		protocol.MsgList: func(_ []byte) protocol.Frame {
			return okFrame([]SessionInfo{
				{ID: "s1", State: "alive", Pid: 0, Cols: 0, Rows: 0, Shell: ""},
			})
		},
	})
	defer cleanup()

	c, err := New(
		WithSocket(socketPath),
		WithTokenPath(tokenPath),
		WithAutoStart(false),
		WithRPCTimeout(rpcTimeout),
	)
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	// Attach should timeout, not block forever.
	start := time.Now()
	_, err = c.Attach("s1")
	elapsed := time.Since(start)

	require.Error(t, err)
	require.ErrorIs(t, err, ErrRequestTimeout)
	t.Logf("Attach returned error in %v: %v", elapsed, err)

	// After the timeout, List should work (rpcMu released, drain goroutine
	// will consume the stale Attach response once it arrives).
	// Small delay to let the drain goroutine start waiting on responses.
	time.Sleep(50 * time.Millisecond)

	listStart := time.Now()
	sessions, err := c.List()
	listElapsed := time.Since(listStart)

	require.NoError(t, err)
	assert.Len(t, sessions, 1)
	t.Logf("List() after timeout: %v", listElapsed)

	// List must complete within 1s: drain waits ~(attachDelay-rpcTimeout) = 300ms
	// for the stale Attach response, then List executes in a few ms.
	if listElapsed > 1*time.Second {
		t.Errorf("List() still blocked after Attach timeout: %v", listElapsed)
	}
}

// TestRegression_RPCWithoutContentionIsFast verifies RPCs are fast when
// there is no contention (sanity check).
func TestRegression_RPCWithoutContentionIsFast(t *testing.T) {
	socketPath, tokenPath, _, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgList: func(_ []byte) protocol.Frame {
			return okFrame([]SessionInfo{
				{ID: "s1", State: "alive", Pid: 0, Cols: 0, Rows: 0, Shell: ""},
			})
		},
	})
	defer cleanup()

	c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	start := time.Now()
	sessions, err := c.List()
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Len(t, sessions, 1)
	t.Logf("List() without contention: %v", elapsed)

	if elapsed > 200*time.Millisecond {
		t.Errorf("List() too slow even without contention: %v", elapsed)
	}
}
