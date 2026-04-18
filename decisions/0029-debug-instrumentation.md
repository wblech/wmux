# 29. Debug Instrumentation for PTY Data Flow

Date: 2026-04-18

## Status

Accepted

## Context

Downstream consumers (e.g., Watchtower) cannot diagnose whether data flow
issues — lost characters, cursor drift, backpressure stalls — originate in the
daemon (readLoop, Buffer, emulator, broadcast) or in their own code. The only
options were reading consumer-side logs and guessing, or hand-patching wmux
with fmt.Printf to bisect the problem.

## Decision

Add an opt-in debug tracing facility in `internal/platform/debug` that writes
structured JSON Lines via `slog.JSONHandler` to a rotating file (`lumberjack`).

Key design choices:

- **`internal/platform/debug`, not `pkg/debug`:** The tracing engine stays
  private. Consumers configure it via `pkg/client` options (`WithDebugLog`,
  `WithDebugLevel`, `WithDebugMaxSize`, `WithDebugMaxFiles`) and env vars
  (`WMUX_DEBUG`, `WMUX_DEBUG_PATH`, `WMUX_DEBUG_LEVEL`, `WMUX_DEBUG_MAX_SIZE_MB`,
  `WMUX_DEBUG_MAX_FILES`). No new public types exported.

- **slog + lumberjack, not hook callback:** Standardized JSON Lines format
  across all consumers. No consumer code needed — an env var is enough. File
  rotation prevents unbounded growth.

- **4 levels (Off, Lifecycle, Chunk, Full):** Level controls which fields are
  populated in each event. Higher levels add more data (hex payloads, SHA-1).
  The SDK filters fields before writing — no wasted computation at lower levels.

- **16 stages covering the full pipeline:** pty.read → buffer.append →
  buffer.flush → frame.send, plus lifecycle events (session create/close,
  attach/detach, resize, snapshot, backpressure pause/resume, emulator
  in/out/drop).

- **Per-session sequence counters:** Enable gap detection in the log file.
  Lifecycle events use seq -1.

- **Typed flat fields on Event, not map[string]any:** Zero allocation, type
  safe, zero-value fields omitted from JSON.

- **Nil-safe Tracer:** A nil `*Tracer` returns false from `Enabled()`. All
  callsites use `if t.Enabled() { t.Emit(...) }` — zero overhead when off.

- **Env var activation, not build tags:** A single binary supports debug
  tracing without rebuild. The overhead of a nil check per callsite is
  negligible (branch prediction).

## Alternatives Rejected

- **Hook callback API:** Inconsistent formats across consumers, pushes logging
  complexity onto each consumer, harder to support ("send me your log" works
  only if everyone uses the same format).

- **Middleware/wrapper approach:** Overengineering — requires interfaces for
  every component, cannot cover non-method points (channel drops, sleep loops).

- **Event bus (ADR-0008):** Designed for domain events, not hot-path tracing.
  Marshaling and dispatch overhead unacceptable on every PTY read.

- **Build tags:** Build complexity (two binaries, CI tests both) without
  meaningful gain over nil-check gating.

- **`pkg/debug` public package:** Unnecessary API surface. Options on
  `pkg/client` are sufficient.

## Consequences

- New dependency: `gopkg.in/natefinch/lumberjack.v2`.
- `session.Service` and `daemon.Daemon` gain a `tracer` field and `WithTracer`
  option.
- `Buffer` gains a `tracer` field (passed from parent Service).
- Consumers can enable diagnosis with `WMUX_DEBUG=1` — no code changes.
- Consumers can correlate daemon logs with their own logs via timestamp +
  session_id + seq.
