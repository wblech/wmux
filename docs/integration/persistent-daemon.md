# Persistent Daemon

Run the wmux daemon as a separate background process so sessions survive
application restarts. The daemon is spawned automatically from your own binary
-- no external install step, no sidecar process.

## When to use

- Desktop applications (Electron, Wails) where terminal tabs must outlive the UI
- CLI tools that need long-running sessions across invocations
- Any scenario where cold-restart recovery matters

## ServeDaemon pattern

Add a single guard at the top of `main()`. When the binary is re-executed in
daemon mode, `ServeDaemon` runs the daemon and returns `true`. Otherwise it
returns `false` and your application continues normally.

```go
package main

import (
    "log"
    "os"

    "github.com/wblech/wmux/addons/charmvt"
    "github.com/wblech/wmux/pkg/client"
)

func main() {
    // If this process was spawned as a daemon, run it and exit.
    // Pass addon options here — they are created in-process in the
    // daemon and survive the integrator app's lifecycle.
    if handled, err := client.ServeDaemon(os.Args, charmvt.Backend()); handled {
        if err != nil {
            log.Fatal(err)
        }
        return
    }

    // Normal application startup — no addon options needed here.
    c, err := client.New(
        client.WithNamespace("watchtower"),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer c.Close()

    info, err := c.Create("build-1", client.CreateParams{
        Shell: "/bin/bash",
        Args:  []string{"-l"},
        Cols:  120,
        Rows:  40,
    })
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("session %s ready", info.ID)
}
```

## How it works

1. `client.New()` tries to connect to the daemon socket.
2. If no daemon is listening and `WithAutoStart` is `true` (the default), it
   re-executes the current binary with a sentinel argument
   (`__wmux_daemon__`).
3. The child process detaches into its own session (`setsid`) so it is not
   killed when the parent exits.
4. `ServeDaemon(os.Args, ...)` in the child detects the sentinel, parses the
   forwarded flags, merges any integrator-provided options (e.g. emulator
   factories), and calls `Daemon.Serve`. It blocks until the daemon receives
   a termination signal.
5. Back in the parent, `New` polls the socket and connects once the daemon is
   ready (timeout: 3 seconds).

Because the daemon is your own compiled binary, there are no external
dependencies to install or manage. Scalar configuration (namespace, socket
path, cold-restore, scrollback size) is forwarded through CLI flags
automatically. Non-serializable options like emulator factories are provided
by the integrator via `ServeDaemon`'s extra options parameter.

## Lifecycle

The daemon process handles `SIGINT` and `SIGTERM` for clean shutdown. Active
PTY sessions are terminated and, if cold-restore is enabled, scrollback is
flushed to disk before exit.

On the next application start, if the daemon is still running, `client.New()`
connects to the existing process. If it has exited, a new daemon is spawned
transparently.

## Multiple namespaces

Each namespace gets its own socket, token, and data directory. You can run
several independent daemon instances from the same binary:

```go
devClient, _ := client.New(client.WithNamespace("dev"))
prodClient, _ := client.New(client.WithNamespace("prod"))
```

Sessions in different namespaces are fully isolated.
