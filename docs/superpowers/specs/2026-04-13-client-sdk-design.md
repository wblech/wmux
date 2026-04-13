# Client SDK Redesign — Functional Options + Embedded Daemon

**Date:** 2026-04-13
**Motivation:** Transform `pkg/client/` from a low-level protocol wrapper into a proper SDK that any Go application can embed without requiring a separate `wmux` binary.

---

## Problem Statement

The current `pkg/client/` API has three issues:

1. **No functional options.** `Connect(Options{})` uses a flat struct, inconsistent with the project's goframe conventions and the `internal/daemon` options pattern.
2. **No daemon management.** The client assumes a daemon is already running. Consumers must manually spawn `wmux daemon` and know socket/token paths.
3. **No namespace isolation.** Multiple apps sharing the same daemon would conflict. Each consumer needs its own daemon instance with isolated paths.

---

## Design

### Public API

```go
// Connect to an existing daemon or auto-start one
c, err := client.New(opts ...Option)

// Embed a daemon in your process (for integrators like Watchtower)
d, err := client.NewDaemon(opts ...Option)
d.Serve(ctx)           // blocks, listens on socket
c, err := client.New(client.WithNamespace("watchtower"))  // connect to it
```

### Functional Options

```go
type Option func(*config)

// Namespace and paths
WithNamespace(name string)          // default: "default"
WithBaseDir(path string)            // default: ~/.wmux
WithSocket(path string)             // override: {baseDir}/{namespace}/daemon.sock
WithTokenPath(path string)          // override: {baseDir}/{namespace}/daemon.token
WithDataDir(path string)            // override: {baseDir}/{namespace}/sessions/

// Daemon configuration (used by NewDaemon, written to config.toml by New on auto-start)
WithColdRestore(enabled bool)
WithMaxScrollbackSize(n int64)
WithEmulatorBackend(backend string) // "none" | "xterm"

// Client behavior
WithAutoStart(enabled bool)         // default: true — start daemon if not running
```

### Namespace Resolution

Given `WithNamespace("watchtower")` and default base dir:

```
~/.wmux/watchtower/
├── daemon.sock
├── daemon.token
├── wmux.pid
├── config.toml          (generated from options on first auto-start)
└── sessions/
    ├── {session-id}/
    │   ├── scrollback.bin
    │   └── metadata.json
    ...
```

Explicit `With*` overrides take precedence over namespace-derived paths.

### `client.New` Flow

```
1. Build config from options (apply namespace defaults, then overrides)
2. Ensure base directories exist
3. Try connect to socket
   ├─ Success → authenticate → return Client
   └─ Fail (ECONNREFUSED / ENOENT)
      ├─ AutoStart=false → return error
      └─ AutoStart=true
         a. Ensure auth token (auth.Ensure)
         b. Write config.toml from daemon options (if not exists or options changed)
         c. Spawn daemon: exec.Command(os.Executable(), "__wmux_daemon__", flags...)
            - SysProcAttr: Setsid=true (detached, survives parent)
         d. Poll socket ready (max 3s, 50ms interval)
         e. Connect → authenticate → return Client
```

### `client.NewDaemon` Flow

Creates an embedded daemon that the caller manages:

```go
d, err := client.NewDaemon(
    client.WithNamespace("watchtower"),
    client.WithColdRestore(true),
    client.WithEmulatorBackend("xterm"),
)
// d.Serve(ctx) blocks — run in goroutine if needed
go d.Serve(ctx)
```

Internally does the same wiring as `cmd/wmux/daemon.go`:
1. Resolve paths from options
2. Ensure directories and auth token
3. Create transport.Server, session.Service, event.Bus
4. Create internal daemon.NewDaemon with translated options
5. `Serve(ctx)` calls `d.Start(ctx)` — blocks until context cancelled

### `client.ServeDaemon` — Auto-start Hook

For integrators that want sessions to survive app restarts (the Superset pattern):

```go
// In Watchtower's main.go — must be first thing
func main() {
    if client.ServeDaemon(os.Args) {
        return  // was invoked in daemon mode, already handled
    }
    // normal Watchtower app...
}
```

`ServeDaemon` checks if `os.Args` contains `__wmux_daemon__`. If yes, parses the flags, creates a daemon via `NewDaemon`, runs `Serve`, and returns `true`. If no, returns `false` immediately.

When `New` auto-starts, it runs `exec.Command(os.Executable(), "__wmux_daemon__", ...)` — re-executing the same binary in daemon mode. The consumer only needs to add the `ServeDaemon` hook.

**This is optional.** Consumers who don't add the hook simply can't use auto-start for external daemon mode. They can still use `NewDaemon` for in-process embedding, or connect to a manually-started `wmux daemon`.

### `cmd/wmux/daemon.go` Dogfooding

The CLI switches to using `client.NewDaemon`:

```go
// Before (manual wiring)
server := transport.NewServer(listener, token)
sessionSvc := session.NewService(spawner)
d := daemon.NewDaemon(server, sessionSvc, daemon.WithDataDir(dir), ...)

// After
d, err := client.NewDaemon(
    client.WithSocket(socketPath),
    client.WithDataDir(dataDir),
    client.WithColdRestore(cfg.History.ColdRestore),
    client.WithMaxScrollbackSize(maxScrollback),
)
d.Serve(ctx)
```

---

## What Changes

### Files Modified
- `pkg/client/entity.go` — remove `Options` struct, add `Option` type + `config` struct
- `pkg/client/client.go` — `New` replaces `Connect`, add auto-start logic
- `pkg/client/options.go` — new file, all `With*` functions
- `pkg/client/daemon.go` — new file, `NewDaemon`, `ServeDaemon`, `Daemon` type
- `pkg/client/namespace.go` — new file, path resolution logic
- `pkg/client/*_test.go` — update all tests from `Connect(Options{})` to `New(opts...)`
- `cmd/wmux/daemon.go` — use `client.NewDaemon`
- `docs/integration-guide.md` — full rewrite

### Files Removed
- Nothing — `Connect` is removed from `client.go`, not a separate file

### What Does NOT Change
- Session API: `Create`, `Attach`, `Detach`, `Kill`, `Write`, `Resize`, `List`, `Info`
- `ForwardEnv`, `MetaSet`, `MetaGet`, `MetaGetAll`
- `OnData`, `OnEvent`
- `LoadSessionHistory`, `CleanSessionHistory` (signature changes to accept options)
- Binary protocol, auth, transport — all internal, unchanged
- `internal/daemon/` — unchanged, still internal

---

## Integration Guide Structure

The updated guide covers:

1. **Quick Start** — `go get`, 3-line example with `client.New()`
2. **Embedded Daemon** — `NewDaemon` + `Serve` for in-process usage
3. **Persistent Daemon** — `ServeDaemon` hook for sessions that survive app restarts
4. **Configuration** — all options with defaults
5. **Session Operations** — create, attach, detach, kill, resize, write
6. **Warm Attach** — snapshot handling (Go + xterm.js frontend code)
7. **Cold Restore** — `LoadSessionHistory` / `CleanSessionHistory` flow
8. **Environment Forwarding** — `ForwardEnv` usage
9. **Session Metadata** — `MetaSet` / `MetaGet` / `MetaGetAll`
10. **Events** — `OnData` / `OnEvent` with event types
11. **Migration from creack/pty** — before/after table
12. **Migration from Connect** — old API → new API

---

## Testing Strategy

- Namespace resolution: paths derived correctly from namespace + base dir
- Options override: explicit paths take precedence over namespace
- `NewDaemon`: creates, serves, accepts connections
- `New` with `WithAutoStart(false)`: returns error if no daemon
- `New` with auto-start: spawns daemon, connects
- `ServeDaemon`: detects `__wmux_daemon__` args correctly
- All existing client tests migrated to `New(opts...)` pattern
- `cmd/wmux/daemon.go` works with `client.NewDaemon`
- Coverage target: >= 90% on `pkg/client/`
