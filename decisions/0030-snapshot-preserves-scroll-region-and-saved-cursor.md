---
status: accepted
date: 2026-04-21
decision-makers: wblech
---

# Snapshot Preserves Scroll Region, Saved Cursor, and Scrollback Count

## Context and Problem Statement

`charmvt.Snapshot()` produces a byte stream intended to reconstruct the current
emulator state in any VT-compliant consumer (see [ADR-0013](0013-two-phase-snapshot.md)
and [ADR-0026](0026-unified-replay-snapshot.md)). A round-trip test suite
(feed writes into charmvt, take snapshot, feed snapshot into a fresh
`@xterm/headless`, compare states) identified three scenarios where the
replay produced a final state that diverged from charmvt's ground truth:

1. **Scrollback off-by-one.** With N scrollback lines present, the replayed
   viewport ended up shifted +1 row: the last serialized scrollback line
   remained visible at the top of the viewport instead of scrolling off.
   Root cause: `trimTrailingEmptyRows` removed the viewport's trailing empty
   row, leaving one fewer `\n` in the rendered viewport. Combined with the N
   scrollback lines written ahead of it, the replay triggered only N-1 scroll
   events instead of N.

2. **DECSTBM scroll region lost.** When a scroll region was active
   (e.g. `ESC[5;20r`, typical of TUIs that pin a status bar), the snapshot
   never re-emitted the region setter. After replay, subsequent `\n` bursts
   scrolled the full screen instead of the region, causing cursor divergence
   on the next LF-heavy chunk.

3. **DECSC saved cursor lost.** When a tab had a saved cursor from a prior
   `ESC 7`, the snapshot did not emit `ESC 7` at the saved position during
   replay. A subsequent `ESC 8` in the live stream was a no-op on the
   replayed emulator — there was nothing to restore to.

The upstream `x/vt.Emulator` exposed neither scroll region nor saved cursor
via its public API, so charmvt could not read these fields to re-emit them.

## Decision

### Add two public getters on `x/vt.Emulator` (wblech fork)

On branch `wmux-patches`:

- `Emulator.ScrollRegion() (top, bottom uint16, defined bool)` — reads the
  internal scroll region state maintained by the CSI `r` handler.
- `Emulator.SavedCursor() (x, y int, defined bool)` — reads the saved-cursor
  buffer maintained by the DECSC/DECRC handlers.

Both are pure reads of state the parser already tracks. No new internal
state, no synchronization concerns.

### Re-emit preserved state in `charmvt.Snapshot()`

After writing scrollback + viewport, before the final live-cursor CUP:

- If `term.ScrollRegion()` returns `defined=true`, write `ESC[<top+1>;<bottom+1> r`.
- If `term.SavedCursor()` returns `defined=true`, write the CUP to the saved
  position followed by `ESC 7` (DECSC). Only the cursor position is captured;
  the full DECSC state (SGR attributes, character set, origin mode) is not
  restored — a limitation accepted pending a real regression requiring it.

### Fix the scrollback off-by-one

Skip `trimTrailingEmptyRows` on the viewport when any scrollback line was
serialized ahead of it. The number of `\n` in the viewport must match the
number of scroll events needed to clear all serialized scrollback from the
replayed viewport.

## Consequences

- Good, because `Snapshot()` is now a true authoritative serializer — a
  consumer that writes `Replay` into a fresh VT-compliant emulator gets a
  buffer state equal to charmvt's at snapshot time, across all tested
  scenarios (alt-screen, primary+scrollback, DECSTBM, DECSC, pending SGR,
  mid-stream resize).
- Good, because the two new `x/vt` getters are useful to any consumer of
  `x/vt`, not just charmvt or wmux. Non-multiplexer applications (testing
  harnesses, debugging tools) can inspect scroll region and saved cursor
  without reimplementing the parser.
- Good, because the round-trip test suite at
  `internal/platform/terminal/snapshot_roundtrip_test.go` in the watchtower
  consumer provides a regression gate; any future charmvt change that breaks
  a scenario surfaces there.
- Bad, because the fork of `charmbracelet/x/vt` grows by two getters. Still
  upstreamable as a standalone contribution (pure read-only accessors); no
  behavioral change.
- Bad, because the DECSC re-emit only preserves cursor position, not the full
  DECSC-saved state (SGR, charset, origin mode). TUIs relying on `ESC 8` to
  restore those would still diverge after replay. No such regression has been
  observed with current known TUIs (Claude Code, vim, less, man) but the gap
  is documented for future work.
- Bad, because the off-by-one fix depends on `trimTrailingEmptyRows` running
  before the scrollback count is known. A future refactor of the render
  pipeline that reorders these could regress without failing existing unit
  tests inside charmvt — the round-trip test on the consumer side is the
  safety net.

## Related

- [ADR-0013](0013-two-phase-snapshot.md) — snapshot timing protocol.
- [ADR-0026](0026-unified-replay-snapshot.md) — Replay bytes format.
- [ADR-0028](0028-ed2-scrollback-behavior.md) — prior charmvt scrollback fix
  (ED2 viewport push) that shared the same fork-and-forward pattern.

Commits: `253552c` (scrollback), `251f11d` (DECSTBM), `d747cec` (DECSC) in
`wblech/wmux:main`. `486576d` (ScrollRegion getter), `70614a7` (SavedCursor
getter) in `wblech/x:wmux-patches`.
