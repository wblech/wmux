# Phase 2 Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix 6 issues found in the Phase 2 code review — 1 critical path bug, 3 major missing-wiring issues, 2 minor hygiene items.

**Architecture:** All fixes are independent and localized. No new packages. Uses existing patterns: functional options for daemon config, mock server for client tests, `history.ParseSize` for size strings.

**Tech Stack:** Go, testify, go.uber.org/mock

**Prerequisites:** Phase 2 Sub-plans 1-5 complete. All existing tests passing.

**References:**
- [Phase 2 Fixes Spec](../specs/2026-04-13-phase2-fixes-design.md)
- [Phase 2 Design Spec](../specs/2026-04-13-phase2-watchtower-integration-design.md)

---

### Task 1: Fix double "sessions" path in `handleEnvForward`

**Files:**
- Modify: `internal/daemon/service.go:741`

- [ ] **Step 1: Run existing env forward test to confirm it passes**

```bash
go test ./internal/daemon/ -run TestDaemon_EnvForward -v -count=1
```

Expected: PASS (test uses temp dir so the double "sessions" creates nested dirs silently)

- [ ] **Step 2: Fix the path**

In `internal/daemon/service.go`, change line 741 from:

```go
sessionDir := filepath.Join(d.dataDir, "sessions", req.SessionID)
```

to:

```go
sessionDir := filepath.Join(d.dataDir, req.SessionID)
```

- [ ] **Step 3: Run env forward tests**

```bash
go test ./internal/daemon/ -run TestDaemon_EnvForward -v -count=1
```

Expected: all 3 env forward tests PASS

- [ ] **Step 4: Commit**

```bash
git add internal/daemon/service.go
git -c commit.gpgsign=false commit -m "fix(daemon): remove double sessions segment in handleEnvForward path"
```

---

### Task 2: Export config defaults + load config in `cmd/wmux/daemon.go`

**Files:**
- Modify: `internal/platform/config/config.go` — export `Defaults()`
- Modify: `internal/platform/config/config_test.go` — add test for `Defaults()`
- Modify: `cmd/wmux/daemon.go` — load config, wire `WithColdRestore`

- [ ] **Step 1: Write test for exported Defaults**

In `internal/platform/config/config_test.go`, add:

```go
func TestDefaults_ReturnsValidConfig(t *testing.T) {
	cfg := Defaults()
	assert.Equal(t, "none", cfg.Emulator.Backend)
	assert.False(t, cfg.History.ColdRestore)
	assert.Equal(t, "0", cfg.History.MaxPerSession)
	assert.Equal(t, "same-user", cfg.Daemon.AutomationMode)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/platform/config/ -run TestDefaults_ReturnsValidConfig -v -count=1
```

Expected: FAIL — `Defaults` is not exported (only `defaults` exists)

- [ ] **Step 3: Export Defaults**

In `internal/platform/config/config.go`, rename `defaults` to `Defaults`:

Change:
```go
func defaults() *Config {
```
to:
```go
// Defaults returns a Config populated with all PRD-specified default values.
func Defaults() *Config {
```

Also update the one caller in `Load`:

Change:
```go
cfg := defaults()
```
to:
```go
cfg := Defaults()
```

- [ ] **Step 4: Run test**

```bash
go test ./internal/platform/config/ -run TestDefaults -v -count=1
```

Expected: PASS (both `TestDefaults_ReturnsValidConfig` and existing `TestDefaults_ColdRestoreFalse`)

- [ ] **Step 5: Wire config loading in cmd/wmux/daemon.go**

In `cmd/wmux/daemon.go`, add config import and loading. Add to imports:

```go
"github.com/wblech/wmux/internal/platform/config"
```

After the flag parsing loop (after line 50) and before the `MkdirAll` calls, add:

```go
	// Load config (optional — defaults used if file absent).
	configPath := filepath.Join(baseDir, "config.toml")
	cfg := config.Defaults()
	if _, err := os.Stat(configPath); err == nil {
		loaded, err := config.Load(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: load config: %v\n", err)
			return 1
		}
		cfg = loaded
	}
```

Then add `WithColdRestore` to the daemon constructor (after line 92):

```go
		daemon.WithColdRestore(cfg.History.ColdRestore),
```

- [ ] **Step 6: Build to verify compilation**

```bash
go build ./cmd/wmux/
```

Expected: compiles cleanly

- [ ] **Step 7: Commit**

```bash
git add internal/platform/config/config.go internal/platform/config/config_test.go cmd/wmux/daemon.go
git -c commit.gpgsign=false commit -m "feat(cmd): load config.toml and wire WithColdRestore in daemon startup"
```

---

### Task 3: Add `WithMaxScrollbackSize` option and wire it

**Files:**
- Modify: `internal/daemon/options.go` — add `WithMaxScrollbackSize`
- Modify: `internal/daemon/service.go:135-166` — add field, use in `persistSessionCreate`
- Modify: `internal/daemon/service_test.go` — add test
- Modify: `cmd/wmux/daemon.go` — parse and pass

- [ ] **Step 1: Write test for WithMaxScrollbackSize**

In `internal/daemon/service_test.go`, add:

```go
func TestDaemon_ColdRestore_MaxScrollbackSize(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	dataDir := t.TempDir()
	eb := event.NewBus()
	defer eb.Close()

	d, token, sock := testDaemon(t,
		WithDataDir(dataDir),
		WithColdRestore(true),
		WithMaxScrollbackSize(1024),
		WithEventBus(eb),
	)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	createResp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "scroll-test",
		Shell: "/bin/sh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
	})
	require.Equal(t, protocol.MsgOK, createResp.Type)

	// Verify the scrollback writer was created.
	sessionDir := filepath.Join(dataDir, "scroll-test")
	scrollbackPath := filepath.Join(sessionDir, "scrollback.bin")

	require.Eventually(t, func() bool {
		_, err := os.Stat(scrollbackPath)
		return err == nil
	}, 2*time.Second, 50*time.Millisecond)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/daemon/ -run TestDaemon_ColdRestore_MaxScrollbackSize -v -count=1
```

Expected: FAIL — `WithMaxScrollbackSize` undefined

- [ ] **Step 3: Add field to Daemon struct**

In `internal/daemon/service.go`, add to the Daemon struct fields (after `coldRestore bool`):

```go
	maxScrollbackSize int64
```

And in `NewDaemon` defaults (after `coldRestore: false`):

```go
		maxScrollbackSize: 0,
```

- [ ] **Step 4: Add WithMaxScrollbackSize option**

In `internal/daemon/options.go`, add:

```go
// WithMaxScrollbackSize sets the maximum scrollback file size in bytes for cold restore.
// A value of 0 means unlimited.
func WithMaxScrollbackSize(n int64) Option {
	return func(d *Daemon) {
		d.maxScrollbackSize = n
	}
}
```

- [ ] **Step 5: Use field in persistSessionCreate**

In `internal/daemon/service.go`, change line 786 from:

```go
	w, err := history.NewWriter(filepath.Join(sessionDir, "scrollback.bin"), 0)
```

to:

```go
	w, err := history.NewWriter(filepath.Join(sessionDir, "scrollback.bin"), d.maxScrollbackSize)
```

- [ ] **Step 6: Run test**

```bash
go test ./internal/daemon/ -run TestDaemon_ColdRestore_MaxScrollbackSize -v -count=1
```

Expected: PASS

- [ ] **Step 7: Wire in cmd/wmux/daemon.go**

In `cmd/wmux/daemon.go`, after the config loading block (added in Task 2), add before the daemon constructor:

```go
	maxScrollback, err := history.ParseSize(cfg.History.MaxPerSession)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: parse max_per_session: %v\n", err)
		return 1
	}
```

Add to imports:

```go
"github.com/wblech/wmux/internal/platform/history"
```

Add to daemon constructor options:

```go
		daemon.WithMaxScrollbackSize(maxScrollback),
```

- [ ] **Step 8: Build**

```bash
go build ./cmd/wmux/
```

Expected: compiles cleanly

- [ ] **Step 9: Commit**

```bash
git add internal/daemon/options.go internal/daemon/service.go internal/daemon/service_test.go cmd/wmux/daemon.go
git -c commit.gpgsign=false commit -m "feat(daemon): add WithMaxScrollbackSize option and wire from config"
```

---

### Task 4: Add `pkg/client/environment.go` with ForwardEnv

**Files:**
- Create: `pkg/client/environment.go`
- Create: `pkg/client/environment_test.go`

- [ ] **Step 1: Write tests**

Create `pkg/client/environment_test.go`:

```go
package client

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wblech/wmux/internal/platform/protocol"
)

func TestClient_ForwardEnv(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgEnvForward: func(payload []byte) protocol.Frame {
			var req struct {
				SessionID string            `json:"session_id"`
				Env       map[string]string `json:"env"`
			}
			if err := json.Unmarshal(payload, &req); err != nil {
				return errFrame("bad payload")
			}
			if req.SessionID != "s1" {
				return errFrame("unexpected session")
			}
			if req.Env["SSH_AUTH_SOCK"] != "/tmp/ssh-xxx" {
				return errFrame("unexpected env")
			}
			return okFrame(nil)
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	err = c.ForwardEnv("s1", map[string]string{"SSH_AUTH_SOCK": "/tmp/ssh-xxx"})
	assert.NoError(t, err)
}

func TestClient_ForwardEnv_EmptyMap(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgEnvForward: func(_ []byte) protocol.Frame {
			return okFrame(nil)
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	err = c.ForwardEnv("s1", map[string]string{})
	assert.NoError(t, err)
}

func TestClient_ForwardEnv_Error(t *testing.T) {
	socketPath, tokenPath, cleanup := startMockServerWithHandlers(t, map[protocol.MessageType]handlerFunc{
		protocol.MsgEnvForward: func(_ []byte) protocol.Frame {
			return errFrame("session not found")
		},
	})
	defer cleanup()

	c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	err = c.ForwardEnv("no-such", map[string]string{"K": "V"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./pkg/client/ -run TestClient_ForwardEnv -v -count=1
```

Expected: FAIL — `c.ForwardEnv` undefined

- [ ] **Step 3: Implement ForwardEnv**

Create `pkg/client/environment.go`:

```go
package client

import (
	"encoding/json"
	"fmt"

	"github.com/wblech/wmux/internal/platform/protocol"
)

// ForwardEnv sends environment variables to the daemon for a session.
// The daemon creates stable symlinks for socket/file paths and writes
// an env file for other values.
func (c *Client) ForwardEnv(sessionID string, env map[string]string) error {
	payload, err := json.Marshal(struct {
		SessionID string            `json:"session_id"`
		Env       map[string]string `json:"env"`
	}{SessionID: sessionID, Env: env})
	if err != nil {
		return fmt.Errorf("client: marshal env forward: %w", err)
	}

	resp, err := c.sendRequest(protocol.MsgEnvForward, payload)
	if err != nil {
		return err
	}
	if resp.Type == protocol.MsgError {
		return c.parseError(resp)
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./pkg/client/ -run TestClient_ForwardEnv -v -count=1
```

Expected: all 3 PASS

- [ ] **Step 5: Run full client test suite**

```bash
go test ./pkg/client/ -v -count=1
```

Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/client/environment.go pkg/client/environment_test.go
git -c commit.gpgsign=false commit -m "feat(client): add ForwardEnv method for environment variable forwarding"
```

---

### Task 5: Add `addons/xterm/.gitignore`

**Files:**
- Create: `addons/xterm/.gitignore`

- [ ] **Step 1: Create gitignore**

Create `addons/xterm/.gitignore`:

```
node_modules/
dist/
```

- [ ] **Step 2: Commit**

```bash
git add addons/xterm/.gitignore
git -c commit.gpgsign=false commit -m "chore: add .gitignore for xterm addon build artifacts"
```

---

### Task 6: Log errors in `handleEnvForward`

**Files:**
- Modify: `internal/daemon/service.go:728-757`

The daemon `service.go` currently has zero logging — all errors are discarded with `_ =`. Rather than introducing `fmt.Fprintf(os.Stderr, ...)` inconsistently in one function, we add a `logErr` helper at the package level that writes to stderr. This can be replaced with structured logging later (Phase 3).

- [ ] **Step 1: Add logErr helper**

At the top of `internal/daemon/service.go`, after the imports, add:

```go
// logErr logs a non-fatal error to stderr. Intended for best-effort operations
// where the error should be observable but does not affect the response.
func logErr(context string, err error) {
	fmt.Fprintf(os.Stderr, "wmux: %s: %v\n", context, err)
}
```

Add `"fmt"` and `"os"` to imports if not already present.

- [ ] **Step 2: Use logErr in handleEnvForward**

In `handleEnvForward`, change lines 746-753 from:

```go
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
```

to:

```go
	nonPathEnv := make(map[string]string)
	for k, v := range req.Env {
		if _, err := os.Stat(v); err == nil {
			if err := ForwardEnv(sessionDir, k, v); err != nil {
				logErr("env forward symlink", err)
			}
		} else {
			nonPathEnv[k] = v
		}
	}
	if len(nonPathEnv) > 0 {
		if err := WriteEnvFile(sessionDir, nonPathEnv); err != nil {
			logErr("env forward write", err)
		}
	}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/daemon/ -run TestDaemon_EnvForward -v -count=1
```

Expected: all PASS

- [ ] **Step 4: Run lint**

```bash
make lint
```

Expected: no new errors

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/service.go
git -c commit.gpgsign=false commit -m "fix(daemon): log env forwarding errors instead of silently discarding"
```

---

### Task 7: Full verification

- [ ] **Step 1: Run all tests**

```bash
make test
```

Expected: all PASS

- [ ] **Step 2: Run lint**

```bash
make lint
```

Expected: clean (0 new errors)

- [ ] **Step 3: Check coverage for affected packages**

```bash
go test -coverprofile=cover.out ./internal/daemon/ ./pkg/client/ ./internal/platform/config/ && go tool cover -func=cover.out | grep -E "^(internal/daemon|pkg/client|internal/platform/config)" | tail -5
```

Expected: all ≥ 90%
