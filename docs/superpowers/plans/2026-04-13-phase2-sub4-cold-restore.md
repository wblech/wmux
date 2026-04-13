# Phase 2 Sub-plan 4: Cold Restore

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make cold restore opt-in via config. When enabled, daemon persists session history to disk. When disabled, zero disk I/O. Add `LoadSessionHistory` and `CleanSessionHistory` to the client library for lazy per-session recovery.

**Architecture:** The existing `history/` package already handles metadata and scrollback persistence. This sub-plan adds a `ColdRestore bool` field to config, guards history writes in the daemon with that flag, and adds read/cleanup functions to `pkg/client/`. The Watchtower flow is: try attach → fail → load history → render read-only → user acts → create new session with old cwd → clean up.

**Tech Stack:** Go 1.25+, existing packages

**Prerequisites:** Sub-plans 1-3 complete. Key existing code:
- `history.WriteMetadata(dir, meta)`, `history.ReadMetadata(dir)`, `history.UpdateMetadataExit(dir, time, code)`
- `history.ScrollbackWriter` with `Write([]byte)` and `Close()`
- `history.ListSessionDirs(dataDir)` returns session directories
- `config.HistoryConfig` — `MaxPerSession`, `MaxTotal`, `Recording`
- `daemon.Daemon.dataDir` — root directory for session data

---

## File Structure

```
internal/
├── platform/
│   └── config/
│       ├── config.go          # MODIFIED: add ColdRestore to HistoryConfig
│       └── config_test.go     # MODIFIED: test new field
├── daemon/
│   ├── service.go             # MODIFIED: guard history writes with cold_restore check
│   └── service_test.go        # MODIFIED: test with cold_restore on/off

pkg/
└── client/
    ├── entity.go              # MODIFIED: add SessionHistory type
    ├── restore.go             # NEW: LoadSessionHistory, CleanSessionHistory
    └── restore_test.go        # NEW
```

---

### Task 1: Config — add ColdRestore field

**Files:**
- Modify: `internal/platform/config/config.go`
- Modify: `internal/platform/config/config_test.go`

- [ ] **Step 1: Write failing test**

Add to `internal/platform/config/config_test.go`:

```go
func TestLoad_ColdRestore(t *testing.T) {
	content := `
[history]
cold_restore = true
`
	path := filepath.Join(t.TempDir(), "wmux.toml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.True(t, cfg.History.ColdRestore)
}

func TestDefaults_ColdRestoreFalse(t *testing.T) {
	cfg := defaults()
	assert.False(t, cfg.History.ColdRestore)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/platform/config/ -run "TestLoad_ColdRestore|TestDefaults_ColdRestoreFalse" -v`
Expected: FAIL — `ColdRestore` field not defined

- [ ] **Step 3: Add ColdRestore to HistoryConfig**

In `internal/platform/config/config.go`, update `HistoryConfig`:

```go
type HistoryConfig struct {
	MaxPerSession string `koanf:"max_per_session"`
	MaxTotal      string `koanf:"max_total"`
	Recording     bool   `koanf:"recording"`
	ColdRestore   bool   `koanf:"cold_restore"`
}
```

No change needed in `defaults()` — zero value `false` is correct.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/platform/config/ -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add internal/platform/config/config.go internal/platform/config/config_test.go
git -c commit.gpgsign=false commit -m "feat(config): add history.cold_restore setting (default false)"
```

---

### Task 2: Daemon — guard history writes

**Files:**
- Modify: `internal/daemon/service.go`
- Modify: `internal/daemon/options.go`
- Modify: `internal/daemon/service_test.go`

The daemon currently always writes history. We need to make it conditional on a `coldRestore` flag.

- [ ] **Step 1: Add coldRestore flag to Daemon**

In `internal/daemon/options.go`, add option:

```go
// WithColdRestore enables or disables cold restore (history persistence to disk).
func WithColdRestore(enabled bool) Option {
	return func(d *Daemon) {
		d.coldRestore = enabled
	}
}
```

Add `coldRestore bool` field to `Daemon` struct in `internal/daemon/service.go`.

- [ ] **Step 2: Guard history writes**

In `internal/daemon/service.go`, wherever history is written (session creation, exit), wrap with:

```go
if d.coldRestore {
    // write metadata, create scrollback writer, etc.
}
```

Find the exact locations by searching for `history.WriteMetadata`, `history.UpdateMetadataExit`, `ScrollbackWriter` usage in service.go. Wrap each with the guard.

- [ ] **Step 3: Write test for cold_restore disabled**

Add to `internal/daemon/service_test.go`:

```go
func TestDaemon_ColdRestore_Disabled_NoHistoryWritten(t *testing.T) {
	// Create daemon with WithColdRestore(false) and a dataDir
	// Create a session, let it exit
	// Verify no files were written to dataDir
}

func TestDaemon_ColdRestore_Enabled_WritesHistory(t *testing.T) {
	// Create daemon with WithColdRestore(true) and a dataDir
	// Create a session, let it exit
	// Verify meta.json and scrollback.bin exist in dataDir/session-id/
}
```

- [ ] **Step 4: Run tests**

Run: `make test`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/service.go internal/daemon/options.go internal/daemon/service_test.go
git -c commit.gpgsign=false commit -m "feat(daemon): guard history writes with cold_restore config flag"
```

---

### Task 3: Client library — SessionHistory type

**Files:**
- Modify: `pkg/client/entity.go`

- [ ] **Step 1: Add SessionHistory type**

In `pkg/client/entity.go`:

```go
// SessionHistory holds restored session data from disk (cold restore).
type SessionHistory struct {
	// Scrollback is the raw PTY output from the previous session.
	Scrollback []byte
	// SessionID is the session identifier.
	SessionID string
	// Shell is the shell binary path.
	Shell string
	// Cwd is the working directory at session creation.
	Cwd string
	// Cols is the terminal width at creation.
	Cols int
	// Rows is the terminal height at creation.
	Rows int
}
```

- [ ] **Step 2: Commit**

```bash
git add pkg/client/entity.go
git -c commit.gpgsign=false commit -m "feat(client): add SessionHistory type for cold restore"
```

---

### Task 4: Client library — LoadSessionHistory and CleanSessionHistory

**Files:**
- Create: `pkg/client/restore.go`
- Create: `pkg/client/restore_test.go`

- [ ] **Step 1: Write failing tests**

```go
// pkg/client/restore_test.go
package client

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wblech/wmux/internal/platform/history"
)

func TestLoadSessionHistory_Success(t *testing.T) {
	dataDir := t.TempDir()
	sessionDir := filepath.Join(dataDir, "test-session")
	require.NoError(t, os.MkdirAll(sessionDir, 0755))

	// Write metadata (no endedAt = unclean shutdown = restorable)
	meta := history.Metadata{
		SessionID: "test-session",
		Shell:     "/bin/zsh",
		Cwd:       "/home/user",
		Cols:      80,
		Rows:      24,
		StartedAt: time.Now().Add(-time.Hour),
	}
	require.NoError(t, history.WriteMetadata(sessionDir, meta))

	// Write scrollback
	scrollbackPath := filepath.Join(sessionDir, "scrollback.bin")
	require.NoError(t, os.WriteFile(scrollbackPath, []byte("hello world\n"), 0644))

	h, err := LoadSessionHistory(dataDir, "test-session")
	require.NoError(t, err)
	assert.Equal(t, "test-session", h.SessionID)
	assert.Equal(t, "/bin/zsh", h.Shell)
	assert.Equal(t, "/home/user", h.Cwd)
	assert.Equal(t, 80, h.Cols)
	assert.Equal(t, 24, h.Rows)
	assert.Equal(t, []byte("hello world\n"), h.Scrollback)
}

func TestLoadSessionHistory_NotFound(t *testing.T) {
	dataDir := t.TempDir()
	_, err := LoadSessionHistory(dataDir, "nonexistent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrColdRestoreNotAvailable)
}

func TestLoadSessionHistory_CleanExit(t *testing.T) {
	dataDir := t.TempDir()
	sessionDir := filepath.Join(dataDir, "test-session")
	require.NoError(t, os.MkdirAll(sessionDir, 0755))

	now := time.Now()
	exitCode := 0
	meta := history.Metadata{
		SessionID: "test-session",
		Shell:     "/bin/zsh",
		Cwd:       "/home/user",
		Cols:      80,
		Rows:      24,
		StartedAt: now.Add(-time.Hour),
		EndedAt:   &now,
		ExitCode:  &exitCode,
	}
	require.NoError(t, history.WriteMetadata(sessionDir, meta))

	_, err := LoadSessionHistory(dataDir, "test-session")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrColdRestoreNotAvailable)
}

func TestCleanSessionHistory(t *testing.T) {
	dataDir := t.TempDir()
	sessionDir := filepath.Join(dataDir, "test-session")
	require.NoError(t, os.MkdirAll(sessionDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sessionDir, "meta.json"), []byte("{}"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(sessionDir, "scrollback.bin"), []byte("data"), 0644))

	err := CleanSessionHistory(dataDir, "test-session")
	require.NoError(t, err)

	_, err = os.Stat(sessionDir)
	assert.True(t, os.IsNotExist(err))
}

func TestCleanSessionHistory_NotFound(t *testing.T) {
	dataDir := t.TempDir()
	err := CleanSessionHistory(dataDir, "nonexistent")
	assert.NoError(t, err) // idempotent
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/client/ -run "TestLoadSession|TestCleanSession" -v`
Expected: FAIL — functions not defined

- [ ] **Step 3: Implement restore.go**

```go
// pkg/client/restore.go
package client

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wblech/wmux/internal/platform/history"
)

// ErrColdRestoreNotAvailable is returned when no restorable history exists for a session.
var ErrColdRestoreNotAvailable = errors.New("client: cold restore not available")

// LoadSessionHistory reads the history files for a specific session from disk.
// Returns ErrColdRestoreNotAvailable if the session directory doesn't exist,
// has no metadata, or the session exited cleanly (endedAt is set).
func LoadSessionHistory(dataDir, sessionID string) (SessionHistory, error) {
	sessionDir := filepath.Join(dataDir, sessionID)

	meta, err := history.ReadMetadata(sessionDir)
	if err != nil {
		return SessionHistory{}, fmt.Errorf("%w: %s", ErrColdRestoreNotAvailable, err)
	}

	// Clean exit = not restorable
	if meta.EndedAt != nil {
		return SessionHistory{}, fmt.Errorf("%w: session exited cleanly", ErrColdRestoreNotAvailable)
	}

	var scrollback []byte
	scrollbackPath := filepath.Join(sessionDir, "scrollback.bin")
	if data, err := os.ReadFile(scrollbackPath); err == nil {
		scrollback = data
	}

	return SessionHistory{
		Scrollback: scrollback,
		SessionID:  meta.SessionID,
		Shell:      meta.Shell,
		Cwd:        meta.Cwd,
		Cols:       meta.Cols,
		Rows:       meta.Rows,
	}, nil
}

// CleanSessionHistory removes the history files for a session after the
// integrator has consumed them. This is idempotent — returns nil if the
// directory doesn't exist.
func CleanSessionHistory(dataDir, sessionID string) error {
	sessionDir := filepath.Join(dataDir, sessionID)

	if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
		return nil
	}

	if err := os.RemoveAll(sessionDir); err != nil {
		return fmt.Errorf("client: clean history: %w", err)
	}

	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/client/ -run "TestLoadSession|TestCleanSession" -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/client/restore.go pkg/client/restore_test.go
git -c commit.gpgsign=false commit -m "feat(client): add LoadSessionHistory and CleanSessionHistory for cold restore"
```

---

### Task 5: Lint and coverage check

- [ ] **Step 1: Run linter**

Run: `make lint`
Expected: PASS

- [ ] **Step 2: Check coverage**

Run: `go test -coverprofile=coverage.out ./pkg/client/ ./internal/daemon/ ./internal/platform/config/ && go tool cover -func=coverage.out`
Expected: >= 90% on new/modified files

- [ ] **Step 3: Commit any fixes**

```bash
git add -A && git -c commit.gpgsign=false commit -m "fix: lint and coverage fixes for cold restore"
```
