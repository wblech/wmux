# Phase 2 Sub-plan 2: Go Client Library + Warm Attach + Integration Doc

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create a public Go client library at `pkg/client/` that abstracts the binary wire protocol, implements warm attach with two-phase snapshot (scrollback + viewport from xterm headless), and includes an integration guide for Watchtower.

**Architecture:** The client library connects to the daemon via Unix socket using the existing `protocol.Conn` for frame I/O and `auth.Read` for token loading. It provides a high-level API: `Connect`, `Create`, `Attach` (returns snapshot), `Detach`, `Kill`, `Write`, `Resize`, `List`, `Info`. Events and data output are dispatched via callbacks. The daemon's `handleAttach` is modified to request a snapshot from the emulator and include it in the MsgOK response.

**Tech Stack:** Go 1.25+, existing packages: `internal/platform/protocol`, `internal/platform/auth`, `internal/platform/ipc`

**Prerequisites:** Sub-plan 1 complete. Addon emulator providing snapshots. Key types:
- `protocol.Frame`, `protocol.Conn`, `protocol.MessageType`
- `ipc.Dial(socketPath)` → `net.Conn`
- `auth.Read(tokenPath)` → `[]byte`
- `daemon.SessionResponse`, `daemon.CreateRequest`, `daemon.SessionIDRequest`, `daemon.ResizeRequest`
- `session.Snapshot{Scrollback, Viewport}`

---

## File Structure

```
pkg/
└── client/
    ├── entity.go        # Public types: Options, CreateParams, SessionInfo, Snapshot, Event
    ├── client.go        # Connect, Close, internal frame I/O, event loop
    ├── session.go       # Create, Attach, Detach, Kill, Write, Resize, List, Info
    ├── event.go         # OnData, OnEvent, callback registration
    ├── client_test.go   # Tests using mock server
    └── entity_test.go   # Tests for public types

internal/
├── daemon/
│   ├── service.go       # MODIFIED: handleAttach returns snapshot
│   ├── entity.go        # MODIFIED: AttachResponse with snapshot
│   └── service_test.go  # MODIFIED: test snapshot in attach
├── platform/
│   └── protocol/
│       └── entity.go    # MODIFIED: add MsgSnapshot (0x12)

docs/
└── integration-guide.md # NEW: Watchtower integration guide
```

---

### Task 1: Public types — entity.go

**Files:**
- Create: `pkg/client/entity.go`
- Create: `pkg/client/entity_test.go`

- [ ] **Step 1: Write tests for public types**

```go
// pkg/client/entity_test.go
package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOptions_Defaults(t *testing.T) {
	opts := Options{}
	assert.Empty(t, opts.SocketPath)
	assert.Empty(t, opts.TokenPath)
}

func TestCreateParams_Zero(t *testing.T) {
	p := CreateParams{}
	assert.Empty(t, p.Shell)
	assert.Equal(t, 0, p.Cols)
	assert.Equal(t, 0, p.Rows)
}

func TestSnapshot_Empty(t *testing.T) {
	s := Snapshot{}
	assert.Nil(t, s.Scrollback)
	assert.Nil(t, s.Viewport)
}

func TestSessionInfo_Fields(t *testing.T) {
	info := SessionInfo{
		ID:    "test",
		State: "alive",
		Pid:   1234,
		Cols:  80,
		Rows:  24,
		Shell: "/bin/zsh",
	}
	assert.Equal(t, "test", info.ID)
	assert.Equal(t, 1234, info.Pid)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/client/ -run Test -v`
Expected: FAIL — package doesn't exist

- [ ] **Step 3: Implement entity.go**

```go
// pkg/client/entity.go
package client

// Options configures the client connection to the wmux daemon.
type Options struct {
	// SocketPath is the Unix socket to connect to (default: ~/.wmux/daemon.sock).
	SocketPath string
	// TokenPath is the token file for authentication (default: ~/.wmux/daemon.token).
	TokenPath string
}

// CreateParams holds parameters for creating a new session.
type CreateParams struct {
	// Shell is the path to the shell binary.
	Shell string
	// Args contains additional shell arguments.
	Args []string
	// Cols is the initial terminal width.
	Cols int
	// Rows is the initial terminal height.
	Rows int
	// Cwd is the initial working directory.
	Cwd string
	// Env is the environment variable list.
	Env []string
}

// SessionInfo holds metadata about a session returned by the daemon.
type SessionInfo struct {
	// ID is the unique session identifier.
	ID string
	// State is the human-readable lifecycle state.
	State string
	// Pid is the shell process ID.
	Pid int
	// Cols is the terminal width.
	Cols int
	// Rows is the terminal height.
	Rows int
	// Shell is the shell binary path.
	Shell string
}

// Snapshot holds the terminal state captured at attach time.
type Snapshot struct {
	// Scrollback contains lines scrolled off the viewport.
	Scrollback []byte
	// Viewport contains the visible terminal content.
	Viewport []byte
}

// AttachResult holds the session info and snapshot from an attach operation.
type AttachResult struct {
	// Session is the session metadata.
	Session SessionInfo
	// Snapshot is the terminal state (empty if backend is "none").
	Snapshot Snapshot
}

// Event represents a daemon event delivered to the client.
type Event struct {
	// Type is the event type string (e.g., "session.created").
	Type string
	// SessionID is the session this event relates to.
	SessionID string
	// Data contains event-specific key-value pairs.
	Data map[string]any
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/client/ -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/client/entity.go pkg/client/entity_test.go
git -c commit.gpgsign=false commit -m "feat(client): add public types for wmux client library"
```

---

### Task 2: Client connection — client.go

**Files:**
- Create: `pkg/client/client.go`
- Create: `pkg/client/client_test.go`

- [ ] **Step 1: Write failing tests for Connect and Close**

```go
// pkg/client/client_test.go
package client

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wblech/wmux/internal/platform/auth"
	"github.com/wblech/wmux/internal/platform/protocol"
)

// startMockServer creates a Unix socket server that accepts one connection
// and performs the auth handshake, then returns the listener and token.
func startMockServer(t *testing.T) (socketPath, tokenPath string, cleanup func()) {
	t.Helper()
	dir := t.TempDir()
	socketPath = filepath.Join(dir, "daemon.sock")
	tokenPath = filepath.Join(dir, "daemon.token")

	// Generate token
	token, err := auth.Generate(tokenPath)
	require.NoError(t, err)

	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)

	// Accept and handle auth in background
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		pConn := protocol.NewConn(conn)
		frame, err := pConn.ReadFrame()
		if err != nil {
			conn.Close()
			return
		}
		if frame.Type == protocol.MsgAuth {
			// Verify token
			if auth.Verify(frame.Payload, token) {
				_ = pConn.WriteFrame(protocol.Frame{
					Version: protocol.ProtocolVersion,
					Type:    protocol.MsgOK,
					Payload: nil,
				})
			}
		}
		// Keep connection alive until cleanup
		<-make(chan struct{})
	}()

	return socketPath, tokenPath, func() {
		ln.Close()
	}
}

func TestConnect_Success(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServer(t)
	defer cleanup()

	c, err := Connect(Options{
		SocketPath: socketPath,
		TokenPath:  tokenPath,
	})
	require.NoError(t, err)
	require.NotNil(t, c)
	defer c.Close()
}

func TestConnect_BadSocket(t *testing.T) {
	_, err := Connect(Options{
		SocketPath: "/nonexistent/daemon.sock",
		TokenPath:  "/nonexistent/token",
	})
	assert.Error(t, err)
}

func TestClose(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServer(t)
	defer cleanup()

	c, err := Connect(Options{
		SocketPath: socketPath,
		TokenPath:  tokenPath,
	})
	require.NoError(t, err)

	err = c.Close()
	assert.NoError(t, err)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/client/ -run "TestConnect|TestClose" -v`
Expected: FAIL — `Connect` not defined

- [ ] **Step 3: Implement client.go**

```go
// pkg/client/client.go
package client

import (
	"fmt"
	"net"
	"sync"

	"github.com/wblech/wmux/internal/platform/auth"
	"github.com/wblech/wmux/internal/platform/protocol"
)

// Client is a connection to a wmux daemon.
type Client struct {
	mu          sync.Mutex
	conn        net.Conn
	pConn       *protocol.Conn
	dataHandler func(sessionID string, data []byte)
	evtHandler  func(Event)
}

// Connect establishes a connection to the wmux daemon, authenticates, and
// returns a ready-to-use Client.
func Connect(opts Options) (*Client, error) {
	token, err := auth.Read(opts.TokenPath)
	if err != nil {
		return nil, fmt.Errorf("client: read token: %w", err)
	}

	conn, err := net.Dial("unix", opts.SocketPath)
	if err != nil {
		return nil, fmt.Errorf("client: dial: %w", err)
	}

	pConn := protocol.NewConn(conn)

	// Send auth frame
	if err := pConn.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgAuth,
		Payload: token,
	}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("client: auth write: %w", err)
	}

	// Read auth response
	frame, err := pConn.ReadFrame()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("client: auth read: %w", err)
	}

	if frame.Type != protocol.MsgOK {
		conn.Close()
		return nil, fmt.Errorf("client: auth failed")
	}

	return &Client{
		conn:  conn,
		pConn: pConn,
	}, nil
}

// Close closes the connection to the daemon.
func (c *Client) Close() error {
	return c.conn.Close()
}

// sendRequest sends a control frame and reads the response.
func (c *Client) sendRequest(msgType protocol.MessageType, payload []byte) (protocol.Frame, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.pConn.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    msgType,
		Payload: payload,
	}); err != nil {
		return protocol.Frame{}, fmt.Errorf("client: write: %w", err)
	}

	resp, err := c.pConn.ReadFrame()
	if err != nil {
		return protocol.Frame{}, fmt.Errorf("client: read: %w", err)
	}

	return resp, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/client/ -run "TestConnect|TestClose" -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/client/client.go pkg/client/client_test.go
git -c commit.gpgsign=false commit -m "feat(client): add Connect/Close with auth handshake"
```

---

### Task 3: Session operations — session.go

**Files:**
- Create: `pkg/client/session.go`
- Modify: `pkg/client/client_test.go`

- [ ] **Step 1: Write failing tests for Create and List**

Add to `pkg/client/client_test.go`:

```go
func TestClient_Create(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgCreate: func(payload []byte) protocol.Frame {
			return okFrame(SessionInfo{ID: "s1", State: "alive", Pid: 42, Cols: 80, Rows: 24, Shell: "/bin/zsh"})
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close()

	info, err := c.Create("s1", CreateParams{Shell: "/bin/zsh", Cols: 80, Rows: 24})
	require.NoError(t, err)
	assert.Equal(t, "s1", info.ID)
	assert.Equal(t, "alive", info.State)
	assert.Equal(t, 42, info.Pid)
}

func TestClient_List(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgList: func(payload []byte) protocol.Frame {
			return okFrame([]SessionInfo{
				{ID: "s1", State: "alive"},
				{ID: "s2", State: "detached"},
			})
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close()

	sessions, err := c.List()
	require.NoError(t, err)
	assert.Len(t, sessions, 2)
	assert.Equal(t, "s1", sessions[0].ID)
}
```

Note: `startMockServerWithHandlers` is a helper that dispatches frames by type — implement it in the test file as an extension of `startMockServer`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/client/ -run "TestClient_Create|TestClient_List" -v`
Expected: FAIL — methods not defined

- [ ] **Step 3: Implement session.go**

```go
// pkg/client/session.go
package client

import (
	"encoding/json"
	"fmt"

	"github.com/wblech/wmux/internal/platform/protocol"
)

// Create sends a create session request to the daemon.
func (c *Client) Create(id string, params CreateParams) (SessionInfo, error) {
	payload, err := json.Marshal(struct {
		ID    string   `json:"id"`
		Shell string   `json:"shell"`
		Args  []string `json:"args,omitempty"`
		Cols  int      `json:"cols"`
		Rows  int      `json:"rows"`
		Cwd   string   `json:"cwd,omitempty"`
		Env   []string `json:"env,omitempty"`
	}{
		ID:    id,
		Shell: params.Shell,
		Args:  params.Args,
		Cols:  params.Cols,
		Rows:  params.Rows,
		Cwd:   params.Cwd,
		Env:   params.Env,
	})
	if err != nil {
		return SessionInfo{}, fmt.Errorf("client: marshal create: %w", err)
	}

	resp, err := c.sendRequest(protocol.MsgCreate, payload)
	if err != nil {
		return SessionInfo{}, err
	}

	return c.parseSessionInfo(resp)
}

// Attach attaches to an existing session and returns the snapshot.
func (c *Client) Attach(sessionID string) (AttachResult, error) {
	payload, err := json.Marshal(struct {
		SessionID string `json:"session_id"`
	}{SessionID: sessionID})
	if err != nil {
		return AttachResult{}, fmt.Errorf("client: marshal attach: %w", err)
	}

	resp, err := c.sendRequest(protocol.MsgAttach, payload)
	if err != nil {
		return AttachResult{}, err
	}

	if resp.Type == protocol.MsgError {
		return AttachResult{}, c.parseError(resp)
	}

	var attachResp struct {
		ID       string `json:"id"`
		State    string `json:"state"`
		Pid      int    `json:"pid"`
		Cols     int    `json:"cols"`
		Rows     int    `json:"rows"`
		Shell    string `json:"shell"`
		Snapshot *struct {
			Scrollback []byte `json:"scrollback"`
			Viewport   []byte `json:"viewport"`
		} `json:"snapshot,omitempty"`
	}
	if err := json.Unmarshal(resp.Payload, &attachResp); err != nil {
		return AttachResult{}, fmt.Errorf("client: unmarshal attach: %w", err)
	}

	result := AttachResult{
		Session: SessionInfo{
			ID:    attachResp.ID,
			State: attachResp.State,
			Pid:   attachResp.Pid,
			Cols:  attachResp.Cols,
			Rows:  attachResp.Rows,
			Shell: attachResp.Shell,
		},
	}
	if attachResp.Snapshot != nil {
		result.Snapshot = Snapshot{
			Scrollback: attachResp.Snapshot.Scrollback,
			Viewport:   attachResp.Snapshot.Viewport,
		}
	}

	return result, nil
}

// Detach detaches from a session.
func (c *Client) Detach(sessionID string) error {
	payload, _ := json.Marshal(struct {
		SessionID string `json:"session_id"`
	}{SessionID: sessionID})

	resp, err := c.sendRequest(protocol.MsgDetach, payload)
	if err != nil {
		return err
	}
	if resp.Type == protocol.MsgError {
		return c.parseError(resp)
	}
	return nil
}

// Kill terminates a session.
func (c *Client) Kill(sessionID string) error {
	payload, _ := json.Marshal(struct {
		SessionID string `json:"session_id"`
	}{SessionID: sessionID})

	resp, err := c.sendRequest(protocol.MsgKill, payload)
	if err != nil {
		return err
	}
	if resp.Type == protocol.MsgError {
		return c.parseError(resp)
	}
	return nil
}

// Write sends input data to a session's PTY.
func (c *Client) Write(sessionID string, data []byte) error {
	// Use the same binary encoding as the daemon's MsgInput
	idBytes := []byte(sessionID)
	payload := make([]byte, 0, 1+len(idBytes)+len(data))
	payload = append(payload, byte(len(idBytes)))
	payload = append(payload, idBytes...)
	payload = append(payload, data...)

	resp, err := c.sendRequest(protocol.MsgInput, payload)
	if err != nil {
		return err
	}
	if resp.Type == protocol.MsgError {
		return c.parseError(resp)
	}
	return nil
}

// Resize changes the terminal dimensions of a session.
func (c *Client) Resize(sessionID string, cols, rows int) error {
	payload, _ := json.Marshal(struct {
		SessionID string `json:"session_id"`
		Cols      int    `json:"cols"`
		Rows      int    `json:"rows"`
	}{SessionID: sessionID, Cols: cols, Rows: rows})

	resp, err := c.sendRequest(protocol.MsgResize, payload)
	if err != nil {
		return err
	}
	if resp.Type == protocol.MsgError {
		return c.parseError(resp)
	}
	return nil
}

// List returns all sessions.
func (c *Client) List() ([]SessionInfo, error) {
	resp, err := c.sendRequest(protocol.MsgList, nil)
	if err != nil {
		return nil, err
	}
	if resp.Type == protocol.MsgError {
		return nil, c.parseError(resp)
	}

	var sessions []SessionInfo
	if err := json.Unmarshal(resp.Payload, &sessions); err != nil {
		return nil, fmt.Errorf("client: unmarshal list: %w", err)
	}
	return sessions, nil
}

// Info returns metadata about a specific session.
func (c *Client) Info(sessionID string) (SessionInfo, error) {
	payload, _ := json.Marshal(struct {
		SessionID string `json:"session_id"`
	}{SessionID: sessionID})

	resp, err := c.sendRequest(protocol.MsgInfo, payload)
	if err != nil {
		return SessionInfo{}, err
	}
	return c.parseSessionInfo(resp)
}

// parseSessionInfo extracts SessionInfo from a response frame.
func (c *Client) parseSessionInfo(resp protocol.Frame) (SessionInfo, error) {
	if resp.Type == protocol.MsgError {
		return SessionInfo{}, c.parseError(resp)
	}

	var info SessionInfo
	if err := json.Unmarshal(resp.Payload, &info); err != nil {
		return SessionInfo{}, fmt.Errorf("client: unmarshal session: %w", err)
	}
	return info, nil
}

// parseError extracts the error message from an MsgError frame.
func (c *Client) parseError(resp protocol.Frame) error {
	var errResp struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(resp.Payload, &errResp); err != nil {
		return fmt.Errorf("client: daemon error (unparsable)")
	}
	return fmt.Errorf("client: daemon: %s", errResp.Error)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/client/ -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/client/session.go
git -c commit.gpgsign=false commit -m "feat(client): add session operations (create, attach, detach, kill, write, resize, list, info)"
```

---

### Task 4: Event callbacks — event.go

**Files:**
- Create: `pkg/client/event.go`

- [ ] **Step 1: Implement event.go**

```go
// pkg/client/event.go
package client

// OnData registers a callback for PTY output data.
// The callback receives the session ID and raw output bytes.
func (c *Client) OnData(handler func(sessionID string, data []byte)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dataHandler = handler
}

// OnEvent registers a callback for daemon events.
func (c *Client) OnEvent(handler func(Event)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evtHandler = handler
}
```

- [ ] **Step 2: Run full test suite**

Run: `go test ./pkg/client/ -v`
Expected: ALL PASS

- [ ] **Step 3: Commit**

```bash
git add pkg/client/event.go
git -c commit.gpgsign=false commit -m "feat(client): add OnData and OnEvent callbacks"
```

---

### Task 5: Daemon — modify handleAttach to return snapshot

**Files:**
- Modify: `internal/daemon/entity.go`
- Modify: `internal/daemon/service.go`
- Modify: `internal/daemon/service_test.go`

- [ ] **Step 1: Add AttachResponse to entity.go**

Add to `internal/daemon/entity.go`:

```go
// AttachResponse is the JSON payload for MsgAttach responses with snapshot.
type AttachResponse struct {
	ID       string            `json:"id"`
	State    string            `json:"state"`
	Pid      int               `json:"pid"`
	Cols     int               `json:"cols"`
	Rows     int               `json:"rows"`
	Shell    string            `json:"shell"`
	Snapshot *SnapshotResponse `json:"snapshot,omitempty"`
}

// SnapshotResponse carries the two-phase snapshot data.
type SnapshotResponse struct {
	Scrollback []byte `json:"scrollback"`
	Viewport   []byte `json:"viewport"`
}
```

- [ ] **Step 2: Add Snapshot method to SessionManager interface**

In `internal/daemon/service.go`, add to `SessionManager` interface:

```go
// Snapshot returns the current terminal screen state for the session.
Snapshot(id string) (session.Snapshot, error)
```

Note: This method already exists on `session.Service` (line 357). The interface just needs to include it.

- [ ] **Step 3: Modify handleAttach to return snapshot**

Replace the current `handleAttach` in `internal/daemon/service.go`:

```go
func (d *Daemon) handleAttach(c ConnectedClient, frame protocol.Frame) {
	var req SessionIDRequest
	if err := json.Unmarshal(frame.Payload, &req); err != nil {
		_ = c.Control().WriteFrame(errorFrame("invalid attach request"))
		return
	}

	info, err := d.sessionSvc.Get(req.SessionID)
	if err != nil {
		_ = c.Control().WriteFrame(errorFrame(err.Error()))
		return
	}

	if err := d.sessionSvc.Attach(req.SessionID); err != nil {
		_ = c.Control().WriteFrame(errorFrame(err.Error()))
		return
	}

	d.mu.Lock()
	if _, ok := d.attachments[req.SessionID]; !ok {
		d.attachments[req.SessionID] = make(map[string]struct{})
	}
	d.attachments[req.SessionID][c.ClientID()] = struct{}{}
	d.clientSession[c.ClientID()] = req.SessionID
	d.mu.Unlock()

	// Build response with snapshot
	resp := AttachResponse{
		ID:    info.ID,
		State: info.State,
		Pid:   info.Pid,
		Cols:  info.Cols,
		Rows:  info.Rows,
		Shell: info.Shell,
	}

	snap, err := d.sessionSvc.Snapshot(req.SessionID)
	if err == nil && (len(snap.Scrollback) > 0 || len(snap.Viewport) > 0) {
		resp.Snapshot = &SnapshotResponse{
			Scrollback: snap.Scrollback,
			Viewport:   snap.Viewport,
		}
	}

	_ = c.Control().WriteFrame(okFrame(resp))

	d.publishEvent(event.Event{
		Type:      event.SessionAttached,
		SessionID: req.SessionID,
		Payload:   map[string]any{"client_id": c.ClientID()},
	})
}
```

- [ ] **Step 4: Write test for snapshot in attach**

Add to `internal/daemon/service_test.go`:

```go
func TestHandleAttach_ReturnsSnapshot(t *testing.T) {
	// Create a daemon with a mock session manager that returns a non-empty snapshot
	// Verify the MsgOK response contains the snapshot field
	// (Exact implementation depends on existing test infrastructure)
}
```

- [ ] **Step 5: Run tests**

Run: `make test`
Expected: ALL PASS

- [ ] **Step 6: Commit**

```bash
git add internal/daemon/entity.go internal/daemon/service.go internal/daemon/service_test.go
git -c commit.gpgsign=false commit -m "feat(daemon): return snapshot in attach response for warm restore"
```

---

### Task 6: Integration guide

**Files:**
- Create: `docs/integration-guide.md`

- [ ] **Step 1: Write integration guide**

```markdown
# wmux Integration Guide

Guide for integrating wmux as a terminal backend in Go applications (e.g., Watchtower).

## Adding the Dependency

    go get github.com/wblech/wmux

## Basic Usage

### Connect to the Daemon

    import "github.com/wblech/wmux/pkg/client"

    c, err := client.Connect(client.Options{
        SocketPath: "~/.wmux/daemon.sock",
        TokenPath:  "~/.wmux/daemon.token",
    })
    if err != nil {
        // Daemon not running — start it, or show error
    }
    defer c.Close()

### Create a Session

    info, err := c.Create("my-session", client.CreateParams{
        Shell: "/bin/zsh",
        Cols:  80,
        Rows:  24,
        Cwd:   "/home/user",
    })

### Attach with Warm Restore

    result, err := c.Attach("my-session")
    // result.Snapshot.Scrollback — feed to xterm.js as history
    // result.Snapshot.Viewport   — apply as current screen state
    // Then start streaming live output via OnData

### I/O Loop

    c.OnData(func(sessionID string, data []byte) {
        // Forward to xterm.js
    })

    c.Write("my-session", []byte("ls -la\n"))

### Resize

    c.Resize("my-session", newCols, newRows)

### Detach (session keeps running)

    c.Detach("my-session")

### Kill

    c.Kill("my-session")

## Migrating from In-Process PTY

If your app currently uses `creack/pty` directly:

1. Replace `pty.Start(cmd)` with `c.Create(id, params)`
2. Replace `pty.Read()` loop with `c.OnData()` callback
3. Replace `pty.Setsize()` with `c.Resize()`
4. On app close: call `c.Detach()` instead of killing the process
5. On app open: call `c.Attach()` — if session exists, get snapshot; if not, create new

## Cold Restore (after reboot)

When the daemon is dead and you need to recover session history:

    history, err := client.LoadSessionHistory("~/.wmux/history", "session-id")
    if err != nil {
        // No history — create fresh session
    }
    // history.Scrollback — render as read-only in terminal
    // history.Metadata.Cwd — use as cwd for new session

    // User interacts → create new session
    info, err := c.Create("session-id", client.CreateParams{
        Cwd: history.Metadata.Cwd,
    })

    // Clean up consumed history
    client.CleanSessionHistory("~/.wmux/history", "session-id")

## Error Handling

- `Connect` fails if daemon is not running — handle by starting daemon first
- `Attach` fails with session not found — handle by creating new session or trying cold restore
- Network errors on any call — reconnect and retry
```

- [ ] **Step 2: Commit**

```bash
git add -f docs/integration-guide.md
git -c commit.gpgsign=false commit -m "docs: add Watchtower integration guide for wmux client library"
```

---

### Task 7: Lint and coverage check

- [ ] **Step 1: Run linter**

Run: `make lint`
Expected: PASS

- [ ] **Step 2: Check coverage on pkg/client/**

Run: `go test -coverprofile=coverage.out ./pkg/client/ && go tool cover -func=coverage.out`
Expected: >= 90% on new files

- [ ] **Step 3: Commit any fixes**

```bash
git add -A && git -c commit.gpgsign=false commit -m "fix(client): lint and coverage fixes"
```

(Only if there are changes)
