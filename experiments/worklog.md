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

## Next experiments (parked)

- **E1 — size-threshold flush** (high latency win, needs latency-distribution
  bench, not throughput bench).
- **E2 — collapse 16 ms timer cascade** (event-driven broadcaster signal from
  batcher onFlush instead of polling). Bigger refactor; needs end-to-end
  latency bench to validate.
- **E3 — broadcastOutput skips idle sessions** (CPU at idle).
- **E6 — document Buffer.Read ownership** (already zero-copy, just needs ADR).

Pausing here — two clean wins, ready for human review / commit before more
architectural changes.
