# Client SDK Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor `pkg/client/` from a flat-struct `Connect(Options{})` API to a functional-options `New(opts...)` SDK with embedded daemon support (`NewDaemon`, `ServeDaemon`).

**Architecture:** New `options.go` defines `Option func(*config)` with `With*` constructors. `namespace.go` resolves paths from namespace. `New` replaces `Connect` as the entry point with auto-start support. `NewDaemon` encapsulates the daemon wiring from `cmd/wmux/daemon.go` into a public API. `ServeDaemon` provides the auto-start hook for integrators.

**Tech Stack:** Go, testify, creack/pty, uber/fx (internal only)

**Prerequisites:** Phase 2 complete with all fixes applied.

**References:**
- [Client SDK Spec](../specs/2026-04-13-client-sdk-design.md)
- [Phase 2 Design Spec](../specs/2026-04-13-phase2-watchtower-integration-design.md)

---

## File Structure

| File | Responsibility | Action |
|---|---|---|
| `pkg/client/options.go` | `Option` type, `config` struct, all `With*` functions | Create |
| `pkg/client/options_test.go` | Tests for options and namespace resolution | Create |
| `pkg/client/namespace.go` | `resolveConfig` — derive paths from namespace + overrides | Create |
| `pkg/client/namespace_test.go` | Tests for path resolution | Create |
| `pkg/client/entity.go` | Remove `Options` struct | Modify |
| `pkg/client/entity_test.go` | Update if needed | Modify |
| `pkg/client/client.go` | `New` replaces `Connect`, auto-start logic | Modify |
| `pkg/client/client_test.go` | Migrate all `Connect(Options{})` → `New(WithSocket, WithTokenPath)` | Modify |
| `pkg/client/metadata_test.go` | Migrate `Connect` calls | Modify |
| `pkg/client/environment_test.go` | Migrate `Connect` calls | Modify |
| `pkg/client/restore.go` | Keep as-is (dataDir passed directly) | — |
| `pkg/client/daemon.go` | `Daemon` type, `NewDaemon`, `ServeDaemon` | Create |
| `pkg/client/daemon_test.go` | Tests for NewDaemon and ServeDaemon | Create |
| `cmd/wmux/daemon.go` | Dogfood via `client.NewDaemon` | Modify |
| `cmd/wmux/adapter.go` | Remove (adapters move into daemon.go) | Modify/Remove |
| `docs/integration-guide.md` | Full rewrite | Modify |

---

### Task 1: Create options.go with Option type and With* functions

**Files:**
- Create: `pkg/client/options.go`
- Create: `pkg/client/options_test.go`

- [ ] **Step 1: Write tests for options**

Create `pkg/client/options_test.go`:

```go
package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaults(t *testing.T) {
	cfg := newConfig()
	assert.Equal(t, "default", cfg.namespace)
	assert.Equal(t, "", cfg.socket)
	assert.Equal(t, "", cfg.tokenPath)
	assert.Equal(t, "", cfg.dataDir)
	assert.Equal(t, "", cfg.baseDir)
	assert.True(t, cfg.autoStart)
	assert.False(t, cfg.coldRestore)
	assert.Equal(t, int64(0), cfg.maxScrollbackSize)
	assert.Equal(t, "none", cfg.emulatorBackend)
}

func TestWithNamespace(t *testing.T) {
	cfg := newConfig(WithNamespace("watchtower"))
	assert.Equal(t, "watchtower", cfg.namespace)
}

func TestWithSocket(t *testing.T) {
	cfg := newConfig(WithSocket("/custom/path.sock"))
	assert.Equal(t, "/custom/path.sock", cfg.socket)
}

func TestWithTokenPath(t *testing.T) {
	cfg := newConfig(WithTokenPath("/custom/token"))
	assert.Equal(t, "/custom/token", cfg.tokenPath)
}

func TestWithDataDir(t *testing.T) {
	cfg := newConfig(WithDataDir("/custom/data"))
	assert.Equal(t, "/custom/data", cfg.dataDir)
}

func TestWithBaseDir(t *testing.T) {
	cfg := newConfig(WithBaseDir("/custom/base"))
	assert.Equal(t, "/custom/base", cfg.baseDir)
}

func TestWithAutoStart(t *testing.T) {
	cfg := newConfig(WithAutoStart(false))
	assert.False(t, cfg.autoStart)
}

func TestWithColdRestore(t *testing.T) {
	cfg := newConfig(WithColdRestore(true))
	assert.True(t, cfg.coldRestore)
}

func TestWithMaxScrollbackSize(t *testing.T) {
	cfg := newConfig(WithMaxScrollbackSize(10 * 1024 * 1024))
	assert.Equal(t, int64(10*1024*1024), cfg.maxScrollbackSize)
}

func TestWithEmulatorBackend(t *testing.T) {
	cfg := newConfig(WithEmulatorBackend("xterm"))
	assert.Equal(t, "xterm", cfg.emulatorBackend)
}

func TestMultipleOptions(t *testing.T) {
	cfg := newConfig(
		WithNamespace("watchtower"),
		WithColdRestore(true),
		WithEmulatorBackend("xterm"),
		WithMaxScrollbackSize(1024),
	)
	assert.Equal(t, "watchtower", cfg.namespace)
	assert.True(t, cfg.coldRestore)
	assert.Equal(t, "xterm", cfg.emulatorBackend)
	assert.Equal(t, int64(1024), cfg.maxScrollbackSize)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./pkg/client/ -run TestDefaults -v -count=1
```

Expected: FAIL — `newConfig` undefined

- [ ] **Step 3: Implement options.go**

Create `pkg/client/options.go`:

```go
package client

// Option configures a client or daemon instance.
type Option func(*config)

// config holds resolved configuration for client and daemon construction.
type config struct {
	// Namespace and paths
	namespace string
	baseDir   string
	socket    string
	tokenPath string
	dataDir   string

	// Daemon configuration
	coldRestore       bool
	maxScrollbackSize int64
	emulatorBackend   string

	// Client behavior
	autoStart bool
}

// newConfig creates a config with defaults and applies the given options.
func newConfig(opts ...Option) *config {
	cfg := &config{
		namespace:         "default",
		baseDir:           "",
		socket:            "",
		tokenPath:         "",
		dataDir:           "",
		coldRestore:       false,
		maxScrollbackSize: 0,
		emulatorBackend:   "none",
		autoStart:         true,
	}
	for _, o := range opts {
		o(cfg)
	}
	return cfg
}

// WithNamespace sets the isolation namespace. Each namespace gets its own
// daemon socket, token, PID file, and data directory under the base dir.
// Default: "default".
func WithNamespace(name string) Option {
	return func(c *config) {
		c.namespace = name
	}
}

// WithBaseDir sets the root directory for all wmux data.
// Default: ~/.wmux
func WithBaseDir(path string) Option {
	return func(c *config) {
		c.baseDir = path
	}
}

// WithSocket overrides the daemon socket path derived from namespace.
func WithSocket(path string) Option {
	return func(c *config) {
		c.socket = path
	}
}

// WithTokenPath overrides the auth token path derived from namespace.
func WithTokenPath(path string) Option {
	return func(c *config) {
		c.tokenPath = path
	}
}

// WithDataDir overrides the session data directory derived from namespace.
func WithDataDir(path string) Option {
	return func(c *config) {
		c.dataDir = path
	}
}

// WithAutoStart controls whether New automatically starts a daemon if
// none is running. Default: true.
func WithAutoStart(enabled bool) Option {
	return func(c *config) {
		c.autoStart = enabled
	}
}

// WithColdRestore enables or disables cold restore (scrollback persistence
// to disk). Passed to the daemon on auto-start.
func WithColdRestore(enabled bool) Option {
	return func(c *config) {
		c.coldRestore = enabled
	}
}

// WithMaxScrollbackSize sets the maximum scrollback file size in bytes.
// Passed to the daemon on auto-start. 0 means unlimited.
func WithMaxScrollbackSize(n int64) Option {
	return func(c *config) {
		c.maxScrollbackSize = n
	}
}

// WithEmulatorBackend sets the headless emulator backend ("none" or "xterm").
// Passed to the daemon on auto-start.
func WithEmulatorBackend(backend string) Option {
	return func(c *config) {
		c.emulatorBackend = backend
	}
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./pkg/client/ -run "TestDefaults|TestWith|TestMultiple" -v -count=1
```

Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/client/options.go pkg/client/options_test.go
git -c commit.gpgsign=false commit -m "feat(client): add functional options with Option type and With* constructors"
```

---

### Task 2: Create namespace.go with path resolution

**Files:**
- Create: `pkg/client/namespace.go`
- Create: `pkg/client/namespace_test.go`

- [ ] **Step 1: Write tests for namespace resolution**

Create `pkg/client/namespace_test.go`:

```go
package client

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveConfig_Defaults(t *testing.T) {
	cfg := newConfig(WithBaseDir("/tmp/wmux-test"))
	resolveConfig(cfg)

	assert.Equal(t, filepath.Join("/tmp/wmux-test", "default", "daemon.sock"), cfg.socket)
	assert.Equal(t, filepath.Join("/tmp/wmux-test", "default", "daemon.token"), cfg.tokenPath)
	assert.Equal(t, filepath.Join("/tmp/wmux-test", "default", "sessions"), cfg.dataDir)
}

func TestResolveConfig_CustomNamespace(t *testing.T) {
	cfg := newConfig(WithBaseDir("/tmp/wmux-test"), WithNamespace("watchtower"))
	resolveConfig(cfg)

	assert.Equal(t, filepath.Join("/tmp/wmux-test", "watchtower", "daemon.sock"), cfg.socket)
	assert.Equal(t, filepath.Join("/tmp/wmux-test", "watchtower", "daemon.token"), cfg.tokenPath)
	assert.Equal(t, filepath.Join("/tmp/wmux-test", "watchtower", "sessions"), cfg.dataDir)
}

func TestResolveConfig_OverrideSocket(t *testing.T) {
	cfg := newConfig(
		WithBaseDir("/tmp/wmux-test"),
		WithNamespace("watchtower"),
		WithSocket("/custom/path.sock"),
	)
	resolveConfig(cfg)

	assert.Equal(t, "/custom/path.sock", cfg.socket)
	assert.Equal(t, filepath.Join("/tmp/wmux-test", "watchtower", "daemon.token"), cfg.tokenPath)
}

func TestResolveConfig_OverrideAll(t *testing.T) {
	cfg := newConfig(
		WithSocket("/a.sock"),
		WithTokenPath("/a.token"),
		WithDataDir("/a/data"),
	)
	resolveConfig(cfg)

	assert.Equal(t, "/a.sock", cfg.socket)
	assert.Equal(t, "/a.token", cfg.tokenPath)
	assert.Equal(t, "/a/data", cfg.dataDir)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./pkg/client/ -run TestResolveConfig -v -count=1
```

Expected: FAIL — `resolveConfig` undefined

- [ ] **Step 3: Implement namespace.go**

Create `pkg/client/namespace.go`:

```go
package client

import (
	"os"
	"path/filepath"
)

// defaultBaseDir returns ~/.wmux as the default base directory.
func defaultBaseDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "wmux")
	}
	return filepath.Join(home, ".wmux")
}

// resolveConfig fills in any empty paths using namespace-derived defaults.
// Explicit With* overrides are preserved.
func resolveConfig(cfg *config) {
	if cfg.baseDir == "" {
		cfg.baseDir = defaultBaseDir()
	}

	nsDir := filepath.Join(cfg.baseDir, cfg.namespace)

	if cfg.socket == "" {
		cfg.socket = filepath.Join(nsDir, "daemon.sock")
	}
	if cfg.tokenPath == "" {
		cfg.tokenPath = filepath.Join(nsDir, "daemon.token")
	}
	if cfg.dataDir == "" {
		cfg.dataDir = filepath.Join(nsDir, "sessions")
	}
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./pkg/client/ -run TestResolveConfig -v -count=1
```

Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/client/namespace.go pkg/client/namespace_test.go
git -c commit.gpgsign=false commit -m "feat(client): add namespace-based path resolution"
```

---

### Task 3: Refactor client.go — New replaces Connect

**Files:**
- Modify: `pkg/client/entity.go` — remove `Options` struct
- Modify: `pkg/client/client.go` — `New` replaces `Connect`
- Modify: `pkg/client/client_test.go` — migrate all tests
- Modify: `pkg/client/metadata_test.go` — migrate `Connect` calls
- Modify: `pkg/client/environment_test.go` — migrate `Connect` calls

This task is a mechanical refactor: every `Connect(Options{SocketPath: x, TokenPath: y})` becomes `New(WithSocket(x), WithTokenPath(y), WithAutoStart(false))`. We add `WithAutoStart(false)` because tests use mock servers (no real daemon to auto-start).

- [ ] **Step 1: Add New function to client.go**

Replace the `Connect` function in `pkg/client/client.go` with `New`. The full file becomes:

```go
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

// New creates a client connected to a wmux daemon. Options control namespace,
// paths, and daemon configuration. With default options, New resolves paths
// from the "default" namespace under ~/.wmux and auto-starts a daemon if
// none is running.
//
// For tests or manual daemon management, use WithAutoStart(false) and
// WithSocket/WithTokenPath to connect to a known daemon.
func New(opts ...Option) (*Client, error) {
	cfg := newConfig(opts...)
	resolveConfig(cfg)

	return connect(cfg)
}

// connect performs the low-level dial + auth handshake.
func connect(cfg *config) (*Client, error) {
	token, err := auth.LoadFromFile(cfg.tokenPath)
	if err != nil {
		return nil, fmt.Errorf("client: read token: %w", err)
	}

	conn, err := net.Dial("unix", cfg.socket)
	if err != nil {
		return nil, fmt.Errorf("client: dial: %w", err)
	}

	pConn := protocol.NewConn(conn)

	if err := pConn.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgAuth,
		Payload: token,
	}); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("client: auth write: %w", err)
	}

	frame, err := pConn.ReadFrame()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("client: auth read: %w", err)
	}

	if frame.Type != protocol.MsgOK {
		_ = conn.Close()
		return nil, fmt.Errorf("client: auth failed")
	}

	return &Client{
		mu:          sync.Mutex{},
		conn:        conn,
		pConn:       pConn,
		dataHandler: nil,
		evtHandler:  nil,
	}, nil
}

// Close closes the connection to the daemon.
func (c *Client) Close() error {
	if err := c.conn.Close(); err != nil {
		return fmt.Errorf("client: close: %w", err)
	}
	return nil
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

- [ ] **Step 2: Remove Options struct from entity.go**

In `pkg/client/entity.go`, remove the `Options` struct (lines 4-10):

```go
// Options configures the client connection to the wmux daemon.
type Options struct {
	// SocketPath is the Unix socket to connect to.
	SocketPath string
	// TokenPath is the token file for authentication.
	TokenPath string
}
```

- [ ] **Step 3: Migrate client_test.go**

In `pkg/client/client_test.go`, replace every `Connect(Options{SocketPath: x, TokenPath: y})` with `New(WithSocket(x), WithTokenPath(y), WithAutoStart(false))`.

The pattern is mechanical. For example, change:

```go
c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
```

to:

```go
c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
```

Apply this to all test functions in `client_test.go`. There are 22 call sites. Also update `TestConnect_Success` → `TestNew_Success`, `TestConnect_BadSocket` → `TestNew_BadSocket`, `TestConnect_BadToken` → `TestNew_BadToken`, `TestConnect_AuthRejected` → `TestNew_AuthRejected`.

- [ ] **Step 4: Migrate metadata_test.go**

Same mechanical replacement in `pkg/client/metadata_test.go`. There are 5 call sites:

```go
c, err := Connect(Options{SocketPath: socketPath, TokenPath: tokenPath})
```
→
```go
c, err := New(WithSocket(socketPath), WithTokenPath(tokenPath), WithAutoStart(false))
```

- [ ] **Step 5: Migrate environment_test.go**

Same replacement in `pkg/client/environment_test.go`. There are 3 call sites.

- [ ] **Step 6: Run all client tests**

```bash
go test ./pkg/client/ -v -count=1
```

Expected: all PASS

- [ ] **Step 7: Run lint**

```bash
make lint 2>&1 | head -20
```

Expected: 0 errors

- [ ] **Step 8: Commit**

```bash
git add pkg/client/client.go pkg/client/entity.go pkg/client/client_test.go pkg/client/metadata_test.go pkg/client/environment_test.go
git -c commit.gpgsign=false commit -m "refactor(client): replace Connect(Options) with New(opts ...Option)"
```

---

### Task 4: Create daemon.go with NewDaemon and ServeDaemon

**Files:**
- Create: `pkg/client/daemon.go`
- Create: `pkg/client/daemon_test.go`

- [ ] **Step 1: Write tests for NewDaemon**

Create `pkg/client/daemon_test.go`:

```go
package client

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDaemon(t *testing.T) {
	dir := shortTempDir(t)
	d, err := NewDaemon(
		WithBaseDir(dir),
		WithNamespace("test"),
	)
	require.NoError(t, err)
	require.NotNil(t, d)
}

func TestNewDaemon_ServeAndConnect(t *testing.T) {
	dir := shortTempDir(t)
	d, err := NewDaemon(
		WithBaseDir(dir),
		WithNamespace("test"),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start daemon in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Serve(ctx)
	}()

	// Wait for socket to be ready
	require.Eventually(t, func() bool {
		c, err := New(
			WithBaseDir(dir),
			WithNamespace("test"),
			WithAutoStart(false),
		)
		if err != nil {
			return false
		}
		_ = c.Close()
		return true
	}, 3*time.Second, 50*time.Millisecond)

	// Connect and use
	c, err := New(
		WithBaseDir(dir),
		WithNamespace("test"),
		WithAutoStart(false),
	)
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	sessions, err := c.List()
	require.NoError(t, err)
	assert.Empty(t, sessions)

	// Shutdown
	cancel()
}

func TestServeDaemon_NotDaemonMode(t *testing.T) {
	handled := ServeDaemon([]string{"watchtower", "--some-flag"})
	assert.False(t, handled)
}

func TestServeDaemon_DetectsSentinel(t *testing.T) {
	// Just verify it detects the sentinel — actual daemon start
	// would block, so we only test detection.
	args := []string{"watchtower", "__wmux_daemon__", "--base-dir", "/tmp/test", "--namespace", "test"}
	// We can't easily test full ServeDaemon without it blocking,
	// but we can test the detection logic.
	assert.True(t, isDaemonMode(args))
	assert.False(t, isDaemonMode([]string{"watchtower", "--help"}))
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./pkg/client/ -run "TestNewDaemon|TestServeDaemon" -v -count=1
```

Expected: FAIL — `NewDaemon` undefined

- [ ] **Step 3: Implement daemon.go**

Create `pkg/client/daemon.go`:

```go
package client

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wblech/wmux/internal/daemon"
	"github.com/wblech/wmux/internal/platform/auth"
	"github.com/wblech/wmux/internal/platform/event"
	"github.com/wblech/wmux/internal/platform/ipc"
	"github.com/wblech/wmux/internal/platform/pty"
	"github.com/wblech/wmux/internal/session"
	"github.com/wblech/wmux/internal/transport"
)

// Daemon is an embeddable wmux daemon that integrators can run in-process
// or as a background service.
type Daemon struct {
	cfg      *config
	eventBus *event.Bus
}

// NewDaemon creates a daemon configured from the given options.
// Call Serve to start listening for client connections.
func NewDaemon(opts ...Option) (*Daemon, error) {
	cfg := newConfig(opts...)
	resolveConfig(cfg)

	nsDir := filepath.Dir(cfg.socket)
	if err := os.MkdirAll(nsDir, 0o700); err != nil {
		return nil, fmt.Errorf("client: create namespace dir: %w", err)
	}

	if err := os.MkdirAll(cfg.dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("client: create data dir: %w", err)
	}

	return &Daemon{
		cfg:      cfg,
		eventBus: event.NewBus(),
	}, nil
}

// Serve starts the daemon, listening for client connections on the configured
// socket. Blocks until ctx is cancelled. Returns nil on clean shutdown.
func (d *Daemon) Serve(ctx context.Context) error {
	defer d.eventBus.Close()

	token, err := auth.Ensure(d.cfg.tokenPath)
	if err != nil {
		return fmt.Errorf("client: ensure token: %w", err)
	}

	listener, err := ipc.Listen(d.cfg.socket)
	if err != nil {
		return fmt.Errorf("client: listen: %w", err)
	}

	server := transport.NewServer(listener, token)
	spawner := &pty.UnixSpawner{}
	sessionSvc := session.NewService(spawner)

	pidFile := filepath.Join(filepath.Dir(d.cfg.socket), "wmux.pid")

	dd := daemon.NewDaemon(
		&serverAdapter{srv: server},
		&sessionAdapter{svc: sessionSvc},
		daemon.WithPIDFilePath(pidFile),
		daemon.WithDataDir(d.cfg.dataDir),
		daemon.WithVersion("0.1.0"),
		daemon.WithEventBus(d.eventBus),
		daemon.WithColdRestore(d.cfg.coldRestore),
		daemon.WithMaxScrollbackSize(d.cfg.maxScrollbackSize),
	)

	return dd.Start(ctx)
}

// daemonSentinel is the CLI argument that triggers daemon mode in ServeDaemon.
const daemonSentinel = "__wmux_daemon__"

// isDaemonMode checks if the args contain the daemon sentinel.
func isDaemonMode(args []string) bool {
	for _, a := range args {
		if a == daemonSentinel {
			return true
		}
	}
	return false
}

// ServeDaemon checks if the current process was invoked in daemon mode
// (by a prior client.New auto-start). If so, it runs the daemon and
// returns true. If not, returns false immediately.
//
// Integrators add this as the first line in main():
//
//	func main() {
//	    if client.ServeDaemon(os.Args) {
//	        return
//	    }
//	    // normal app...
//	}
func ServeDaemon(args []string) bool {
	if !isDaemonMode(args) {
		return false
	}

	opts := parseDaemonArgs(args)
	d, err := NewDaemon(opts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wmux daemon: %v\n", err)
		os.Exit(1)
	}

	ctx := daemon.HandleSignals(context.Background())
	if err := d.Serve(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "wmux daemon: %v\n", err)
		os.Exit(1)
	}

	return true
}

// parseDaemonArgs extracts options from the sentinel-style args.
func parseDaemonArgs(args []string) []Option {
	var opts []Option
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--base-dir":
			if i+1 < len(args) {
				opts = append(opts, WithBaseDir(args[i+1]))
				i++
			}
		case "--namespace":
			if i+1 < len(args) {
				opts = append(opts, WithNamespace(args[i+1]))
				i++
			}
		case "--socket":
			if i+1 < len(args) {
				opts = append(opts, WithSocket(args[i+1]))
				i++
			}
		case "--data-dir":
			if i+1 < len(args) {
				opts = append(opts, WithDataDir(args[i+1]))
				i++
			}
		}
	}
	return opts
}
```

- [ ] **Step 4: Move adapters from cmd to pkg/client**

The `serverAdapter` and `sessionAdapter` types currently live in `cmd/wmux/adapter.go`. They need to be accessible from `pkg/client/daemon.go`. Move them into `pkg/client/daemon.go` (or a new `pkg/client/adapter.go`).

Create `pkg/client/adapter.go`:

```go
package client

import (
	"fmt"
	"time"

	"github.com/wblech/wmux/internal/daemon"
	"github.com/wblech/wmux/internal/platform/protocol"
	"github.com/wblech/wmux/internal/session"
	"github.com/wblech/wmux/internal/transport"
)

// serverAdapter wraps *transport.Server to implement daemon.TransportServer.
type serverAdapter struct {
	srv *transport.Server
}

func (a *serverAdapter) OnClient(fn func(daemon.ConnectedClient)) {
	a.srv.OnClient(func(c *transport.Client) {
		fn(&clientAdapter{c: c})
	})
}

func (a *serverAdapter) Serve(ctx context.Context) error {
	if err := a.srv.Serve(ctx); err != nil {
		return fmt.Errorf("server adapter: serve: %w", err)
	}
	return nil
}

func (a *serverAdapter) BroadcastTo(clientID string, f protocol.Frame) error {
	if err := a.srv.BroadcastTo(clientID, f); err != nil {
		return fmt.Errorf("server adapter: broadcast: %w", err)
	}
	return nil
}

// clientAdapter wraps *transport.Client to implement daemon.ConnectedClient.
type clientAdapter struct {
	c *transport.Client
}

func (a *clientAdapter) ClientID() string {
	return a.c.ID
}

func (a *clientAdapter) Control() daemon.ControlConn {
	return a.c.Control
}

// sessionAdapter wraps *session.Service to implement daemon.SessionManager.
type sessionAdapter struct {
	svc *session.Service
}

func (a *sessionAdapter) Create(id string, opts daemon.SessionCreateOptions) (daemon.SessionInfo, error) {
	sess, err := a.svc.Create(id, session.CreateOptions{
		Shell:         opts.Shell,
		Args:          opts.Args,
		Cols:          opts.Cols,
		Rows:          opts.Rows,
		Cwd:           opts.Cwd,
		Env:           opts.Env,
		HighWatermark: 0,
		LowWatermark:  0,
		BatchInterval: 0,
		HistoryWriter: nil,
	})
	if err != nil {
		return daemon.SessionInfo{}, fmt.Errorf("session adapter: create: %w", err)
	}
	return toInfo(sess), nil
}

func (a *sessionAdapter) Get(id string) (daemon.SessionInfo, error) {
	sess, err := a.svc.Get(id)
	if err != nil {
		return daemon.SessionInfo{}, fmt.Errorf("session adapter: get: %w", err)
	}
	return toInfo(sess), nil
}

func (a *sessionAdapter) List() []daemon.SessionInfo {
	sessions := a.svc.List()
	infos := make([]daemon.SessionInfo, 0, len(sessions))
	for _, sess := range sessions {
		infos = append(infos, toInfo(sess))
	}
	return infos
}

func (a *sessionAdapter) Kill(id string) error {
	if err := a.svc.Kill(id); err != nil {
		return fmt.Errorf("session adapter: kill: %w", err)
	}
	return nil
}

func (a *sessionAdapter) Resize(id string, cols, rows int) error {
	if err := a.svc.Resize(id, cols, rows); err != nil {
		return fmt.Errorf("session adapter: resize: %w", err)
	}
	return nil
}

func (a *sessionAdapter) WriteInput(id string, data []byte) error {
	if err := a.svc.WriteInput(id, data); err != nil {
		return fmt.Errorf("session adapter: write input: %w", err)
	}
	return nil
}

func (a *sessionAdapter) ReadOutput(id string) ([]byte, error) {
	data, err := a.svc.ReadOutput(id)
	if err != nil {
		return nil, fmt.Errorf("session adapter: read output: %w", err)
	}
	return data, nil
}

func (a *sessionAdapter) Attach(id string) error {
	if err := a.svc.Attach(id); err != nil {
		return fmt.Errorf("session adapter: attach: %w", err)
	}
	return nil
}

func (a *sessionAdapter) Detach(id string) error {
	if err := a.svc.Detach(id); err != nil {
		return fmt.Errorf("session adapter: detach: %w", err)
	}
	return nil
}

func (a *sessionAdapter) Snapshot(id string) (daemon.SnapshotData, error) {
	snap, err := a.svc.Snapshot(id)
	if err != nil {
		return daemon.SnapshotData{}, fmt.Errorf("session adapter: snapshot: %w", err)
	}
	return daemon.SnapshotData{Scrollback: snap.Scrollback, Viewport: snap.Viewport}, nil
}

func (a *sessionAdapter) LastActivity(id string) (time.Time, error) {
	t, err := a.svc.LastActivity(id)
	if err != nil {
		return time.Time{}, fmt.Errorf("session adapter: last activity: %w", err)
	}
	return t, nil
}

func (a *sessionAdapter) MetaSet(id, key, value string) error {
	if err := a.svc.MetaSet(id, key, value); err != nil {
		return fmt.Errorf("session adapter: meta set: %w", err)
	}
	return nil
}

func (a *sessionAdapter) MetaGet(id, key string) (string, error) {
	val, err := a.svc.MetaGet(id, key)
	if err != nil {
		return "", fmt.Errorf("session adapter: meta get: %w", err)
	}
	return val, nil
}

func (a *sessionAdapter) MetaGetAll(id string) (map[string]string, error) {
	meta, err := a.svc.MetaGetAll(id)
	if err != nil {
		return nil, fmt.Errorf("session adapter: meta get all: %w", err)
	}
	return meta, nil
}

func (a *sessionAdapter) OnExit(fn func(id string, exitCode int)) {
	a.svc.OnExit(fn)
}

func toInfo(sess *session.Session) daemon.SessionInfo {
	return daemon.SessionInfo{
		ID:    sess.ID,
		State: sess.State.String(),
		Pid:   sess.Pid,
		Cols:  sess.Cols,
		Rows:  sess.Rows,
		Shell: sess.Shell,
	}
}
```

Add `"context"` to the import in `adapter.go` (it's needed by `serverAdapter.Serve`).

- [ ] **Step 5: Run tests**

```bash
go test ./pkg/client/ -run "TestNewDaemon|TestServeDaemon" -v -count=1
```

Expected: all PASS

- [ ] **Step 6: Run full client test suite**

```bash
go test ./pkg/client/ -v -count=1
```

Expected: all PASS

- [ ] **Step 7: Commit**

```bash
git add pkg/client/daemon.go pkg/client/daemon_test.go pkg/client/adapter.go
git -c commit.gpgsign=false commit -m "feat(client): add NewDaemon and ServeDaemon for embedded daemon support"
```

---

### Task 5: Dogfood cmd/wmux/daemon.go

**Files:**
- Modify: `cmd/wmux/daemon.go` — use `client.NewDaemon` + `Serve`
- Modify: `cmd/wmux/adapter.go` — remove (adapters moved to pkg/client)

- [ ] **Step 1: Rewrite cmd/wmux/daemon.go**

Replace the contents of `cmd/wmux/daemon.go` with:

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wblech/wmux/internal/daemon"
	"github.com/wblech/wmux/internal/platform/config"
	"github.com/wblech/wmux/internal/platform/history"
	"github.com/wblech/wmux/pkg/client"
)

func cmdDaemon(args []string) int {
	// Defaults derived from the global socketPath.
	expandedSocket := expandHome(socketPath)
	baseDir := filepath.Dir(expandedSocket)

	var opts []client.Option
	opts = append(opts, client.WithSocket(expandedSocket))
	opts = append(opts, client.WithDataDir(filepath.Join(baseDir, "sessions")))

	// Parse daemon-specific flags.
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--socket":
			if i+1 < len(args) {
				expandedSocket = expandHome(args[i+1])
				baseDir = filepath.Dir(expandedSocket)
				opts = append(opts, client.WithSocket(expandedSocket))
				i++
			}
		case "--data-dir":
			if i+1 < len(args) {
				opts = append(opts, client.WithDataDir(expandHome(args[i+1])))
				i++
			}
		case "--log-level":
			if i+1 < len(args) {
				i++
			}
		}
	}

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

	maxScrollback, err := history.ParseSize(cfg.History.MaxPerSession)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: parse max_per_session: %v\n", err)
		return 1
	}

	opts = append(opts,
		client.WithColdRestore(cfg.History.ColdRestore),
		client.WithMaxScrollbackSize(maxScrollback),
	)

	d, err := client.NewDaemon(opts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "wmux daemon listening on %s\n", expandedSocket)

	ctx := daemon.HandleSignals(context.Background())

	if err := d.Serve(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	fmt.Fprintln(os.Stderr, "wmux daemon stopped")
	return 0
}
```

Add `"context"` to the imports.

- [ ] **Step 2: Remove cmd/wmux/adapter.go**

Delete `cmd/wmux/adapter.go` — the adapters now live in `pkg/client/adapter.go`.

```bash
rm cmd/wmux/adapter.go
```

- [ ] **Step 3: Build**

```bash
go build ./cmd/wmux/
```

Expected: compiles cleanly

- [ ] **Step 4: Run full test suite**

```bash
make test
```

Expected: all PASS

- [ ] **Step 5: Run lint**

```bash
make lint 2>&1 | head -20
```

Expected: 0 errors

- [ ] **Step 6: Commit**

```bash
git add cmd/wmux/daemon.go pkg/client/adapter.go && git rm cmd/wmux/adapter.go
git -c commit.gpgsign=false commit -m "refactor(cmd): dogfood client.NewDaemon in wmux CLI"
```

---

### Task 6: Rewrite integration guide

**Files:**
- Modify: `docs/integration-guide.md`

- [ ] **Step 1: Write the new integration guide**

Replace the full contents of `docs/integration-guide.md` with the guide covering:

1. **Quick Start** — `go get` + 3-line `New()` example
2. **Configuration** — all options with defaults table
3. **Embedded Daemon** — `NewDaemon` + `Serve` for in-process
4. **Persistent Daemon** — `ServeDaemon` hook + architecture diagram
5. **Session Operations** — create, attach, detach, kill, resize, write
6. **Warm Attach** — snapshot handling (Go + xterm.js frontend)
7. **Cold Restore** — `LoadSessionHistory` / `CleanSessionHistory`
8. **Environment Forwarding** — `ForwardEnv`
9. **Session Metadata** — `MetaSet` / `MetaGet` / `MetaGetAll`
10. **Events** — `OnData` / `OnEvent`
11. **Migration from creack/pty** — before/after table
12. **Migration from Connect** — old API → new API

This is a documentation-only task. Full content to be written following the spec sections.

- [ ] **Step 2: Commit**

```bash
git add -f docs/integration-guide.md
git -c commit.gpgsign=false commit -m "docs: rewrite integration guide for client SDK v2"
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

Expected: 0 errors

- [ ] **Step 3: Check coverage**

```bash
go test -coverprofile=/tmp/cover.out ./pkg/client/ && go tool cover -func=/tmp/cover.out | tail -1
```

Expected: >= 90%

- [ ] **Step 4: Verify no Connect references remain**

```bash
grep -r "Connect(Options" pkg/client/ || echo "CLEAN"
grep -r "client\.Connect" cmd/ || echo "CLEAN"
```

Expected: both CLEAN

- [ ] **Step 5: Build binary**

```bash
go build ./cmd/wmux/
```

Expected: compiles
