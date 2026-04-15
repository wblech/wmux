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
          |  |  (PTY/shell)  |<>| (xterm)   |  |
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
| `WithEmulatorBackend(backend string)` | `"none"` | Terminal emulator backend (`"none"` or `"xterm"`) |

## Further reading

- [Session Operations](session-operations.md) -- create, attach, write, resize, list, kill
- [Events](events.md) -- subscribe to PTY output and lifecycle events
