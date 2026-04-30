# Autoresearch — wmux PTY throughput & latency

Started: 2026-04-29
Branch: master (working dir = `/Users/wlech/private/wmux`)
Roadmap: `/Users/wlech/private/watchtower/docs/perf/autoresearch-roadmap.md`

## Objective

Reduce **end-to-end latency** and **allocations** on the wmux PTY → client
broadcast hot path:

```
PTY read  →  batcher.Add  →  doFlush  →  Buffer.Write
                                              ↓
                                     ticker(16ms)  →  flushOutput  →  ReadOutput  →  EncodeDataPayload  →  client
```

Two targets are believed to dominate:

1. **Worst-case latency 32ms** from cascade of two 16ms timers
   (`session.defaultBatchInterval` + `daemon.broadcastInterval`).
2. **One allocation per flush** at every stage of the chain
   (batcher `out`, buffer `out`, `EncodeDataPayload` payload).

## Primary metric

Per-iteration nanoseconds and allocations on these benchmarks (32 KB chunk):

| Bench | Package |
|---|---|
| `BenchmarkBatcherAddFlush` | `internal/session` |
| `BenchmarkDoFlush` | `internal/session` |
| `BenchmarkBufferWriteRead` | `internal/session` |
| `BenchmarkEncodeDataPayload` | `internal/daemon` |

Lower ns/op and lower allocs/op are wins. We track all four so a win in one
that regresses another is visible.

## Baseline (2026-04-29, M3, `-benchtime=3s`)

| Bench | ns/op | MB/s | B/op | allocs/op |
|---|---:|---:|---:|---:|
| BatcherAddFlush | 4273 | 7667 | 32768 | 1 |
| DoFlush | 3571 | 9175 | 32792 | 2 |
| BufferWriteRead | 3836 | 8541 | 32768 | 1 |
| EncodeDataPayload | 2997 | 10933 | 40960 | 1 |

## Constraints (NON-NEGOTIABLE)

- All existing tests must pass with `-race -shuffle=on`.
- No public API changes to packages outside `internal/`.
- Behavior under backpressure (`Buffer.Paused`) must remain identical.
- No regression in `BenchmarkBatcherAddFlush` worst-case ns/op > 5%.

## Experiment log

Each entry: hypothesis → change → benchmark delta → kept/discarded.
Append-only. See `autoresearch.jsonl` for machine-readable history and
`experiments/worklog.md` for the narrative.

## Final Results (2026-04-29, M3, `-benchtime=5s`)

| Bench | ns/op | MB/s | B/op | allocs/op | Δns | Δallocs |
|---|---:|---:|---:|---:|---:|---:|
| BatcherAddFlush | 1668 | 19650 | 0 | 0 | -61% | -100% |
| DoFlush | 939 | 34891 | 0 | 0 | -66% | -100% |
| BufferWriteRead | 613 | 53421 | 0 | 0 | -84% | -100% |
| ReadLoopChunk | 3285 | 9975 | 0 | 0 | new | -100% |
| AcquireDataPayload | 595 | 55037 | 0 | 0 | -80% vs Encode | -100% |
| ParseOSC_NoOSC | 422 | 77714 | 0 | 0 | -81% | -100% |
| BurstFlushLatency | ~4000 ns | — | 0 | 0 | -99.97% | — |
| SignalLatency | ~6134 ns | — | 0 | 0 | -99.96% | — |

**Hot-path allocs in steady state: 0** (PTY read → batcher → buffer → broadcast, single-client).

## Experiments Completed (8/10)

| Run | ID | Technique | Impact |
|---|---|---|---|
| 1 | E4 | sync.Pool for doFlush buffer | -65% DoFlush, 0 allocs |
| 2 | E5 | Pool EncodeDataPayload | -80% payload alloc, 0 allocs |
| 3 | E1 | Size-threshold burst flush | 16ms → 4µs burst latency |
| 4 | E2 | Event-driven broadcast | 16ms → 6µs signal latency |
| 5 | E9 | ParseOSC zero-copy scan | -81% time, 0 allocs |
| 6 | E10 | Buffer 2-cycle swap | -84% BufferWriteRead, 0 allocs |
| 7 | E11 | Pool emulator chunks | readLoop 0 allocs per PTY read |
| 8 | E12 | Single-client string fast path | flushSessionOutput 0 allocs |

## Remaining backlog (diminishing returns)

🟢 Low potential
- E3. broadcastOutput skips idle sessions (CPU only, not latency)
- E6. Document/test `Buffer.Read` ownership semantics
- E7. RWMutex tuning on `Buffer`
- E8. Pre-cap channel buffer sizes in hotpaths
- Ticker flushOutput map-of-maps → slice-of-structs (62.5 allocs/sec savings)
