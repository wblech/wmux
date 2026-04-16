# Embedded Daemon

Run the wmux daemon as an in-process goroutine. This is the simplest deployment
mode -- no external processes, no child management. The trade-off is that
sessions are lost when the host process exits.

## When to use

- Web servers that expose terminals to browser clients
- Development tools where session persistence is not required
- Test harnesses that need disposable PTY sessions

## Full example

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/wblech/wmux/pkg/client"
    "github.com/wblech/wmux/addons/charmvt"
)

func main() {
    ctx, cancel := signal.NotifyContext(context.Background(),
        os.Interrupt, syscall.SIGTERM)
    defer cancel()

    // 1. Create and start the embedded daemon
    d, err := client.NewDaemon(
        client.WithNamespace("myapp"),
        charmvt.Backend(),
        client.WithColdRestore(true),
    )
    if err != nil {
        log.Fatal(err)
    }

    daemonErr := make(chan error, 1)
    go func() {
        daemonErr <- d.Serve(ctx)
    }()

    // 2. Wait for the socket to become ready
    c, err := client.New(
        client.WithNamespace("myapp"),
        client.WithAutoStart(false),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer c.Close()

    // 3. Use the client
    info, err := c.Create("main", client.CreateParams{
        Cols: 120,
        Rows: 40,
    })
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("session %s (pid %d)", info.ID, info.Pid)

    // Wait for shutdown
    select {
    case err := <-daemonErr:
        if err != nil {
            log.Fatal(err)
        }
    case <-ctx.Done():
    }
}
```

## Socket readiness

With `WithAutoStart(false)`, `client.New()` attempts a single connection to the
daemon socket. It does **not** poll or retry -- if the socket is not ready,
`New` returns an error immediately. When embedding the daemon in-process, use a
retry loop or poll the socket before calling `New`:

```go
var c *client.Client
for range 30 {
    c, err = client.New(
        client.WithNamespace("myapp"),
        client.WithAutoStart(false),
    )
    if err == nil {
        break
    }
    time.Sleep(100 * time.Millisecond)
}
```

When the daemon is embedded, the socket is typically ready in under 10 ms.

!!! note
    With `WithAutoStart(true)` (the default), the client handles daemon startup
    and socket readiness internally. The retry loop is only needed for embedded
    mode where you start the daemon goroutine yourself.

## Warm attach

Because the daemon lives in the same process, attaching to an existing session
returns instantly with the current viewport snapshot:

```go
result, err := c.Attach("main")
if err != nil {
    log.Fatal(err)
}

// result.Snapshot.Viewport contains the visible terminal state.
// result.Snapshot.Scrollback contains lines above the viewport.
log.Printf("attached to %s (%d x %d)",
    result.Session.ID, result.Session.Cols, result.Session.Rows)
```

The snapshot is populated when an emulator addon is configured (e.g.,
`charmvt.Backend()`). Without an addon, both fields are nil.

### Available backends

**charmvt (recommended)** — Pure Go, no external dependencies:

    import "github.com/wblech/wmux/addons/charmvt"

    d, err := client.NewDaemon(
        charmvt.Backend(),
    )

Install: `go get github.com/wblech/wmux/addons/charmvt`

**xterm addon (legacy)** — Requires Node.js on the target:

For maximum xterm.js fidelity, use the xterm addon with the CLI's `--xterm-bin`
flag or build a custom `EmulatorFactory` wrapping the addon process.
