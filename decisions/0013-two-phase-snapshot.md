---
status: superseded
date: 2026-04-13
decision-makers: wblech
superseded-by: 0026
---

# Two-Phase Warm Attach Snapshot

## Context and Problem Statement

When a client attaches to an existing session, it needs the current terminal state to render immediately. The xterm headless emulator maintains this state. We need a way to deliver it to the client at attach time.

## Decision Drivers

* Instant visual restore on workspace switch in Watchtower
* Pixel-perfect with xterm.js frontend (zero divergence)
* No ongoing overhead when not attaching

## Considered Options

* Send only SIGWINCH (shell redraws — current Phase 1 behavior with `none` backend)
* Single-phase: send entire terminal buffer as one blob
* Two-phase: scrollback (history) + viewport (current screen) as separate payloads

## Decision Outcome

Chosen option: "Two-phase snapshot", because Watchtower's xterm.js can load scrollback into history and viewport into the active buffer separately, enabling immediate rendering. The daemon requests the snapshot from the addon only at attach time (synchronous), so there's no ongoing overhead.

### Consequences

* Good, because client can render scrollback and viewport independently
* Good, because snapshot is only computed on demand (attach)
* Bad, because snapshot of a busy terminal can be large (hundreds of KB) — acceptable for one-time attach

## Superseded by 0026

xterm.js has no public API to load scrollback separately; the two-phase split produced a fragile ordering contract. See 0026.
