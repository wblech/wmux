---
status: accepted
date: 2026-04-13
decision-makers: wblech
---

# Single Node Process for N xterm Headless Instances

## Context and Problem Statement

Each terminal session needs an xterm headless instance. Running one Node process per session would consume ~30-50MB each, leading to ~3GB for 100 sessions. We need an efficient approach.

## Decision Drivers

* Memory efficiency: typical 10-30 sessions, worst case 100
* Instant snapshot availability (no lazy processing)
* Acceptable crash blast radius

## Considered Options

* One Node process per session (~30MB × N)
* Single Node process managing N instances (~50MB base + ~2MB × N)
* Lazy processing: process only on snapshot request

## Decision Outcome

Chosen option: "Single Node process with N instances", because memory usage drops from ~3GB to ~250MB for 100 sessions. The xterm headless library is lightweight (~2MB per instance). The emulator runs continuously so snapshots are always instantly available for warm attach.

### Consequences

* Good, because ~110MB typical memory vs ~900MB with per-session processes
* Good, because snapshots are always ready (no processing delay on attach)
* Bad, because if the Node process crashes, all sessions lose their emulator temporarily (mitigated by crash recovery: respawn + re-feed buffered output)
