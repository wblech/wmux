package session

import (
	"os/exec"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandProcessStarter_Start(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("not supported on Windows")
	}

	starter := NewCommandProcessStarter("cat")

	proc, err := starter.Start()
	require.NoError(t, err)

	_, err = proc.Stdin().Write([]byte("hello"))
	require.NoError(t, err)

	buf := make([]byte, 5)
	n, err := proc.Stdout().Read(buf)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(buf[:n]))

	require.NoError(t, proc.Kill())
}

func TestCommandProcessStarter_StartWithArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("not supported on Windows")
	}

	starter := NewCommandProcessStarter("/bin/sh", "-c", "cat")

	proc, err := starter.Start()
	require.NoError(t, err)

	_, err = proc.Stdin().Write([]byte("hello"))
	require.NoError(t, err)

	buf := make([]byte, 5)
	n, err := proc.Stdout().Read(buf)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(buf[:n]))

	require.NoError(t, proc.Kill())
}

func TestCommandProcessStarter_Start_BadBinary(t *testing.T) {
	starter := NewCommandProcessStarter("nonexistent-binary-xyz")

	_, err := starter.Start()
	require.Error(t, err)

	var execErr *exec.Error
	assert.ErrorAs(t, err, &execErr)
}
