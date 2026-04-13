package session

import (
	"bytes"
	"encoding/binary"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockAddonProcess simulates the addon stdin/stdout for testing.
type mockAddonProcess struct {
	stdin  *bytes.Buffer
	stdout *bytes.Buffer
	mu     sync.Mutex
	exited bool
}

func newMockAddonProcess() *mockAddonProcess {
	return &mockAddonProcess{
		stdin:  &bytes.Buffer{},
		stdout: &bytes.Buffer{},
	}
}

func (m *mockAddonProcess) Stdin() io.Writer  { return m.stdin }
func (m *mockAddonProcess) Stdout() io.Reader { return m.stdout }
func (m *mockAddonProcess) Wait() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exited = true
	return nil
}
func (m *mockAddonProcess) Kill() error { return nil }

// writeResponse writes a length-prefixed response frame into the mock stdout.
func (m *mockAddonProcess) writeResponse(method AddonMethod, sessionID string, status AddonStatus, payload []byte) {
	idBytes := []byte(sessionID)
	frame := make([]byte, 0)
	frame = append(frame, byte(method))
	frame = append(frame, byte(len(idBytes)))
	frame = append(frame, idBytes...)
	frame = append(frame, byte(status))
	pLen := make([]byte, 4)
	binary.BigEndian.PutUint32(pLen, uint32(len(payload)))
	frame = append(frame, pLen...)
	frame = append(frame, payload...)
	fLen := make([]byte, 4)
	binary.BigEndian.PutUint32(fLen, uint32(len(frame)))
	m.stdout.Write(fLen)
	m.stdout.Write(frame)
}

func TestAddonEmulator_Process(t *testing.T) {
	mock := newMockAddonProcess()
	em := NewAddonEmulatorWithProcess(mock, "test-session")

	// Clear the Create request written by the constructor.
	mock.stdin.Reset()

	data := []byte("hello world")
	em.Process(data)

	written := mock.stdin.Bytes()
	require.True(t, len(written) > 4, "expected data in stdin buffer")
	frameLen := binary.BigEndian.Uint32(written[:4])
	frame := written[4 : 4+frameLen]
	assert.Equal(t, byte(AddonMethodProcess), frame[0])
}

func TestAddonEmulator_Snapshot_Nil(t *testing.T) {
	em := NewAddonEmulatorWithProcess(nil, "test-session")
	snap := em.Snapshot()
	assert.Nil(t, snap.Scrollback)
	assert.Nil(t, snap.Viewport)
}

func TestAddonEmulator_Snapshot_WithResponse(t *testing.T) {
	mock := newMockAddonProcess()
	em := NewAddonEmulatorWithProcess(mock, "s1")

	// Prepare the snapshot response that the addon would return.
	snapshotPayload := EncodeSnapshotPayload([]byte("scroll"), []byte("view"))
	mock.writeResponse(AddonMethodSnapshot, "s1", AddonStatusOK, snapshotPayload)

	snap := em.Snapshot()
	assert.Equal(t, []byte("scroll"), snap.Scrollback)
	assert.Equal(t, []byte("view"), snap.Viewport)
}

func TestAddonEmulator_Resize(t *testing.T) {
	mock := newMockAddonProcess()
	em := NewAddonEmulatorWithProcess(mock, "test-session")

	// Clear the Create request written by the constructor.
	mock.stdin.Reset()

	em.Resize(120, 40)

	written := mock.stdin.Bytes()
	require.True(t, len(written) > 4, "expected data in stdin buffer")
	frameLen := binary.BigEndian.Uint32(written[:4])
	frame := written[4 : 4+frameLen]
	assert.Equal(t, byte(AddonMethodResize), frame[0])
}

func TestAddonEmulator_Destroy(t *testing.T) {
	mock := newMockAddonProcess()
	em := NewAddonEmulatorWithProcess(mock, "test-session")

	// Clear the Create request written by the constructor.
	mock.stdin.Reset()

	em.Destroy()

	written := mock.stdin.Bytes()
	require.True(t, len(written) > 4, "expected data in stdin buffer")
	frameLen := binary.BigEndian.Uint32(written[:4])
	frame := written[4 : 4+frameLen]
	assert.Equal(t, byte(AddonMethodDestroy), frame[0])
}

func TestNewAddonEmulator_SendsCreate(t *testing.T) {
	mock := newMockAddonProcess()
	_ = NewAddonEmulatorWithProcess(mock, "new-sess")

	written := mock.stdin.Bytes()
	require.True(t, len(written) > 4, "expected Create frame in stdin")
	frameLen := binary.BigEndian.Uint32(written[:4])
	frame := written[4 : 4+frameLen]
	assert.Equal(t, byte(AddonMethodCreate), frame[0])
}

func TestAddonEmulator_NilProcess_NoOps(t *testing.T) {
	em := NewAddonEmulatorWithProcess(nil, "test")

	// All operations should be safe no-ops.
	em.Process([]byte("data"))
	em.Resize(80, 24)
	em.Destroy()
	snap := em.Snapshot()
	assert.Nil(t, snap.Scrollback)
	assert.Nil(t, snap.Viewport)
}

// Compile-time check: AddonEmulator must satisfy ScreenEmulator.
var _ ScreenEmulator = (*AddonEmulator)(nil)
