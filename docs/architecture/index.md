# Architecture

wmux is a Go PTY daemon that manages persistent terminal sessions over Unix
sockets. The codebase follows Domain-Driven Design (DDD) combined with
Package-Oriented Design, enforced by
[goframe](https://github.com/wblech/goframe).

---

## High-Level Overview

```
┌──────────────────────────────────────┐
│            cmd/wmux                  │  CLI / entry point
│            cmd/wmux-tmux             │  tmux compatibility shim
├──────────────────────────────────────┤
│            pkg/client                │  Public Go SDK for integrators
├──────────────────────────────────────┤
│            internal/                 │  Core implementation
│  ┌────────────────────────────────┐  │
│  │  daemon    session   transport │  │  Domain layer (business logic)
│  ├────────────────────────────────┤  │
│  │  platform/                     │  │  Shared infrastructure
│  │  ansi  auth  config  event     │  │
│  │  history  ipc  logger          │  │
│  │  protocol  pty  recording      │  │
│  └────────────────────────────────┘  │
└──────────────────────────────────────┘
```

**cmd/** contains executable entry points. `wmux` is the main CLI binary that
bootstraps the fx application. `wmux-tmux` is a thin shim that translates tmux
commands into wmux equivalents.

**pkg/client** is the public Go SDK. External programs import this package to
communicate with a running wmux daemon without going through the CLI.

**internal/** holds all private implementation code, split into two layers:
domain packages and platform packages.

---

## DDD + goframe Conventions

Each domain package follows a strict file catalog:

| File | Purpose |
|------|---------|
| `entity.go` | Types, structs, sentinel errors (`var Err* = errors.New(...)`) |
| `service.go` | `Repository` interface, `Service` struct, business logic |
| `module.go` | `var Module = fx.Options(fx.Provide(...))` -- fx wiring only |
| `options.go` | Functional options: `type Option func(*T)`, `With*` constructors |
| `events.go` | Domain event types |
| `values.go` | Extra value objects |
| `*_mock.go` | Generated mocks (go.uber.org/mock) |
| `*_test.go` | Tests (same package, not `_test` suffix) |

Domains communicate exclusively through the fx dependency injection container.
A domain never imports another domain directly; instead, it depends on
interfaces that fx satisfies at startup.

---

## Import Rules

```
                   can import
  ┌────────────┐ ──────────────► stdlib
  │ cmd/*      │ ──────────────► anything (domains, platform, external)
  └────────────┘

  ┌────────────┐ ──────────────► stdlib
  │ domain/*   │ ──────────────► internal/platform/*
  └────────────┘ ──────╳──────► other domains
                 ──────╳──────► external libs (except fx in module.go)

  ┌────────────┐ ──────────────► stdlib
  │ platform/* │ ──────────────► external libs
  └────────────┘ ──────╳──────► internal/<domain>/
```

| Source | Allowed targets | Forbidden targets |
|--------|----------------|-------------------|
| `cmd/*` | stdlib, domains, platform, external libs | -- |
| `internal/<domain>/` | stdlib, `internal/platform/*` | other domains, external libs (except `go.uber.org/fx` in `module.go`) |
| `internal/platform/*` | stdlib, external libs | `internal/<domain>/` |

goframe enforces these rules at lint time (`make lint`).

---

## Key Design Decisions

### Why DDD + Package-Oriented Design

A PTY daemon touches many concerns: process lifecycle, terminal emulation,
IPC, authentication, configuration, and more. DDD provides clear boundaries
between these concerns. Each domain owns its entities, business rules, and
persistence interfaces. Package-Oriented Design ensures that Go packages map
one-to-one to these bounded contexts, preventing the "everything imports
everything" problem common in Go monoliths.

### Why fx (Dependency Injection)

Domains cannot import each other, yet they need to collaborate. fx wires
dependencies at startup through constructor injection: each domain declares
what it provides and what it requires via its `module.go`. This keeps domain
code free of infrastructure coupling while allowing the `cmd/` layer to
compose the full application.

### Why a Binary Protocol

wmux uses a custom binary framing protocol (version + type + length + payload)
for communication between the CLI client and the daemon over Unix sockets.
Compared to text-based protocols like JSON-over-newline:

- **Efficiency**: Terminal output is high-bandwidth (screen redraws, scrolling).
  Binary framing avoids escaping overhead and enables zero-copy forwarding of
  raw PTY output.
- **Framing clarity**: Length-prefixed frames eliminate the need for delimiter
  parsing, which is error-prone when the payload itself contains arbitrary
  bytes (terminal escape sequences, binary data).
- **Two-channel architecture**: The protocol supports separate control and
  stream channels. Control handles low-frequency RPCs (create, resize, list).
  Stream carries high-frequency PTY I/O with backpressure signaling. This
  separation prevents a flood of terminal output from starving control
  messages.
