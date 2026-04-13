# Phase 2 Sub-plan 1: Emulator Addon Protocol + xterm Addon

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Define a generic binary protocol for daemon ↔ emulator addon communication via stdin/stdout, implement the `AddonEmulator` in Go, and build the `wmux-emulator-xterm` Node addon that manages N xterm headless instances in a single process.

**Architecture:** The daemon spawns one addon process (discovered via `$PATH` or config) on the first session `create` with a non-`none` backend. Communication uses length-prefixed binary frames over stdin/stdout. The `AddonEmulator` struct implements the existing `session.ScreenEmulator` interface by proxying calls to the addon process. The xterm addon is a TypeScript Node program that reads binary frames from stdin, manages multiple `@xterm/headless` Terminal instances keyed by session ID, and writes response frames to stdout. If the addon crashes, the daemon detects EOF, respawns it, re-creates all active instances, and re-feeds buffered output.

**Tech Stack:** Go 1.25+, Node.js 20+, TypeScript, `@xterm/headless`

**Prerequisites:** Phase 1 complete. Key interfaces:
- `session.ScreenEmulator` — `Process([]byte)`, `Snapshot() Snapshot`, `Resize(cols, rows int)`
- `session.Snapshot` — `Scrollback []byte`, `Viewport []byte`
- `config.EmulatorConfig` — currently `Backend string`

---

## File Structure

```
internal/
├── session/
│   ├── entity.go              # EXISTING: ScreenEmulator interface, Snapshot
│   ├── emulator.go            # EXISTING: NoneEmulator
│   ├── addon_emulator.go      # NEW: AddonEmulator (proxies to external process)
│   └── addon_emulator_test.go # NEW: tests with mock process
├── platform/
│   └── config/
│       └── config.go          # MODIFIED: add EmulatorXtermConfig

addons/
└── xterm/
    ├── package.json           # NEW: Node project
    ├── tsconfig.json          # NEW: TypeScript config
    ├── src/
    │   ├── index.ts           # NEW: entry point, main loop
    │   ├── protocol.ts        # NEW: binary frame parser/serializer
    │   └── manager.ts         # NEW: xterm instance manager
    └── test/
        ├── protocol.test.ts   # NEW: protocol tests
        └── manager.test.ts    # NEW: manager tests
```

---

### Task 1: Addon binary protocol — Go encoder/decoder

**Files:**
- Create: `internal/session/addon_protocol.go`
- Create: `internal/session/addon_protocol_test.go`

This task defines the binary frame format for daemon ↔ addon communication. Frame format:

```
Request:  [method:1][session_id_len:1][session_id:N][payload_len:4][payload:N]
Response: [method:1][session_id_len:1][session_id:N][status:1][payload_len:4][payload:N]
Snapshot: [scrollback_len:4][scrollback:N][viewport:N]
```

Methods: `0x01` create, `0x02` process, `0x03` snapshot, `0x04` resize, `0x05` destroy, `0x06` shutdown.
Status: `0x00` ok, `0x01` error.

- [ ] **Step 1: Write failing tests for EncodeAddonRequest / DecodeAddonRequest**

```go
// internal/session/addon_protocol_test.go
package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeAddonRequest_Create(t *testing.T) {
	frame := EncodeAddonRequest(AddonMethodCreate, "sess-1", []byte(`{"cols":80,"rows":24}`))

	assert.Equal(t, byte(AddonMethodCreate), frame[0])
	assert.Equal(t, byte(6), frame[1]) // len("sess-1")
	assert.Equal(t, "sess-1", string(frame[2:8]))
	// payload_len (4 bytes big-endian) + payload
	payloadLen := int(frame[8])<<24 | int(frame[9])<<16 | int(frame[10])<<8 | int(frame[11])
	assert.Equal(t, 21, payloadLen) // len(`{"cols":80,"rows":24}`)
	assert.Equal(t, `{"cols":80,"rows":24}`, string(frame[12:]))
}

func TestEncodeAddonRequest_Process(t *testing.T) {
	data := []byte("hello terminal output")
	frame := EncodeAddonRequest(AddonMethodProcess, "s1", data)

	assert.Equal(t, byte(AddonMethodProcess), frame[0])
	assert.Equal(t, byte(2), frame[1])
	assert.Equal(t, "s1", string(frame[2:4]))
	payloadLen := int(frame[4])<<24 | int(frame[5])<<16 | int(frame[6])<<8 | int(frame[7])
	assert.Equal(t, len(data), payloadLen)
	assert.Equal(t, data, frame[8:])
}

func TestEncodeAddonRequest_Shutdown(t *testing.T) {
	frame := EncodeAddonRequest(AddonMethodShutdown, "", nil)

	assert.Equal(t, byte(AddonMethodShutdown), frame[0])
	assert.Equal(t, byte(0), frame[1]) // no session_id
	// payload_len = 0
	assert.Equal(t, []byte{0, 0, 0, 0}, frame[2:6])
}

func TestDecodeAddonResponse_OK(t *testing.T) {
	// Build a valid response frame: method=0x01, session_id="s1", status=0x00, payload=empty
	resp := []byte{
		0x01,       // method
		2,          // session_id_len
		's', '1',   // session_id
		0x00,       // status ok
		0, 0, 0, 0, // payload_len = 0
	}
	method, sessID, status, payload, err := DecodeAddonResponse(resp)
	require.NoError(t, err)
	assert.Equal(t, AddonMethodCreate, method)
	assert.Equal(t, "s1", sessID)
	assert.Equal(t, AddonStatusOK, status)
	assert.Empty(t, payload)
}

func TestDecodeAddonResponse_WithPayload(t *testing.T) {
	// Snapshot response with scrollback + viewport
	scrollback := []byte("scrollback data")
	viewport := []byte("viewport data")
	snapshotPayload := EncodeSnapshotPayload(scrollback, viewport)

	resp := make([]byte, 0)
	resp = append(resp, 0x03) // snapshot method
	resp = append(resp, 2)    // session_id_len
	resp = append(resp, 's', '1')
	resp = append(resp, 0x00) // status ok
	pLen := len(snapshotPayload)
	resp = append(resp, byte(pLen>>24), byte(pLen>>16), byte(pLen>>8), byte(pLen))
	resp = append(resp, snapshotPayload...)

	method, sessID, status, payload, err := DecodeAddonResponse(resp)
	require.NoError(t, err)
	assert.Equal(t, AddonMethodSnapshot, method)
	assert.Equal(t, "s1", sessID)
	assert.Equal(t, AddonStatusOK, status)

	sb, vp, err := DecodeSnapshotPayload(payload)
	require.NoError(t, err)
	assert.Equal(t, scrollback, sb)
	assert.Equal(t, viewport, vp)
}

func TestDecodeAddonResponse_Error(t *testing.T) {
	resp := []byte{0x01, 2, 's', '1', 0x01, 0, 0, 0, 0}
	_, _, status, _, err := DecodeAddonResponse(resp)
	require.NoError(t, err)
	assert.Equal(t, AddonStatusError, status)
}

func TestDecodeAddonResponse_TooShort(t *testing.T) {
	_, _, _, _, err := DecodeAddonResponse([]byte{0x01})
	assert.Error(t, err)
}

func TestEncodeDecodeSnapshotPayload(t *testing.T) {
	scrollback := []byte("line1\nline2\nline3")
	viewport := []byte("\x1b[1;1Hscreen content")

	encoded := EncodeSnapshotPayload(scrollback, viewport)
	sb, vp, err := DecodeSnapshotPayload(encoded)
	require.NoError(t, err)
	assert.Equal(t, scrollback, sb)
	assert.Equal(t, viewport, vp)
}

func TestDecodeSnapshotPayload_Empty(t *testing.T) {
	sb, vp, err := DecodeSnapshotPayload(nil)
	require.NoError(t, err)
	assert.Empty(t, sb)
	assert.Empty(t, vp)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/session/ -run TestEncodeAddon -v && go test ./internal/session/ -run TestDecodeAddon -v && go test ./internal/session/ -run TestEncodeDecodeSnapshot -v`
Expected: FAIL — functions not defined

- [ ] **Step 3: Implement the protocol encoder/decoder**

```go
// internal/session/addon_protocol.go
package session

import (
	"encoding/binary"
	"errors"
)

// AddonMethod identifies the operation in an addon protocol frame.
type AddonMethod byte

// Addon method constants.
const (
	AddonMethodCreate   AddonMethod = 0x01
	AddonMethodProcess  AddonMethod = 0x02
	AddonMethodSnapshot AddonMethod = 0x03
	AddonMethodResize   AddonMethod = 0x04
	AddonMethodDestroy  AddonMethod = 0x05
	AddonMethodShutdown AddonMethod = 0x06
)

// AddonStatus indicates success or failure in an addon response.
type AddonStatus byte

// Addon status constants.
const (
	AddonStatusOK    AddonStatus = 0x00
	AddonStatusError AddonStatus = 0x01
)

// Sentinel errors for addon protocol operations.
var (
	ErrAddonFrameTooShort    = errors.New("addon: frame too short")
	ErrAddonSnapshotTooShort = errors.New("addon: snapshot payload too short")
)

// EncodeAddonRequest builds a binary request frame:
// [method:1][session_id_len:1][session_id:N][payload_len:4][payload:N]
func EncodeAddonRequest(method AddonMethod, sessionID string, payload []byte) []byte {
	idBytes := []byte(sessionID)
	frame := make([]byte, 0, 1+1+len(idBytes)+4+len(payload))
	frame = append(frame, byte(method))
	frame = append(frame, byte(len(idBytes)))
	frame = append(frame, idBytes...)
	pLen := make([]byte, 4)
	binary.BigEndian.PutUint32(pLen, uint32(len(payload)))
	frame = append(frame, pLen...)
	frame = append(frame, payload...)
	return frame
}

// DecodeAddonResponse parses a binary response frame:
// [method:1][session_id_len:1][session_id:N][status:1][payload_len:4][payload:N]
func DecodeAddonResponse(data []byte) (method AddonMethod, sessionID string, status AddonStatus, payload []byte, err error) {
	if len(data) < 3 { // minimum: method + id_len(0) + status
		return 0, "", 0, nil, ErrAddonFrameTooShort
	}
	method = AddonMethod(data[0])
	idLen := int(data[1])
	offset := 2
	if len(data) < offset+idLen+1+4 {
		return 0, "", 0, nil, ErrAddonFrameTooShort
	}
	sessionID = string(data[offset : offset+idLen])
	offset += idLen
	status = AddonStatus(data[offset])
	offset++
	if len(data) < offset+4 {
		return 0, "", 0, nil, ErrAddonFrameTooShort
	}
	pLen := binary.BigEndian.Uint32(data[offset : offset+4])
	offset += 4
	if uint32(len(data)-offset) < pLen {
		return 0, "", 0, nil, ErrAddonFrameTooShort
	}
	payload = data[offset : offset+int(pLen)]
	return method, sessionID, status, payload, nil
}

// EncodeSnapshotPayload builds: [scrollback_len:4][scrollback:N][viewport:N]
func EncodeSnapshotPayload(scrollback, viewport []byte) []byte {
	buf := make([]byte, 4+len(scrollback)+len(viewport))
	binary.BigEndian.PutUint32(buf[:4], uint32(len(scrollback)))
	copy(buf[4:], scrollback)
	copy(buf[4+len(scrollback):], viewport)
	return buf
}

// DecodeSnapshotPayload parses: [scrollback_len:4][scrollback:N][viewport:N]
func DecodeSnapshotPayload(data []byte) (scrollback, viewport []byte, err error) {
	if len(data) == 0 {
		return nil, nil, nil
	}
	if len(data) < 4 {
		return nil, nil, ErrAddonSnapshotTooShort
	}
	sbLen := binary.BigEndian.Uint32(data[:4])
	if uint32(len(data)-4) < sbLen {
		return nil, nil, ErrAddonSnapshotTooShort
	}
	scrollback = data[4 : 4+sbLen]
	viewport = data[4+sbLen:]
	return scrollback, viewport, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/session/ -run "TestEncodeAddon|TestDecodeAddon|TestEncodeDecodeSnapshot" -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add internal/session/addon_protocol.go internal/session/addon_protocol_test.go
git -c commit.gpgsign=false commit -m "feat(session): add addon binary protocol encoder/decoder"
```

---

### Task 2: AddonEmulator — process manager and ScreenEmulator proxy

**Files:**
- Create: `internal/session/addon_emulator.go`
- Create: `internal/session/addon_emulator_test.go`

The `AddonEmulator` manages the addon child process and proxies `ScreenEmulator` calls. It uses a `ProcessStarter` interface so tests can inject a mock process.

- [ ] **Step 1: Write failing tests for AddonEmulator**

```go
// internal/session/addon_emulator_test.go
package session

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockAddonProcess simulates the addon stdin/stdout for testing.
type mockAddonProcess struct {
	stdin  *bytes.Buffer // daemon writes here (addon reads)
	stdout *bytes.Buffer // addon writes here (daemon reads)
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

// writeResponse writes a response frame into the mock stdout.
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
	// Write length-prefixed frame to stdout
	fLen := make([]byte, 4)
	binary.BigEndian.PutUint32(fLen, uint32(len(frame)))
	m.stdout.Write(fLen)
	m.stdout.Write(frame)
}

func TestAddonEmulator_Process(t *testing.T) {
	mock := newMockAddonProcess()
	em := NewAddonEmulatorWithProcess(mock, "test-session")

	data := []byte("hello world")
	em.Process(data)

	// Verify the request was written to stdin
	// Read length prefix
	written := mock.stdin.Bytes()
	require.True(t, len(written) > 4)
	frameLen := binary.BigEndian.Uint32(written[:4])
	frame := written[4 : 4+frameLen]
	assert.Equal(t, byte(AddonMethodProcess), frame[0])
}

func TestAddonEmulator_Snapshot_Empty(t *testing.T) {
	em := NewAddonEmulatorWithProcess(nil, "test-session")
	snap := em.Snapshot()
	assert.Nil(t, snap.Scrollback)
	assert.Nil(t, snap.Viewport)
}

func TestAddonEmulator_Resize(t *testing.T) {
	mock := newMockAddonProcess()
	em := NewAddonEmulatorWithProcess(mock, "test-session")

	em.Resize(120, 40)

	written := mock.stdin.Bytes()
	require.True(t, len(written) > 4)
	frameLen := binary.BigEndian.Uint32(written[:4])
	frame := written[4 : 4+frameLen]
	assert.Equal(t, byte(AddonMethodResize), frame[0])

	// Decode the payload to verify cols/rows
	_, _, _, payload, err := DecodeAddonResponse(append(frame, 0x00, 0, 0, 0, 0)) // hack to reuse decoder shape
	_ = payload
	_ = err
	// Just verify the method byte is correct
	assert.Equal(t, byte(AddonMethodResize), frame[0])
}

func TestNewAddonEmulator_CreatesSession(t *testing.T) {
	mock := newMockAddonProcess()
	// Pre-load a create response
	mock.writeResponse(AddonMethodCreate, "new-sess", AddonStatusOK, nil)

	em := NewAddonEmulatorWithProcess(mock, "new-sess")
	require.NotNil(t, em)

	// Verify create request was sent
	written := mock.stdin.Bytes()
	require.True(t, len(written) > 4)
	frameLen := binary.BigEndian.Uint32(written[:4])
	frame := written[4 : 4+frameLen]
	assert.Equal(t, byte(AddonMethodCreate), frame[0])
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/session/ -run TestAddonEmulator -v`
Expected: FAIL — `NewAddonEmulatorWithProcess` not defined

- [ ] **Step 3: Implement AddonEmulator**

```go
// internal/session/addon_emulator.go
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
// It communicates using length-prefixed binary frames over stdin/stdout.
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

// Process sends raw PTY output to the addon (fire-and-forget, no response expected).
func (a *AddonEmulator) Process(data []byte) {
	if a.process == nil {
		return
	}
	a.sendRequest(AddonMethodProcess, data)
}

// Snapshot requests the current terminal state from the addon.
// Returns empty snapshot if the process is nil or communication fails.
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
// Used for fire-and-forget operations (process, resize, create, destroy).
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
// Used only for snapshot (the only synchronous addon call).
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

	// Read length-prefixed response
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
		return nil, ErrAddonFrameTooShort // reuse for now
	}
	return respPayload, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/session/ -run TestAddonEmulator -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add internal/session/addon_emulator.go internal/session/addon_emulator_test.go
git -c commit.gpgsign=false commit -m "feat(session): add AddonEmulator proxying to external process"
```

---

### Task 3: Config — add emulator xterm settings

**Files:**
- Modify: `internal/platform/config/config.go`
- Modify: `internal/platform/config/config_test.go`

- [ ] **Step 1: Write failing test for new config fields**

Add to `internal/platform/config/config_test.go`:

```go
func TestLoad_EmulatorXtermConfig(t *testing.T) {
	content := `
[emulator]
backend = "xterm"

[emulator.xterm]
bin = "/usr/local/bin/wmux-emulator-xterm"
`
	path := filepath.Join(t.TempDir(), "wmux.toml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "xterm", cfg.Emulator.Backend)
	assert.Equal(t, "/usr/local/bin/wmux-emulator-xterm", cfg.Emulator.Xterm.Bin)
}

func TestDefaults_EmulatorXterm(t *testing.T) {
	cfg := defaults()
	assert.Equal(t, "none", cfg.Emulator.Backend)
	assert.Empty(t, cfg.Emulator.Xterm.Bin)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/platform/config/ -run "TestLoad_EmulatorXterm|TestDefaults_EmulatorXterm" -v`
Expected: FAIL — `Xterm` field not defined

- [ ] **Step 3: Add XtermConfig to config**

In `internal/platform/config/config.go`, add the struct and update EmulatorConfig:

```go
// XtermEmulatorConfig holds xterm addon settings.
type XtermEmulatorConfig struct {
	Bin string `koanf:"bin"`
}
```

Update `EmulatorConfig`:

```go
type EmulatorConfig struct {
	Backend string              `koanf:"backend"`
	Xterm   XtermEmulatorConfig `koanf:"xterm"`
}
```

No change needed in `defaults()` — the zero value of `XtermEmulatorConfig` is correct (empty Bin = use $PATH).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/platform/config/ -v`
Expected: ALL PASS

- [ ] **Step 5: Run full test suite**

Run: `make test`
Expected: ALL PASS

- [ ] **Step 6: Commit**

```bash
git add internal/platform/config/config.go internal/platform/config/config_test.go
git -c commit.gpgsign=false commit -m "feat(config): add emulator.xterm.bin setting"
```

---

### Task 4: xterm addon — Node project setup and binary protocol

**Files:**
- Create: `addons/xterm/package.json`
- Create: `addons/xterm/tsconfig.json`
- Create: `addons/xterm/src/protocol.ts`
- Create: `addons/xterm/test/protocol.test.ts`

- [ ] **Step 1: Create Node project structure**

```bash
mkdir -p addons/xterm/src addons/xterm/test
```

- [ ] **Step 2: Write package.json**

```json
{
  "name": "wmux-emulator-xterm",
  "version": "0.1.0",
  "private": true,
  "type": "module",
  "main": "dist/index.js",
  "bin": {
    "wmux-emulator-xterm": "dist/index.js"
  },
  "scripts": {
    "build": "tsc",
    "test": "node --test test/*.test.ts --loader ts-node/esm",
    "start": "node dist/index.js"
  },
  "dependencies": {
    "@xterm/headless": "^6.0.0"
  },
  "devDependencies": {
    "typescript": "^5.4.0",
    "ts-node": "^10.9.0",
    "@types/node": "^20.0.0"
  }
}
```

- [ ] **Step 3: Write tsconfig.json**

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ES2022",
    "moduleResolution": "node",
    "outDir": "dist",
    "rootDir": "src",
    "strict": true,
    "esModuleInterop": true,
    "declaration": true,
    "sourceMap": true
  },
  "include": ["src/**/*"]
}
```

- [ ] **Step 4: Implement protocol.ts**

```typescript
// addons/xterm/src/protocol.ts

export const METHOD_CREATE = 0x01;
export const METHOD_PROCESS = 0x02;
export const METHOD_SNAPSHOT = 0x03;
export const METHOD_RESIZE = 0x04;
export const METHOD_DESTROY = 0x05;
export const METHOD_SHUTDOWN = 0x06;

export const STATUS_OK = 0x00;
export const STATUS_ERROR = 0x01;

export interface AddonRequest {
  method: number;
  sessionId: string;
  payload: Buffer;
}

export interface AddonResponse {
  method: number;
  sessionId: string;
  status: number;
  payload: Buffer;
}

/**
 * Decode a request frame (after length prefix has been stripped):
 * [method:1][session_id_len:1][session_id:N][payload_len:4][payload:N]
 */
export function decodeRequest(buf: Buffer): AddonRequest {
  let offset = 0;
  const method = buf[offset++];
  const idLen = buf[offset++];
  const sessionId = buf.subarray(offset, offset + idLen).toString("utf-8");
  offset += idLen;
  const payloadLen = buf.readUInt32BE(offset);
  offset += 4;
  const payload = buf.subarray(offset, offset + payloadLen);
  return { method, sessionId, payload };
}

/**
 * Encode a response frame:
 * [method:1][session_id_len:1][session_id:N][status:1][payload_len:4][payload:N]
 */
export function encodeResponse(resp: AddonResponse): Buffer {
  const idBuf = Buffer.from(resp.sessionId, "utf-8");
  const totalLen = 1 + 1 + idBuf.length + 1 + 4 + resp.payload.length;
  const buf = Buffer.alloc(totalLen);
  let offset = 0;
  buf[offset++] = resp.method;
  buf[offset++] = idBuf.length;
  idBuf.copy(buf, offset);
  offset += idBuf.length;
  buf[offset++] = resp.status;
  buf.writeUInt32BE(resp.payload.length, offset);
  offset += 4;
  resp.payload.copy(buf, offset);
  return buf;
}

/**
 * Encode a snapshot payload: [scrollback_len:4][scrollback:N][viewport:N]
 */
export function encodeSnapshotPayload(
  scrollback: Buffer,
  viewport: Buffer
): Buffer {
  const buf = Buffer.alloc(4 + scrollback.length + viewport.length);
  buf.writeUInt32BE(scrollback.length, 0);
  scrollback.copy(buf, 4);
  viewport.copy(buf, 4 + scrollback.length);
  return buf;
}

/**
 * Read one length-prefixed frame from a buffer starting at offset.
 * Returns the frame and new offset, or null if not enough data.
 */
export function readFrame(
  buf: Buffer,
  offset: number
): { frame: Buffer; newOffset: number } | null {
  if (buf.length - offset < 4) return null;
  const frameLen = buf.readUInt32BE(offset);
  if (buf.length - offset - 4 < frameLen) return null;
  const frame = buf.subarray(offset + 4, offset + 4 + frameLen);
  return { frame, newOffset: offset + 4 + frameLen };
}
```

- [ ] **Step 5: Write protocol tests**

```typescript
// addons/xterm/test/protocol.test.ts
import { describe, it } from "node:test";
import * as assert from "node:assert/strict";
import {
  decodeRequest,
  encodeResponse,
  encodeSnapshotPayload,
  readFrame,
  METHOD_CREATE,
  METHOD_SNAPSHOT,
  STATUS_OK,
} from "../src/protocol.js";

describe("protocol", () => {
  it("decodes a create request", () => {
    // method=0x01, session_id_len=2, session_id="s1", payload_len=21, payload=JSON
    const payload = Buffer.from('{"cols":80,"rows":24}');
    const buf = Buffer.alloc(1 + 1 + 2 + 4 + payload.length);
    let offset = 0;
    buf[offset++] = METHOD_CREATE;
    buf[offset++] = 2;
    buf.write("s1", offset, "utf-8");
    offset += 2;
    buf.writeUInt32BE(payload.length, offset);
    offset += 4;
    payload.copy(buf, offset);

    const req = decodeRequest(buf);
    assert.equal(req.method, METHOD_CREATE);
    assert.equal(req.sessionId, "s1");
    assert.deepEqual(JSON.parse(req.payload.toString()), {
      cols: 80,
      rows: 24,
    });
  });

  it("encodes a response", () => {
    const resp = encodeResponse({
      method: METHOD_CREATE,
      sessionId: "s1",
      status: STATUS_OK,
      payload: Buffer.alloc(0),
    });
    assert.equal(resp[0], METHOD_CREATE);
    assert.equal(resp[1], 2); // id len
    assert.equal(resp.subarray(2, 4).toString(), "s1");
    assert.equal(resp[4], STATUS_OK);
    assert.equal(resp.readUInt32BE(5), 0); // empty payload
  });

  it("encodes snapshot payload roundtrip", () => {
    const sb = Buffer.from("scrollback");
    const vp = Buffer.from("viewport");
    const encoded = encodeSnapshotPayload(sb, vp);
    const sbLen = encoded.readUInt32BE(0);
    assert.equal(sbLen, sb.length);
    assert.deepEqual(encoded.subarray(4, 4 + sbLen), sb);
    assert.deepEqual(encoded.subarray(4 + sbLen), vp);
  });

  it("reads a length-prefixed frame", () => {
    const inner = Buffer.from("hello");
    const buf = Buffer.alloc(4 + inner.length);
    buf.writeUInt32BE(inner.length, 0);
    inner.copy(buf, 4);
    const result = readFrame(buf, 0);
    assert.ok(result);
    assert.deepEqual(result.frame, inner);
    assert.equal(result.newOffset, 4 + inner.length);
  });

  it("returns null when frame is incomplete", () => {
    const buf = Buffer.from([0, 0, 0, 10, 1, 2]); // says 10 bytes but only 2
    assert.equal(readFrame(buf, 0), null);
  });
});
```

- [ ] **Step 6: Install dependencies and run tests**

```bash
cd addons/xterm && npm install && npm test
```

Expected: ALL PASS

- [ ] **Step 7: Commit**

```bash
git add addons/xterm/package.json addons/xterm/tsconfig.json addons/xterm/src/protocol.ts addons/xterm/test/protocol.test.ts
git -c commit.gpgsign=false commit -m "feat(addon-xterm): add Node project with binary protocol encoder/decoder"
```

---

### Task 5: xterm addon — instance manager and main loop

**Files:**
- Create: `addons/xterm/src/manager.ts`
- Create: `addons/xterm/src/index.ts`
- Create: `addons/xterm/test/manager.test.ts`

- [ ] **Step 1: Implement the xterm instance manager**

```typescript
// addons/xterm/src/manager.ts
import { Terminal } from "@xterm/headless";
import { encodeSnapshotPayload } from "./protocol.js";

export class InstanceManager {
  private instances: Map<string, Terminal> = new Map();

  create(sessionId: string, cols: number, rows: number): void {
    if (this.instances.has(sessionId)) {
      return;
    }
    const term = new Terminal({ cols, rows, scrollback: 10000 });
    this.instances.set(sessionId, term);
  }

  process(sessionId: string, data: Buffer): void {
    const term = this.instances.get(sessionId);
    if (!term) return;
    term.write(data);
  }

  snapshot(sessionId: string): Buffer {
    const term = this.instances.get(sessionId);
    if (!term) return encodeSnapshotPayload(Buffer.alloc(0), Buffer.alloc(0));

    const buffer = term.buffer.active;
    const lines: string[] = [];

    // Scrollback: lines from 0 to baseY
    for (let i = 0; i < buffer.baseY; i++) {
      const line = buffer.getLine(i);
      if (line) lines.push(line.translateToString(true));
    }
    const scrollback = Buffer.from(lines.join("\n"), "utf-8");

    // Viewport: lines from baseY to baseY + rows
    const vpLines: string[] = [];
    for (let i = buffer.baseY; i < buffer.baseY + term.rows; i++) {
      const line = buffer.getLine(i);
      if (line) vpLines.push(line.translateToString(true));
    }
    const viewport = Buffer.from(vpLines.join("\n"), "utf-8");

    return encodeSnapshotPayload(scrollback, viewport);
  }

  resize(sessionId: string, cols: number, rows: number): void {
    const term = this.instances.get(sessionId);
    if (!term) return;
    term.resize(cols, rows);
  }

  destroy(sessionId: string): void {
    const term = this.instances.get(sessionId);
    if (!term) return;
    term.dispose();
    this.instances.delete(sessionId);
  }

  destroyAll(): void {
    for (const [id] of this.instances) {
      this.destroy(id);
    }
  }

  size(): number {
    return this.instances.size;
  }
}
```

- [ ] **Step 2: Write manager tests**

```typescript
// addons/xterm/test/manager.test.ts
import { describe, it, beforeEach } from "node:test";
import * as assert from "node:assert/strict";
import { InstanceManager } from "../src/manager.js";

describe("InstanceManager", () => {
  let mgr: InstanceManager;

  beforeEach(() => {
    mgr = new InstanceManager();
  });

  it("creates and destroys instances", () => {
    mgr.create("s1", 80, 24);
    assert.equal(mgr.size(), 1);
    mgr.destroy("s1");
    assert.equal(mgr.size(), 0);
  });

  it("ignores duplicate create", () => {
    mgr.create("s1", 80, 24);
    mgr.create("s1", 120, 40); // should not replace
    assert.equal(mgr.size(), 1);
  });

  it("processes data and returns snapshot", () => {
    mgr.create("s1", 80, 24);
    mgr.process("s1", Buffer.from("hello world\r\n"));
    const snap = mgr.snapshot("s1");
    assert.ok(snap.length > 0);
  });

  it("returns empty snapshot for unknown session", () => {
    const snap = mgr.snapshot("unknown");
    // Should be [0,0,0,0] = empty scrollback + empty viewport
    assert.equal(snap.readUInt32BE(0), 0);
  });

  it("resizes instance", () => {
    mgr.create("s1", 80, 24);
    mgr.resize("s1", 120, 40);
    // No assertion beyond no-crash — resize is internal to xterm
  });

  it("destroyAll cleans up all instances", () => {
    mgr.create("s1", 80, 24);
    mgr.create("s2", 80, 24);
    mgr.create("s3", 80, 24);
    mgr.destroyAll();
    assert.equal(mgr.size(), 0);
  });
});
```

- [ ] **Step 3: Implement the main loop (index.ts)**

```typescript
// addons/xterm/src/index.ts
import {
  decodeRequest,
  encodeResponse,
  readFrame,
  METHOD_CREATE,
  METHOD_PROCESS,
  METHOD_SNAPSHOT,
  METHOD_RESIZE,
  METHOD_DESTROY,
  METHOD_SHUTDOWN,
  STATUS_OK,
  STATUS_ERROR,
} from "./protocol.js";
import { InstanceManager } from "./manager.js";

const manager = new InstanceManager();
let inputBuffer = Buffer.alloc(0);

function handleRequest(frame: Buffer): Buffer | null {
  const req = decodeRequest(frame);

  switch (req.method) {
    case METHOD_CREATE: {
      const params = JSON.parse(req.payload.toString());
      manager.create(req.sessionId, params.cols, params.rows);
      return encodeResponse({
        method: req.method,
        sessionId: req.sessionId,
        status: STATUS_OK,
        payload: Buffer.alloc(0),
      });
    }

    case METHOD_PROCESS: {
      manager.process(req.sessionId, req.payload);
      return null; // fire-and-forget, no response
    }

    case METHOD_SNAPSHOT: {
      const snap = manager.snapshot(req.sessionId);
      return encodeResponse({
        method: req.method,
        sessionId: req.sessionId,
        status: STATUS_OK,
        payload: snap,
      });
    }

    case METHOD_RESIZE: {
      const params = JSON.parse(req.payload.toString());
      manager.resize(req.sessionId, params.cols, params.rows);
      return encodeResponse({
        method: req.method,
        sessionId: req.sessionId,
        status: STATUS_OK,
        payload: Buffer.alloc(0),
      });
    }

    case METHOD_DESTROY: {
      manager.destroy(req.sessionId);
      return encodeResponse({
        method: req.method,
        sessionId: req.sessionId,
        status: STATUS_OK,
        payload: Buffer.alloc(0),
      });
    }

    case METHOD_SHUTDOWN: {
      manager.destroyAll();
      process.exit(0);
    }

    default: {
      return encodeResponse({
        method: req.method,
        sessionId: req.sessionId,
        status: STATUS_ERROR,
        payload: Buffer.from("unknown method"),
      });
    }
  }
}

function writeResponse(resp: Buffer): void {
  const lenBuf = Buffer.alloc(4);
  lenBuf.writeUInt32BE(resp.length, 0);
  process.stdout.write(lenBuf);
  process.stdout.write(resp);
}

process.stdin.on("data", (chunk: Buffer) => {
  inputBuffer = Buffer.concat([inputBuffer, chunk]);

  let result: ReturnType<typeof readFrame>;
  while ((result = readFrame(inputBuffer, 0)) !== null) {
    const resp = handleRequest(result.frame);
    if (resp !== null) {
      writeResponse(resp);
    }
    inputBuffer = inputBuffer.subarray(result.newOffset);
  }
});

process.stdin.on("end", () => {
  manager.destroyAll();
  process.exit(0);
});

process.stderr.write("wmux-emulator-xterm: started\n");
```

- [ ] **Step 4: Run all addon tests**

```bash
cd addons/xterm && npm test
```

Expected: ALL PASS

- [ ] **Step 5: Build the addon**

```bash
cd addons/xterm && npm run build
```

Expected: `dist/` directory created with compiled JS

- [ ] **Step 6: Commit**

```bash
git add addons/xterm/src/manager.ts addons/xterm/src/index.ts addons/xterm/test/manager.test.ts
git -c commit.gpgsign=false commit -m "feat(addon-xterm): add xterm instance manager and main loop"
```

---

### Task 6: Integration — wire AddonEmulator into session creation

**Files:**
- Modify: `internal/session/service.go` (pass emulator into session creation based on config)
- Modify: `internal/session/options.go` (add WithEmulatorBackend option)
- Create: `internal/session/addon_manager.go` (manages the singleton addon process)
- Create: `internal/session/addon_manager_test.go`

This task connects the addon to the session service. The `AddonManager` starts the addon process lazily on first use and provides `AddonEmulator` instances per session.

- [ ] **Step 1: Write failing tests for AddonManager**

```go
// internal/session/addon_manager_test.go
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
	// Pre-load create response
	proc.writeResponse(AddonMethodCreate, "", AddonStatusOK, nil)
	m.processes = append(m.processes, proc)
	return proc, nil
}

func TestAddonManager_EmulatorFor_StartsProcess(t *testing.T) {
	starter := &mockProcessStarter{}
	mgr := NewAddonManager(starter)

	em := mgr.EmulatorFor("sess-1", 80, 24)
	require.NotNil(t, em)
	assert.Len(t, starter.processes, 1)
}

func TestAddonManager_EmulatorFor_ReusesSingleProcess(t *testing.T) {
	starter := &mockProcessStarter{}
	mgr := NewAddonManager(starter)

	_ = mgr.EmulatorFor("sess-1", 80, 24)
	_ = mgr.EmulatorFor("sess-2", 80, 24)
	assert.Len(t, starter.processes, 1) // same process
}

func TestAddonManager_Shutdown(t *testing.T) {
	starter := &mockProcessStarter{}
	mgr := NewAddonManager(starter)
	_ = mgr.EmulatorFor("sess-1", 80, 24)

	mgr.Shutdown()
	// Should not panic
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/session/ -run TestAddonManager -v`
Expected: FAIL — `NewAddonManager` not defined

- [ ] **Step 3: Implement AddonManager**

```go
// internal/session/addon_manager.go
package session

import (
	"encoding/json"
	"sync"
)

// ProcessStarter creates a new addon child process.
type ProcessStarter interface {
	Start() (AddonProcess, error)
}

// AddonManager manages a singleton addon process and provides
// AddonEmulator instances for individual sessions.
type AddonManager struct {
	mu      sync.Mutex
	starter ProcessStarter
	process AddonProcess
}

// NewAddonManager creates an AddonManager with the given process starter.
func NewAddonManager(starter ProcessStarter) *AddonManager {
	return &AddonManager{starter: starter}
}

// EmulatorFor returns an AddonEmulator for the given session, starting the
// addon process if it is not already running.
func (m *AddonManager) EmulatorFor(sessionID string, cols, rows int) *AddonEmulator {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.process == nil {
		proc, err := m.starter.Start()
		if err != nil {
			return NewAddonEmulatorWithProcess(nil, sessionID)
		}
		m.process = proc
	}

	em := &AddonEmulator{
		process:   m.process,
		sessionID: sessionID,
	}

	payload, _ := json.Marshal(struct {
		Cols int `json:"cols"`
		Rows int `json:"rows"`
	}{Cols: cols, Rows: rows})
	em.sendRequest(AddonMethodCreate, payload)

	return em
}

// Shutdown sends a shutdown command to the addon process.
func (m *AddonManager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.process == nil {
		return
	}

	shutdownEm := &AddonEmulator{process: m.process, sessionID: ""}
	shutdownEm.sendRequest(AddonMethodShutdown, nil)
	_ = m.process.Kill()
	m.process = nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/session/ -run TestAddonManager -v`
Expected: ALL PASS

- [ ] **Step 5: Add WithEmulatorBackend option**

In `internal/session/options.go`, add:

```go
// WithAddonManager sets the addon manager for creating addon-backed emulators.
func WithAddonManager(mgr *AddonManager) Option {
	return func(s *Service) {
		s.addonManager = mgr
	}
}
```

Add `addonManager *AddonManager` field to `Service` struct in `service.go` and use it in `Create` to select emulator:

```go
// In Create, after spawning PTY, before starting read loop:
var emulator ScreenEmulator
if s.addonManager != nil {
    emulator = s.addonManager.EmulatorFor(id, cols, rows)
} else {
    emulator = NoneEmulator{}
}
```

- [ ] **Step 6: Run full test suite**

Run: `make test`
Expected: ALL PASS

- [ ] **Step 7: Commit**

```bash
git add internal/session/addon_manager.go internal/session/addon_manager_test.go internal/session/options.go internal/session/service.go
git -c commit.gpgsign=false commit -m "feat(session): add AddonManager and wire into session creation"
```

---

### Task 7: Lint and coverage check

- [ ] **Step 1: Run linter**

Run: `make lint`
Expected: PASS (fix any issues)

- [ ] **Step 2: Check coverage**

Run: `go test -coverprofile=coverage.out ./internal/session/ && go tool cover -func=coverage.out`
Expected: >= 90% on new files

- [ ] **Step 3: Commit any fixes**

```bash
git add -A && git -c commit.gpgsign=false commit -m "fix(session): lint and coverage fixes for addon emulator"
```

(Only if there are changes to commit)
