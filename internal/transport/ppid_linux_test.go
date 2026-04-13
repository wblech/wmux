//go:build linux

package transport

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlatformParentPID_InvalidPID(t *testing.T) {
	// A nonexistent PID should cause os.ReadFile to fail.
	_, err := platformParentPID(99999999)
	assert.Error(t, err)
}

func TestPlatformParentPID_CurrentProcess(t *testing.T) {
	pid := int32(os.Getpid()) //nolint:gosec // safe narrowing in tests

	ppid, err := platformParentPID(pid)
	require.NoError(t, err)
	assert.Positive(t, ppid)
}
