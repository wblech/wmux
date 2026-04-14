package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockProcessStarter struct {
	processes []*mockAddonProcess
}

func (m *mockProcessStarter) Start() (AddonProcess, error) {
	proc := newMockAddonProcess()
	// Pre-load create response so EmulatorFor can complete.
	proc.writeResponse(AddonMethodCreate, "", AddonStatusOK, nil)
	m.processes = append(m.processes, proc)
	return proc, nil
}

func TestAddonManager_EmulatorFor_StartsProcess(t *testing.T) {
	starter := &mockProcessStarter{processes: nil}
	mgr := newAddonManager(starter)

	em := mgr.EmulatorFor("sess-1", 80, 24)
	require.NotNil(t, em)
	assert.Len(t, starter.processes, 1)
}

func TestAddonManager_EmulatorFor_ReusesSingleProcess(t *testing.T) {
	starter := &mockProcessStarter{processes: nil}
	mgr := newAddonManager(starter)

	// Pre-load a second create response for the second EmulatorFor call.
	_ = mgr.EmulatorFor("sess-1", 80, 24)
	starter.processes[0].writeResponse(AddonMethodCreate, "", AddonStatusOK, nil)
	_ = mgr.EmulatorFor("sess-2", 80, 24)
	assert.Len(t, starter.processes, 1) // same process
}

func TestAddonManager_Shutdown(_ *testing.T) {
	starter := &mockProcessStarter{processes: nil}
	mgr := newAddonManager(starter)
	_ = mgr.EmulatorFor("sess-1", 80, 24)

	mgr.Shutdown()
	// Should not panic, process should be nil after shutdown.
}

func TestAddonManager_Shutdown_NilProcess(_ *testing.T) {
	starter := &mockProcessStarter{processes: nil}
	mgr := newAddonManager(starter)

	// Shutdown without ever starting a process should be safe.
	mgr.Shutdown()
}
