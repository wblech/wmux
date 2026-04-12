package session

import (
	"fmt"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wblech/wmux/internal/platform/pty"
)

func TestWatchdog_KillsOrphanedSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{})

	opts := CreateOptions{
		Shell:         "/bin/sh",
		Args:          []string{"-c", "sleep 60"},
		Cols:          80,
		Rows:          24,
		Cwd:           "",
		Env:           nil,
		HighWatermark: 0,
		LowWatermark:  0,
		BatchInterval: 5 * time.Millisecond,
		HistoryWriter: nil,
	}

	sess, err := svc.Create("wd-1", opts)
	require.NoError(t, err)

	// Kill the process externally.
	cmd := exec.Command("kill", "-9", fmt.Sprintf("%d", sess.Pid)) //nolint:gosec
	require.NoError(t, cmd.Run())

	killed := make(chan string, 1)
	w := NewWatchdog(svc, 50*time.Millisecond)
	w.OnKill = func(id string) { killed <- id }
	stop := w.Start()
	defer stop()

	select {
	case id := <-killed:
		assert.Equal(t, "wd-1", id)
	case <-time.After(3 * time.Second):
		// The waitLoop may have already cleaned up — also valid.
		_, err := svc.Get("wd-1")
		assert.ErrorIs(t, err, ErrSessionNotFound)
	}
}

func TestWatchdog_SkipsAliveProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{})

	opts := CreateOptions{
		Shell:         "/bin/sh",
		Args:          []string{"-c", "sleep 60"},
		Cols:          80,
		Rows:          24,
		Cwd:           "",
		Env:           nil,
		HighWatermark: 0,
		LowWatermark:  0,
		BatchInterval: 5 * time.Millisecond,
		HistoryWriter: nil,
	}

	_, err := svc.Create("wd-alive", opts)
	require.NoError(t, err)

	killed := false
	w := NewWatchdog(svc, 50*time.Millisecond)
	w.OnKill = func(_ string) { killed = true }
	stop := w.Start()

	time.Sleep(200 * time.Millisecond)
	stop()

	assert.False(t, killed)

	_ = svc.Kill("wd-alive")
}

func TestWatchdog_StopIsIdempotent(_ *testing.T) {
	svc := NewService(&pty.UnixSpawner{})
	w := NewWatchdog(svc, time.Second)
	stop := w.Start()
	stop()
	stop() // second call should not panic
}
