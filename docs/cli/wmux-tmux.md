# wmux-tmux (tmux shim)

`wmux-tmux` translates tmux commands into wmux SDK calls. It is not installed globally -- it is injected into child process `PATH` so that tools expecting `tmux` (e.g., Claude Code) interact with wmux instead.

## Setup

Symlink or copy the `wmux-tmux` binary as `tmux` and prepend its directory to `PATH` for the child process:

```bash
ln -s /path/to/wmux-tmux /some/dir/tmux
export PATH="/some/dir:$PATH"
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `WMUX_NAMESPACE` | Namespace for daemon isolation. Sets the socket path to `~/.wmux/<namespace>/daemon.sock`. |

---

## Commands

### new-session

Create a new session.

```
tmux new-session -d -s NAME [-x COLS] [-y ROWS]
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `-d` | No | -- | Detached mode (always detached; flag is accepted but ignored) |
| `-s` | Yes | -- | Session name |
| `-x` | No | `80` | Terminal width in columns |
| `-y` | No | `24` | Terminal height in rows |

Uses `$SHELL` or `/bin/sh` as the shell.

---

### send-keys

Send keystrokes to a session.

```
tmux send-keys -t NAME KEYS...
```

| Flag | Required | Description |
|------|----------|-------------|
| `-t` | Yes | Target session name |

All remaining arguments are joined with spaces and sent as input.

#### Key Translation

Named keys are translated to escape sequences before sending:

| Key Name | Escape Sequence |
|----------|-----------------|
| `Enter` | `\n` |
| `C-c` | `\x03` |
| `C-d` | `\x04` |
| `C-z` | `\x1a` |
| `C-l` | `\x0c` |
| `Escape` | `\x1b` |
| `Tab` | `\t` |
| `Space` | ` ` (literal space) |
| `BSpace` | `\x7f` |

---

### capture-pane

Capture and print the current pane content.

```
tmux capture-pane -t NAME -p
```

| Flag | Required | Description |
|------|----------|-------------|
| `-t` | Yes | Target session name |
| `-p` | Yes | Print mode (output to stdout) |

#### Output

Raw viewport content written to stdout.

---

### kill-session

Kill a session.

```
tmux kill-session -t NAME
```

| Flag | Required | Description |
|------|----------|-------------|
| `-t` | Yes | Target session name |

---

### list-sessions

List all sessions.

```
tmux list-sessions
```

#### Output

One line per session in the format:

```
<id>: 1 windows (<state>) [<cols>x<rows>]
```

---

### has-session

Check whether a session exists.

```
tmux has-session -t NAME
```

| Flag | Required | Description |
|------|----------|-------------|
| `-t` | Yes | Target session name |

#### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Session exists |
| 1 | Session does not exist (or daemon not running) |

No output is written to stdout.
