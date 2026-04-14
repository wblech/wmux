# CLI Reference

wmux provides two binaries:

- **`wmux`** -- the main CLI for daemon management, session lifecycle, and automation. See [wmux commands](wmux.md).
- **`wmux-tmux`** -- a tmux-compatible shim for tools that expect `tmux` (e.g., Claude Code). See [wmux-tmux commands](wmux-tmux.md).

## Global Flags

These flags apply to all `wmux` commands:

| Flag | Default | Description |
|------|---------|-------------|
| `--socket <path>` | `~/.wmux/daemon.sock` | Daemon Unix socket path |
| `--token <path>` | `~/.wmux/daemon.token` | Auth token file path |

If only `--socket` is provided, `--token` is derived from the socket path by replacing `.sock` with `.token`.

!!! note
    The CLI defaults to `~/.wmux/daemon.sock` (no namespace directory). The Go
    SDK defaults to `~/.wmux/default/daemon.sock` (using the `"default"`
    namespace). When using both, pass `--socket ~/.wmux/default/daemon.sock` to
    the CLI to connect to a namespace-aware daemon.
