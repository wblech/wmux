# Phase 2 Sub-plan 5: Architecture Decision Records

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Write ADRs for all architectural decisions from Phase 1 (retroactive) and Phase 2 using MADR 4.0.0 format. Templates already exist at `decisions/`.

**Architecture:** Each ADR is a standalone Markdown file using the bare template. Phase 1 ADRs document decisions already implemented. Phase 2 ADRs document decisions from the design spec.

**Tech Stack:** Markdown, MADR 4.0.0

**Prerequisites:** Sub-plans 1-4 complete. Templates at `decisions/adr-template-bare.md`. Design spec at `docs/superpowers/specs/2026-04-13-phase2-watchtower-integration-design.md`. PRD at `docs/PRD.md`.

**References:**
- [PRD](../../PRD.md)
- [Phase 1 Spec](../specs/2026-04-04-pty-daemon-design.md)
- [Phase 2 Spec](../specs/2026-04-13-phase2-watchtower-integration-design.md)
- [MADR 4.0.0](https://adr.github.io/madr/)

---

## ADR Index

| ADR | Title | Phase |
|---|---|---|
| 0000 | Use Markdown Architectural Decision Records | 1 |
| 0001 | DDD + Package-Oriented Design with goframe | 1 |
| 0002 | Binary length-prefixed wire protocol | 1 |
| 0003 | Two-channel IPC design (control + stream) | 1 |
| 0004 | Token file authentication | 1 |
| 0005 | Pluggable headless emulator interface | 1 |
| 0006 | High/low watermark backpressure | 1 |
| 0007 | Use uber/fx for dependency injection | 1 |
| 0008 | In-process event bus with fan-out and type filtering | 1 |
| 0009 | Autodaemonize with PID file and heartbeat | 1 |
| 0010 | Emulator addon as external process with binary protocol | 2 |
| 0011 | Single Node process for N xterm headless instances | 2 |
| 0012 | Go client library in pkg/client/ | 2 |
| 0013 | Two-phase warm attach snapshot | 2 |
| 0014 | Opt-in cold restore | 2 |
| 0015 | Stable symlinks for environment forwarding | 2 |
| 0016 | Single-client per session (multi-client deferred) | 2 |

---

### Task 1: ADR-0000 — Use MADR

**Files:**
- Create: `decisions/0000-use-madr.md`

- [ ] **Step 1: Write ADR**

```markdown
---
status: accepted
date: 2026-04-13
decision-makers: wblech
---

# Use Markdown Architectural Decision Records

## Context and Problem Statement

We want to record architectural decisions made in this project so that future contributors and AI agents can understand why things were built a certain way. We need a format that is lightweight, version-controlled, and consistent with the Watchtower project.

## Considered Options

* MADR 4.0.0
* Michael Nygard's template
* Formless notes in code comments

## Decision Outcome

Chosen option: "MADR 4.0.0", because it provides structured capturing of decisions with a lean format that fits our development style. Using the same format as Watchtower ensures consistency across projects.
```

- [ ] **Step 2: Commit**

```bash
git add decisions/0000-use-madr.md
git -c commit.gpgsign=false commit -m "docs(adr): 0000 use MADR 4.0.0 for architectural decisions"
```

---

### Task 2: ADR-0001 through ADR-0004 (Phase 1 foundation)

**Files:**
- Create: `decisions/0001-ddd-goframe.md`
- Create: `decisions/0002-binary-wire-protocol.md`
- Create: `decisions/0003-two-channel-ipc.md`
- Create: `decisions/0004-token-file-auth.md`

- [ ] **Step 1: Write ADR-0001 — DDD + Package-Oriented Design with goframe**

```markdown
---
status: accepted
date: 2026-04-11
decision-makers: wblech
---

# DDD + Package-Oriented Design with goframe

## Context and Problem Statement

wmux needs a code organization strategy that enforces clear domain boundaries, prevents import cycles, and scales as the project grows across multiple phases.

## Decision Drivers

* Prevent coupling between domain packages
* Enforce file naming conventions automatically
* Keep external dependencies out of domain logic

## Considered Options

* Flat package structure
* Traditional layered architecture (handler/service/repo)
* DDD with goframe enforced conventions

## Decision Outcome

Chosen option: "DDD with goframe", because it enforces domain isolation via import rules (domain packages cannot import each other), standardizes file naming (entity.go, service.go, module.go), and uses linting to catch violations. Platform packages in `internal/platform/` provide shared infrastructure.

### Consequences

* Good, because import cycles are structurally impossible
* Good, because new contributors know exactly where to put code
* Bad, because goframe conventions require discipline (every package needs entity.go, service.go, module.go)
```

- [ ] **Step 2: Write ADR-0002 — Binary length-prefixed wire protocol**

```markdown
---
status: accepted
date: 2026-04-11
decision-makers: wblech
---

# Binary Length-Prefixed Wire Protocol

## Context and Problem Statement

wmux needs a wire protocol between daemon and clients for control messages and data streaming. It must be fast for high-throughput PTY output while supporting structured control messages.

## Decision Drivers

* Zero perceptible latency (PRD: "the daemon must feel like it doesn't exist")
* Support both structured control messages and raw binary data streams
* Simple to implement and debug

## Considered Options

* JSON over newline-delimited streams
* gRPC with protobuf
* Custom binary: `[version:1][type:1][length:4][payload:N]`

## Decision Outcome

Chosen option: "Custom binary frames", because the 6-byte header adds minimal overhead for high-throughput PTY data, the format is trivial to parse in any language, and we avoid external dependencies (gRPC) or encoding overhead (JSON for binary data).

### Consequences

* Good, because sub-millisecond framing overhead for PTY output
* Good, because version byte enables future protocol evolution
* Bad, because debugging requires hex inspection (mitigated by CLI adapter that outputs JSON)
```

- [ ] **Step 3: Write ADR-0003 — Two-channel IPC design**

```markdown
---
status: accepted
date: 2026-04-11
decision-makers: wblech
---

# Two-Channel IPC Design (Control + Stream)

## Context and Problem Statement

Mixing control messages (create, kill, resize) with data streams (PTY output) on a single connection causes head-of-line blocking — a large data payload delays time-sensitive control responses. Superset encountered this exact problem.

## Decision Drivers

* Avoid head-of-line blocking between control and data
* Keep the protocol simple (single socket)
* Support independent backpressure on stream channel

## Considered Options

* Single channel with priority flags
* Two separate Unix sockets
* Single socket, two logical channels (control + stream) multiplexed by message type

## Decision Outcome

Chosen option: "Single socket, two logical channels", because it avoids the complexity of managing two sockets while eliminating head-of-line blocking. Control messages are request-response on the control channel; data and events flow on the stream channel with independent backpressure.

### Consequences

* Good, because control latency is unaffected by data throughput
* Good, because single socket simplifies connection management
* Bad, because the transport layer must demux frames to the correct channel
```

- [ ] **Step 4: Write ADR-0004 — Token file authentication**

```markdown
---
status: accepted
date: 2026-04-11
decision-makers: wblech
---

# Token File Authentication

## Context and Problem Statement

Clients connecting to the daemon need authentication to prevent unauthorized access. The mechanism must work without user interaction (no passwords) and integrate with automation security modes (open, same-user, children).

## Considered Options

* Unix socket peer credentials only (SO_PEERCRED)
* Token file at `~/.wmux/daemon.token`
* TLS client certificates

## Decision Outcome

Chosen option: "Token file", because it's simple (32 random bytes, file-permission protected), works across all platforms (including future Windows named pipes), and combines well with peer credential checks for defense in depth.

### Consequences

* Good, because zero user interaction — token is auto-generated on first daemon spawn
* Good, because file permissions restrict access to the owning user
* Bad, because any process that can read the token file can connect (mitigated by automation_mode setting)
```

- [ ] **Step 5: Commit all four**

```bash
git add decisions/0001-ddd-goframe.md decisions/0002-binary-wire-protocol.md decisions/0003-two-channel-ipc.md decisions/0004-token-file-auth.md
git -c commit.gpgsign=false commit -m "docs(adr): 0001-0004 Phase 1 foundation decisions"
```

---

### Task 3: ADR-0005 through ADR-0009 (Phase 1 remaining)

**Files:**
- Create: `decisions/0005-pluggable-emulator.md`
- Create: `decisions/0006-backpressure-watermarks.md`
- Create: `decisions/0007-uber-fx.md`
- Create: `decisions/0008-event-bus.md`
- Create: `decisions/0009-autodaemonize.md`

- [ ] **Step 1: Write ADR-0005 — Pluggable headless emulator interface**

```markdown
---
status: accepted
date: 2026-04-11
decision-makers: wblech
---

# Pluggable Headless Emulator Interface

## Context and Problem Statement

To provide warm attach with visual state restore, the daemon needs to maintain a headless terminal emulator per session. Different integrators need different backends: xterm.js users want @xterm/headless (zero divergence), others may want ghostty-vt, and lightweight/embedded use cases want no emulator at all.

## Considered Options

* Hardcode xterm headless as the only emulator
* Pluggable ScreenEmulator interface with multiple backends

## Decision Outcome

Chosen option: "Pluggable ScreenEmulator interface", because it decouples the daemon from any specific emulator implementation. The interface has three methods: `Process([]byte)`, `Snapshot() Snapshot`, `Resize(cols, rows)`. Phase 1 ships with `NoneEmulator` (no-op). Phase 2 adds the xterm addon backend.

### Consequences

* Good, because integrators choose the emulator that matches their frontend
* Good, because `none` backend keeps the daemon lightweight when restore isn't needed
* Bad, because each backend has different fidelity characteristics
```

- [ ] **Step 2: Write ADR-0006 — High/low watermark backpressure**

```markdown
---
status: accepted
date: 2026-04-11
decision-makers: wblech
---

# High/Low Watermark Backpressure

## Context and Problem Statement

Programs that produce output faster than clients can consume (e.g., `cat` on a large file) will cause unbounded memory growth in the daemon's output buffer. The daemon needs flow control.

## Considered Options

* Fixed-size ring buffer (drop old data)
* Backpressure with high/low watermarks (pause/resume PTY reads)
* Rate limiting output per second

## Decision Outcome

Chosen option: "High/low watermark backpressure", because it preserves all data (no drops), automatically adapts to client speed, and uses the proven TCP-style flow control pattern. When the buffer reaches the high watermark (default 1MB), PTY reads are paused. When it drains to the low watermark (default 256KB), reads resume.

### Consequences

* Good, because zero data loss — all PTY output is eventually delivered
* Good, because bounded memory usage per session
* Bad, because pausing PTY reads can cause the program to block on write() — but this is the correct backpressure signal
```

- [ ] **Step 3: Write ADR-0007 — uber/fx for DI**

```markdown
---
status: accepted
date: 2026-04-11
decision-makers: wblech
---

# Use uber/fx for Dependency Injection

## Context and Problem Statement

wmux has multiple packages that need to be wired together (config, session, transport, daemon). Manual wiring in main() gets complex as the project grows.

## Considered Options

* Manual wiring in main()
* uber/fx dependency injection
* google/wire compile-time DI

## Decision Outcome

Chosen option: "uber/fx", because each domain package exposes a `Module = fx.Options(fx.Provide(...))` that declares its dependencies, and fx resolves the graph at startup. This integrates well with the goframe convention of `module.go` per package.

### Consequences

* Good, because adding new packages is declarative — just add to Module
* Good, because fx detects missing dependencies at startup, not runtime
* Bad, because fx uses reflection, which can make debugging harder
```

- [ ] **Step 4: Write ADR-0008 — In-process event bus**

```markdown
---
status: accepted
date: 2026-04-12
decision-makers: wblech
---

# In-Process Event Bus with Fan-Out and Type Filtering

## Context and Problem Statement

The daemon needs to emit lifecycle events (session created, exited, etc.) that clients can subscribe to. The event system must be non-blocking for the publisher and support multiple subscribers with type-based filtering.

## Considered Options

* Direct callback registration
* Channel-based fan-out event bus
* External message broker (NATS, Redis pub/sub)

## Decision Outcome

Chosen option: "Channel-based fan-out event bus", because it's in-process (no external dependencies), non-blocking for publishers (events are sent to buffered channels per subscriber, 256 buffer), and supports multiple subscribers. Subscribers receive events on a channel and can filter by type.

### Consequences

* Good, because publishing never blocks the daemon's main loop
* Good, because subscriber disconnection is handled via Unsubscribe()
* Bad, because if a subscriber's buffer fills, events are dropped (acceptable trade-off for non-blocking)
```

- [ ] **Step 5: Write ADR-0009 — Autodaemonize**

```markdown
---
status: accepted
date: 2026-04-12
decision-makers: wblech
---

# Autodaemonize with PID File and Heartbeat

## Context and Problem Statement

CLI commands like `wmux create` need a running daemon. Users shouldn't have to manually start/stop the daemon. The daemon needs to be auto-started on first use and discovered by subsequent CLI commands.

## Considered Options

* Require manual `wmux daemon start`
* Auto-fork daemon on first CLI command
* Systemd/launchd service (Phase 4)

## Decision Outcome

Chosen option: "Auto-fork daemon", because it provides zero-config startup. The CLI checks if a daemon is running (via PID file at `~/.wmux/daemon.pid`), and if not, forks a background daemon process. The PID file contains JSON with PID, version, and start time. A heartbeat file is updated periodically so clients can detect stale daemons.

### Consequences

* Good, because zero user configuration needed
* Good, because PID file enables discovery and version checking
* Bad, because orphaned PID files need cleanup (handled by reconciliation on startup)
```

- [ ] **Step 6: Commit all five**

```bash
git add decisions/0005-pluggable-emulator.md decisions/0006-backpressure-watermarks.md decisions/0007-uber-fx.md decisions/0008-event-bus.md decisions/0009-autodaemonize.md
git -c commit.gpgsign=false commit -m "docs(adr): 0005-0009 Phase 1 remaining decisions"
```

---

### Task 4: ADR-0010 through ADR-0013 (Phase 2 core)

**Files:**
- Create: `decisions/0010-emulator-addon-external-process.md`
- Create: `decisions/0011-single-node-process.md`
- Create: `decisions/0012-client-library-pkg.md`
- Create: `decisions/0013-two-phase-snapshot.md`

- [ ] **Step 1: Write ADR-0010 — Emulator addon as external process**

```markdown
---
status: accepted
date: 2026-04-13
decision-makers: wblech
---

# Emulator Addon as External Process with Binary Protocol

## Context and Problem Statement

The xterm headless emulator requires Node.js runtime. Embedding Node in the Go daemon would add a mandatory dependency. We need a way to keep the daemon pure Go while supporting xterm headless.

## Decision Drivers

* Daemon must remain pure Go with zero Node dependency
* Emulator backend must be pluggable (not just xterm)
* Performance overhead must be minimal

## Considered Options

* Embed Node via cgo bindings
* External process with JSON-RPC over stdin/stdout
* External process with binary protocol over stdin/stdout

## Decision Outcome

Chosen option: "External process with binary protocol", because it keeps the daemon pure Go, the binary protocol avoids JSON/base64 encoding overhead (~33% data inflation eliminated vs JSON), and the addon architecture is generic enough for any emulator backend.

### Consequences

* Good, because daemon has zero Node dependency
* Good, because binary protocol matches the daemon's existing wire format style
* Good, because addons can be written in any language
* Bad, because IPC adds latency vs in-process calls (mitigated by fire-and-forget for process, only snapshot is synchronous)
```

- [ ] **Step 2: Write ADR-0011 — Single Node process for N instances**

```markdown
---
status: accepted
date: 2026-04-13
decision-makers: wblech
---

# Single Node Process for N xterm Headless Instances

## Context and Problem Statement

Each terminal session needs an xterm headless instance. Running one Node process per session would consume ~30-50MB each, leading to ~3GB for 100 sessions. We need an efficient approach.

## Decision Drivers

* Memory efficiency: typical 10-30 sessions, worst case 100
* Instant snapshot availability (no lazy processing)
* Acceptable crash blast radius

## Considered Options

* One Node process per session (~30MB × N)
* Single Node process managing N instances (~50MB base + ~2MB × N)
* Lazy processing: process only on snapshot request

## Decision Outcome

Chosen option: "Single Node process with N instances", because memory usage drops from ~3GB to ~250MB for 100 sessions. The xterm headless library is lightweight (~2MB per instance). The emulator runs continuously so snapshots are always instantly available for warm attach.

### Consequences

* Good, because ~110MB typical memory vs ~900MB with per-session processes
* Good, because snapshots are always ready (no processing delay on attach)
* Bad, because if the Node process crashes, all sessions lose their emulator temporarily (mitigated by crash recovery: respawn + re-feed buffered output)
```

- [ ] **Step 3: Write ADR-0012 — Go client library in pkg/client/**

```markdown
---
status: accepted
date: 2026-04-13
decision-makers: wblech
---

# Go Client Library in pkg/client/

## Context and Problem Statement

Watchtower and other Go applications need a convenient way to interact with the wmux daemon without speaking the binary protocol directly. We need a client library that abstracts the protocol.

## Considered Options

* Client library in a separate repository
* Client library inside wmux repo at `pkg/client/`
* No client library (consumers speak raw protocol)

## Decision Outcome

Chosen option: "Inside wmux repo at `pkg/client/`", because it evolves alongside the daemon (protocol changes are updated atomically), consumers import it as a Go module dependency, and `pkg/` is the standard Go convention for public packages.

### Consequences

* Good, because protocol and client stay in sync
* Good, because single `go get` adds the dependency
* Bad, because consumers pull the entire wmux module (acceptable for Go applications)
```

- [ ] **Step 4: Write ADR-0013 — Two-phase warm attach snapshot**

```markdown
---
status: accepted
date: 2026-04-13
decision-makers: wblech
---

# Two-Phase Warm Attach Snapshot

## Context and Problem Statement

When a client attaches to an existing session, it needs the current terminal state to render immediately. The xterm headless emulator maintains this state. We need a way to deliver it to the client at attach time.

## Decision Drivers

* Instant visual restore on workspace switch in Watchtower
* Pixel-perfect with xterm.js frontend (zero divergence)
* No ongoing overhead when not attaching

## Considered Options

* Send only SIGWINCH (shell redraws — current Phase 1 behavior with `none` backend)
* Single-phase: send entire terminal buffer as one blob
* Two-phase: scrollback (history) + viewport (current screen) as separate payloads

## Decision Outcome

Chosen option: "Two-phase snapshot", because Watchtower's xterm.js can load scrollback into history and viewport into the active buffer separately, enabling immediate rendering. The daemon requests the snapshot from the addon only at attach time (synchronous), so there's no ongoing overhead.

### Consequences

* Good, because client can render scrollback and viewport independently
* Good, because snapshot is only computed on demand (attach)
* Bad, because snapshot of a busy terminal can be large (hundreds of KB) — acceptable for one-time attach
```

- [ ] **Step 5: Commit all four**

```bash
git add decisions/0010-emulator-addon-external-process.md decisions/0011-single-node-process.md decisions/0012-client-library-pkg.md decisions/0013-two-phase-snapshot.md
git -c commit.gpgsign=false commit -m "docs(adr): 0010-0013 Phase 2 core decisions"
```

---

### Task 5: ADR-0014 through ADR-0016 (Phase 2 remaining)

**Files:**
- Create: `decisions/0014-opt-in-cold-restore.md`
- Create: `decisions/0015-env-forwarding-symlinks.md`
- Create: `decisions/0016-single-client.md`

- [ ] **Step 1: Write ADR-0014 — Opt-in cold restore**

```markdown
---
status: accepted
date: 2026-04-13
decision-makers: wblech
---

# Opt-In Cold Restore

## Context and Problem Statement

Cold restore allows session recovery after a reboot by persisting scrollback and metadata to disk. However, not all users need this, and continuous disk writes have a performance cost.

## Considered Options

* Always-on cold restore (write history for every session)
* Opt-in via config (`history.cold_restore = true`)
* Per-session opt-in

## Decision Outcome

Chosen option: "Opt-in via config", because it gives users a simple global toggle. When disabled (default), the daemon writes zero history files — no disk I/O overhead. When enabled, the existing `history/` package handles persistence. Per-session granularity was considered unnecessary complexity.

### Consequences

* Good, because default is zero disk overhead
* Good, because existing history package already handles the write path
* Bad, because users who want cold restore must explicitly enable it
```

- [ ] **Step 2: Write ADR-0015 — Stable symlinks for env forwarding**

```markdown
---
status: accepted
date: 2026-04-13
decision-makers: wblech
---

# Stable Symlinks for Environment Forwarding

## Context and Problem Statement

When a client re-attaches after an SSH reconnection, environment variables like SSH_AUTH_SOCK point to a new socket path. The shell inside the session still uses the old path. We need to update the session's environment on re-attach.

## Considered Options

* Inject environment variables into the shell process (not portable)
* Write an env file for the shell to source manually
* Create stable symlinks that always point to the current path

## Decision Outcome

Chosen option: "Stable symlinks", because the session shell sees a constant path (`~/.wmux/sessions/{id}/SSH_AUTH_SOCK`) that is a symlink to the real socket. On re-attach, the daemon updates the symlink target without the shell needing to do anything. Non-path values fall back to an env file.

### Consequences

* Good, because transparent to the shell — no sourcing required for socket paths
* Good, because works for SSH agent forwarding, X11, etc.
* Bad, because only works for file/socket paths — other env vars require manual sourcing
```

- [ ] **Step 3: Write ADR-0016 — Single-client per session**

```markdown
---
status: accepted
date: 2026-04-13
decision-makers: wblech
---

# Single-Client Per Session (Multi-Client Deferred)

## Context and Problem Statement

The PRD defines multi-client support with broadcast, leader control, and capabilities. However, Watchtower's primary use case is single-user with one client per session. Multi-client is needed for pair programming, which is not an immediate priority.

## Decision Drivers

* Simplify Phase 2 scope
* Watchtower operates as single-client per session
* Multi-client infrastructure is significant complexity (broadcast, leader, capabilities, resize strategy)

## Considered Options

* Full multi-client in Phase 2
* Single-client in Phase 2, multi-client deferred
* Basic multi-client (broadcast only, no leader/capabilities)

## Decision Outcome

Chosen option: "Single-client, multi-client deferred", because it significantly reduces Phase 2 scope while covering Watchtower's actual use case. The existing attachment tracking in the daemon already handles one client per session. Multi-client with pair programming will be added when that use case becomes a priority.

### Consequences

* Good, because Phase 2 scope is focused and achievable
* Good, because the daemon architecture doesn't preclude multi-client later (attachment maps already support N clients)
* Bad, because pair programming use case is blocked until multi-client is implemented
```

- [ ] **Step 4: Commit all three**

```bash
git add decisions/0014-opt-in-cold-restore.md decisions/0015-env-forwarding-symlinks.md decisions/0016-single-client.md
git -c commit.gpgsign=false commit -m "docs(adr): 0014-0016 Phase 2 remaining decisions"
```

---

### Task 6: Update ROADMAP with Phase 2 tracking

**Files:**
- Modify: `docs/ROADMAP.md`

- [ ] **Step 1: Add sub-plan checkboxes to Phase 2 section**

In the Phase 2 implementation status section, replace the flat checklist with sub-plan grouped checkboxes:

```markdown
### Phase 2: Watchtower Integration

**Sub-plan 1: Emulator Addon Protocol + xterm Addon**
- [ ] Addon binary protocol (Go encoder/decoder)
- [ ] AddonEmulator (process manager, ScreenEmulator proxy)
- [ ] Config: emulator.xterm settings
- [ ] xterm addon: Node project + binary protocol
- [ ] xterm addon: instance manager + main loop
- [ ] Wire AddonEmulator into session creation

**Sub-plan 2: Go Client Library + Warm Attach**
- [ ] Public types (entity.go)
- [ ] Client connection (client.go)
- [ ] Session operations (session.go)
- [ ] Event callbacks (event.go)
- [ ] Daemon: handleAttach returns snapshot
- [ ] Integration guide

**Sub-plan 3: Metadata + Events + Env Forwarding**
- [ ] New event types (6 types)
- [ ] New protocol message types (MetaSet, MetaGet, EnvForward)
- [ ] Session metadata (entity + daemon handlers)
- [ ] OSC parser (7/9/99/777)
- [ ] Environment forwarding (symlinks + env file)
- [ ] Wire OSC + env into daemon
- [ ] Client library: metadata + environment

**Sub-plan 4: Cold Restore**
- [ ] Config: history.cold_restore
- [ ] Daemon: guard history writes
- [ ] Client: SessionHistory type
- [ ] Client: LoadSessionHistory + CleanSessionHistory

**Sub-plan 5: Architecture Decision Records**
- [ ] ADR-0000: Use MADR
- [ ] ADR-0001 through ADR-0009: Phase 1 decisions
- [ ] ADR-0010 through ADR-0013: Phase 2 core decisions
- [ ] ADR-0014 through ADR-0016: Phase 2 remaining decisions
```

- [ ] **Step 2: Commit**

```bash
git add -f docs/ROADMAP.md
git -c commit.gpgsign=false commit -m "docs: add Phase 2 sub-plan tracking checkboxes to ROADMAP"
```
