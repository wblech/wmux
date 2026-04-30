package daemon

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wblech/wmux/internal/cmdlifecycle"
	"github.com/wblech/wmux/internal/platform/protocol"
)

// stubCmdRepo is a minimal cmdlifecycle.Repository implementation that
// drops Save calls. Used to satisfy the constructor without writing to disk.
type stubCmdRepo struct{}

func (stubCmdRepo) Save(_ string, _ cmdlifecycle.SessionState) {}

// TestMsgAttach_IncludesCommandHistory verifies that handleAttach includes
// the cmdSvc snapshot (history and in-flight command) in the AttachResponse
// payload when the daemon has command-lifecycle state for the session.
func TestMsgAttach_IncludesCommandHistory(t *testing.T) {
	const sessID = "history-sess"

	// Deterministic clock so DurationMs is computable: each tick == 1s.
	tick := time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC)
	now := func() time.Time {
		t := tick
		tick = tick.Add(time.Second)
		return t
	}
	cmdSvc := cmdlifecycle.NewService(stubCmdRepo{}, cmdlifecycle.WithClock(now))

	require.NoError(t, cmdSvc.Register(sessID))
	// First command: prompt → user-typing → exec (row 5) → done (row 8, exit 0).
	require.NoError(t, cmdSvc.HandleOSC(sessID, 'A', 0, 1))
	require.NoError(t, cmdSvc.HandleOSC(sessID, 'B', 0, 2))
	require.NoError(t, cmdSvc.HandleOSC(sessID, 'C', 0, 5))
	require.NoError(t, cmdSvc.HandleOSC(sessID, 'D', 0, 8))
	// Second command in-flight: prompt → user-typing → exec (row 10), no D yet.
	require.NoError(t, cmdSvc.HandleOSC(sessID, 'A', 0, 9))
	require.NoError(t, cmdSvc.HandleOSC(sessID, 'B', 0, 10))
	require.NoError(t, cmdSvc.HandleOSC(sessID, 'C', 0, 10))

	sm := &snapshotSpySessionManager{
		noopSessionManager: noopSessionManager{},
		snapshotData:       SnapshotData{Replay: nil},
	}
	bus := &spyEventBus{mu: sync.Mutex{}, events: nil}
	d := newTestDaemonUnit(sm, bus, map[string]map[string]struct{}{})
	d.cmdSvc = cmdSvc

	mockCtrl := &mockControlConn{
		frames:   nil,
		writeMu:  sync.Mutex{},
		written:  nil,
		writeErr: nil,
	}
	mockClient := &mockConnectedClient{id: "test-client", ctrl: mockCtrl}

	reqPayload, _ := json.Marshal(SessionIDRequest{SessionID: sessID})
	d.handleAttach(mockClient, protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgAttach,
		Payload: reqPayload,
	})

	require.Len(t, mockCtrl.written, 1)
	resp := mockCtrl.written[0]
	require.Equal(t, protocol.MsgOK, resp.Type)

	var attachData AttachResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &attachData))

	require.Len(t, attachData.CommandHistory, 1)
	assert.Equal(t, 0, attachData.CommandHistory[0].ExitCode)
	assert.Equal(t, int64(5), attachData.CommandHistory[0].StartRow)
	assert.Equal(t, int64(8), attachData.CommandHistory[0].EndRow)
	assert.Equal(t, int64(1000), attachData.CommandHistory[0].DurationMs)

	require.NotNil(t, attachData.InFlightCommand)
	assert.Equal(t, int64(10), attachData.InFlightCommand.StartRow)
	assert.Equal(t, int64(0), attachData.InFlightCommand.DurationMs)
}

// TestMsgAttach_NoCmdSvc_OmitsHistoryFields verifies that when the daemon
// has no cmdSvc wired, the response still serializes cleanly with empty
// CommandHistory and nil InFlightCommand.
func TestMsgAttach_NoCmdSvc_OmitsHistoryFields(t *testing.T) {
	const sessID = "no-cmdsvc"

	sm := &snapshotSpySessionManager{
		noopSessionManager: noopSessionManager{},
		snapshotData:       SnapshotData{Replay: nil},
	}
	bus := &spyEventBus{mu: sync.Mutex{}, events: nil}
	d := newTestDaemonUnit(sm, bus, map[string]map[string]struct{}{})

	mockCtrl := &mockControlConn{
		frames:   nil,
		writeMu:  sync.Mutex{},
		written:  nil,
		writeErr: nil,
	}
	mockClient := &mockConnectedClient{id: "test-client", ctrl: mockCtrl}

	reqPayload, _ := json.Marshal(SessionIDRequest{SessionID: sessID})
	d.handleAttach(mockClient, protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgAttach,
		Payload: reqPayload,
	})

	require.Len(t, mockCtrl.written, 1)
	var attachData AttachResponse
	require.NoError(t, json.Unmarshal(mockCtrl.written[0].Payload, &attachData))
	assert.Empty(t, attachData.CommandHistory)
	assert.Nil(t, attachData.InFlightCommand)
}
