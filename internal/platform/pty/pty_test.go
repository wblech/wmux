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
