package pty

import (
	"bytes"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnixSpawner_SpawnAndRead(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	s := &UnixSpawner{}
	proc, err := s.Spawn(SpawnOptions{
		Command: "/bin/sh",
		Args:    []string{"-c", "echo hello"},
		Cols:    80,
		Rows:    24,
		Cwd:     "",
		Env:     nil,
	})
	require.NoError(t, err)
	defer proc.Close() //nolint:errcheck

	var buf bytes.Buffer
	tmp := make([]byte, 256)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		n, readErr := proc.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
		}
		if readErr != nil {
			break
		}
	}

	assert.Contains(t, buf.String(), "hello")
}

func TestUnixSpawner_Resize(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	s := &UnixSpawner{}
	proc, err := s.Spawn(SpawnOptions{
		Command: "/bin/sh",
		Args:    []string{},
		Cols:    80,
		Rows:    24,
		Cwd:     "",
		Env:     nil,
	})
	require.NoError(t, err)
	defer proc.Close() //nolint:errcheck
	defer proc.Kill()  //nolint:errcheck

	err = proc.Resize(120, 40)
	assert.NoError(t, err)
}

func TestUnixSpawner_Kill(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	s := &UnixSpawner{}
	proc, err := s.Spawn(SpawnOptions{
		Command: "/bin/sh",
		Args:    []string{"-c", "sleep 60"},
		Cols:    80,
		Rows:    24,
		Cwd:     "",
		Env:     nil,
	})
	require.NoError(t, err)
	defer proc.Close() //nolint:errcheck

	killErr := proc.Kill()
	require.NoError(t, killErr)

	done := make(chan struct{})
	go func() {
		proc.Wait() //nolint:errcheck
		close(done)
	}()

	select {
	case <-done:
		// process exited as expected
	case <-time.After(2 * time.Second):
		t.Fatal("process did not exit within 2 seconds after Kill")
	}
}

func TestUnixSpawner_Wait(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	s := &UnixSpawner{}
	proc, err := s.Spawn(SpawnOptions{
		Command: "/bin/sh",
		Args:    []string{"-c", "exit 42"},
		Cols:    80,
		Rows:    24,
		Cwd:     "",
		Env:     nil,
	})
	require.NoError(t, err)
	defer proc.Close() //nolint:errcheck

	code, waitErr := proc.Wait()
	require.NoError(t, waitErr)
	assert.Equal(t, 42, code)
}

func TestUnixSpawner_Pid(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	s := &UnixSpawner{}
	proc, err := s.Spawn(SpawnOptions{
		Command: "/bin/sh",
		Args:    []string{},
		Cols:    80,
		Rows:    24,
		Cwd:     "",
		Env:     nil,
	})
	require.NoError(t, err)
	defer proc.Close() //nolint:errcheck
	defer proc.Kill()  //nolint:errcheck

	assert.Positive(t, proc.Pid())
}

func TestUnixSpawner_Write(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	s := &UnixSpawner{}
	proc, err := s.Spawn(SpawnOptions{
		Command: "/bin/sh",
		Args:    []string{},
		Cols:    80,
		Rows:    24,
		Cwd:     "",
		Env:     nil,
	})
	require.NoError(t, err)
	defer proc.Close() //nolint:errcheck
	defer proc.Kill()  //nolint:errcheck

	n, writeErr := proc.Write([]byte("echo hi\n"))
	require.NoError(t, writeErr)
	assert.Equal(t, 8, n)
}

func TestUnixSpawner_Fd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	s := &UnixSpawner{}
	proc, err := s.Spawn(SpawnOptions{
		Command: "/bin/sh",
		Args:    []string{},
		Cols:    80,
		Rows:    24,
		Cwd:     "",
		Env:     nil,
	})
	require.NoError(t, err)
	defer proc.Close() //nolint:errcheck
	defer proc.Kill()  //nolint:errcheck

	fd := proc.Fd()
	assert.Greater(t, fd, uintptr(0))
}

func TestUnixSpawner_Close(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	s := &UnixSpawner{}
	proc, err := s.Spawn(SpawnOptions{
		Command: "/bin/sh",
		Args:    []string{},
		Cols:    80,
		Rows:    24,
		Cwd:     "",
		Env:     nil,
	})
	require.NoError(t, err)
	defer proc.Kill() //nolint:errcheck

	err = proc.Close()
	assert.NoError(t, err)
}

func TestUnixSpawner_SpawnWithCwdAndEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	s := &UnixSpawner{}
	proc, err := s.Spawn(SpawnOptions{
		Command: "/bin/sh",
		Args:    []string{"-c", "echo $TEST_VAR"},
		Cols:    80,
		Rows:    24,
		Cwd:     t.TempDir(),
		Env:     []string{"TEST_VAR=hello_env", "HOME=/tmp", "TERM=xterm"},
	})
	require.NoError(t, err)
	defer proc.Close() //nolint:errcheck

	var buf bytes.Buffer
	tmp := make([]byte, 256)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		n, readErr := proc.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
		}
		if readErr != nil {
			break
		}
	}

	assert.Contains(t, buf.String(), "hello_env")
}

func TestUnixSpawner_SpawnDefaultDimensions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	// Cols and Rows of 0 should use defaults (80x24).
	s := &UnixSpawner{}
	proc, err := s.Spawn(SpawnOptions{
		Command: "/bin/sh",
		Args:    []string{"-c", "exit 0"},
		Cols:    0,
		Rows:    0,
		Cwd:     "",
		Env:     nil,
	})
	require.NoError(t, err)
	defer proc.Close() //nolint:errcheck
	assert.Positive(t, proc.Pid())
}

func TestUnixSpawner_SpawnInvalidCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	s := &UnixSpawner{}
	_, err := s.Spawn(SpawnOptions{
		Command: "/nonexistent/command",
		Args:    []string{},
		Cols:    80,
		Rows:    24,
		Cwd:     "",
		Env:     nil,
	})
	require.Error(t, err)
}

func TestUnixSpawner_Kill_SIGKILLFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	// Spawn a process that traps SIGHUP so Kill() must fall back to SIGKILL.
	s := &UnixSpawner{}
	proc, err := s.Spawn(SpawnOptions{
		Command: "/bin/sh",
		// Trap SIGHUP and keep running; only SIGKILL can terminate it.
		Args: []string{"-c", "trap '' HUP; sleep 60"},
		Cols: 80,
		Rows: 24,
		Cwd:  "",
		Env:  nil,
	})
	require.NoError(t, err)
	defer proc.Close() //nolint:errcheck

	// Kill should return without error even when SIGKILL fallback is needed.
	killErr := proc.Kill()
	assert.NoError(t, killErr)
}

func TestProcess_Wait_ExitCode0(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	s := &UnixSpawner{}
	proc, err := s.Spawn(SpawnOptions{
		Command: "/bin/sh",
		Args:    []string{"-c", "exit 0"},
		Cols:    80,
		Rows:    24,
		Cwd:     "",
		Env:     nil,
	})
	require.NoError(t, err)
	defer proc.Close() //nolint:errcheck

	code, waitErr := proc.Wait()
	require.NoError(t, waitErr)
	assert.Equal(t, 0, code)
}

func TestUnixSpawner_Resize_InvalidAfterClose(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	s := &UnixSpawner{}
	proc, err := s.Spawn(SpawnOptions{
		Command: "/bin/sh",
		Args:    []string{},
		Cols:    80,
		Rows:    24,
		Cwd:     "",
		Env:     nil,
	})
	require.NoError(t, err)
	defer proc.Kill() //nolint:errcheck

	// Close the PTY master so that Resize will fail.
	require.NoError(t, proc.Close())

	err = proc.Resize(100, 30)
	assert.Error(t, err)
}
