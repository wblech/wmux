---
status: accepted
date: 2026-04-17
decision-makers: wblech
---

# Drain Emulator Response Pipe to Prevent Write Deadlock

## Context and Problem Statement

When Watchtower is closed and reopened while a TUI app (Claude Code, vim, htop) is running, **all terminals show blank** — including bash terminals unrelated to the TUI app. The daemon becomes completely unresponsive: every client RPC request times out.

ADR 0024 introduced async emulator processing to prevent slow `Process()` from blocking PTY reads. However, a different failure mode was discovered: `Process()` can **block indefinitely**, not from slow processing or panics, but from a **deadlock in the underlying `io.Pipe`**.

### Root cause investigation

The `charmbracelet/x/vt` emulator uses an internal `io.Pipe` (`pr`/`pw`) for terminal responses. When the emulator processes escape sequences that require a response — DA1, DA2, DSR, CPR — it calls `io.WriteString(e.pw, response)`. Since `io.Pipe` is synchronous, `Write()` blocks until someone calls `Read()` on the other end.

The charmvt wrapper never reads from this pipe. When a TUI app emits a DA1 query (`\x1b[c`, 3 bytes), the cascade is:

```
vt.Write([]byte("\x1b[c"))
  → parser dispatches CSI 'c' (DA1) handler
    → handler calls io.WriteString(e.pw, DA1 response)
      → pw.Write() BLOCKS (nobody reads e.pr)
        → emulator.Process() never returns
          → emulator mutex held forever
            → Snapshot() blocks (same mutex)
              → daemon handleAttach blocks
                → ALL client RPCs queue and timeout
```

The original investigation (2026-04-17-emulator-blocking-investigation.md) hypothesized an "infinite loop in the vt parser." Evidence tests proved this was incorrect — the parser loop is bounded by `len(p)`. The actual cause is the undrained `io.Pipe`.

### Sequences that trigger the deadlock

| Handler | Escape sequence | Bytes | Response written to pipe |
|---------|----------------|-------|--------------------------|
| DA1     | `\x1b[c`       | 3     | `\x1b[?62;1;6;22c`      |
| DA2     | `\x1b[>c`      | 4     | Secondary device attrs   |
| DSR CPR | `\x1b[6n`      | 4     | Cursor position report   |
| DSR     | `\x1b[5n`      | 4     | Operating status         |
| DECXCPR | `\x1b[?6n`     | 5     | Extended cursor position  |

The 3-byte DA1 query matches the finding from the debug investigation ("blocking chunk was 3 bytes, `dataLen=3`"). TUI frameworks commonly send DA1 during terminal capability detection.

## Decision Drivers

* TUI apps that emit terminal queries must not freeze the daemon
* The fix must not require changes to the upstream `charmbracelet/x/vt` library
* Snapshot accuracy must be preserved — the emulator is used only for snapshots
* The `ScreenEmulator` interface must remain unchanged (no breaking changes)
* Resources (goroutines) must be properly cleaned up when sessions end

## Considered Options

1. **Drain goroutine: read and discard pipe responses**
2. Drain goroutine with response forwarding to PTY
3. Process timeout with emulator replacement via `atomic.Pointer`

## Decision Outcome

Chosen option: "Drain goroutine: read and discard pipe responses", because it addresses the root cause directly with minimal code, no interface changes, and proper cleanup.

### Implementation

**charmvt/emulator.go** — drain goroutine started in `newEmulator()`:

```go
// Drain the emulator's response pipe to prevent Write() from blocking.
go drainResponsePipe(term)

func drainResponsePipe(term *vt.Emulator) {
    buf := make([]byte, 256)
    for {
        if _, err := term.Read(buf); err != nil {
            return // term.Close() → pipe EOF → goroutine exits
        }
    }
}
```

**charmvt/emulator.go** — `io.Closer` implementation for cleanup:

```go
func (e *emulator) Close() error {
    return e.term.Close() // closes pipe → drain goroutine exits
}
```

**session/service.go** — emulator cleanup in `waitLoop`, following the existing `historyWriter` pattern:

```go
if closer, ok := ms.emulator.(io.Closer); ok {
    _ = closer.Close()
}
```

### Why discard responses (not forward)

The vt emulator in wmux serves **only for snapshots**. It is not the interactive terminal. The real terminal (xterm.js in Watchtower) receives the same PTY output via the batcher broadcast and responds to queries directly. Forwarding vt responses back to the PTY would cause the program to receive **duplicate responses** — one from xterm.js and one from the vt emulator. Discarding is correct.

### Consequences

**Positive:**

* All TUI apps work: Claude Code, vim, htop, Ink apps, Bubble Tea apps, etc.
* Daemon never freezes from emulator pipe deadlock
* Zero goroutine leaks — `Close()` called in `waitLoop` when session ends
* No interface changes — `ScreenEmulator` unchanged; cleanup uses `io.Closer` type assertion
* No changes to the async architecture from ADR 0024 — the buffered channel and panic recovery remain
* Fix is 15 lines of production code

**Negative:**

* Terminal query responses (DA1, CPR) are discarded, so programs running inside wmux sessions do not receive vt-generated responses. In practice this is a non-issue because the real terminal (xterm.js) responds. If wmux were used without a real terminal frontend, programs depending on DA1/CPR responses would not receive them.

### Why not the other options

**Option 2 (forward responses to PTY):** Would require plumbing a PTY writer reference through the emulator factory, changing the `EmulatorFactory.Create` signature or adding a callback config. Adds complexity for a scenario that doesn't occur — the real terminal already responds. Forwarding would cause duplicate responses.

**Option 3 (timeout + emulator replacement):** The original design spec proposed replacing the emulator via `atomic.Pointer` when `Process()` exceeds 5 seconds, with a snapshot cache via `atomic.Value`. This adds ~100 lines of code, introduces a leaked goroutine on each replacement (~8KB), and solves a problem that doesn't exist (the vt parser does not have infinite loops — the blocking is purely from the pipe). The drain goroutine solves the root cause in 15 lines.

## Confirmation

### Evidence tests (prove the bug exists in vt)

These tests use a raw `vt.Emulator` without the drain to demonstrate that `Write()` blocks when the pipe has no reader:

* `TestEvidence_DA1_Blocks` — `\x1b[c` blocks Write() without reader
* `TestEvidence_DA2_Blocks` — `\x1b[>c` blocks
* `TestEvidence_DSR_CursorPosition_Blocks` — `\x1b[6n` blocks
* `TestEvidence_DSR_OperatingStatus_Blocks` — `\x1b[5n` blocks
* `TestEvidence_DECXCPR_Blocks` — `\x1b[?6n` blocks
* `TestEvidence_DrainedPipe_Unblocks` — all sequences complete instantly with a reader
* `TestEvidence_MixedContent_BlocksMidStream` — normal content + DA1 blocks mid-Write
* `TestEvidence_PipeResponse_Content` — DA1 response is valid VT220 (`\x1b[?62;1;6;22c`)

### Regression tests (break if drain is removed)

* `TestRegression_ProcessDA1Completes` through `TestRegression_ProcessDECXCPRCompletes` — all 5 query types complete within 100ms
* `TestRegression_SnapshotAfterDA1` — content integrity after query processing
* `TestRegression_SnapshotDuringConcurrentDA1` — concurrent Process(DA1) + Snapshot
* `TestRegression_AllQueryTypesInSingleChunk` — all queries concatenated in one Write
* `TestRegression_RepeatedDA1DoesNotDegrade` — 1000 DA1 queries without backpressure
* `TestRegression_HighThroughputWithQueries` — 500 lines + periodic queries
* `TestRegression_ViewportNotCorruptedByQueries` — viewport content integrity
* `TestRegression_ScrollbackPreservedAcrossQueries` — scrollback unaffected by queries
* `TestRegression_ConcurrentProcessSnapshotResize` — three concurrent operations

### Edge case tests

* Close lifecycle: `CloseStopsDrainGoroutine`, `DoubleCloseNoPanic`, `ProcessAfterClose`, `SnapshotAfterClose`
* Parser state: `DA1AsFirstBytesEver`, `DA1AfterPartialEscapeSequence`, `DA1SplitAcrossChunks`, `DA1ByteByByte`
* UTF-8: `DA1AfterUTF8Content`, `DA1InterleavedWithUTF8`
* Alt screen: `DA1InAltScreen`, `DA1DuringAltScreenTransition`
* Concurrency: `ConcurrentCloseAndProcess`, `ConcurrentCloseAndSnapshot`
* Boundaries: `SmallTerminalDA1` (1×1), `LargeTerminalDA1` (500×200)

### E2E tests

* `TestE2E_ClaudeCodeOutputPattern` — simulates Claude Code startup with DA1 + TUI rendering
* `TestE2E_VimLikeOutputPattern` — simulates vim with DA1 + DA2 + CPR
* `TestE2E_AttachDetachCycle` — reproduces the production failure: TUI output → detach → reattach → Snapshot must work
* `TestE2E_FullLifecycle` — create → process queries → snapshot → close → post-close safety
* `TestE2E_MultipleEmulatorInstances` — 10 concurrent emulators with independent drains
* `TestE2E_SustainedOperationNoDegradation` — 2000 lines + queries, no performance degradation
* `TestE2E_FactoryCreatedEmulatorDrains` — production code path through `Backend()` factory

### Session layer tests

* `TestEmulatorCloseTypeAssertion` — `io.Closer` type assertion works for closer and non-closer
* `TestWaitLoopClosesEmulator` — cleanup code path calls Close
* `TestWaitLoopDoesNotCloseNonCloser` — non-closer emulators handled gracefully

## Follow-up: addon_emulator has the same deadlock pattern

A post-fix audit found that `internal/session/addon_emulator.go` has the same class of vulnerability. `sendRequestWithResponse()` (line 98) uses `io.ReadFull()` on the addon process stdout **without any timeout**. If the addon process crashes, hangs, or stops responding, the read blocks forever — same cascading deadlock as the vt pipe issue.

Affected call sites:
* `Snapshot()` (line 53) — called from daemon `handleAttach`
* `Resize()` (line 73) — called on terminal dimension changes

Recommended fix: add `context.Context` with timeout to `sendRequestWithResponse()`, or wrap the read in a goroutine with `select` timeout (same pattern as the original investigation's "Fix 1: Snapshot timeout").

## More Information

* **Investigation:** `docs/studies/2026-04-17-emulator-blocking-investigation.md`
* **Extends:** ADR 0024 (Async Emulator Processing) — ADR 0024 prevents slow/panicking Process from blocking PTY reads. This ADR prevents indefinitely-blocking Process from holding the mutex.
* **Upstream issue:** The `io.Pipe` design in `charmbracelet/x/vt` requires consumers to drain the response pipe. This is documented behavior for `io.Pipe` but is easy to miss when using the emulator for rendering-only (no interactive I/O).
