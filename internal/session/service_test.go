package session

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wblech/wmux/internal/platform/pty"
)

// defaultShell returns a shell path suitable for integration tests.
func defaultShell() string {
	return "/bin/sh"
}

// defaultCreateOpts returns minimal CreateOptions for test sessions.
func defaultCreateOpts() CreateOptions {
	return CreateOptions{
		Shell:         defaultShell(),
		Args:          nil,
		Cols:          80,
		Rows:          24,
		Cwd:           "",
		Env:           nil,
		HighWatermark: 0,
		LowWatermark:  0,
		BatchInterval: 5 * time.Millisecond,
	}
}

func TestService_Create(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{})

	sess, err := svc.Create("test-create", defaultCreateOpts())
	require.NoError(t, err)
	require.NotNil(t, sess)

	assert.Equal(t, "test-create", sess.ID)
	assert.Equal(t, StateAlive, sess.State)
	assert.Positive(t, sess.Pid)
	assert.Equal(t, 80, sess.Cols)
	assert.Equal(t, 24, sess.Rows)
	assert.Equal(t, defaultShell(), sess.Shell)
}

func TestService_CreateDuplicate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{})

	_, err := svc.Create("dup", defaultCreateOpts())
	require.NoError(t, err)

	_, err = svc.Create("dup", defaultCreateOpts())
	assert.ErrorIs(t, err, ErrSessionExists)
}

func TestService_CreateInvalidID(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{})

	_, err := svc.Create("", defaultCreateOpts())
	assert.ErrorIs(t, err, ErrInvalidSessionID)
}

func TestService_Get(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{})

	created, err := svc.Create("get-me", defaultCreateOpts())
	require.NoError(t, err)

	got, err := svc.Get("get-me")
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
}

func TestService_GetNotFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{})

	_, err := svc.Get("does-not-exist")
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestService_List(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{})

	_, err := svc.Create("list-a", defaultCreateOpts())
	require.NoError(t, err)

	_, err = svc.Create("list-b", defaultCreateOpts())
	require.NoError(t, err)

	sessions := svc.List()
	assert.Len(t, sessions, 2)
}

func TestService_Kill(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{})

	_, err := svc.Create("kill-me", defaultCreateOpts())
	require.NoError(t, err)

	err = svc.Kill("kill-me")
	require.NoError(t, err)

	_, err = svc.Get("kill-me")
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestService_KillNotFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{})

	err := svc.Kill("ghost")
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestService_Resize(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{})

	_, err := svc.Create("resize-me", defaultCreateOpts())
	require.NoError(t, err)

	err = svc.Resize("resize-me", 120, 40)
	require.NoError(t, err)

	sess, err := svc.Get("resize-me")
	require.NoError(t, err)
	assert.Equal(t, 120, sess.Cols)
	assert.Equal(t, 40, sess.Rows)
}

func TestService_WriteInput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{})

	_, err := svc.Create("write-me", defaultCreateOpts())
	require.NoError(t, err)

	err = svc.WriteInput("write-me", []byte("echo hello\n"))
	assert.NoError(t, err)
}

func TestService_Snapshot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{})

	_, err := svc.Create("snap-me", defaultCreateOpts())
	require.NoError(t, err)

	snap, err := svc.Snapshot("snap-me")
	require.NoError(t, err)

	// NoneEmulator always returns empty snapshots.
	assert.Nil(t, snap.Scrollback)
	assert.Nil(t, snap.Viewport)
}

func TestService_ProcessExitDetected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{})

	opts := CreateOptions{
		Shell:         "/bin/sh",
		Args:          []string{"-c", "exit 0"},
		Cols:          80,
		Rows:          24,
		Cwd:           "",
		Env:           nil,
		HighWatermark: 0,
		LowWatermark:  0,
		BatchInterval: 5 * time.Millisecond,
	}

	_, err := svc.Create("exit-session", opts)
	require.NoError(t, err)

	// Wait for the waitLoop to remove the session.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		_, err = svc.Get("exit-session")
		if err != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestService_MaxSessions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{}, WithMaxSessions(1))

	_, err := svc.Create("max-1", defaultCreateOpts())
	require.NoError(t, err)

	_, err = svc.Create("max-2", defaultCreateOpts())
	assert.ErrorIs(t, err, ErrMaxSessions)
}
