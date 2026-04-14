# Session Operations

All session methods are called on a connected `*client.Client`. See the
[Go SDK overview](index.md) for how to obtain one.

## Create

Start a new PTY session with the given ID and parameters.

```go
info, err := c.Create("build-1", client.CreateParams{
    Shell: "/bin/bash",
    Args:  []string{"-l"},
    Cols:  120,
    Rows:  40,
    Cwd:   "/home/user/project",
    Env:   []string{"TERM=xterm-256color", "LANG=en_US.UTF-8"},
})
```

`CreateParams` fields:

| Field | Type | Description |
|---|---|---|
| `Shell` | `string` | Path to the shell binary. Empty uses the system default. |
| `Args` | `[]string` | Arguments passed to the shell. |
| `Cols` | `int` | Initial terminal width in columns. |
| `Rows` | `int` | Initial terminal height in rows. |
| `Cwd` | `string` | Working directory. Empty uses the daemon's cwd. |
| `Env` | `[]string` | Environment variables in `KEY=VALUE` format. |

Returns a `SessionInfo` with the session metadata (ID, state, PID, dimensions,
shell path).

## Attach

Attach to an existing session to receive its output and capture a terminal
snapshot.

```go
result, err := c.Attach("build-1")
if err != nil {
    log.Fatal(err)
}

// Session metadata
log.Printf("pid=%d cols=%d rows=%d", result.Session.Pid, result.Session.Cols, result.Session.Rows)

// Terminal snapshot (requires emulator backend "xterm")
scrollback := result.Snapshot.Scrollback
viewport := result.Snapshot.Viewport
```

`AttachResult` contains:

- `Session` -- a `SessionInfo` struct with current metadata.
- `Snapshot` -- a `Snapshot` struct with `Scrollback` and `Viewport` byte
  slices. Both are nil when the emulator backend is `"none"`.

## Detach

Stop receiving output from a session without terminating it.

```go
err := c.Detach("build-1")
```

## Kill

Terminate a session and its underlying PTY process.

```go
err := c.Kill("build-1")
```

To kill all sessions matching a prefix:

```go
result, err := c.KillPrefix("build-")
// result.Killed: []string of terminated session IDs
// result.Errors: map of session ID to error string (best-effort)
```

## Write

Send raw bytes to a session's PTY input. This is the low-level input method.

```go
err := c.Write("build-1", []byte("ls -la\n"))
```

## Exec

Send a string to a session. By default, a newline is appended.

```go
err := c.Exec("build-1", "make test")
```

To suppress the trailing newline:

```go
err := c.Exec("build-1", "\x03", client.WithNewline(false)) // send Ctrl-C
```

To send input to multiple sessions at once:

```go
results, err := c.ExecSync("make clean", "build-1", "build-2", "build-3")
for _, r := range results {
    if !r.OK {
        log.Printf("session %s: %s", r.SessionID, r.Error)
    }
}
```

To broadcast to all sessions matching a prefix:

```go
results, err := c.ExecPrefix("build-", "echo done")
```

## Resize

Change a session's terminal dimensions.

```go
err := c.Resize("build-1", 200, 50)
```

## List

Retrieve all active sessions.

```go
sessions, err := c.List()
for _, s := range sessions {
    log.Printf("%s state=%s pid=%d (%dx%d)", s.ID, s.State, s.Pid, s.Cols, s.Rows)
}
```

Filter by prefix:

```go
sessions, err := c.List(client.WithListPrefix("build-"))
```

## Info

Get metadata for a single session.

```go
info, err := c.Info("build-1")
log.Printf("state=%s pid=%d", info.State, info.Pid)
```

`SessionInfo` fields:

| Field | Type | Description |
|---|---|---|
| `ID` | `string` | Unique session identifier |
| `State` | `string` | Lifecycle state (e.g., `"running"`, `"exited"`) |
| `Pid` | `int` | Shell process ID |
| `Cols` | `int` | Terminal width |
| `Rows` | `int` | Terminal height |
| `Shell` | `string` | Shell binary path |
