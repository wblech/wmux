---
status: accepted
date: 2026-04-17
decision-makers: wblech
supersedes: 0013
---

# Unified Replay Snapshot

## Context and Problem Statement

ADR 0013 chose a two-phase snapshot (`Scrollback` + `Viewport` as separate
payloads) based on the expectation that xterm.js could load scrollback into
history and viewport into the active buffer through distinct APIs. In
practice the xterm.js public API only exposes a write path through the VT
parser — `addon-serialize` is write-only and there is no public scrollback
load API. Watchtower therefore wrote both fields as sequential bytes, and
the absence of an explicit clear prefix allowed prior destination state
to leak across workspace switches ("duplicated banner" bug).

## Decision

Replace `Snapshot{Scrollback, Viewport}` with `Snapshot{Replay []byte}`. The
byte stream is self-contained:

* Begins with `\e[2J\e[H\e[3J` — clears the visible area, homes the cursor,
  and clears the scrollback buffer. This makes the replay idempotent against
  any prior destination state.
* Contains the scrollback cells followed by the viewport cells in order.
* Ends with a `CUP` sequence restoring the source cursor position.

Consumers write `Replay` as-is — no ordering contract, no metadata, no
post-processing.

## Consequences

* Good, because the replay is idempotent by construction — the bug that
  triggered this ADR cannot reoccur via this path.
* Good, because the API surface shrinks to one field.
* Good, because there is no consumer-side ordering contract to document or
  maintain.
* Bad, because separating scrollback from viewport at the API layer is
  possible only by re-parsing the stream. No current consumer needs this,
  and the wmux emulator still tracks them separately internally.

## Addendum (2026-04-17)

The Replay stream content depends on the charmvt scrollback mode. See
ADR 0028 for details on `SnapshotScrollbackSinceLastClear`, which excludes
pre-ED2 scrollback from the Replay to prevent stale shell content from
appearing above TUI banners on reconnect.
