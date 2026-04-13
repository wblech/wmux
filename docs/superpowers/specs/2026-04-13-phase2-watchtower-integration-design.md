# Phase 2: Watchtower Integration — Design Spec

**Date:** 2026-04-13
**Author:** wblech
**Status:** Draft
**Source:** [PRD](../../PRD.md) | [Roadmap](../../ROADMAP.md) | [Phase 1 Spec](./2026-04-04-pty-daemon-design.md)

---

## 1. Goal

Enable Watchtower to use wmux as its terminal backend. Close app → reopen → sessions restored with full visual state. Attach is instantaneous with pixel-perfect snapshot from xterm headless emulator.

**Deliverable:** Watchtower replaces in-process `creack/pty` with `pkg/client`, sessions survive app restarts, warm attach with two-phase snapshot, cold restore after reboot (opt-in).

---

## 2. Decisions Summary

| Decision | Choice | Rationale |
|---|---|---|
| Emulator architecture | External addon process with binary protocol | Keep daemon pure Go, zero Node dependency in core |
| Addon multiplexing | Single Node process, N xterm instances | ~110MB typical vs ~900MB with 1-process-per-session |
| Client library location | `pkg/client/` in wmux repo | Single repo, consumed as Go module dependency |
| Multi-client | Single-client per session in Phase 2 | Pair programming (multi-client) deferred to later phase |
| Cold restore | Opt-in via config | No disk I/O cost for users who don't need it |
| Watchtower transition | Decided during integration | Doc provided for Watchtower to implement migration |
| ADRs | MADR 4.0.0, same convention as Watchtower | Consistent documentation across projects |

---

## 3. Sub-plan Decomposition

### Sub-plan 1: Emulator Addon Protocol + xterm Addon

**Goal:** Define the generic binary protocol for daemon ↔ emulator addon communication and implement the first addon (`wmux-emulator-xterm`).

#### Architecture

```
┌──────────┐    stdin/stdout     ┌───────────────────────┐
│  daemon   │◄── binary frames ─►│ wmux-emulator-xterm    │
│  (Go)     │                    │ (1 Node process,       │
└──────────┘                     │  N xterm instances)    │
                                 └───────────────────────┘
```

- Daemon spawns one addon process on first session `create` with backend `xterm`
- Addon manages multiple xterm headless instances internally, keyed by `session_id`
- Communication via stdin/stdout with binary frames
- stderr from addon goes to daemon structured log

#### Binary Protocol

Frame format:

```
[method:1][session_id_len:1][session_id:N][payload_len:4][payload:N]
```

Methods (daemon → addon):

| Method | Byte | Payload |
|---|---|---|
| `create` | `0x01` | JSON: `{"cols":80,"rows":24}` |
| `process` | `0x02` | Raw PTY output bytes |
| `snapshot` | `0x03` | Empty |
| `resize` | `0x04` | JSON: `{"cols":120,"rows":40}` |
| `destroy` | `0x05` | Empty |
| `shutdown` | `0x06` | Empty (no session_id) |

Response frame format:

```
[method:1][session_id_len:1][session_id:N][status:1][payload_len:4][payload:N]
```

Status: `0x00` = ok, `0x01` = error.

Response payloads:

| Method | Response payload |
|---|---|
| `create` | Empty |
| `process` | No response (fire-and-forget) |
| `snapshot` | Raw bytes: `[scrollback_len:4][scrollback:N][viewport:N]` |
| `resize` | Empty |
| `destroy` | Empty |

- `process` is fire-and-forget — daemon sends the frame and does not read a response. No response frame is expected for `process`. The data path to clients is unaffected by emulator processing speed.
- `snapshot` is the only synchronous call — invoked at attach time.
- Time-budgeted processing per PRD: daemon limits data sent to addon per cycle (5ms with clients attached, 25ms detached).

#### Crash Recovery

1. Daemon detects EOF on addon stdout
2. Respawns the addon process
3. Re-creates instances (`create` for each active session)
4. Re-feeds output from each session's circular buffer
5. Sessions never die — worst case is temporarily empty snapshot

#### Discovery

Daemon locates addon binary via config or `$PATH`:

```toml
[emulator]
backend = "xterm"  # "none" | "xterm" | custom binary name

[emulator.xterm]
bin = "/usr/local/bin/wmux-emulator-xterm"  # optional, defaults to $PATH lookup
```

#### Files

| File | Purpose |
|---|---|
| `internal/session/addon_emulator.go` | `ScreenEmulator` implementation, proxies to addon via binary protocol |
| `internal/session/addon_emulator_test.go` | Tests with mock process |
| `addons/xterm/package.json` | Node project for xterm addon |
| `addons/xterm/src/index.ts` | Entry point — reads binary frames on stdin, manages N xterm headless instances |
| `addons/xterm/src/protocol.ts` | Binary frame parser/serializer |

---

### Sub-plan 2: Go Client Library + Warm Attach + Integration Doc

**Goal:** Public Go client library at `pkg/client/` that abstracts the binary protocol, with warm attach delivering two-phase snapshot. Integration guide for Watchtower.

#### API Surface

```go
import "github.com/wblech/wmux/pkg/client"

// Connect to daemon
c, err := client.Connect(client.Options{
    SocketPath: "~/.wmux/daemon.sock",
    TokenPath:  "~/.wmux/daemon.token",
})
defer c.Close()

// Create session
session, err := c.Create("my-session", client.CreateParams{
    Shell: "/bin/zsh",
    Cols:  80,
    Rows:  24,
    Cwd:   "/home/user",
})

// Attach with snapshot
snap, err := c.Attach("my-session")
// snap.Scrollback — history bytes
// snap.Viewport   — current screen state (from xterm headless)

// I/O
c.Write("my-session", []byte("ls\n"))
c.OnData("my-session", func(data []byte) {
    // raw PTY output
})

// Events
c.OnEvent(func(e client.Event) {
    // session lifecycle, resize, cwd changes, etc.
})

// Other operations
c.Detach("my-session")
c.Resize("my-session", 120, 40)
c.Kill("my-session")
sessions, err := c.List()
```

#### Warm Attach — Two-Phase

1. Client sends `MsgAttach` with session ID
2. Daemon requests `snapshot` from emulator addon
3. Daemon responds with `MsgOK` containing snapshot in payload
4. Client (Watchtower) loads scrollback into xterm.js history, applies viewport
5. Live output begins streaming via stream channel

`MsgAttach` response payload:

```json
{
    "id": "my-session",
    "state": "alive",
    "pid": 1234,
    "cols": 80,
    "rows": 24,
    "snapshot": {
        "scrollback": "<raw bytes, length-prefixed>",
        "viewport": "<raw bytes, length-prefixed>"
    }
}
```

If backend is `none` or addon is unavailable, snapshot is empty and daemon sends SIGWINCH as fallback.

#### Integration Guide

`docs/integration-guide.md` covering:

- Adding `pkg/client` as Go module dependency
- Connect → create/attach → I/O loop → detach flow
- Using snapshot with xterm.js for warm attach
- Migrating from in-process PTY (creack/pty) to wmux client
- Error handling and reconnection patterns
- Cold restore flow (lazy, per-session)

#### Files

| File | Purpose |
|---|---|
| `pkg/client/client.go` | `Connect`, connection management, protocol I/O |
| `pkg/client/session.go` | Create, Attach, Detach, Kill, Write, Resize, List |
| `pkg/client/event.go` | OnData, OnEvent, event dispatching |
| `pkg/client/entity.go` | Public types: Options, CreateParams, Event, Snapshot, SessionInfo |
| `pkg/client/client_test.go` | Integration tests with mock server |
| `docs/integration-guide.md` | Watchtower integration guide |

---

### Sub-plan 3: Session Metadata + Full Events + Environment Forwarding

**Goal:** Extensible session metadata, complete event system, and environment variable forwarding with stable symlinks.

#### Session Metadata

Extensible `map[string]string` per session for integrator-defined key-value pairs.

New protocol messages:

| Message | Type byte | Payload |
|---|---|---|
| `MsgMetaSet` | `0x12` | `{"session_id":"abc","key":"k","value":"v"}` |
| `MsgMetaGet` | `0x13` | `{"session_id":"abc","key":"k"}` (empty key = get all) |

Client library:

```go
c.MetaSet("my-session", "workspace_id", "ws-42")
val, err := c.MetaGet("my-session", "workspace_id")
meta, err := c.MetaGetAll("my-session")
```

Daemon-managed metadata (automatic, read-only via MetaGet):

- `pid`, `cwd`, `shell`, `cols`, `rows`, `started_at`, `ended_at`, `exit_code`

Integrator-managed metadata (read-write via MetaSet/MetaGet):

- Any key-value pair (e.g., `git_branch`, `project_name`, `workspace_id`)

#### Full Events

Phase 1 has 4 events. Phase 2 adds:

| Event | Type | Trigger |
|---|---|---|
| `session.idle` | `SessionIdle` | N seconds without output (configurable) |
| `session.killed` | `SessionKilled` | Watchdog, reaper, or explicit kill |
| `resize` | `Resize` | Terminal dimensions changed |
| `cwd.changed` | `CwdChanged` | OSC 7 detected in output |
| `notification` | `Notification` | OSC 9/99/777 detected in output |
| `output.flood` | `OutputFlood` | Backpressure activated (high watermark) |

Events carry session ID and type-specific data:

```go
c.OnEvent(func(e client.Event) {
    // e.SessionID, e.Type, e.Data (map[string]string)
    switch e.Type {
    case client.EventCwdChanged:
        newCwd := e.Data["cwd"]
    case client.EventNotification:
        body := e.Data["body"]
    }
})
```

#### Environment Forwarding

On attach, client sends current environment variables. Daemon handles:

1. **Socket/file paths** (e.g., `SSH_AUTH_SOCK`): creates stable symlink at `~/.wmux/sessions/{id}/{VAR_NAME}` → real path. Session shell uses the symlink, which stays constant across re-attaches.
2. **Other values**: writes to `~/.wmux/sessions/{id}/env` for shell to source.

Config:

```toml
[environment]
forward = ["SSH_AUTH_SOCK", "SSH_CONNECTION", "DISPLAY"]
```

Client library sends env vars automatically on attach (configurable):

```go
snap, err := c.Attach("my-session") // sends current env vars from process
```

New protocol message:

| Message | Type byte | Payload |
|---|---|---|
| `MsgEnvForward` | `0x14` | `{"session_id":"abc","env":{"SSH_AUTH_SOCK":"/tmp/ssh-xxx"}}` |

#### OSC Parsing

Daemon passively scans a copy of the PTY output for:

- **OSC 7** (cwd change) → updates session metadata `cwd`, emits `cwd.changed` event
- **OSC 9/99/777** (notifications) → emits `notification` event with parsed body

Data stream to clients is unmodified.

#### Files

| File | Purpose |
|---|---|
| `internal/session/entity.go` | Add `Metadata map[string]string` to `Session` |
| `internal/platform/event/entity.go` | New event `Type` constants |
| `internal/platform/protocol/entity.go` | `MsgMetaSet`, `MsgMetaGet`, `MsgEnvForward` |
| `internal/daemon/service.go` | Handlers for new message types |
| `internal/daemon/environment.go` | Symlink creation, env file writing |
| `internal/daemon/environment_test.go` | Tests |
| `internal/daemon/osc.go` | OSC 7/9/99/777 parser |
| `internal/daemon/osc_test.go` | Tests |
| `pkg/client/metadata.go` | MetaSet, MetaGet, MetaGetAll |
| `pkg/client/environment.go` | Env forwarding on attach |

---

### Sub-plan 4: Cold Restore

**Goal:** Opt-in persistent history for session recovery after daemon death (reboot, crash).

#### Configuration

```toml
[history]
cold_restore = true        # default: false
data_dir = "~/.wmux/history"
max_file_size = "10MB"
```

When `cold_restore = false`:

- Daemon writes no history files to disk
- `LoadSessionHistory` returns `ErrColdRestoreNotAvailable`
- Zero disk I/O overhead

When `cold_restore = true`:

- Daemon persists `scrollback.bin` and `meta.json` per session (existing `history/` package)
- Files written to `{data_dir}/{session_id}/`

#### Client Library API

```go
// Load history for a specific session (lazy, on-demand)
history, err := client.LoadSessionHistory("~/.wmux/history", "abc-123")
// history.Scrollback  []byte
// history.Metadata    history.Metadata (shell, cwd, cols, rows, started_at)

// Clean up after consuming
err = client.CleanSessionHistory("~/.wmux/history", "abc-123")
```

#### Watchtower Flow (lazy, per-workspace)

```
User opens workspace in Watchtower
  → Watchtower knows workspace had session "abc-123" (own metadata)
  → tries c.Attach("abc-123")
  → daemon returns ErrSessionNotFound (daemon restarted)
  → calls client.LoadSessionHistory("~/.wmux/history", "abc-123")
  → if ok: renders scrollback read-only in xterm.js
  → user interacts → c.Create("abc-123", CreateParams{Cwd: history.Metadata.Cwd})
  → calls client.CleanSessionHistory(...)
  → if err: no history, creates fresh session
```

#### Changes to Existing Code

Minimal — `history/` package already implements most of the persistence:

| Existing | Change needed |
|---|---|
| `history.WriteMetadata` | No change |
| `history.ReadMetadata` | No change |
| `history.UpdateMetadataExit` | No change |
| `history.ScrollbackWriter` | No change |
| `config.Config` | Add `ColdRestore bool` field |
| `daemon.Service` | Conditional guard on history writes |

#### Files

| File | Purpose |
|---|---|
| `internal/platform/config/entity.go` | Add `ColdRestore bool` |
| `internal/daemon/service.go` | Guard history writes with config check |
| `pkg/client/restore.go` | `LoadSessionHistory`, `CleanSessionHistory` |
| `pkg/client/entity.go` | `SessionHistory` type |
| `pkg/client/restore_test.go` | Tests |

---

## 4. Architecture Decision Records

Phase 2 introduces MADR 4.0.0 (same convention as Watchtower) for documenting architectural decisions. ADRs cover both Phase 1 (retroactive) and Phase 2 decisions.

**Location:** `docs/decisions/`

### Phase 1 (retroactive)

| ADR | Title |
|---|---|
| 0000 | Use Markdown Architectural Decision Records |
| 0001 | DDD + Package-Oriented Design with goframe |
| 0002 | Binary length-prefixed wire protocol |
| 0003 | Two-channel IPC design (control + stream) |
| 0004 | Token file authentication |
| 0005 | Pluggable headless emulator interface |
| 0006 | High/low watermark backpressure |
| 0007 | Use uber/fx for dependency injection |
| 0008 | In-process event bus with fan-out and type filtering |
| 0009 | Autodaemonize with PID file and heartbeat |

### Phase 2

| ADR | Title |
|---|---|
| 0010 | Emulator addon as external process with binary protocol |
| 0011 | Single Node process for N xterm headless instances |
| 0012 | Go client library in pkg/client/ |
| 0013 | Two-phase warm attach snapshot |
| 0014 | Opt-in cold restore |
| 0015 | Stable symlinks for environment forwarding |
| 0016 | Single-client per session (multi-client deferred) |

---

## 5. Out of Scope (Phase 2)

These items are explicitly deferred:

- Multi-client management (broadcast, leader, kick) → Phase with pair programming
- Session sharing (time-limited tokens, roles) → Phase 4
- tmux compatibility shim → Phase 3
- `wmux exec`, `wmux wait` → Phase 3
- Ghostty emulator backend → Phase 4
- Windows support (Named pipes, ConPTY) → Phase 4
- C ABI shared library → Phase 4

---

## 6. Dependencies

No new external dependencies for the daemon core. The xterm addon is a separate Node project:

| Component | Dependencies |
|---|---|
| daemon (Go) | No new deps beyond Phase 1 |
| addon xterm (Node) | `@xterm/headless` |
| `pkg/client` (Go) | No external deps (uses `internal/platform/protocol`) |
