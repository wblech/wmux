# Go SDK

wmux provides a Go client library for managing persistent terminal sessions
backed by a PTY daemon. Import the `pkg/client` package to create, attach, and
control sessions from any Go application.

## Install

```bash
go get github.com/wblech/wmux
```

## Quick start

Zero-configuration connects to (or spawns) a daemon in the `default` namespace:

```go
package main

import (
    "log"

    "github.com/wblech/wmux/pkg/client"
)

func main() {
    c, err := client.New()
    if err != nil {
        log.Fatal(err)
    }
    defer c.Close()

    info, err := c.Create("hello", client.CreateParams{
        Cols: 80,
        Rows: 24,
    })
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("session %s running pid %d", info.ID, info.Pid)
}
```

## Architecture

```
+----------------------------------------------+
|            Your App (Go + Frontend)          |
|                                              |
|  +---------------+     +------------------+  |
|  |  Frontend     |     |   Go backend     |  |
|  |  (xterm.js)   |<--->|  (pkg/client)    |  |
|  +---------------+     +--------+---------+  |
+-----------------------------+----------------+
                              | Unix socket
          +-------------------v----------------+
          |          wmux daemon               |
          |                                    |
          |  +---------------+  +-----------+  |
          |  |   Sessions    |  | Emulator  |  |
          |  |  (PTY/shell)  |<>| (charmvt) |  |
          |  +---------------+  +-----------+  |
          +------------------------------------+
```

The client communicates with the daemon over a Unix socket using a
length-prefixed binary protocol with two channels: **control** for RPCs
(create, resize, list) and **stream** for PTY output. Both channels are
authenticated automatically via a shared token file.

Two deployment modes are available:

- **Embedded daemon** -- the daemon runs in-process as a goroutine. Best for
  applications where sessions do not need to survive restarts.
  See [Embedded Daemon](embedded-daemon.md).

- **Persistent daemon** -- the daemon runs as a separate background process.
  Sessions survive application restarts.
  See [Persistent Daemon](persistent-daemon.md).

## Configuration options

All options work with both `client.New()` and `client.NewDaemon()`.

| Option | Default | Description |
|---|---|---|
| `WithNamespace(name string)` | `"default"` | Logical isolation group |
| `WithBaseDir(path string)` | `~/.wmux` | Root directory for all state |
| `WithSocket(path string)` | `<baseDir>/<ns>/daemon.sock` | Explicit Unix socket path |
| `WithTokenPath(path string)` | `<baseDir>/<ns>/daemon.token` | Auth token file path |
| `WithDataDir(path string)` | `<baseDir>/<ns>/sessions` | Session history directory |
| `WithAutoStart(enabled bool)` | `true` | Auto-start daemon if not running |
| `WithColdRestore(enabled bool)` | `false` | Restore scrollback from disk on startup |
| `WithMaxScrollbackSize(n int64)` | `0` (unlimited) | Max scrollback bytes per session |
| `WithEmulatorFactory(f EmulatorFactory)` | `nil` | Emulator backend via addon module (e.g., `charmvt.Backend()`) |

## Addons

Emulator addons provide server-side terminal state tracking (scrollback and
viewport snapshots). Without an addon, `Attach()` returns empty snapshots.

Addons are **optional, separate Go modules** — your binary only includes an
addon if you import it.

### charmvt (recommended)

Pure Go terminal emulator. No external dependencies (no Node.js). Uses
[charmbracelet/x/vt](https://github.com/charmbracelet/x) under the hood.

**Install:**

```bash
go get github.com/wblech/wmux/addons/charmvt
```

**Use:**

```go
import (
    "github.com/wblech/wmux/pkg/client"
    "github.com/wblech/wmux/addons/charmvt"
)

// Embedded daemon with emulator addon.
d, err := client.NewDaemon(
    client.WithNamespace("myapp"),
    charmvt.Backend(),
)

// Or with options.
d, err := client.NewDaemon(
    charmvt.Backend(
        charmvt.WithScrollbackSize(50000),
        charmvt.WithCallbacks(charmvt.Callbacks{
            Title: func(sid, title string) {
                log.Printf("session %s: %s", sid, title)
            },
        }),
    ),
)
```

### Snapshot scrollback filtering

By default, `Snapshot()` includes the entire scrollback buffer. For applications
where a TUI (e.g., Claude Code) takes over the screen, pre-TUI shell content
can appear above the TUI banner on reconnect. Use `SinceLastClear` mode to
exclude scrollback from before the last screen clear:

```go
d, err := client.NewDaemon(
    charmvt.Backend(
        charmvt.WithSnapshotScrollbackMode(charmvt.SnapshotScrollbackSinceLastClear),
    ),
)
```

In this mode, the emulator tracks the scrollback length at each ED2
(`\x1b[2J`) on the main screen. `Snapshot()` renders only scrollback lines
added after that point. Shell output, TUI output that scrolled off after the
last clear, and viewport content are all included. Pre-clear content is
excluded. See [ADR 0028](../../decisions/0028-ed2-scrollback-behavior.md).

### Runtime scrollback configuration

The scrollback buffer size can be changed on a live session without restarting
the daemon:

```go
err := c.UpdateEmulatorScrollback("sess-1", 50000)
```

This works with addons that support it (charmvt does). If the addon does not
support runtime configuration, the call returns an error. Increasing the size
preserves existing data. Decreasing discards the oldest lines.

### Writing your own addon

Implement the `client.EmulatorFactory` interface:

```go
type EmulatorFactory interface {
    Create(sessionID string, cols, rows int) ScreenEmulator
    Close()
}

type ScreenEmulator interface {
    Process(data []byte)
    Snapshot() Snapshot
    Resize(cols, rows int)
}

// Snapshot is the warm-attach payload. Replay is a self-contained VT byte
// stream: when written to an xterm.js-compatible terminal it reproduces the
// source screen and cursor exactly, regardless of prior destination state.
//
// Implementations should build Replay as:
//   1. \e[2J\e[H\e[3J  — clear viewport + scrollback, home the cursor.
//   2. The serialized scrollback history.
//   3. The serialized current viewport.
//   4. A final CUP (\e[row;colH) restoring the source cursor position.
//
// See ADR 0026 for the full contract.
type Snapshot struct {
    Replay []byte
}
```

Pass it via `client.WithEmulatorFactory(yourFactory)`.

## Further reading

- [Session Operations](session-operations.md) -- create, attach, write, resize, list, kill
- [Events](events.md) -- subscribe to PTY output and lifecycle events
