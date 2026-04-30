---
status: accepted
date: 2026-04-29
decision-makers: autoresearch (runs 1–8)
consulted: experiments/worklog.md
informed: internal/session, internal/daemon
---

# Zero-alloc PTY→buffer→broadcast hot path via pooling and 2-cycle swap

## Context and Problem Statement

The wmux PTY broadcast chain processes raw PTY reads and delivers them to
attached clients. In the baseline, each 32 KB chunk incurred **5+ allocations**:

```
readLoop:         make([]byte, n)           ← 32 KB per read
batcher.doFlush:  make([]byte, n) + copy    ← 32 KB per flush
buf.Write:        append(nil, data...)      ← 32 KB per Write
EncodeDataPayload: make([]byte, n)          ← 40 KB per broadcast
flushSessionOutput: make(map…)              ← small per signal
```

All of these fired on the hot path — every 32 KB of PTY output — generating
sustained GC pressure and non-deterministic latency spikes.

The goal: **0 allocs per chunk in steady state** without changing public APIs
or adding explicit caller-driven release calls.

## Decision Drivers

* GC pauses on the broadcast hot path degrade interactive latency.
* Each `sync.Pool` or structural trick has a correctness risk (borrow
  violations, data races); approaches must be verifiable with `-race`.
* Ownership contracts for borrowed slices must be local and documented,
  not spread across package boundaries.

## Considered Options

For each allocation site, multiple approaches were evaluated:

### A. doFlush buffer — sync.Pool (chosen: E4)

`doFlush` owns the slice only for the duration of `onFlush`. A `sync.Pool`
of `*[]byte` lets the pool supply a pre-allocated buffer; the slice is
returned immediately after `onFlush` returns. Zero-copy borrow; caller
(`Buffer.Write`) copies via `append`.

### B. Payload buffer — sync.Pool with Acquire/Release (chosen: E5)

`EncodeDataPayload` built a new slice every call. `AcquireDataPayload` /
`ReleaseDataPayload` wrap a pool the same way: borrow for the broadcast
loop duration (synchronous), return immediately after. Old `EncodeDataPayload`
is kept as a non-pooled variant for tests/non-hot paths.

### C. Buffer.Write alloc — explicit Release API (rejected)

Adding `RecycleOutput(id, []byte)` to `SessionManager` would let the daemon
return the slice to a pool inside `Buffer`. Rejected: spreads ownership
tracking across package boundary, requires interface change, easy to misuse
in future callers.

### D. Buffer.Write alloc — 2-cycle backing-array swap (chosen: E10)

Observation: in the hot path, the daemon reads from `Buffer`, copies the
data into `AcquireDataPayload` synchronously, broadcasts, and returns. The
slice from `Read` is no longer needed before the *next-next* `Write`. A
2-cycle chain (`inFlight → spare`) tracks the two most-recently drained
backing arrays:

* `Read`: `spare = inFlight; inFlight = out; data = nil`
* `Write` (when `data` is nil): `data = spare[:0]; spare = nil`

After 2 warm-up allocations, every subsequent `Write` reuses `spare`'s
backing array (allocated 2 cycles ago, caller is certainly done). Result:
**0 allocs/op** in steady state, no API change, no cross-package contract.

Safety proof:
- `spare` is the backing array from 2 reads ago.
- Between that read and the current write, the caller processed the data
  synchronously (copy → broadcast → return). The backing array is free.
- `inFlight` (1 read ago) may still be in use; we never recycle it — only
  `spare` (2 reads ago) is recycled.

### E. readLoop chunk alloc — pool for emulator, direct slice for batcher (chosen: E11)

`readLoop` allocated `make([]byte, n)` to share between batcher and emulator.
Analysis:
- `batcher.Add(data)` copies `data` into its internal pool buffer immediately.
  It does not retain the slice after `Add` returns.
- Emulator is async (`emulatorCh`): chunk must outlive `Add` until
  `emulatorLoop` calls `Process`.

Fix: pass `buf[:n]` directly to `batcher.Add` (no alloc, batcher copies);
give emulator a pooled `*[]byte` via `emulatorChunkPool`. `emulatorLoop`
returns it to the pool after `Process` (including on panic-recovery path).
Changed `emulatorCh` from `chan []byte` to `chan *[]byte` to track the
pool handle. Channel-full drops `Put` the slice back immediately.

### F. flushSessionOutput client map — string variable for single client (chosen: E12)

`flushSessionOutput` allocated `make(map[string]struct{})` for every chunk
signal. For the common case of exactly 1 attached client, capture the client
ID as a `string` under RLock and skip the map entirely. Multi-client falls
back to `[]string` (lighter than map).

## Decision Outcome

All six techniques (A–F) were implemented and kept. Together they achieve:

| Hot-path stage | Before | After |
|---|---|---|
| `readLoop` chunk | 1 alloc / 32 KB | **0** |
| `batcher.doFlush` buffer | 1 alloc / 32 KB | **0** |
| `Buffer.Write` backing array | 1 alloc / 32 KB | **0** (steady state) |
| Broadcast payload | 1 alloc / 40 KB | **0** |
| `flushSessionOutput` client map | 1 alloc | **0** (single-client) |
| **Total per chunk** | **5 allocs / ~160 KB** | **0** |

Latency results (M3 Mac, `-benchtime=5s`):

| Bench | Baseline | After | Δ |
|---|---|---|---|
| BatcherAddFlush | 4273 ns / 1 alloc | 1668 ns / 0 | -61% |
| BufferWriteRead | 3836 ns / 1 alloc | 613 ns / 0 | -84% |
| ParseOSC (no match) | 2261 ns / 1 alloc | 422 ns / 0 | -81% |
| Burst flush latency | ~16 ms | ~4 µs | -99.97% |
| End-to-end signal latency | ~16 ms | ~6 µs | -99.96% |

### Consequences

* **Good**: zero GC pressure from the broadcast hot path in steady state.
* **Good**: no public API changes; all techniques are internal to their packages.
* **Good**: all existing tests pass under `-race -shuffle=on`.
* **Good**: regression gates in benchmarks (0 allocs/op) prevent silent regressions.
* **Neutral**: Buffer now has `inFlight` and `spare` fields with a documented
  2-cycle invariant; future maintainers must understand the ownership contract
  before modifying `Read`/`Write`.
* **Neutral**: `emulatorCh` is now `chan *[]byte`; the pool-return responsibility
  is in `emulatorLoop`. The contract is documented in comments.
* **Bad**: `ReadN` partial-drain case still allocates (not on the hot path).

### Confirmation

`BenchmarkBatcherAddFlush`, `BenchmarkDoFlush`, `BenchmarkBufferWriteRead`,
and `BenchmarkReadLoopChunk` are regression gates: they must all report
`0 allocs/op`. `BenchmarkBatcherBurstLatency` gates the 16ms → 4µs
threshold flush. CI enforces `-race -shuffle=on` across all 19 packages.

## More Information

- `experiments/worklog.md` — narrative of each experiment with before/after
- `autoresearch.jsonl` — machine-readable results for all 8 runs
- `internal/session/batcher.go` — `flushBufPool` contract comment
- `internal/session/backpressure.go` — 2-cycle swap contract comment
- `internal/session/service.go` — `emulatorChunkPool` contract comment
- `internal/daemon/values.go` — `dataPayloadPool` contract comment
