package session

import (
	"fmt"
	"os/exec"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wblech/wmux/internal/platform/pty"
)

// TestRace_ListStateDuringProcessExit evidences the data race between
// Service.waitLoop writing session.State and callers reading State on
// pointers returned by List(). The race detector catches this because
// List() returns shared *Session pointers — the RLock only protects the
// map iteration, not subsequent field reads after the lock is released.
func TestRace_ListStateDuringProcessExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{})

	// Create multiple sessions to increase the race window.
	const n = 5
	for i := range n {
		id := fmt.Sprintf("race-list-%d", i)
		opts := defaultCreateOpts()
		opts.Args = []string{"-c", "sleep 60"}
		sess, err := svc.Create(id, opts)
		require.NoError(t, err)

		// Kill externally so waitLoop will fire and write State = StateExited.
		cmd := exec.Command("kill", "-9", fmt.Sprintf("%d", sess.Pid)) //nolint:gosec
		require.NoError(t, cmd.Run())
	}

	// Concurrently call List() and read State while waitLoop is writing it.
	var wg sync.WaitGroup
	stop := make(chan struct{})

	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					for _, sess := range svc.List() {
						// This read races with waitLoop writing
						// ms.session.State = StateExited.
						_ = sess.State.IsTerminal()
					}
				}
			}
		}()
	}

	// Let the readers spin while waitLoop processes the killed sessions.
	time.Sleep(500 * time.Millisecond)
	close(stop)
	wg.Wait()
}

// TestRace_GetStateDuringProcessExit evidences the same pointer-sharing
// race through Get() instead of List(). Get() also returns *Session, so
// reading fields after the RLock is released is unsynchronized.
func TestRace_GetStateDuringProcessExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{})

	opts := defaultCreateOpts()
	opts.Args = []string{"-c", "sleep 60"}
	sess, err := svc.Create("race-get", opts)
	require.NoError(t, err)

	cmd := exec.Command("kill", "-9", fmt.Sprintf("%d", sess.Pid)) //nolint:gosec
	require.NoError(t, cmd.Run())

	var wg sync.WaitGroup
	stop := make(chan struct{})

	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					s, err := svc.Get("race-get")
					if err != nil {
						return // session removed by waitLoop
					}
					_ = s.State.IsTerminal()
					_ = s.ExitCode
				}
			}
		}()
	}

	time.Sleep(500 * time.Millisecond)
	close(stop)
	wg.Wait()
}

// TestRace_WatchdogTickDuringProcessExit reproduces the exact CI failure:
// Watchdog.tick reads sess.State via List() while waitLoop writes it.
func TestRace_WatchdogTickDuringProcessExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{})

	const n = 3
	for i := range n {
		id := fmt.Sprintf("race-wd-%d", i)
		opts := defaultCreateOpts()
		opts.Args = []string{"-c", "sleep 60"}
		sess, err := svc.Create(id, opts)
		require.NoError(t, err)

		cmd := exec.Command("kill", "-9", fmt.Sprintf("%d", sess.Pid)) //nolint:gosec
		require.NoError(t, cmd.Run())
	}

	// Start a fast watchdog so tick() runs many times during the race window.
	w := newWatchdog(svc, 5*time.Millisecond)
	stop := w.Start()

	time.Sleep(500 * time.Millisecond)
	stop()
}

// TestRace_ReaperTickDuringProcessExit evidences the same race in
// Reaper.tick which reads sess.State from List() pointers.
func TestRace_ReaperTickDuringProcessExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{})

	const n = 3
	for i := range n {
		id := fmt.Sprintf("race-reap-%d", i)
		opts := defaultCreateOpts()
		opts.Args = []string{"-c", "sleep 60"}
		sess, err := svc.Create(id, opts)
		require.NoError(t, err)

		// Detach first so reaper considers them eligible.
		require.NoError(t, svc.Detach(id))

		cmd := exec.Command("kill", "-9", fmt.Sprintf("%d", sess.Pid)) //nolint:gosec
		require.NoError(t, cmd.Run())
	}

	r := newReaper(svc, 1*time.Millisecond, 5*time.Millisecond)
	stop := r.Start()

	time.Sleep(500 * time.Millisecond)
	stop()
}
