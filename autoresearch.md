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

## Backlog (from roadmap, ranked)

🔴 High potential
- E1. Size-threshold flush in batcher (no more 16ms wait on bursts)
- E2. Collapse two 16ms timers (batcher onFlush → broadcaster signal)
- E3. broadcastOutput skips idle sessions (only iterate sessions with pending data)

🟡 Medium potential
- E4. `sync.Pool` for `doFlush` `out` buffer (target: 0 allocs)
- E5. Pool the `EncodeDataPayload` payload buffer
- E6. Document/test `Buffer.Read` ownership semantics

🟢 Low potential
- E7. RWMutex tuning on `Buffer`
- E8. Pre-cap channel buffer sizes in hotpaths
