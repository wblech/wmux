package session

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// AddonProcess abstracts a child process for the emulator addon.
type AddonProcess interface {
	Stdin() io.Writer
	Stdout() io.Reader
	Wait() error
	Kill() error
}

// AddonEmulator implements ScreenEmulator by proxying to an external addon process.
type AddonEmulator struct {
	mu        sync.Mutex
	process   AddonProcess
	sessionID string
}

// NewAddonEmulatorWithProcess creates an AddonEmulator for a specific session.
// If process is nil, all operations are no-ops (like NoneEmulator).
func NewAddonEmulatorWithProcess(proc AddonProcess, sessionID string) *AddonEmulator {
	em := &AddonEmulator{
		process:   proc,
		sessionID: sessionID,
	}
	if proc != nil {
		em.sendRequest(AddonMethodCreate, nil)
	}
	return em
}

// Process sends raw PTY output to the addon (fire-and-forget).
func (a *AddonEmulator) Process(data []byte) {
	if a.process == nil {
		return
	}
	a.sendRequest(AddonMethodProcess, data)
}

// Snapshot requests the current terminal state from the addon.
func (a *AddonEmulator) Snapshot() Snapshot {
	if a.process == nil {
		return Snapshot{}
	}
	resp, err := a.sendRequestWithResponse(AddonMethodSnapshot, nil)
	if err != nil {
		return Snapshot{}
	}
	scrollback, viewport, err := DecodeSnapshotPayload(resp)
	if err != nil {
		return Snapshot{}
	}
	return Snapshot{Scrollback: scrollback, Viewport: viewport}
}

// Resize sends new terminal dimensions to the addon and waits for acknowledgement.
func (a *AddonEmulator) Resize(cols, rows int) {
	if a.process == nil {
		return
	}
	payload, _ := json.Marshal(struct {
		Cols int `json:"cols"`
		Rows int `json:"rows"`
	}{Cols: cols, Rows: rows})
	_, _ = a.sendRequestWithResponse(AddonMethodResize, payload)
}

// Destroy tells the addon to remove this session's xterm instance.
func (a *AddonEmulator) Destroy() {
	if a.process == nil {
		return
	}
	a.sendRequest(AddonMethodDestroy, nil)
}

// sendRequest writes a length-prefixed request frame to the addon's stdin.
// Used for fire-and-forget operations (Process, Destroy).
func (a *AddonEmulator) sendRequest(method AddonMethod, payload []byte) {
	frame := EncodeAddonRequest(method, a.sessionID, payload)
	a.mu.Lock()
	defer a.mu.Unlock()

	buf := make([]byte, 4+len(frame))
	binary.BigEndian.PutUint32(buf[:4], uint32(len(frame)))
	copy(buf[4:], frame)
	_, _ = a.process.Stdin().Write(buf)
}

// sendRequestWithResponse writes a request and reads the response.
func (a *AddonEmulator) sendRequestWithResponse(method AddonMethod, payload []byte) ([]byte, error) {
	frame := EncodeAddonRequest(method, a.sessionID, payload)
	a.mu.Lock()
	defer a.mu.Unlock()

	buf := make([]byte, 4+len(frame))
	binary.BigEndian.PutUint32(buf[:4], uint32(len(frame)))
	copy(buf[4:], frame)
	if _, err := a.process.Stdin().Write(buf); err != nil {
		return nil, fmt.Errorf("addon: write request: %w", err)
	}

	respLenBuf := make([]byte, 4)
	if _, err := io.ReadFull(a.process.Stdout(), respLenBuf); err != nil {
		return nil, fmt.Errorf("addon: read response length: %w", err)
	}
	respLen := binary.BigEndian.Uint32(respLenBuf)
	respBuf := make([]byte, respLen)
	if _, err := io.ReadFull(a.process.Stdout(), respBuf); err != nil {
		return nil, fmt.Errorf("addon: read response: %w", err)
	}

	_, _, status, respPayload, err := DecodeAddonResponse(respBuf)
	if err != nil {
		return nil, fmt.Errorf("addon: decode response: %w", err)
	}
	if status != AddonStatusOK {
		return nil, ErrAddonRequestFailed
	}
	return respPayload, nil
}
