package session

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wblech/wmux/internal/platform/pty"
)

func TestReaper_KillsIdleDetachedSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{})

	_, err := svc.Create("idle-1", defaultCreateOpts())
	require.NoError(t, err)

	// Detach so it's eligible for reaping.
	require.NoError(t, svc.Detach("idle-1"))

	reaped := make(chan string, 1)
	r := NewReaper(svc, 50*time.Millisecond, 30*time.Millisecond)
	r.OnReap = func(id string) { reaped <- id }
	stop := r.Start()
	defer stop()

	select {
	case id := <-reaped:
		assert.Equal(t, "idle-1", id)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for reap")
	}
}

func TestReaper_SkipsAliveSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{})

	_, err := svc.Create("alive-1", defaultCreateOpts())
	require.NoError(t, err)

	reaped := false
	r := NewReaper(svc, 10*time.Millisecond, 20*time.Millisecond)
	r.OnReap = func(_ string) { reaped = true }
	stop := r.Start()

	time.Sleep(100 * time.Millisecond)
	stop()

	assert.False(t, reaped)
}

func TestReaper_SkipsRecentActivity(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{})

	_, err := svc.Create("recent-1", defaultCreateOpts())
	require.NoError(t, err)

	require.NoError(t, svc.Detach("recent-1"))

	reaped := false
	r := NewReaper(svc, 200*time.Millisecond, 50*time.Millisecond)
	r.OnReap = func(_ string) { reaped = true }
	stop := r.Start()

	// Keep session active by writing input.
	for range 5 {
		_ = svc.WriteInput("recent-1", []byte("echo keep alive\n"))
		time.Sleep(30 * time.Millisecond)
	}

	stop()
	assert.False(t, reaped)
}

func TestReaper_StopPreventsReaping(_ *testing.T) {
	svc := NewService(&pty.UnixSpawner{})

	r := NewReaper(svc, time.Hour, time.Minute)
	stop := r.Start()
	stop()
	// Should not panic or hang.
}
