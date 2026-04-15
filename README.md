# wmux

[![CI](https://github.com/wblech/wmux/actions/workflows/ci.yml/badge.svg)](https://github.com/wblech/wmux/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/wblech/wmux.svg)](https://pkg.go.dev/github.com/wblech/wmux)

A lightweight, cross-platform PTY daemon written in Go that enables terminal persistence across application restarts. It keeps terminal sessions alive in a background daemon that survives app restarts and crashes, allowing users and applications to reconnect seamlessly without losing running processes or scrollback history.

## Key Features

- **Persistent sessions** -- terminal sessions survive application restarts and crashes
- **Zero perceptible latency** -- designed to add no perceptible overhead to terminal I/O
- **Transparent escape code passthrough** -- no translation or interference with terminal escape sequences
- **Cross-platform** -- runs on macOS and Linux
- **tmux compatibility shim** -- drop-in replacement for common tmux workflows
- **Resource-bounded** -- configurable memory limits and backpressure to prevent runaway resource usage
- **Observable** -- structured events for monitoring and integration

## Install

### CLI

```bash
go install github.com/wblech/wmux/cmd/wmux@latest
```

### Go SDK

```bash
go get github.com/wblech/wmux
```

## Quick Start

```bash
wmux create my-session    # create a new persistent session
wmux attach my-session    # attach to the session interactively
wmux list                 # list all active sessions
wmux version              # show wmux version
```

## Go SDK

```go
package main

import (
    "log"

    "github.com/wblech/wmux/pkg/client"
)

func main() {
    c, err := client.New()
    if err != nil {
        log.Fatal(err)
    }
    defer c.Close()

    info, err := c.Create("hello", client.CreateParams{
        Cols: 80,
        Rows: 24,
    })
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("session %s running pid %d", info.ID, info.Pid)
}
```

## Shell Completions

Completions are available for Bash, Zsh, and Fish in the `completions/` directory.

## Development

```bash
make lint      # golangci-lint + goframe check
make test      # go test -race -shuffle=on ./...
make build     # build binaries with version injection
```

## Documentation

Full documentation is available at [wblech.github.io/wmux](https://wblech.github.io/wmux/).
