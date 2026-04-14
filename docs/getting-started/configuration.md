# Configuration

## Global CLI Flags

The following flags are available on all wmux commands:

| Flag       | Description                          | Default                    |
|------------|--------------------------------------|----------------------------|
| `--socket` | Path to the daemon Unix socket       | `~/.wmux/default/daemon.sock`  |
| `--token`  | Path to the daemon authentication token | `~/.wmux/default/daemon.token` |

## Directory Layout

wmux stores all runtime data under `~/.wmux/`. The default namespace layout:

```
~/.wmux/
  default/
    daemon.sock
    daemon.token
    wmux.pid
    sessions/
      <session-id>/
        scrollback.bin
        meta.json
```

- `daemon.sock` -- Unix domain socket used for client-daemon communication
- `daemon.token` -- authentication token for securing connections
- `wmux.pid` -- PID file for the running daemon process
- `sessions/` -- per-session directory containing scrollback data and metadata

## Configuration File

Place a `config.toml` file in the daemon base directory (e.g., `~/.wmux/default/config.toml`):

```toml
[history]
max_per_session = "1048576"  # max scrollback per session in bytes ("0" = unlimited)
cold_restore = false         # persist scrollback to disk
```

### History Options

| Key               | Type    | Default | Description                                      |
|--------------------|---------|---------|--------------------------------------------------|
| `max_per_session`  | string  | `"0"`   | Maximum scrollback buffer size in bytes per session. `"0"` means unlimited. |
| `cold_restore`     | boolean | `false` | When enabled, scrollback is persisted to disk so sessions can be restored after daemon restart. |

## Namespace Isolation

wmux supports namespace isolation for running multiple independent daemon instances. Each namespace gets its own socket, token, and data directory under `~/.wmux/<namespace>/`:

```
~/.wmux/
  default/
    daemon.sock
    daemon.token
    ...
  staging/
    daemon.sock
    daemon.token
    ...
```

Use the `--socket` and `--token` flags to target a specific namespace from the CLI.

## Environment Variables

| Variable         | Description                                                                 |
|------------------|-----------------------------------------------------------------------------|
| `WMUX_NAMESPACE` | Sets the namespace for the tmux compatibility shim (`wmux-tmux`). Does **not** affect the main `wmux` CLI. |
