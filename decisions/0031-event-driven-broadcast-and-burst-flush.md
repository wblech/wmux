---
status: accepted
date: 2026-04-29
decision-makers: wblech
---

# Event-Driven Broadcast Wakeup with Size-Threshold Burst Flush

## Context and Problem Statement

The PTY → client broadcast hot path was bounded by two cascaded 16 ms timers:

1. **Session batcher** (`internal/session/batcher.go`) accumulated PTY reads and
   flushed to the per-session ring buffer only on a `time.Ticker` of
   `batchInterval` (16 ms). Even when a single PTY read filled 32 KB in <1 µs,
   the data sat in the batcher waiting for the next tick.
2. **Daemon broadcaster** (`internal/daemon/service.go:broadcastOutput`)
   polled every session's buffer on a second `time.Ticker` of
   `broadcastInterval` (16 ms) and only then emitted `MsgData` frames to
   attached clients.

End-to-end worst case from "PTY read returns" to "frame on the wire" was the
sum of both intervals — up to ~32 ms of artificial latency, with a typical
~16 ms in steady state. This is felt as input lag in interactive TUIs (vim,
less, claude code streaming). A microbenchmark
(`BenchmarkBatcherSubThresholdLatency`) confirmed ~16 ms wall-clock from
`Add` to consumer wakeup.

The two timers were originally chosen for batching benefit: collapsing many
small writes into fewer frames reduces socket and protocol overhead. That
benefit is real for low-volume traffic (one keystroke at a time) but is pure
waste for bursts that already fill a frame in microseconds.

## Decision

### Size-threshold flush in the batcher

`internal/session/batcher.go`:

- New constant `defaultFlushThreshold = 32 * 1024` (sized to match
  PTY `readChunkSize`, so a single saturated read crosses the threshold).
- New `Batcher.flushThreshold int` field, set in `newBatcher`.
- `Add` checks `len(b.buf) >= flushThreshold` after appending; if so, sends
  a non-blocking signal on the existing `flush` channel. The flush goroutine
  selects between `flush`, `done`, and `ticker.C` and drains the buffer
  immediately on either trigger.
- The timer is **kept** unchanged: sub-threshold writes still batch on
  `interval`, preserving the framing benefit for low-volume traffic.

### Event-driven broadcast wakeup

`internal/session/service.go` and `internal/daemon/service.go`:

- New `Service.onDataReady func(id string)` field, configurable via
  `session.WithOnDataReady` and exposed as `Service.OnDataReady(fn)`.
- The session batcher's `onFlush` callback invokes `s.onDataReady(id)`
  immediately after `buf.Write` succeeds, on the batcher goroutine.
- `Daemon` owns a buffered channel `dataReady chan string` (cap 256). On
  `Start`, it registers a non-blocking sender via
  `sessionSvc.OnDataReady(func(id) { select { case d.dataReady <- id:
  default: } })`.
- `broadcastOutput` selects on **both** `dataReady` (fast path,
  `flushSessionOutput(sessID)` — single-session drain) **and** `ticker.C`
  (fallback, full `flushOutput()`).
- `flushSessionOutput` is a new method that drains exactly one session's
  buffer and broadcasts to its attached clients. It is the same hot-path
  logic as `flushOutput` scoped to one session ID — the common case after a
  data-ready signal.

The 16 ms ticker is **kept on purpose** as a fallback for two reasons:

1. The non-blocking send drops signals when the channel is full (slow
   broadcaster, many simultaneous bursts). The ticker guarantees eventual
   drain.
2. Buffered output may need to be drained for waiters/persistence even when
   no client is currently attached. `flushSessionOutput` defers to the
   slow-path `flushOutput` in that case, but the ticker is the simpler
   guarantee.

### `SessionManager` interface gains `OnDataReady`

The daemon already abstracts the session backend behind `SessionManager`.
`OnDataReady(fn func(id string))` is a new required method. The contract
on the callback is documented inline: must be non-blocking, runs on the
batcher goroutine. Adapters in `pkg/client`, the daemon test fake, and the
e2e harness forward the call (most as no-ops, since they don't drive the
batcher).

## Consequences

- Good, because end-to-end signal latency on bursts drops from ~16 ms
  (poll-only) to ~9 µs (signaled), measured by
  `BenchmarkBatcherSignalLatency` — roughly 1800×.
- Good, because the batcher's burst latency alone drops from ~16 ms to ~4 µs
  (`BenchmarkBatcherBurstLatency`), independently useful for any consumer of
  the buffer (waiters, persistence, recording).
- Good, because the change is layered: `Batcher` change is local and
  testable with no daemon wiring. `OnDataReady` is opt-in (default `nil`)
  so existing tests that build a `Service` without a daemon are unaffected.
- Good, because the ticker fallback bounds the failure mode: if the
  channel is saturated and signals are dropped, the worst observable
  behavior is the original 16 ms latency — the same as the previous
  steady state, never worse.
- Good, because sub-threshold traffic (one keystroke at a time) still
  batches on the timer, preserving the framing benefit. Confirmed by
  `BenchmarkBatcherSubThresholdLatency` remaining at ~16 ms.
- Bad, because `OnDataReady` adds a callback that runs on a hot goroutine
  with strict non-blocking contract. A future caller that violates this
  (e.g. registers a callback that takes a mutex contended with the read
  path) will stall the batcher and silently degrade latency back to the
  ticker fallback. The doc comment on `WithOnDataReady` and `OnDataReady`
  is the only enforcement.
- Bad, because `flushSessionOutput` and `flushOutput` are now near-duplicates
  of the same broadcast logic with slightly different scopes. A future
  refactor could unify them; for now the duplication is small and the
  fast path benefits from being scoped to one session ID without map
  iteration.
- Bad, because `defaultFlushThreshold = 32 KB` is coupled to PTY
  `readChunkSize`. If `readChunkSize` ever changes, the threshold should
  follow — there is no test that fails on divergence.

## Validation

Three regression tests gate the behavior (added with this ADR):

- `TestBatcher_AddCrossingThreshold_FlushesImmediately` — single `Add`
  crossing 32 KB calls `onFlush` within 5 ms, much shorter than the
  100 ms timer used in the test.
- `TestBatcher_SubThresholdAdd_WaitsForTimer` — `Add` of 1 KB does not
  trigger a flush before the timer interval (negative case; ensures the
  threshold is not "any data triggers immediate flush").
- `TestService_OnDataReadyFiresAfterBufferWrite` — registering a callback
  via `WithOnDataReady` causes the callback to fire after a PTY read,
  with the correct session ID, before the daemon's 16 ms ticker would.

The four hot-path microbenchmarks
(`BenchmarkBatcherBurstLatency`, `BenchmarkBatcherSignalLatency`,
`BenchmarkBatcherSubThresholdLatency`, `BenchmarkBatcherAddFlush`) document
the performance characteristics and serve as performance regression
detectors for `make bench`.

## Related

- [ADR-0024](0024-async-emulator-processing.md) — async emulator pipeline;
  same general pattern of decoupling a hot path from a fixed-rate poller.
- [ADR-0025](0025-drain-emulator-response-pipe.md) — earlier change to the
  same broadcast goroutine.
- `experiments/worklog.md`, runs E1 (size threshold) and E2 (event-driven
  wakeup), record the autoresearch experiments and cumulative numbers.

Commits: `6126a0d` (E1 size-threshold flush), `3e6682b` (E2 event-driven
broadcast wakeup) on `wblech/wmux:main`.
