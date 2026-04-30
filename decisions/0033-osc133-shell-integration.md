---
status: accepted
date: 2026-04-30
decision-makers: brainstorming + 2 review rounds (subagent-driven implementation)
consulted: ADR 0032 (zero-alloc hot path), ADR 0024 (async emulator), CLAUDE.md (goframe)
informed: internal/cmdlifecycle, internal/platform/commands, internal/daemon, internal/platform/event
---

# OSC 133 shell-integration — per-session command lifecycle tracking

## Context and Problem Statement

Shells emit OSC 133 escape sequences to mark structural boundaries in terminal
output: prompt drawn (`A`), user typing (`B`), command executing (`C`), and
command finished with exit code (`D`). Prior to this work, wmux received these
as opaque bytes in the PTY broadcast stream — they were forwarded to clients
unchanged but their semantics were invisible to the daemon.

The goal: recognize OSC 133 sequences on the daemon side, maintain a per-session
command lifecycle state machine, expose three new IPC events
(`command.prompt_shown`, `command.started`, `command.finished`), and persist
command history to `commands.json` alongside the existing `scrollback.bin` for
cold-restore. Downstream clients (Watchtower) can then show exit-code badges,
jump between prompt boundaries, and recover history after a daemon restart.

Seven architectural decisions were required to make this fit the wmux design
without regressing the zero-alloc PTY broadcast hot path from ADR 0032.

## Decision Drivers

* **ADR 0032 invariant**: `BenchmarkParseOSC_NoOSC` must remain 0 allocs/op;
  any extension to `ParseOSC` that regresses the no-match path blocks merge.
* **No charmvt fork**: must use only the public charmvt API; the emulator path
  is lossy and must not carry reliable event delivery.
* **Cold-restore consistency**: `commands.json` and `scrollback.bin` are both
  written by wmux and must align — row numbers in command records reference
  scrollback line positions.
* **Single source of truth for rows**: no per-client recomputation that could
  drift across multi-client sessions or cold-restore reads.
* **Additive IPC**: Watchtower and other integrators receive new event types on
  the existing `MsgEvent` frame; old clients hit `default` and ignore them.

## Considered Options

### 1. Detection mechanism: `ParseOSC` extension vs. `charmvt.RegisterOscHandler`

**Chosen: passive scanner (`ParseOSC` extension)**

`charmvt` exposes `RegisterOscHandler(133, fn)` which would fire inside the
emulator on parsed sequences. This was rejected on three grounds:

- The emulator channel (`emulatorCh`) is **lossy by design** — a `select { default }`
  drops chunks under load (ADR 0024). OSC 133 events on a lossy path cannot be
  the source of truth for a state machine.
- When `EmulatorFactory == nil`, sessions use `NoneEmulator{}` which discards
  all input. OSC 133 events would never fire in that mode.
- OSC 7/9/777 are already handled via the passive scanner in
  `internal/daemon/osc.go`; adding 133 in the same place is consistent,
  requires no new interfaces, and puts all OSC logic in one file.

The passive scanner (`ParseOSC`) was extended with an `Offset int` field on
`OSCResult` and a new `OSCType133` constant. The cursor-based loop that replaced
the old slice-walk turned out marginally faster (410 ns/op vs. 422 ns/op) while
preserving 0 allocs on the no-match path.

### 2. Row tracking ownership: daemon vs. client

**Chosen: daemon**

Row numbers in `Command.StartRow` / `EndRow` are line positions in
`scrollback.bin`, which wmux owns exclusively. Client-side recomputation would
cause drift in multi-client sessions and break cold-restore alignment
(`commands.json` and `scrollback.bin` must reference the same coordinate space).
Daemon-side tracking uses a `map[string]*atomic.Int64` (`lineCounters`) keyed by
session ID; the pointer is captured under `RLock` once per matching chunk and
incremented lock-free thereafter.

### 3. Row counting strategy: eager per-chunk vs. lazy on match

**Chosen: lazy on match**

Eager `bytes.Count(data, '\n')` on every chunk would burn CPU when no OSC
sequences are present (the common case). Lazy counting fires only inside the
`OSCType133` case branch of `scanOSC`, processing only the prefix between
markers. A tail-count after the loop accumulates remaining newlines so the
cross-chunk invariant holds: the counter always reflects all newlines up to
the current chunk boundary before returning.

### 4. State machine placement: new domain `internal/cmdlifecycle/` vs. extend `internal/daemon/`

**Chosen: new domain package**

The state machine has clear boundaries: inputs are OSC codes + rows; outputs are
`Command` records and lifecycle events. It has no I/O dependencies. The daemon
package was already ~1700 lines; adding more state machine logic would deepen the
god-object problem. A dedicated package achieves ≥95% coverage in pure-logic
unit tests without mocking daemon internals.

### 5. Persistence ownership: `cmdlifecycle.Repository` vs. daemon side-channel

**Chosen: `cmdlifecycle.Repository` interface, daemon implements**

`cmdlifecycle.Repository` exposes one method: `Save(sessID, state)`. The daemon
implements it via `cmdRepository` wrapping `map[string]*commands.Writer`.
Per-session writer lifecycle (`Open`/`Close`) is daemon-private — not on the
interface. This keeps `cmdlifecycle` I/O-free (mock Repository is trivial in
tests) while placing file lifecycle next to the existing `scrollbackWriters` map.

### 6. `ParseOSC` refactor: in-place struct extension vs. parallel API

**Chosen: in-place extension**

Adding `Offset int` to `OSCResult` is backward-compatible — existing callers
(OSC 7/9/777 paths) ignore the new field. A parallel `ParseOSC2` API would
have duplicated parsing logic and complicated the call sites. The correctness
risk (regressing the no-match 0 allocs path) was mitigated upfront:
`BenchmarkParseOSC_NoOSC` already existed; this work added an explicit
`TestBenchmarkParseOSC_NoOSC_ZeroAllocs` regression gate using
`testing.AllocsPerRun` so CI fails immediately if the no-match path allocates.

### 7. In-flight command on cold-restore: resume vs. orphan

**Chosen: orphan with `OrphanReason = "daemon_restart"`**

When the daemon dies, PTY child processes die with it (PPID-tied via the
reconcile loop). The in-flight command was definitionally aborted. Marking it
as an orphan (`ExitCode = -1`, `OrphanReason = "daemon_restart"`,
`EndedAt = file.SavedAt`) produces honest history. Resuming
`StateCommandRunning` was rejected: it requires detecting shell reconnection
and is unsound when the shell is dead.

## Decision Outcome

All seven decisions were implemented across 21 commits and 19 tasks.

| Decision | Choice | Mechanism |
|---|---|---|
| Detection | Passive scanner | `ParseOSC` extended in `internal/daemon/osc.go` |
| Row tracking | Daemon-side | `lineCounters map[string]*atomic.Int64` in `Daemon` |
| Row counting | Lazy on match | `bytes.Count(data[cursor:m.Offset], '\n')` inside `OSCType133` case |
| State machine | New domain | `internal/cmdlifecycle/` (entity, service, events, options, module) |
| Persistence | `cmdlifecycle.Repository` | `commands.Writer` — debounced 200ms, atomic rename via `.tmp` |
| `ParseOSC` | In-place struct extension | `Offset int` field on `OSCResult`; gated by bench |
| Cold-restore in-flight | Orphan | `OrphanReason = "daemon_restart"`, `EndedAt = file.SavedAt` |

### Consequences

**Good:**

- Zero-alloc broadcast hot path preserved. All ADR 0032 invariants intact:
  `BenchmarkParseOSC_NoOSC` reports 0 allocs/op; Batcher, Buffer, and ReadLoop
  benches are unaffected.
- Three new IPC event types (`command.prompt_shown`, `command.started`,
  `command.finished`) available to all subscribers via the existing `MsgEvent`
  frame — no protocol changes.
- `commands.json` enables cold-restore of full command history (exit codes,
  timestamps, row ranges, orphan records) across daemon restarts.
- `cmdlifecycle` is pure logic with no I/O — ≥95% unit-test coverage achievable
  without environment setup or file system mocks.
- No charmvt fork. Works with both the full charmvt emulator and `NoneEmulator`.
- Watchtower and other integrators receive additive changes only; no breaking changes.

**Neutral:**

- One additional goroutine per session for the `commands.Writer` debouncer.
  wmux already runs ≥3 goroutines per session; one more is within Go's envelope.
- Maximum disk footprint for `commands.json` is ~100 KB per session
  (500 commands × ~200 bytes). The FIFO cap (default 500) bounds growth.

**Bad / known limitations:**

- **OSC 133 events fire and persist only when the session has at least one
  attached client.** `flushSessionOutput` iterates attached and waiter sessions;
  detached sessions are not processed. This is consistent with existing OSC
  7/9/777 behavior. For detached sessions that never re-attach before a daemon
  crash, command history of in-flight commands is lost. A future pass would add
  a third loop in `flushOutput` for detached sessions — deferred until a real
  use case appears.
- The 200ms debounce window means a daemon crash within 200ms of a
  `command.finished` may lose that command's persisted entry. The IPC event was
  delivered; only the durable write is at risk. `commands.Writer.Close()` flushes
  synchronously on session exit and daemon shutdown, so the crash window is narrow.

## Confirmation

Regression gates added by this work:

- `TestBenchmarkParseOSC_NoOSC_ZeroAllocs` — uses `testing.AllocsPerRun` to
  fail CI if the no-match path regresses to >0 allocs.
- `BenchmarkHandleOSC_NoOp` — 0 allocs/op gate on `cmdlifecycle` no-op
  transitions (e.g., `D` received in Idle state).
- `BenchmarkScanOSC_NoMatch` — integration-level 0 allocs/op gate on the full
  `scanOSC` path when no OSC sequences are present.

Existing ADR 0032 gates verified intact:

- `BenchmarkBatcherAddFlush` 0 allocs/op ✅
- `BenchmarkBufferWriteRead` 0 allocs/op ✅
- `BenchmarkReadLoopChunk` 0 allocs/op ✅

Test coverage: 27 `cmdlifecycle` tests, 14 `commands` tests, 19+ `ParseOSC`
tests (133 variants + offset accuracy), 6 `cmdRepository` tests, 3 `cmdwire`
tests, 4 `RestoreCommandsForSession` tests, 2 E2E tests (bash real shell +
direct OSC injection).

## More Information

- `docs/superpowers/specs/2026-04-30-osc133-shell-integration-design.md` — full
  design spec with per-decision rationale, transition table, wire format, and
  cold-restore protocol
- `docs/superpowers/plans/2026-04-30-osc133-shell-integration.md` — task-by-task
  implementation plan (21 commits across 19 tasks)
- `internal/cmdlifecycle/` — state machine domain (entity, service, events,
  options, module)
- `internal/platform/commands/` — disk persistence (Writer, Load)
- `internal/daemon/osc.go` — `ParseOSC` passive scanner with `Offset` extension
- `internal/daemon/cmdrepository.go` — `cmdlifecycle.Repository` implementation
- ADR 0032 — zero-alloc invariants this design is constrained by
- ADR 0024 — emulator async / lossy channel rationale (why `RegisterOscHandler`
  was not used)
