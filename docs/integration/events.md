# Events

The wmux client provides two callback-based mechanisms for receiving data from
the daemon: raw PTY output and structured lifecycle events.

## OnData -- raw PTY output

Register a handler to receive every byte written by a session's PTY. This is
the primary channel for streaming terminal output to a frontend (e.g.,
xterm.js).

```go
c.OnData(func(sessionID string, data []byte) {
    // Forward raw output to the connected frontend.
    // data may contain ANSI escape sequences, partial UTF-8, etc.
    websocket.Send(sessionID, data)
})
```

The callback fires on every PTY read, so `data` can be any size from a single
byte to several kilobytes. Do not assume line boundaries.

## OnEvent -- lifecycle events

Register a handler for structured events emitted by the daemon. Each event
carries a type string, the session ID it relates to, and an optional data map
with event-specific fields.

```go
c.OnEvent(func(evt client.Event) {
    switch evt.Type {
    case "session.created":
        log.Printf("new session %s (shell=%s %dx%d)",
            evt.SessionID, evt.Data["shell"], evt.Data["cols"], evt.Data["rows"])

    case "session.exited":
        log.Printf("session %s exited (code=%v)",
            evt.SessionID, evt.Data["exit_code"])

    case "session.idle":
        log.Printf("session %s is idle", evt.SessionID)

    case "cwd.changed":
        log.Printf("session %s cwd -> %s",
            evt.SessionID, evt.Data["cwd"])

    case "notification":
        log.Printf("session %s notification: %s",
            evt.SessionID, evt.Data["message"])

    case "output.flood":
        log.Printf("session %s flood detected", evt.SessionID)
    }
})
```

## Event type reference

| Type | Trigger | Notable `Data` keys |
|---|---|---|
| `session.created` | A new session is spawned | `shell`, `cols`, `rows` |
| `session.attached` | A client attaches to a session | -- |
| `session.detached` | A client detaches from a session | -- |
| `session.exited` | The shell process exits | `exit_code` |
| `session.killed` | A session is explicitly killed | -- |
| `session.idle` | No output for the idle threshold | -- |
| `resize` | Terminal dimensions change | `cols`, `rows` |
| `cwd.changed` | Working directory changes (OSC 7) | `cwd` |
| `notification` | Application-level notification (OSC 9/99/777) | `message` |
| `output.flood` | Output rate exceeds flood threshold | -- |
| `recording.limit_reached` | Recording file size limit reached | -- |
| `shell.ready` | Shell has finished initializing | -- |

## Event struct

```go
type Event struct {
    // Type is the event type string (e.g., "session.created").
    Type string

    // SessionID is the session this event relates to.
    SessionID string

    // Data contains event-specific key-value pairs.
    Data map[string]any
}
```

## Notes

- Both `OnData` and `OnEvent` replace any previously registered handler. Only
  one handler of each type is active at a time.
- Handlers are called synchronously on the client's read loop. Long-running
  work should be dispatched to a separate goroutine to avoid blocking event
  delivery.
- If no handler is registered, events and data are silently discarded.
