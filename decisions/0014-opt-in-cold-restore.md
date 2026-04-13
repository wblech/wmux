---
status: accepted
date: 2026-04-13
decision-makers: wblech
---

# Opt-In Cold Restore

## Context and Problem Statement

Cold restore allows session recovery after a reboot by persisting scrollback and metadata to disk. However, not all users need this, and continuous disk writes have a performance cost.

## Considered Options

* Always-on cold restore (write history for every session)
* Opt-in via config (`history.cold_restore = true`)
* Per-session opt-in

## Decision Outcome

Chosen option: "Opt-in via config", because it gives users a simple global toggle. When disabled (default), the daemon writes zero history files — no disk I/O overhead. When enabled, the existing `history/` package handles persistence. Per-session granularity was considered unnecessary complexity.

### Consequences

* Good, because default is zero disk overhead
* Good, because existing history package already handles the write path
* Bad, because users who want cold restore must explicitly enable it
