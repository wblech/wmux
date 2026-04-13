package session

import (
	"fmt"
	"runtime"
	"sync"
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
		HistoryWriter: nil,
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
		HistoryWriter: nil,
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

func TestService_CreateDefaultDimensions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{})

	opts := CreateOptions{
		Shell:         defaultShell(),
		Args:          nil,
		Cols:          0, // should default to 80
		Rows:          0, // should default to 24
		Cwd:           "",
		Env:           nil,
		HighWatermark: 0, // should default to defaultHighWatermark
		LowWatermark:  0, // should default to defaultLowWatermark
		BatchInterval: 0, // should default to defaultBatchInterval
		HistoryWriter: nil,
	}

	sess, err := svc.Create("default-dims", opts)
	require.NoError(t, err)
	assert.Equal(t, 80, sess.Cols)
	assert.Equal(t, 24, sess.Rows)
}

func TestService_ResizeNotFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{})

	err := svc.Resize("no-such-session", 120, 40)
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestService_WriteInputNotFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{})

	err := svc.WriteInput("no-such-session", []byte("hello"))
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestService_SnapshotNotFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{})

	_, err := svc.Snapshot("no-such-session")
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestService_ReadOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{})

	_, err := svc.Create("read-out", defaultCreateOpts())
	require.NoError(t, err)

	// ReadOutput on a live session should succeed (may return nil if no output yet).
	out, err := svc.ReadOutput("read-out")
	require.NoError(t, err)
	// out may be nil or non-nil depending on timing; just verify no error.
	_ = out
}

func TestService_ReadOutputNotFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{})

	_, err := svc.ReadOutput("no-such-session")
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestService_AttachDetach(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}
	svc := NewService(&pty.UnixSpawner{})
	_, err := svc.Create("ad-1", defaultCreateOpts())
	require.NoError(t, err)

	sess, err := svc.Get("ad-1")
	require.NoError(t, err)
	assert.Equal(t, StateAlive, sess.State)

	err = svc.Detach("ad-1")
	require.NoError(t, err)
	sess, err = svc.Get("ad-1")
	require.NoError(t, err)
	assert.Equal(t, StateDetached, sess.State)

	err = svc.Attach("ad-1")
	require.NoError(t, err)
	sess, err = svc.Get("ad-1")
	require.NoError(t, err)
	assert.Equal(t, StateAlive, sess.State)
}

func TestService_AttachNotFound(t *testing.T) {
	svc := NewService(&pty.UnixSpawner{})
	assert.ErrorIs(t, svc.Attach("ghost"), ErrSessionNotFound)
}

func TestService_DetachNotFound(t *testing.T) {
	svc := NewService(&pty.UnixSpawner{})
	assert.ErrorIs(t, svc.Detach("ghost"), ErrSessionNotFound)
}

func TestService_LastActivity(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}
	svc := NewService(&pty.UnixSpawner{})
	_, err := svc.Create("act-1", defaultCreateOpts())
	require.NoError(t, err)

	lastAct, err := svc.LastActivity("act-1")
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), lastAct, 2*time.Second)
}

func TestService_LastActivityNotFound(t *testing.T) {
	svc := NewService(&pty.UnixSpawner{})
	_, err := svc.LastActivity("ghost")
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestService_HistoryWriter(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}
	var mu sync.Mutex
	var captured []byte
	writer := &testCaptureWriter{mu: &mu, data: &captured}

	svc := NewService(&pty.UnixSpawner{})
	opts := defaultCreateOpts()
	opts.HistoryWriter = writer

	_, err := svc.Create("hist-1", opts)
	require.NoError(t, err)

	err = svc.WriteInput("hist-1", []byte("echo test\n"))
	require.NoError(t, err)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := len(captured) > 0
		mu.Unlock()
		if got {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	assert.NotEmpty(t, captured)
	mu.Unlock()
}

func TestService_OnExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}
	exitCh := make(chan int, 1)
	svc := NewService(&pty.UnixSpawner{}, WithOnExit(func(_ string, exitCode int) {
		exitCh <- exitCode
	}))

	opts := CreateOptions{
		Shell:         "/bin/sh",
		Args:          []string{"-c", "exit 42"},
		Cols:          80,
		Rows:          24,
		Cwd:           "",
		Env:           nil,
		HighWatermark: 0,
		LowWatermark:  0,
		BatchInterval: 5 * time.Millisecond,
		HistoryWriter: nil,
	}
	_, err := svc.Create("exit-cb", opts)
	require.NoError(t, err)

	select {
	case code := <-exitCh:
		assert.Equal(t, 42, code)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for OnExit callback")
	}
}

// testCaptureWriter is a test io.Writer that stores written bytes.
type testCaptureWriter struct {
	mu   *sync.Mutex
	data *[]byte
}

func (w *testCaptureWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	*w.data = append(*w.data, p...)
	return len(p), nil
}

func TestService_ReadOutputAfterWrite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	svc := NewService(&pty.UnixSpawner{})

	_, err := svc.Create("read-after-write", defaultCreateOpts())
	require.NoError(t, err)

	// Write something to the PTY so the read loop produces output.
	err = svc.WriteInput("read-after-write", []byte("echo hello\n"))
	require.NoError(t, err)

	// Poll until we get output or time out.
	deadline := time.Now().Add(500 * time.Millisecond)
	var output []byte
	for time.Now().Before(deadline) {
		output, err = svc.ReadOutput("read-after-write")
		require.NoError(t, err)
		if len(output) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	assert.NotEmpty(t, output)
}

func TestService_SpawnSemaphore(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}
	svc := NewService(&pty.UnixSpawner{}, WithSpawnSemaphore(1))

	_, err := svc.Create("sem-1", defaultCreateOpts())
	require.NoError(t, err)

	_, err = svc.Create("sem-2", defaultCreateOpts())
	require.NoError(t, err)
}

func TestService_SpawnSemaphoreConcurrency(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}
	svc := NewService(&pty.UnixSpawner{}, WithSpawnSemaphore(1))

	done := make(chan error, 3)
	for i := range 3 {
		go func(idx int) {
			id := fmt.Sprintf("conc-%d", idx)
			_, err := svc.Create(id, defaultCreateOpts())
			done <- err
		}(i)
	}

	for range 3 {
		err := <-done
		require.NoError(t, err)
	}

	sessions := svc.List()
	assert.Len(t, sessions, 3)
}

func TestService_SpawnSemaphoreZero(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}
	svc := NewService(&pty.UnixSpawner{}, WithSpawnSemaphore(0))

	_, err := svc.Create("no-limit", defaultCreateOpts())
	assert.NoError(t, err)
}

func TestService_MetaSet(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}
	svc := NewService(&pty.UnixSpawner{})
	_, err := svc.Create("meta-1", defaultCreateOpts())
	require.NoError(t, err)

	err = svc.MetaSet("meta-1", "app", "watchtower")
	require.NoError(t, err)

	val, err := svc.MetaGet("meta-1", "app")
	require.NoError(t, err)
	assert.Equal(t, "watchtower", val)
}

func TestService_MetaSetNotFound(t *testing.T) {
	svc := NewService(&pty.UnixSpawner{})
	err := svc.MetaSet("ghost", "k", "v")
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestService_MetaGetNotFound(t *testing.T) {
	svc := NewService(&pty.UnixSpawner{})
	_, err := svc.MetaGet("ghost", "k")
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestService_MetaGetAll(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}
	svc := NewService(&pty.UnixSpawner{})
	_, err := svc.Create("meta-all", defaultCreateOpts())
	require.NoError(t, err)

	require.NoError(t, svc.MetaSet("meta-all", "app", "watchtower"))
	require.NoError(t, svc.MetaSet("meta-all", "env", "prod"))

	meta, err := svc.MetaGetAll("meta-all")
	require.NoError(t, err)
	assert.Equal(t, "watchtower", meta["app"])
	assert.Equal(t, "prod", meta["env"])
}

func TestService_MetaGetAllNotFound(t *testing.T) {
	svc := NewService(&pty.UnixSpawner{})
	_, err := svc.MetaGetAll("ghost")
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestService_MetaGetAllReturnsDefensiveCopy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}
	svc := NewService(&pty.UnixSpawner{})
	_, err := svc.Create("meta-copy", defaultCreateOpts())
	require.NoError(t, err)

	require.NoError(t, svc.MetaSet("meta-copy", "k", "v"))

	meta, err := svc.MetaGetAll("meta-copy")
	require.NoError(t, err)
	meta["k"] = "tampered"

	val, err := svc.MetaGet("meta-copy", "k")
	require.NoError(t, err)
	assert.Equal(t, "v", val, "MetaGetAll should return a defensive copy")
}
