package client

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestServe_OSC133EventsEmitted_ReproducesBug evidences a regression:
// when a daemon is constructed via this package's NewDaemon + Serve
// (the production path used by ServeDaemon), OSC 133 sequences emitted
// by a session's PTY MUST trigger command.* events on the client's
// OnEvent stream.
//
// Currently this test FAILS because Serve never wires the
// cmdlifecycle.Service into the daemon (no daemon.WithCommandLifecycle
// call), leaving d.cmdSvc nil and silently dropping every OSC 133
// sequence the scanner sees.
//
// Spec covered by the test:
//   - emit OSC 133;A from a shell → expect a "command.prompt_shown" event.
//   - emit OSC 133;C, then 133;D;<exit> → expect "command.started"
//     followed by "command.finished" with the exit code in payload.
//
// The test bypasses any shell-integration script by writing OSC 133
// bytes directly via printf in a /bin/sh session.
func TestServe_OSC133EventsEmitted_ReproducesBug(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	dir := shortTempDir(t)
	d, err := NewDaemon(
		WithBaseDir(dir),
		WithNamespace("osc133"),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serveErr := make(chan error, 1)
	go func() { serveErr <- d.Serve(ctx) }()

	// Wait for the daemon socket to be accepting connections.
	require.Eventually(t, func() bool {
		probe, probeErr := New(
			WithBaseDir(dir),
			WithNamespace("osc133"),
			WithAutoStart(false),
		)
		if probeErr != nil {
			return false
		}
		_ = probe.Close()
		return true
	}, 3*time.Second, 50*time.Millisecond)

	c, err := New(
		WithBaseDir(dir),
		WithNamespace("osc133"),
		WithAutoStart(false),
	)
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	var (
		mu     sync.Mutex
		events []Event
	)
	c.OnEvent(func(e Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})

	const sessID = "osc133-evidence"
	_, err = c.Create(sessID, CreateParams{
		Shell: "/bin/sh",
		Cols:  80,
		Rows:  24,
	})
	require.NoError(t, err)

	_, err = c.Attach(sessID)
	require.NoError(t, err)

	// Fire OSC 133;A directly. /bin/sh treats `printf` as a builtin and
	// writes the escape sequence to the PTY's master, which feeds into
	// the daemon's scanOSC code path.
	require.NoError(t, c.Write(sessID, []byte("printf '\\033]133;A\\007'\n")))

	require.Eventuallyf(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, e := range events {
			if e.Type == "command.prompt_shown" && e.SessionID == sessID {
				return true
			}
		}
		return false
	}, 3*time.Second, 50*time.Millisecond,
		"expected command.prompt_shown event after OSC 133;A; got types=%s",
		eventTypesSeen(&mu, &events))

	// Now emit C (command-started) and D;0 (command-finished, exit 0).
	require.NoError(t, c.Write(sessID, []byte("printf '\\033]133;C\\007'\n")))
	require.NoError(t, c.Write(sessID, []byte("printf '\\033]133;D;0\\007'\n")))

	require.Eventuallyf(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		var sawStarted, sawFinished bool
		for _, e := range events {
			if e.SessionID != sessID {
				continue
			}
			if e.Type == "command.started" {
				sawStarted = true
			}
			if e.Type == "command.finished" {
				if code, ok := e.Data["exit_code"].(float64); ok {
					assert.Equal(t, 0, int(code))
				}
				sawFinished = true
			}
		}
		return sawStarted && sawFinished
	}, 3*time.Second, 50*time.Millisecond,
		"expected command.started + command.finished; got types=%s",
		eventTypesSeen(&mu, &events))
}

// eventTypesSeen returns the comma-joined Type strings under the given mutex.
// Used in test-failure diagnostics so the assertion message shows exactly
// which event types were received instead of a bare boolean.
func eventTypesSeen(mu *sync.Mutex, events *[]Event) string {
	mu.Lock()
	defer mu.Unlock()
	if len(*events) == 0 {
		return "(none)"
	}
	out := ""
	for i, e := range *events {
		if i > 0 {
			out += ","
		}
		out += e.Type
	}
	return out
}
