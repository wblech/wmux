# wmux

wmux is a lightweight, cross-platform PTY daemon written in Go that enables terminal persistence across application restarts. It keeps terminal sessions alive in a background daemon that survives app restarts and crashes, allowing users and applications to reconnect seamlessly without losing running processes or scrollback history.

## Key Features

- **Persistent sessions** -- terminal sessions survive application restarts and crashes
- **Zero perceptible latency** -- designed to add no perceptible overhead to terminal I/O
- **Transparent escape code passthrough** -- no translation or interference with terminal escape sequences
- **Cross-platform** -- runs on macOS and Linux (Windows support planned)
- **tmux compatibility shim** -- drop-in replacement for common tmux workflows
- **Resource-bounded** -- configurable memory limits and backpressure to prevent runaway resource usage
- **Observable** -- structured events for monitoring and integration

## Quick Install

```bash
go install github.com/wblech/wmux/cmd/wmux@latest
```

## Quickstart

```bash
wmux create my-session    # create a new persistent session
wmux attach my-session    # attach to the session interactively
wmux list                 # list all active sessions
```

## Next Steps

- [Getting Started](getting-started/index.md) -- installation, first session walkthrough
- [Go SDK](integration/index.md) -- embed wmux in your own Go applications
