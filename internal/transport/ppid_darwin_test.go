//go:build darwin

package transport

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlatformParentPID_InvalidPID(t *testing.T) {
	// PID -1 should cause the sysctl call to fail with an error.
	_, err := platformParentPID(-1)
	assert.Error(t, err)
}

func TestPlatformParentPID_CurrentProcess(t *testing.T) {
	pid := int32(os.Getpid()) //nolint:gosec // safe narrowing in tests

	ppid, err := platformParentPID(pid)
	require.NoError(t, err)
	assert.Positive(t, ppid)
}
