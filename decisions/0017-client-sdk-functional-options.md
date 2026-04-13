---
status: accepted
date: 2026-04-13
decision-makers: wblech
---

# Client SDK with Functional Options and Embedded Daemon

## Context and Problem Statement

The initial `pkg/client/` API used a flat struct (`Connect(Options{SocketPath, TokenPath})`) that required consumers to know daemon implementation details (socket paths, token files). It provided no way to manage the daemon lifecycle, forcing integrators like Watchtower to manually spawn and configure the daemon process. This conflicted with the goal of wmux being an embeddable library where the consumer installs one binary and never needs to know wmux exists.

## Decision Drivers

* Follow the project's goframe functional options convention (`type Option func(*T)`)
* Zero-config experience for simple use cases
* Namespace isolation so multiple apps run independent daemons
* Embeddable daemon without requiring a separate `wmux` binary
* Sessions must survive app restarts (persistent daemon via binary re-execution)

## Considered Options

* Keep flat struct API, add daemon spawning as separate helper
* Refactor to functional options with `New` as single entry point, `NewDaemon` for embedding, `ServeDaemon` for persistent mode
* Expose `internal/daemon` as `pkg/daemon` (move out of internal)

## Decision Outcome

Chosen option: "Functional options with New/NewDaemon/ServeDaemon", because it provides three levels of integration through a single consistent API:

1. **`New(opts...)`** — connects to a daemon, auto-starts one if needed via `os.Executable()` re-execution
2. **`NewDaemon(opts...)`** — embeds the daemon in-process for integrators who manage the lifecycle themselves
3. **`ServeDaemon(args)`** — hook for the auto-start mechanism; detects the `__wmux_daemon__` sentinel in args

Namespace isolation derives all paths from `WithNamespace("name")` → `~/.wmux/{name}/`. Explicit `WithSocket`/`WithDataDir` overrides take precedence.

Exposing `internal/daemon` was rejected because it would leak transport interfaces, connected client types, and other implementation details that consumers don't need. Instead, `pkg/client/adapter.go` encapsulates the wiring internally.

### Consequences

* Good, because consumers get zero-config with `client.New()` or one-option with `client.New(client.WithNamespace("myapp"))`
* Good, because `cmd/wmux/daemon.go` dogfoods `client.NewDaemon`, proving the API works
* Good, because persistent daemon requires only 3 lines in the consumer's `main()` (`ServeDaemon` hook)
* Good, because namespace isolation prevents multi-app conflicts
* Bad, because auto-start spawns `os.Executable()` which means the consumer binary must include the `ServeDaemon` hook — otherwise auto-start fails silently
* Bad, because adapter boilerplate in `pkg/client/adapter.go` reduces measured test coverage (thin wrappers with no logic)
