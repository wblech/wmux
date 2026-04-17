# Investigation: Emulator Process() Blocks on TUI App Output

**Date:** 2026-04-17
**Status:** Root cause confirmed, fix validated in debug build
**Extends:** ADR 0024 (Async Emulator Processing)
**Related:** watchtower study `2026-04-16-debug-wmux-attach-claude-terminal.md`

---

## Problem Statement

When Watchtower is closed and reopened while Claude Code (or other TUI apps) is running, **all terminals show blank** — including bash terminals that have nothing to do with the TUI app.

---

## Investigation Timeline

### Phase 1: Identify where the data flow breaks

**Hypothesis:** The snapshot from the daemon is empty or the frontend isn't writing it.

**Test:** Added diagnostic logging to Watchtower's `ConnectTerminal` (Go backend) and `useTerminal` hook (frontend). Backend writes to `/tmp/watchtower-connect.log`, frontend sends logs via Wails event to the same file.

**Code added (app.go — ConnectTerminal):**
```go
debugLog("[CONNECT] ATTACH session=%s scrollback=%d viewport=%d preview=%q", ...)
debugLog("[CONNECT] CREATE session=%s attach_err=%v", ...)
```

**Code added (useTerminal.ts — effect):**
```typescript
EventsEmit('debug:log', `effect fired tabID=${tabID} container=${!!container} settings=${!!settingsRef.current}`);
EventsEmit('debug:log', `calling connectTerminal tabID=${tabID} cols=${cols} rows=${rows}`);
```

**Result — test with echo hello only (no Claude):**
```
[CONNECT] ATTACH session=233128ac scrollback=0 viewport=286 preview="\r\n\x1b[36;1mwatchtower\x1b[m ... echo hello\r\nhello..."
[CONNECT] ATTACH session=c6311bc7 scrollback=0 viewport=133 preview="\r\n\x1b[36;1mwatchtower\x1b[m ... ➜\x1b[m"
```
Snapshot works. Bash terminals restore correctly on reopen.

**Result — test with Claude Code running:**
```
[FRONTEND] calling connectTerminal tabID=d38294d7 cols=71 rows=36
[FRONTEND] calling connectTerminal tabID=c1c75b8c cols=52 rows=36
[CONNECT] CREATE session=d38294d7 attach_err=client: request timeout
```

The frontend calls `connectTerminal`, but the Go backend **never logs a response** — the daemon times out. `ConnectTerminal` falls through to CREATE (new session) which also times out.

**Conclusion:** The problem is NOT in the frontend or snapshot decoding. The daemon itself is unresponsive.

---

### Phase 2: Identify why the daemon is unresponsive

**Hypothesis:** The daemon's `handleAttach` blocks on `Snapshot()` because the emulator mutex is held.

**Test:** Added snapshot timeout in `handleAttach` (`/tmp/wmux-debug/internal/daemon/service.go`):

```go
snapCh := make(chan snapResult, 1)
go func() {
    s, e := d.sessionSvc.Snapshot(req.SessionID)
    snapCh <- snapResult{snap: s, err: e}
}()

select {
case sr := <-snapCh:
    // use snapshot
case <-time.After(2 * time.Second):
    logErr("attach snapshot timeout", ...)
}
```

**Result:**
```
[DEBUG handleAttach] sessionSvc.Snapshot TIMEOUT — emulator mutex likely blocked elapsed=2.002s
wmux: attach snapshot timeout: session d38294d7: emulator mutex blocked by Process()
```

After the timeout, the daemon responded to the next requests:
```
[CONNECT] ATTACH session=d38294d7 scrollback=0 viewport=0     ← Claude: empty but no freeze
[CONNECT] ATTACH session=c1c75b8c scrollback=0 viewport=145   ← Bash: restored! ✓
```

**Conclusion:** `Snapshot()` blocks because the emulator mutex is held by a stuck `Process()`. The timeout fix keeps the daemon responsive, but the Claude terminal has empty viewport.

---

### Phase 3: Determine why viewport is empty — channel drops vs. stuck Process()

**Hypothesis A:** The async emulator channel (64 slots) is full and chunks are being dropped, so the emulator never renders Claude's output.

**Test:** Added drop counter to `readLoop` (`/tmp/wmux-debug/internal/session/service.go`):

```go
select {
case emulatorCh <- chunk:
    emSent++
default:
    emDropped++
    fmt.Fprintf(os.Stderr, "[DEBUG readLoop] DROPPED chunk len=%d (sent=%d dropped=%d)\n", ...)
}
```

**Result:**
```
grep "DROPPED" /tmp/wmux-daemon-debug.log → (empty)
```

**No chunks dropped.** The channel has room — the problem is not overflow.

**Hypothesis B:** `Process()` is blocking on the first chunk and never returning, so no subsequent chunks are processed.

**Test:** Added timing log inside `emulatorLoop`:

```go
t0 := time.Now()
ms.emulator.Process(chunk)
elapsed := time.Since(t0)
if elapsed > 100*time.Millisecond {
    fmt.Fprintf(os.Stderr, "[DEBUG emulatorLoop] Process() took %v\n", ...)
}
```

**Result:**
```
grep "emulatorLoop" /tmp/wmux-daemon-debug.log → (empty)
```

The log after `Process()` **never appears**. `Process()` is called but never returns.

**Conclusion:** `Process()` enters `charmbracelet/x/vt`'s `Write()` and gets stuck in an infinite loop. Not a panic (the `recover()` from ADR 0024 doesn't trigger), not slow processing — a genuine infinite loop in the parser.

---

### Phase 4: Validate the emulator replacement fix

**Hypothesis:** If we detect the blocked `Process()` (timeout) and replace the emulator with a fresh one, the new emulator can process subsequent chunks normally.

**Test:** Implemented atomic pointer emulator replacement in charmvt (`/tmp/charmvt-debug/emulator.go`):

```go
type emulator struct {
    inner     atomic.Pointer[emulatorInner]  // swappable
    cols, rows int
    sessionID  string
    cfg        *config
}

func (e *emulator) Process(data []byte) {
    in := e.inner.Load()
    done := make(chan struct{})
    go func() {
        in.mu.Lock()
        defer in.mu.Unlock()
        in.term.Write(data)
        close(done)
    }()
    select {
    case <-done:
        // OK
    case <-time.After(5 * time.Second):
        // Stuck — replace emulator
        e.inner.Store(e.createInner(e.cols, e.rows))
    }
}
```

**Result:**
```
wmux: charmvt: Process() stuck for 5s, replacing emulator (session=d38294d7, dataLen=3)
[CONNECT] ATTACH session=d38294d7 scrollback=0 viewport=1350 preview=" Claude Code╭─── Claude Code v2.1.112 ──"
[CONNECT] ATTACH session=c1c75b8c scrollback=0 viewport=145  preview="watchtower on main..."
```

Both terminals restored:
- **Claude terminal**: viewport=1350 bytes with actual Claude Code TUI content ✓
- **Bash terminal**: viewport=145 bytes with shell prompt ✓
- **Daemon responsive**: no freezing, no timeouts ✓

The blocking chunk was **3 bytes** (`dataLen=3`). After the emulator was replaced, the new emulator processed Claude Code's subsequent output (TUI redraws) normally.

**Conclusion:** The fix works. The emulator replacement approach is viable.

---

## Root Cause Summary

> **UPDATE (2026-04-17):** The original hypothesis was "infinite loop in the vt parser." Further analysis with evidence tests proved this was **incorrect**. The actual root cause is an **io.Pipe deadlock** — see Phase 5 below. The production fix is documented in ADR 0025.

```
TUI app emits DA1 query (\x1b[c, 3 bytes) as part of terminal capability detection
  → vt parser dispatches CSI 'c' handler (DA1 — Primary Device Attributes)
    → handler calls io.WriteString(e.pw, DA1 response) to internal io.Pipe
      → io.Pipe.Write() BLOCKS because nobody calls Read() on the other end
        → emulator.Process() never returns (not a loop — a pipe deadlock)
          → emulator mutex held forever
            → Snapshot() blocks forever (same mutex)
              → daemon handleAttach blocks forever
                → ALL client RPC requests queue up and timeout
                  → ALL terminals show blank on app reopen
```

The bug is in **charmvt** (not draining the vt response pipe), not in the upstream `charmbracelet/x/vt` library. The vt library is correct — terminal emulators are expected to respond to DA1/DA2/DSR/CPR queries. The charmvt wrapper used vt as write-only (Process + Snapshot) without consuming the response pipe.

The 3-byte blocking chunk from Phase 3 is `\x1b[c` (DA1 query). The same deadlock occurs with DA2 (`\x1b[>c`), DSR CPR (`\x1b[6n`), DSR Status (`\x1b[5n`), and DECXCPR (`\x1b[?6n`).

---

### Phase 5: Corrected root cause — io.Pipe deadlock (not parser loop)

**Hypothesis:** The blocking is caused by an undrained `io.Pipe`, not a parser infinite loop.

**Evidence:** The vt `Write()` method iterates `for i := range p` — bounded by `len(p)`. Each `parser.Advance()` call is O(1) via state machine lookup. There is no loop that could run forever. However, the DA1 CSI handler writes a response to `e.pw` (an `io.PipeWriter`), and `io.Pipe` is synchronous — `Write()` blocks until `Read()` is called.

**Test:** Created evidence tests in `addons/charmvt/pipe_deadlock_evidence_test.go`:

| Test | Result | Proves |
|------|--------|--------|
| `TestEvidence_DA1_Blocks` | PASS | `\x1b[c` blocks vt.Write() without a pipe reader |
| `TestEvidence_DrainedPipe_Unblocks` | PASS (0ms) | With a reader goroutine, all sequences complete instantly |
| `TestEvidence_PipeResponse_Content` | PASS | DA1 response is valid VT220: `\x1b[?62;1;6;22c` |

**Conclusion:** Not a parser bug. The vt library correctly responds to terminal queries via its internal pipe. The charmvt wrapper never drains the pipe, causing `Write()` to deadlock on any response-generating escape sequence.

---

## Production Fix (ADR 0025)

The debug builds (Fix 1 and Fix 2 below) validated that the emulator was stuck, but the proposed emulator replacement approach was **unnecessary** once the true root cause was identified.

**Actual fix:** A drain goroutine in `newEmulator()` reads and discards all vt response pipe data. The responses are safely discarded because the real terminal (xterm.js in Watchtower) already handles query responses via broadcast. Adding `io.Closer` to the emulator and cleanup in `waitLoop` prevents goroutine leaks.

**Production code:** 15 lines in `addons/charmvt/emulator.go` + 3 lines in `internal/session/service.go`.

**Tests:** 50 tests across 5 files (evidence, regression, edge case, E2E, session layer).

**ADR:** `decisions/0025-drain-emulator-response-pipe.md`

---

## Debug Build Fixes (historical — superseded by ADR 0025)

### Fix 1: Snapshot timeout in `handleAttach` (debug only)
**File:** `/tmp/wmux-debug/internal/daemon/service.go`
**What:** Run `Snapshot()` in goroutine with 2s timeout. On timeout, return attach response without snapshot.
**Effect:** Daemon stays responsive even if emulator is stuck. Useful for diagnosis but unnecessary with drain fix.

### Fix 2: Process timeout + emulator replacement (debug only)
**File:** `/tmp/charmvt-debug/emulator.go`
**What:** Run `vt.Write()` in goroutine with 5s timeout. On timeout, create new `vt.Emulator` via `atomic.Pointer` swap. Old goroutine leaks (~8KB, harmless).
**Effect:** New emulator processes subsequent output normally. Superseded by the simpler drain approach.

---

## Remaining Cleanup

1. ~~**ADR 0025**~~ — Done: `decisions/0025-drain-emulator-response-pipe.md`
2. ~~**Regression tests**~~ — Done: 50 tests in `addons/charmvt/` and `internal/session/`
3. **Cleanup** — remove debug logging from `/tmp/wmux-debug` and `/tmp/charmvt-debug`. Remove diagnostic logging from Watchtower's `app.go` and `useTerminal.ts`.

---

## Key Files Modified During Investigation

### Watchtower (diagnostic only — needs cleanup)
| File | Change | Purpose |
|------|--------|---------|
| `app.go` | `debugLog` in ConnectTerminal | Log attach vs create path, snapshot sizes |
| `app.go` | `EventsOn("debug:log")` in startup | Route frontend logs to file |
| `useTerminal.ts` | `EventsEmit('debug:log')` | Log effect lifecycle and connectTerminal calls |

### wmux debug (`/tmp/wmux-debug`)
| File | Change | Purpose |
|------|--------|---------|
| `internal/daemon/service.go` | Snapshot timeout in handleAttach | Prevent daemon RPC freeze |
| `internal/session/service.go` | Drop counter in readLoop | Verify channel drops (none found) |
| `internal/session/service.go` | Timing log in emulatorLoop | Verify Process() blocking (confirmed) |

### charmvt debug (`/tmp/charmvt-debug`)
| File | Change | Purpose |
|------|--------|---------|
| `emulator.go` | `atomic.Pointer[emulatorInner]` + timeout + replacement | Core fix — validated |

### Design spec
| File | Purpose |
|------|---------|
| `docs/superpowers/specs/2026-04-17-emulator-blocking-resilience-design.md` | Production implementation design |
