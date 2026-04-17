---
status: accepted
date: 2026-04-16
decision-makers: wblech
---

# Async Emulator Processing in readLoop

## Context and Problem Statement

The `readLoop` goroutine reads PTY output and feeds it to both the **batcher** (for real-time broadcast to clients) and the **ScreenEmulator** (for snapshot capture). Before this change, `emulator.Process(chunk)` ran synchronously inside the readLoop. If the emulator blocked or panicked on certain escape sequences, the readLoop goroutine would stall or die, preventing all further PTY reads.

This caused a **complete terminal deadlock** for TUI applications:

1. TUI app (vim, Claude Code, htop) writes complex escape sequences to stdout
2. `emulator.Process()` blocks or panics on non-standard sequences (DCS `\x1bPzz\x1b\\`, extended SGR `\x1b[0%m`, stray bytes)
3. readLoop stops reading from PTY master
4. PTY kernel buffer fills up (default 64KB on macOS)
5. TUI app's `write()` to stdout blocks (backpressure)
6. **Deadlock**: app can't write, daemon can't read

This affected every TUI application, not just specific ones. The charmbracelet/x/vt emulator does not handle all VT100/xterm sequences, and real-world apps emit many non-standard sequences.

## Decision Drivers

* TUI apps (vim, Claude Code, Ink-based apps) must work reliably in wmux terminals
* PTY reads must never be blocked by emulator processing
* Emulator panics must not kill the readLoop goroutine
* Broadcast (real-time output) is higher priority than snapshot accuracy
* The fix must not break existing behavior for simple shell usage

## Considered Options

1. **Run emulator.Process() in a separate goroutine with buffered channel**
2. Fix every sequence in charmbracelet/x/vt that could block
3. Add a timeout to emulator.Process() with context cancellation
4. Remove the emulator entirely and use a different snapshot mechanism

## Decision Outcome

**Option 1: Async goroutine with buffered channel.**

### Implementation

```go
func (s *Service) readLoop(ms *managedSession) {
    emulatorCh := make(chan []byte, 64)
    go s.emulatorLoop(ms, emulatorCh)

    for {
        n, err := ms.process.Read(buf)
        if n > 0 {
            ms.batcher.Add(chunk)      // broadcast — always immediate
            select {
            case emulatorCh <- chunk:  // emulator — async, non-blocking
            default:                   // channel full — skip this chunk
            }
        }
    }
}

func (s *Service) emulatorLoop(ms *managedSession, ch <-chan []byte) {
    for chunk := range ch {
        defer recover()  // panics don't kill the goroutine
        ms.emulator.Process(chunk)
    }
}
```

### Key properties

- **readLoop never blocks on emulator**: `batcher.Add()` is called first, then emulator via non-blocking channel send
- **Panic isolation**: `recover()` in emulatorLoop catches panics and logs them; subsequent chunks continue processing
- **Ordered processing**: channel preserves chunk order for the emulator
- **Graceful degradation**: if the emulator channel is full (64 slots), the chunk is dropped for the emulator only — broadcast is unaffected, and snapshot accuracy self-heals on the next chunk
- **Clean shutdown**: `close(emulatorCh)` on PTY EOF drains remaining chunks

### Why not the other options

- **Option 2** (fix charmbracelet/x/vt): Whack-a-mole — new apps will always emit sequences the emulator doesn't handle. The async approach is defensive regardless of emulator quality.
- **Option 3** (timeout): Adds complexity with context plumbing. Doesn't handle panics. A blocked Process() that eventually returns would still delay subsequent chunks.
- **Option 4** (remove emulator): Loses snapshot capability, which is a core wmux feature for session resume.

## Consequences

### Positive

- All TUI apps work: vim, neovim, Claude Code, htop, less, man, etc.
- readLoop can never deadlock due to emulator issues
- No behavioral change for simple shell usage (emulator processes fast enough to keep up)

### Negative

- Snapshot may lag behind real-time output by up to 64 chunks during heavy output bursts
- If emulator repeatedly panics, snapshot becomes stale (but terminal output is unaffected)
- Slightly higher memory usage from the channel buffer (~2MB worst case at 64 × 32KB chunks)

### Regression tests

- `TestReadLoop_EmulatorSlowDoesNotBlockBroadcast`: verifies broadcast data arrives within ~30ms even when emulator takes 500ms per chunk
- `TestReadLoop_EmulatorPanicDoesNotKillReadLoop`: verifies all output lines are broadcast even when every Process() call panics
