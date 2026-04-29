# Worklog — wmux autoresearch

## Run 0 — Baseline (2026-04-29)

Created benchmarks at `internal/session/bench_test.go` and
`internal/daemon/bench_test.go` covering the four hot-path stages.

| Bench | ns/op | MB/s | B/op | allocs/op |
|---|---:|---:|---:|---:|
| BatcherAddFlush | 4273 | 7667 | 32768 | 1 |
| DoFlush | 3571 | 9175 | 32792 | 2 |
| BufferWriteRead | 3836 | 8541 | 32768 | 1 |
| EncodeDataPayload | 2997 | 10933 | 40960 | 1 |

Observations:
- 1 alloc per stage × 3 stages × every chunk = 3 allocs/chunk minimum on the
  broadcast path (batcher out + buffer write internal + payload).
- `EncodeDataPayload` allocates 40960 B for ~32789 B of payload — Go's size
  class rounding (32 KB + ~20 B → 40 KB class).
- `DoFlush` shows 2 allocs/op; the second is likely the closure/onFlush
  capturing `data` — investigate if reused.
- `MB/s` is per-iter overhead, NOT real-world throughput. Real throughput is
  bounded by the 16 ms timer (≈ 2 MB/s per session at most without bursts).
- After cleaning up the closure capture in `BenchmarkDoFlush` (storing into a
  package-level `flushSinkLen` instead of `atomic.Pointer`), DoFlush dropped
  to **2757 ns / 1 alloc / 32768 B**. Real baseline.

## Run 1 — E4: pool buffer in doFlush (2026-04-29)

**Hypothesis.** `doFlush` does `make([]byte, n) + copy` per flush — 1 alloc
of 32 KB on every chunk. With a `sync.Pool` of preallocated 64 KB slices, we
can serve the slice for free. Both production callers (`Buffer.Write` via
`append`) and tests already copy, so handing back a borrowed slice that
becomes invalid after `onFlush` returns is safe.

**Change.** `internal/session/batcher.go`:
1. New package-level `flushBufPool sync.Pool` returning `*[]byte` (cap 64 KB).
2. `doFlush` does `Get → append → onFlush(out) → reset → Put`.

**Results (32 KB chunk, 3 s benchtime, M3):**

| Bench | Baseline | E4 | Δ ns/op | Δ allocs |
|---|---:|---:|---:|---:|
| BatcherAddFlush | 4273 | 2026 | **-52%** | 1 → 0 |
| DoFlush | 2757 | 968 | **-65% / 2.8×** | 1 → 0 |
| BufferWriteRead | 3836 | 2316 | -40% | 1 → 1 (unchanged shape) |

`BufferWriteRead` improvement is run-to-run variance (no code path changed).

**Correctness.** `go test ./internal/session/ -race -shuffle=on -count=1` passes.

**Verdict: KEPT.** Half-the-cost batcher hot path with zero allocations.

**Side observation.** EncodeDataPayload (40 KB / 1 alloc per chunk) is now
the single biggest waste on the broadcast path. Promote E5 (pool payload
buffer) above E1.

## Run 2 — E5: pool EncodeDataPayload buffer (2026-04-29)

**Hypothesis.** `EncodeDataPayload` allocates ~40 KB once per broadcast
(`make([]byte, 0, …)` rounded to size class). Production calls it once
per active session per 16 ms tick. With a `sync.Pool`, this can drop to
zero allocs.

**Constraint discovered mid-run.** First attempt returned `(payload, release func())`
— the closure capture cost 48 B / 1 alloc / call. Refactored to return
`(payload, *[]byte)` with a separate `ReleaseDataPayload(handle)` function.
No closure → 0 allocs.

**Change.** `internal/daemon/values.go`:
1. New `dataPayloadPool sync.Pool` returning `*[]byte` (cap 64 KB).
2. `AcquireDataPayload(sessionID, data) ([]byte, *[]byte)`.
3. `ReleaseDataPayload(handle *[]byte)`.
4. `internal/daemon/service.go:flushOutput` switched from `EncodeDataPayload`
   to the acquire/release pair. Release happens after the inner broadcast loop;
   `Codec.Encode` is synchronous (`w.Write(payload)`) so the slice is no longer
   referenced once the loop exits.

**Results (32 KB chunk, 3 s benchtime, M3):**

| Bench | Baseline | E5 | Δ |
|---|---:|---:|---:|
| EncodeDataPayload (untouched) | 2997 ns / 40960 B / 1 alloc | 2621 ns / 40960 B / 1 alloc | (variance only) |
| AcquireDataPayload (new) | — | **591 ns / 0 B / 0 alloc** | **-80% / 5× vs baseline Encode** |

**Correctness.** Full repo `go test ./... -race -shuffle=on -count=1` passes
across all 19 packages.

**Verdict: KEPT.** Broadcast hot path now goes from 3 allocs/chunk
(batcher out, payload, Buffer.Write append) to 1 alloc/chunk. Only the
buffer's internal `append` growth remains.

## Cumulative wins (Runs 1 + 2)

| Stage | Baseline ns / allocs | After ns / allocs | Δ |
|---|---:|---:|---:|
| Batcher (full Add+Flush) | 4273 / 1 | 2048 / 0 | **-52% / 2.1×** |
| `doFlush` only | 2757 / 1 | 967 / 0 | **-65% / 2.8×** |
| Payload encode | 2997 / 1 (40 KB) | 591 / 0 (0 B) | **-80% / 5×** |

## Run 3 — E1: size-threshold flush in batcher (2026-04-29)

**Hypothesis.** The batcher waits up to 16 ms (its timer interval) before
flushing, even when a single PTY read fills 32 KB in <1 µs. Adding a
size threshold so `Add` signals an immediate flush when the buffer crosses
the threshold should drop burst latency from ~16 ms to scheduling overhead.

**Change.** `internal/session/batcher.go`:
1. New constant `defaultFlushThreshold = 32 * 1024`.
2. New `Batcher.flushThreshold int` field, set in `newBatcher`.
3. `Add` checks `len(b.buf) >= flushThreshold` after append; if so,
   signals the flush channel (non-blocking).

**Validation strategy.** Two new benchmarks measure wall-clock latency
between `Add` and the `onFlush` callback firing:
- `BenchmarkBatcherBurstLatency` (64 KB chunks → crosses threshold)
- `BenchmarkBatcherSubThresholdLatency` (1 KB chunks → stays under)

The pair is an A/B in the same binary: same code path, threshold-crossing
makes the only difference. If E1 works, big bursts are fast and small
ones are unchanged.

**Results (M3, -benchtime=2s):**

| Bench | ns/op | Interpretation |
|---|---:|---|
| BatcherBurstLatency (64 KB chunks) | **4,227** (~4 µs) | Threshold-driven immediate flush |
| BatcherSubThresholdLatency (1 KB chunks) | 16,439,237 (~16.4 ms) | Timer-driven, baseline behavior |

**Impact.** Bursts ≥32 KB drop from ~16 ms worst-case to ~4 µs —
**~3900×** improvement on the latency-sensitive path (interactive output,
TUI redraws, claude code streaming). Small chunks are unchanged, so the
batching benefit (fewer frames) is preserved for low-volume traffic.

**Correctness.** Full repo `go test ./... -race -shuffle=on -count=1` passes
across all 19 packages.

**Verdict: KEPT.**

## Cumulative wins (Runs 1, 2, 3)

| Stage | Baseline | After | Δ |
|---|---|---|---:|
| Batcher Add+Flush throughput | 4273 ns / 1 alloc | 2048 ns / 0 alloc | -52% / 2.1× |
| `doFlush` only | 2757 ns / 1 alloc | 967 ns / 0 alloc | -65% / 2.8× |
| Payload encode | 2997 ns / 1 alloc / 40 KB | 591 ns / 0 alloc / 0 B | -80% / 5× |
| **Burst flush latency** | **~16 ms** | **~4 µs** | **~3900×** |

## Run 4 — E2: collapse 16 ms timer cascade (2026-04-29)

**Hypothesis.** After E1, the batcher flushes bursts in ~4 µs. But the data
then sits in the session buffer for up to 16 ms waiting for the daemon's
broadcaster ticker. Replacing the poll with an event-driven signal should
collapse this final tail.

**Change.**

- `internal/session/options.go` — new `WithOnDataReady(fn func(id string))`.
- `internal/session/service.go` — new `Service.OnDataReady` method;
  the batcher's onFlush calls `s.onDataReady(id)` after `buf.Write`.
- `internal/daemon/service.go`
  — added `OnDataReady` to the `SessionManager` interface;
  — `Daemon.dataReady chan string` (buffered 256);
  — `Start()` registers a non-blocking sender on the channel;
  — `broadcastOutput` selects on `dataReady` (fast path) AND `ticker.C`
    (fallback for any dropped signals);
  — new `flushSessionOutput(sessID)` for single-session flush.
- All adapters (`pkg/client`, `internal/daemon` test, `test/e2e`)
  forward the new method.

The ticker is kept on purpose: a non-blocking send on a full channel
drops the signal, but the ticker fallback ensures eventual flush. In
practice the channel is 256-buffered and drains immediately, so the
fast path covers ~all production traffic.

**Bench (32 KB×2 burst chunk → buf.Write → signal → reader, M3, -benchtime=2s):**

| Bench | ns/op | Δ |
|---|---:|---:|
| BatcherBurstLatency (Add → onFlush) | 4534 | unchanged |
| **BatcherSignalLatency (Add → buf.Write → signal → consumer)** | **9043** | full path |
| BatcherSubThresholdLatency (timer-only, 1 KB) | 16,117,537 (~16 ms) | unchanged baseline |

**Impact.** End-to-end worst case: **~16 ms (poll-only) → ~9 µs (signaled),
roughly 1800×.** Sub-threshold path unchanged — small writes still
batch via timer.

**Correctness.** Full repo `go test ./... -race -shuffle=on -count=1` passes
across all 19 packages.

**Verdict: KEPT.**

## Cumulative wins (Runs 1–4)

| Stage | Baseline | After | Δ |
|---|---|---|---:|
| Batcher Add+Flush throughput | 4273 ns / 1 alloc | 2048 / 0 | -52% |
| `doFlush` only | 2757 ns / 1 alloc | 967 / 0 | -65% / 2.8× |
| Payload encode | 2997 ns / 1 alloc / 40 KB | 591 / 0 / 0 B | -80% / 5× |
| Burst flush latency (batcher only) | ~16 ms | ~4 µs | ~3900× |
| **End-to-end signal latency** | **~16 ms** | **~9 µs** | **~1800×** |

Allocs on broadcast hot path: **3 → 1 per chunk**. Only `Buffer.Write`
internal append remains.

## Next experiments (parked)

- **E3 — broadcastOutput skips idle sessions** (CPU at idle; separate
  concern from latency, lower priority now that the latency wins are in).
- **E6 — document Buffer.Read ownership** (ADR only; the code is already
  zero-copy via slice swap).
- **E7/E8 — micro-tuning** of mutex/channel sizes (low ROI).
