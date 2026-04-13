# Phase 2 Sub-plan 3: Session Metadata + Full Events + Environment Forwarding

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add extensible session metadata (integrator key-value pairs), complete the event system with 6 new event types, add OSC parsing for cwd detection and notifications, implement environment forwarding with stable symlinks, and expose all new features in the client library.

**Architecture:** Session metadata lives as `map[string]string` on the session struct with new protocol messages `MsgMetaSet`/`MsgMetaGet`. Events extend the existing `event.Type` enum. OSC parsing scans a copy of output in the daemon's broadcast loop. Environment forwarding creates stable symlinks for socket paths and writes an env file for other values. Client library gets `MetaSet`/`MetaGet`/`MetaGetAll` and automatic env forwarding on attach.

**Tech Stack:** Go 1.25+, existing packages

**Prerequisites:** Sub-plans 1-2 complete. Key types:
- `session.Session` with existing fields
- `event.Type` with 4 Phase 1 values
- `protocol.MessageType` through `0x11`
- `pkg/client.Client` with Connect, Create, Attach, etc.

---

## File Structure

```
internal/
├── session/
│   └── entity.go              # MODIFIED: add Metadata map to Session
├── platform/
│   ├── event/
│   │   └── entity.go          # MODIFIED: add 6 new event types
│   └── protocol/
│       └── entity.go          # MODIFIED: add MsgMetaSet, MsgMetaGet, MsgEnvForward
├── daemon/
│   ├── entity.go              # MODIFIED: add MetaSetRequest, MetaGetRequest, EnvForwardRequest
│   ├── service.go             # MODIFIED: dispatch new message types
│   ├── environment.go         # NEW: symlink creation, env file writing
│   ├── environment_test.go    # NEW
│   ├── osc.go                 # NEW: OSC 7/9/99/777 parser
│   ├── osc_test.go            # NEW
│   └── service_test.go        # MODIFIED: tests for new handlers

pkg/
└── client/
    ├── metadata.go            # NEW: MetaSet, MetaGet, MetaGetAll
    ├── metadata_test.go       # NEW
    ├── environment.go         # NEW: env forwarding on attach
    └── environment_test.go    # NEW
```

---

### Task 1: Extend event types

**Files:**
- Modify: `internal/platform/event/entity.go`
- Modify: `internal/platform/event/entity_test.go`

- [ ] **Step 1: Write failing tests for new event types**

Add to `internal/platform/event/entity_test.go`:

```go
func TestType_String_Phase2(t *testing.T) {
	tests := []struct {
		et   Type
		want string
	}{
		{SessionIdle, "session.idle"},
		{SessionKilled, "session.killed"},
		{Resize, "resize"},
		{CwdChanged, "cwd.changed"},
		{Notification, "notification"},
		{OutputFlood, "output.flood"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.et.String())
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/platform/event/ -run TestType_String_Phase2 -v`
Expected: FAIL — constants not defined

- [ ] **Step 3: Add new event types**

In `internal/platform/event/entity.go`, extend the const block:

```go
const (
	_ Type = iota
	SessionCreated
	SessionAttached
	SessionDetached
	SessionExited
	// Phase 2 event types.
	SessionIdle
	SessionKilled
	Resize
	CwdChanged
	Notification
	OutputFlood
)
```

Update the `String()` method to handle the new cases:

```go
case SessionIdle:
	return "session.idle"
case SessionKilled:
	return "session.killed"
case Resize:
	return "resize"
case CwdChanged:
	return "cwd.changed"
case Notification:
	return "notification"
case OutputFlood:
	return "output.flood"
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/platform/event/ -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add internal/platform/event/entity.go internal/platform/event/entity_test.go
git -c commit.gpgsign=false commit -m "feat(event): add Phase 2 event types (idle, killed, resize, cwd, notification, flood)"
```

---

### Task 2: Protocol — new message types

**Files:**
- Modify: `internal/platform/protocol/entity.go`
- Modify: `internal/platform/protocol/entity_test.go`

- [ ] **Step 1: Add new message type constants**

In `internal/platform/protocol/entity.go`:

```go
MsgMetaSet    MessageType = 0x12
MsgMetaGet    MessageType = 0x13
MsgEnvForward MessageType = 0x14
```

Update `String()` with:

```go
case MsgMetaSet:
	return "meta_set"
case MsgMetaGet:
	return "meta_get"
case MsgEnvForward:
	return "env_forward"
```

- [ ] **Step 2: Write tests for new message types**

```go
func TestMessageType_String_Phase2(t *testing.T) {
	assert.Equal(t, "meta_set", protocol.MsgMetaSet.String())
	assert.Equal(t, "meta_get", protocol.MsgMetaGet.String())
	assert.Equal(t, "env_forward", protocol.MsgEnvForward.String())
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/platform/protocol/ -v`
Expected: ALL PASS

- [ ] **Step 4: Commit**

```bash
git add internal/platform/protocol/entity.go internal/platform/protocol/entity_test.go
git -c commit.gpgsign=false commit -m "feat(protocol): add MsgMetaSet, MsgMetaGet, MsgEnvForward message types"
```

---

### Task 3: Session metadata — entity and daemon handlers

**Files:**
- Modify: `internal/session/entity.go`
- Modify: `internal/daemon/entity.go`
- Modify: `internal/daemon/service.go`
- Modify: `internal/daemon/service_test.go`

- [ ] **Step 1: Add Metadata to Session**

In `internal/session/entity.go`, add to `Session` struct:

```go
// Metadata holds integrator-defined key-value pairs.
Metadata map[string]string
```

- [ ] **Step 2: Add request types to daemon entity**

In `internal/daemon/entity.go`:

```go
// MetaSetRequest is the JSON payload for MsgMetaSet.
type MetaSetRequest struct {
	SessionID string `json:"session_id"`
	Key       string `json:"key"`
	Value     string `json:"value"`
}

// MetaGetRequest is the JSON payload for MsgMetaGet.
type MetaGetRequest struct {
	SessionID string `json:"session_id"`
	Key       string `json:"key"` // empty = get all
}

// MetaGetResponse is the JSON payload for MsgMetaGet responses.
type MetaGetResponse struct {
	Value    string            `json:"value,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}
```

- [ ] **Step 3: Add MetaSet/MetaGet to SessionManager interface**

In `internal/daemon/service.go`, add to `SessionManager`:

```go
// MetaSet sets a metadata key-value pair on a session.
MetaSet(id, key, value string) error
// MetaGet returns a metadata value or all metadata for a session.
MetaGet(id, key string) (string, error)
// MetaGetAll returns all metadata for a session.
MetaGetAll(id string) (map[string]string, error)
```

- [ ] **Step 4: Implement handlers in daemon service**

Add to `internal/daemon/service.go`:

```go
func (d *Daemon) handleMetaSet(c ConnectedClient, frame protocol.Frame) {
	var req MetaSetRequest
	if err := json.Unmarshal(frame.Payload, &req); err != nil {
		_ = c.Control().WriteFrame(errorFrame("invalid meta set request"))
		return
	}
	if err := d.sessionSvc.MetaSet(req.SessionID, req.Key, req.Value); err != nil {
		_ = c.Control().WriteFrame(errorFrame(err.Error()))
		return
	}
	_ = c.Control().WriteFrame(okFrame(nil))
}

func (d *Daemon) handleMetaGet(c ConnectedClient, frame protocol.Frame) {
	var req MetaGetRequest
	if err := json.Unmarshal(frame.Payload, &req); err != nil {
		_ = c.Control().WriteFrame(errorFrame("invalid meta get request"))
		return
	}
	if req.Key == "" {
		meta, err := d.sessionSvc.MetaGetAll(req.SessionID)
		if err != nil {
			_ = c.Control().WriteFrame(errorFrame(err.Error()))
			return
		}
		_ = c.Control().WriteFrame(okFrame(MetaGetResponse{Metadata: meta}))
		return
	}
	val, err := d.sessionSvc.MetaGet(req.SessionID, req.Key)
	if err != nil {
		_ = c.Control().WriteFrame(errorFrame(err.Error()))
		return
	}
	_ = c.Control().WriteFrame(okFrame(MetaGetResponse{Value: val}))
}
```

Add to `dispatch()`:

```go
case protocol.MsgMetaSet:
	d.handleMetaSet(c, frame)
case protocol.MsgMetaGet:
	d.handleMetaGet(c, frame)
```

- [ ] **Step 5: Implement MetaSet/MetaGet/MetaGetAll in session.Service**

Add methods to `internal/session/service.go`:

```go
func (s *Service) MetaSet(id, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ms, ok := s.sessions[id]
	if !ok {
		return ErrSessionNotFound
	}
	if ms.session.Metadata == nil {
		ms.session.Metadata = make(map[string]string)
	}
	ms.session.Metadata[key] = value
	return nil
}

func (s *Service) MetaGet(id, key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ms, ok := s.sessions[id]
	if !ok {
		return "", ErrSessionNotFound
	}
	return ms.session.Metadata[key], nil
}

func (s *Service) MetaGetAll(id string) (map[string]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ms, ok := s.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	result := make(map[string]string, len(ms.session.Metadata))
	for k, v := range ms.session.Metadata {
		result[k] = v
	}
	return result, nil
}
```

- [ ] **Step 6: Write tests and run**

Run: `make test`
Expected: ALL PASS

- [ ] **Step 7: Commit**

```bash
git add internal/session/entity.go internal/session/service.go internal/daemon/entity.go internal/daemon/service.go internal/daemon/service_test.go
git -c commit.gpgsign=false commit -m "feat(daemon): add session metadata set/get with MsgMetaSet/MsgMetaGet"
```

---

### Task 4: OSC parser

**Files:**
- Create: `internal/daemon/osc.go`
- Create: `internal/daemon/osc_test.go`

- [ ] **Step 1: Write failing tests for OSC parsing**

```go
// internal/daemon/osc_test.go
package daemon

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseOSC7(t *testing.T) {
	// OSC 7 ; file://hostname/path/to/dir ST
	data := []byte("\x1b]7;file://localhost/home/user/project\x1b\\")
	result := ParseOSC(data)
	assert.Len(t, result, 1)
	assert.Equal(t, OSCTypeCwd, result[0].Type)
	assert.Equal(t, "/home/user/project", result[0].Value)
}

func TestParseOSC9(t *testing.T) {
	data := []byte("\x1b]9;Build complete\x1b\\")
	result := ParseOSC(data)
	assert.Len(t, result, 1)
	assert.Equal(t, OSCTypeNotification, result[0].Type)
	assert.Equal(t, "Build complete", result[0].Value)
}

func TestParseOSC99(t *testing.T) {
	data := []byte("\x1b]99;d=0:p=body;Test done\x1b\\")
	result := ParseOSC(data)
	assert.Len(t, result, 1)
	assert.Equal(t, OSCTypeNotification, result[0].Type)
	assert.Equal(t, "Test done", result[0].Value)
}

func TestParseOSC777(t *testing.T) {
	data := []byte("\x1b]777;notify;Title;Body text\x1b\\")
	result := ParseOSC(data)
	assert.Len(t, result, 1)
	assert.Equal(t, OSCTypeNotification, result[0].Type)
	assert.Equal(t, "Body text", result[0].Value)
}

func TestParseOSC_NoOSC(t *testing.T) {
	data := []byte("just plain text with no escape sequences")
	result := ParseOSC(data)
	assert.Empty(t, result)
}

func TestParseOSC_Multiple(t *testing.T) {
	data := []byte("\x1b]7;file:///tmp\x1b\\\x1b]9;Done\x1b\\")
	result := ParseOSC(data)
	assert.Len(t, result, 2)
}

func TestParseOSC_BELTerminator(t *testing.T) {
	data := []byte("\x1b]9;Alert\x07")
	result := ParseOSC(data)
	assert.Len(t, result, 1)
	assert.Equal(t, "Alert", result[0].Value)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/daemon/ -run TestParseOSC -v`
Expected: FAIL

- [ ] **Step 3: Implement OSC parser**

```go
// internal/daemon/osc.go
package daemon

import (
	"net/url"
	"strings"
)

// OSCType identifies the kind of OSC sequence detected.
type OSCType int

const (
	// OSCTypeCwd indicates an OSC 7 working directory change.
	OSCTypeCwd OSCType = iota
	// OSCTypeNotification indicates an OSC 9/99/777 notification.
	OSCTypeNotification
)

// OSCResult holds a parsed OSC sequence.
type OSCResult struct {
	Type  OSCType
	Value string
}

// ParseOSC scans data for OSC sequences (7, 9, 99, 777) and returns parsed results.
// This is a passive scanner — it does not modify the data.
func ParseOSC(data []byte) []OSCResult {
	var results []OSCResult
	s := string(data)

	for {
		// Find OSC start: ESC ]
		idx := strings.Index(s, "\x1b]")
		if idx < 0 {
			break
		}
		s = s[idx+2:]

		// Find terminator: ST (ESC \) or BEL (0x07)
		endST := strings.Index(s, "\x1b\\")
		endBEL := strings.IndexByte(s, 0x07)

		end := -1
		if endST >= 0 && endBEL >= 0 {
			if endST < endBEL {
				end = endST
			} else {
				end = endBEL
			}
		} else if endST >= 0 {
			end = endST
		} else if endBEL >= 0 {
			end = endBEL
		}

		if end < 0 {
			break
		}

		body := s[:end]
		s = s[end+1:] // skip past terminator

		// Parse OSC number
		semicolon := strings.IndexByte(body, ';')
		if semicolon < 0 {
			continue
		}
		oscNum := body[:semicolon]
		oscValue := body[semicolon+1:]

		switch oscNum {
		case "7":
			// file://hostname/path
			parsed, err := url.Parse(oscValue)
			if err == nil && parsed.Path != "" {
				results = append(results, OSCResult{Type: OSCTypeCwd, Value: parsed.Path})
			}
		case "9":
			results = append(results, OSCResult{Type: OSCTypeNotification, Value: oscValue})
		case "99":
			// Format: d=0:p=body;actual content
			parts := strings.SplitN(oscValue, ";", 2)
			val := oscValue
			if len(parts) == 2 {
				val = parts[1]
			}
			results = append(results, OSCResult{Type: OSCTypeNotification, Value: val})
		case "777":
			// Format: notify;Title;Body
			parts := strings.SplitN(oscValue, ";", 3)
			val := oscValue
			if len(parts) == 3 {
				val = parts[2]
			} else if len(parts) == 2 {
				val = parts[1]
			}
			results = append(results, OSCResult{Type: OSCTypeNotification, Value: val})
		}
	}

	return results
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/daemon/ -run TestParseOSC -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/osc.go internal/daemon/osc_test.go
git -c commit.gpgsign=false commit -m "feat(daemon): add OSC 7/9/99/777 parser for cwd and notification events"
```

---

### Task 5: Environment forwarding

**Files:**
- Create: `internal/daemon/environment.go`
- Create: `internal/daemon/environment_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/daemon/environment_test.go
package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForwardEnv_SymlinkForSocket(t *testing.T) {
	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "sessions", "s1")
	require.NoError(t, os.MkdirAll(sessionDir, 0755))

	// Create a fake socket file
	sockPath := filepath.Join(dir, "ssh-agent.sock")
	require.NoError(t, os.WriteFile(sockPath, nil, 0600))

	err := ForwardEnv(sessionDir, "SSH_AUTH_SOCK", sockPath)
	require.NoError(t, err)

	symlinkPath := filepath.Join(sessionDir, "SSH_AUTH_SOCK")
	target, err := os.Readlink(symlinkPath)
	require.NoError(t, err)
	assert.Equal(t, sockPath, target)
}

func TestForwardEnv_UpdatesExistingSymlink(t *testing.T) {
	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "sessions", "s1")
	require.NoError(t, os.MkdirAll(sessionDir, 0755))

	oldSock := filepath.Join(dir, "old.sock")
	newSock := filepath.Join(dir, "new.sock")
	require.NoError(t, os.WriteFile(oldSock, nil, 0600))
	require.NoError(t, os.WriteFile(newSock, nil, 0600))

	require.NoError(t, ForwardEnv(sessionDir, "SSH_AUTH_SOCK", oldSock))
	require.NoError(t, ForwardEnv(sessionDir, "SSH_AUTH_SOCK", newSock))

	target, err := os.Readlink(filepath.Join(sessionDir, "SSH_AUTH_SOCK"))
	require.NoError(t, err)
	assert.Equal(t, newSock, target)
}

func TestWriteEnvFile(t *testing.T) {
	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "sessions", "s1")
	require.NoError(t, os.MkdirAll(sessionDir, 0755))

	env := map[string]string{
		"DISPLAY":        ":0",
		"SSH_CONNECTION": "1.2.3.4 22",
	}
	err := WriteEnvFile(sessionDir, env)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(sessionDir, "env"))
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "DISPLAY=:0")
	assert.Contains(t, content, "SSH_CONNECTION=1.2.3.4 22")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/daemon/ -run "TestForwardEnv|TestWriteEnvFile" -v`
Expected: FAIL

- [ ] **Step 3: Implement environment.go**

```go
// internal/daemon/environment.go
package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ForwardEnv forwards an environment variable to a session directory.
// If the value is an existing file or socket path, creates a stable symlink.
// The symlink target is updated on each call (for re-attach scenarios).
func ForwardEnv(sessionDir, varName, value string) error {
	symlinkPath := filepath.Join(sessionDir, varName)

	// Check if the value is a path to an existing file/socket
	if _, err := os.Stat(value); err != nil {
		return fmt.Errorf("environment: stat %q: %w", value, err)
	}

	// Remove existing symlink if present
	_ = os.Remove(symlinkPath)

	if err := os.Symlink(value, symlinkPath); err != nil {
		return fmt.Errorf("environment: symlink %q -> %q: %w", symlinkPath, value, err)
	}

	return nil
}

// WriteEnvFile writes non-path environment variables to a sourceable env file.
// Format: KEY=VALUE per line.
func WriteEnvFile(sessionDir string, env map[string]string) error {
	var lines []string
	for k, v := range env {
		lines = append(lines, fmt.Sprintf("%s=%s", k, v))
	}

	path := filepath.Join(sessionDir, "env")
	content := strings.Join(lines, "\n") + "\n"

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("environment: write env file: %w", err)
	}

	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/daemon/ -run "TestForwardEnv|TestWriteEnvFile" -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/environment.go internal/daemon/environment_test.go
git -c commit.gpgsign=false commit -m "feat(daemon): add environment forwarding with stable symlinks"
```

---

### Task 6: Wire OSC parsing and env forwarding into daemon

**Files:**
- Modify: `internal/daemon/service.go`
- Modify: `internal/daemon/entity.go`

- [ ] **Step 1: Add env forward handler**

In `internal/daemon/entity.go`:

```go
// EnvForwardRequest is the JSON payload for MsgEnvForward.
type EnvForwardRequest struct {
	SessionID string            `json:"session_id"`
	Env       map[string]string `json:"env"`
}
```

In `internal/daemon/service.go`, add handler and dispatch:

```go
func (d *Daemon) handleEnvForward(c ConnectedClient, frame protocol.Frame) {
	var req EnvForwardRequest
	if err := json.Unmarshal(frame.Payload, &req); err != nil {
		_ = c.Control().WriteFrame(errorFrame("invalid env forward request"))
		return
	}

	if d.dataDir == "" {
		_ = c.Control().WriteFrame(okFrame(nil))
		return
	}

	sessionDir := filepath.Join(d.dataDir, "sessions", req.SessionID)
	_ = os.MkdirAll(sessionDir, 0755)

	nonPathEnv := make(map[string]string)
	for k, v := range req.Env {
		if _, err := os.Stat(v); err == nil {
			_ = ForwardEnv(sessionDir, k, v)
		} else {
			nonPathEnv[k] = v
		}
	}
	if len(nonPathEnv) > 0 {
		_ = WriteEnvFile(sessionDir, nonPathEnv)
	}

	_ = c.Control().WriteFrame(okFrame(nil))
}
```

Add to `dispatch()`:

```go
case protocol.MsgEnvForward:
	d.handleEnvForward(c, frame)
```

- [ ] **Step 2: Add OSC scanning in flushOutput**

In `flushOutput`, after reading data and before broadcasting, scan for OSC sequences:

```go
// In flushOutput, after data is read from session:
oscResults := ParseOSC(data)
for _, osc := range oscResults {
	switch osc.Type {
	case OSCTypeCwd:
		_ = d.sessionSvc.MetaSet(sessID, "cwd", osc.Value)
		d.publishEvent(event.Event{
			Type:      event.CwdChanged,
			SessionID: sessID,
			Payload:   map[string]any{"cwd": osc.Value},
		})
	case OSCTypeNotification:
		d.publishEvent(event.Event{
			Type:      event.Notification,
			SessionID: sessID,
			Payload:   map[string]any{"body": osc.Value},
		})
	}
}
```

- [ ] **Step 3: Run tests**

Run: `make test`
Expected: ALL PASS

- [ ] **Step 4: Commit**

```bash
git add internal/daemon/entity.go internal/daemon/service.go
git -c commit.gpgsign=false commit -m "feat(daemon): wire OSC parsing and env forwarding into daemon loop"
```

---

### Task 7: Client library — metadata and environment

**Files:**
- Create: `pkg/client/metadata.go`
- Create: `pkg/client/metadata_test.go`

- [ ] **Step 1: Implement metadata methods**

```go
// pkg/client/metadata.go
package client

import (
	"encoding/json"
	"fmt"

	"github.com/wblech/wmux/internal/platform/protocol"
)

// MetaSet sets a metadata key-value pair on a session.
func (c *Client) MetaSet(sessionID, key, value string) error {
	payload, _ := json.Marshal(struct {
		SessionID string `json:"session_id"`
		Key       string `json:"key"`
		Value     string `json:"value"`
	}{SessionID: sessionID, Key: key, Value: value})

	resp, err := c.sendRequest(protocol.MsgMetaSet, payload)
	if err != nil {
		return err
	}
	if resp.Type == protocol.MsgError {
		return c.parseError(resp)
	}
	return nil
}

// MetaGet returns a single metadata value for a session.
func (c *Client) MetaGet(sessionID, key string) (string, error) {
	payload, _ := json.Marshal(struct {
		SessionID string `json:"session_id"`
		Key       string `json:"key"`
	}{SessionID: sessionID, Key: key})

	resp, err := c.sendRequest(protocol.MsgMetaGet, payload)
	if err != nil {
		return "", err
	}
	if resp.Type == protocol.MsgError {
		return "", c.parseError(resp)
	}

	var result struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(resp.Payload, &result); err != nil {
		return "", fmt.Errorf("client: unmarshal meta get: %w", err)
	}
	return result.Value, nil
}

// MetaGetAll returns all metadata for a session.
func (c *Client) MetaGetAll(sessionID string) (map[string]string, error) {
	payload, _ := json.Marshal(struct {
		SessionID string `json:"session_id"`
		Key       string `json:"key"`
	}{SessionID: sessionID, Key: ""})

	resp, err := c.sendRequest(protocol.MsgMetaGet, payload)
	if err != nil {
		return nil, err
	}
	if resp.Type == protocol.MsgError {
		return nil, c.parseError(resp)
	}

	var result struct {
		Metadata map[string]string `json:"metadata"`
	}
	if err := json.Unmarshal(resp.Payload, &result); err != nil {
		return nil, fmt.Errorf("client: unmarshal meta get all: %w", err)
	}
	return result.Metadata, nil
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./pkg/client/ -v`
Expected: ALL PASS

- [ ] **Step 3: Commit**

```bash
git add pkg/client/metadata.go pkg/client/metadata_test.go
git -c commit.gpgsign=false commit -m "feat(client): add MetaSet, MetaGet, MetaGetAll methods"
```

---

### Task 8: Lint and coverage check

- [ ] **Step 1: Run linter**

Run: `make lint`
Expected: PASS

- [ ] **Step 2: Check coverage**

Run: `go test -coverprofile=coverage.out ./internal/daemon/ ./internal/platform/event/ ./pkg/client/ && go tool cover -func=coverage.out`
Expected: >= 90% on new files

- [ ] **Step 3: Commit any fixes**

```bash
git add -A && git -c commit.gpgsign=false commit -m "fix: lint and coverage fixes for metadata/events/env"
```
