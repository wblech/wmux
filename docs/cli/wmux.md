# wmux Commands

Full reference for all 13 commands in the `wmux` CLI.

---

## daemon

Start the daemon in the foreground.

```
wmux daemon [--socket <path>] [--data-dir <path>]
```

The daemon loads `config.toml` from the base directory.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--socket <path>` | `~/.wmux/daemon.sock` | Unix socket path |
| `--data-dir <path>` | Derived from socket path | Sessions data directory |

---

## create

Create a new session.

```
wmux create <session-id> [--shell /bin/zsh] [--cwd /path]
```

### Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `session-id` | Yes | Unique session identifier |

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--shell <path>` | `$SHELL` or `/bin/sh` | Shell binary to use |
| `--cwd <path>` | Current directory | Working directory for the session |

Default terminal size is 80x24.

### Output

```
Created session <id> (pid <pid>)
```

---

## attach

Attach to an existing session interactively.

```
wmux attach <session-id>
```

Streams session I/O bidirectionally. Blocks until detached or the session exits.

### Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `session-id` | Yes | Session to attach to |

---

## detach

Detach from a session.

```
wmux detach <session-id>
```

### Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `session-id` | Yes | Session to detach from |

### Output

```
Detached from session <id>
```

---

## kill

Kill a session, or batch kill sessions by prefix.

```
wmux kill <session-id>
wmux kill --prefix <prefix>
```

### Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `session-id` | Yes (unless `--prefix`) | Session to kill |

### Flags

| Flag | Description |
|------|-------------|
| `--prefix <prefix>` | Kill all sessions whose ID starts with `<prefix>` |

### Output

Single session:

```
Killed session <id>
```

Batch (one line per session):

```
<id>: killed
<id>: error: <msg>
```

---

## list

List sessions.

```
wmux list [--prefix <prefix>] [--quiet|-q]
```

### Flags

| Flag | Description |
|------|-------------|
| `--prefix <prefix>` | Filter sessions by ID prefix |
| `--quiet`, `-q` | Print one session ID per line (no table) |

### Output

Normal mode -- table with columns:

```
ID    STATE    PID    COLS    ROWS    SHELL
```

Quiet mode -- one session ID per line.

When no sessions exist:

```
No sessions
```

---

## info

Show detailed information for a session.

```
wmux info <session-id>
```

### Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `session-id` | Yes | Session to inspect |

### Output

Key-value format:

```
ID:    <id>
State: <state>
PID:   <pid>
Size:  <cols>x<rows>
Shell: <shell>
```

---

## status

Show daemon status.

```
wmux status
```

### Output

```
Version:  <version>
Uptime:   <duration>
Sessions: <count>
Clients:  <count>
```

---

## events

Stream session events as NDJSON.

```
wmux events [session-id] [--all]
```

### Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `session-id` | No | Stream events for a specific session |

### Flags

| Flag | Description |
|------|-------------|
| `--all` | Stream events for all sessions |

### Output

One JSON object per line (NDJSON). Streams until the connection closes.

---

## exec

Send input to a session without attaching.

```
wmux exec <session-id> <input>
wmux exec --sync <id1> <id2> ... -- <input>
wmux exec --prefix <prefix> -- <input>
```

### Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `session-id` | Yes (single mode) | Target session |
| `input` | Yes | Text to send |

### Flags

| Flag | Description |
|------|-------------|
| `--sync` | Send to multiple named sessions |
| `--prefix <prefix>` | Send to all sessions matching prefix |
| `--no-newline` | Do not append `\n` to input |

### Output

Multi-session mode (one line per session):

```
<id>: ok
<id>: error: <msg>
```

---

## wait

Wait for a session condition.

```
wmux wait <session-id> exit [--timeout <ms>]
wmux wait <session-id> idle <idle-ms> [--timeout <ms>]
wmux wait <session-id> match <pattern> [--timeout <ms>]
```

### Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `session-id` | Yes | Session to wait on |

### Modes

| Mode | Extra Argument | Description |
|------|----------------|-------------|
| `exit` | -- | Wait until the session exits |
| `idle` | `<idle-ms>` | Wait until the session has been idle for the given duration |
| `match` | `<pattern>` | Wait until session output matches the pattern |

### Flags

| Flag | Description |
|------|-------------|
| `--timeout <ms>` | Maximum time to wait in milliseconds |

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Condition met |
| 1 | Error or no match |
| 2 | Timeout |

### Output

Depends on mode:

- `exit`: `session <id> exited with code <code>`
- `idle`: `session <id> idle`
- `match`: `session <id> matched`

---

## record

Start or stop session recording.

```
wmux record <start|stop> <session-id>
```

### Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `start` or `stop` | Yes | Action to perform |
| `session-id` | Yes | Session to record |

### Output

Start:

```
Recording started: <id> -> <path>
```

Stop:

```
Recording stopped: <id>
```

---

## history

Export session scrollback.

```
wmux history <session-id> [--format ansi|text|html] [--lines N]
```

### Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `session-id` | Yes | Session to export |

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--format <format>` | `ansi` | Output format: `ansi`, `text`, or `html` |
| `--lines <N>` | All | Number of lines to export |

### Output

Raw formatted history data written to stdout.
