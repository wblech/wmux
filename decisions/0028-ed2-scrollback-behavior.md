---
status: accepted
date: 2026-04-17
decision-makers: wblech
---

# ED2 Scrollback Behavior and Snapshot Filtering

## Context and Problem Statement

The upstream `charmbracelet/x/vt` emulator pushes the current viewport into
scrollback when processing ED2 (`\x1b[2J`), matching VTE/GNOME Terminal
behavior. This causes two problems for terminal multiplexers that serialize
scrollback + viewport into a replay stream:

1. **Duplication** — a TUI that redraws via ED2 has its UI appear twice in the
   replay (once from the ED2 push, once from the viewport).
2. **Stale content** — shell output from before the TUI started persists in
   scrollback and appears above the TUI's banner on reconnect.

The DEC STD 070 specification does not define scrollback interaction for ED2.
xterm and Ghostty do not push viewport to scrollback on ED2. There is no
industry consensus.

## Decision

### Fork `charmbracelet/x/vt`

Maintain a fork at `github.com/wblech/x` (branch `wmux-patches`) with:

- `SetED2SavesScrollback(bool)` — option to disable the viewport push on ED2.
  Default `true` (backward-compatible). charmvt sets it to `false`.
- Scrollback buffer pooling (cherry-picked from upstream PR #822).

### Configurable snapshot scrollback mode

Add `WithSnapshotScrollbackMode(mode)` to charmvt with two modes:

- `SnapshotScrollbackAll` (default) — include entire scrollback. No behavior
  change for existing consumers.
- `SnapshotScrollbackSinceLastClear` — track a baseline at each main-screen ED2.
  Snapshot renders only scrollback lines added after the baseline, excluding
  stale pre-TUI content.

## Consequences

- Good, because the duplication bug is eliminated at the source (no ED2 push).
- Good, because stale scrollback filtering is opt-in and backward-compatible.
- Good, because TUI scroll-off (e.g., Claude Code output) between ED2 redraws
  is preserved in the scrollback.
- Bad, because we maintain a fork of `charmbracelet/x/vt`. The fork is minimal
  (~10 lines of production code) and the option can be upstreamed.
- Bad, because `SinceLastClear` mode has a minor imprecision on scrollback
  buffer rollover: up to N old TUI lines are lost (where N = pre-TUI shell
  scrollback lines, typically <100 out of 10000).
