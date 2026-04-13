package session

import (
	"encoding/binary"
	"encoding/json"
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

// Resize sends new terminal dimensions to the addon.
func (a *AddonEmulator) Resize(cols, rows int) {
	if a.process == nil {
		return
	}
	payload, _ := json.Marshal(struct {
		Cols int `json:"cols"`
		Rows int `json:"rows"`
	}{Cols: cols, Rows: rows})
	a.sendRequest(AddonMethodResize, payload)
}

// Destroy tells the addon to remove this session's xterm instance.
func (a *AddonEmulator) Destroy() {
	if a.process == nil {
		return
	}
	a.sendRequest(AddonMethodDestroy, nil)
}

// sendRequest writes a length-prefixed request frame to the addon's stdin.
func (a *AddonEmulator) sendRequest(method AddonMethod, payload []byte) {
	frame := EncodeAddonRequest(method, a.sessionID, payload)
	a.mu.Lock()
	defer a.mu.Unlock()
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(frame)))
	_, _ = a.process.Stdin().Write(lenBuf)
	_, _ = a.process.Stdin().Write(frame)
}

// sendRequestWithResponse writes a request and reads the response.
func (a *AddonEmulator) sendRequestWithResponse(method AddonMethod, payload []byte) ([]byte, error) {
	frame := EncodeAddonRequest(method, a.sessionID, payload)
	a.mu.Lock()
	defer a.mu.Unlock()

	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(frame)))
	if _, err := a.process.Stdin().Write(lenBuf); err != nil {
		return nil, err
	}
	if _, err := a.process.Stdin().Write(frame); err != nil {
		return nil, err
	}

	respLenBuf := make([]byte, 4)
	if _, err := io.ReadFull(a.process.Stdout(), respLenBuf); err != nil {
		return nil, err
	}
	respLen := binary.BigEndian.Uint32(respLenBuf)
	respBuf := make([]byte, respLen)
	if _, err := io.ReadFull(a.process.Stdout(), respBuf); err != nil {
		return nil, err
	}

	_, _, status, respPayload, err := DecodeAddonResponse(respBuf)
	if err != nil {
		return nil, err
	}
	if status != AddonStatusOK {
		return nil, ErrAddonFrameTooShort
	}
	return respPayload, nil
}
