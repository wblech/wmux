# Getting Started

This guide walks you through installing wmux and running your first persistent terminal session.

## Prerequisites

- **Go 1.26.1+** -- required to build and install wmux from source

## Installation

Install wmux from source using the Go toolchain:

```bash
go install github.com/wblech/wmux/cmd/wmux@latest
```

## Quickstart

### 1. Start the daemon

The wmux daemon manages all sessions and must be running before you can create or attach to sessions.

```bash
wmux daemon
```

This runs the daemon in the foreground. To run it in the background:

```bash
wmux daemon &
```

### 2. Create a session

```bash
wmux create my-session
```

Output:

```
Created session my-session (pid 12345)
```

### 3. Attach to the session

```bash
wmux attach my-session
```

This opens an interactive terminal connected to the session. Press `Ctrl+C` or run `wmux detach` to detach without terminating the session.

### 4. List sessions

```bash
wmux list
```

Output:

```
ID          STATE     PID    COLS  ROWS  SHELL
my-session  running   12345  80    24    /bin/zsh
```

### 5. Kill a session

```bash
wmux kill my-session
```

This terminates the session and its underlying process.

## Using the Go SDK

When embedding wmux through the Go SDK, the daemon starts automatically -- there is no need to launch it manually. See the Go SDK documentation for details.

## Next Steps

- [Configuration](configuration.md) -- socket paths, scrollback limits, namespace isolation
