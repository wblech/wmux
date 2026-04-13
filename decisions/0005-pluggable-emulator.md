---
status: accepted
date: 2026-04-11
decision-makers: wblech
---

# Pluggable Headless Emulator Interface

## Context and Problem Statement

To provide warm attach with visual state restore, the daemon needs to maintain a headless terminal emulator per session. Different integrators need different backends: xterm.js users want @xterm/headless (zero divergence), others may want ghostty-vt, and lightweight/embedded use cases want no emulator at all.

## Considered Options

* Hardcode xterm headless as the only emulator
* Pluggable ScreenEmulator interface with multiple backends

## Decision Outcome

Chosen option: "Pluggable ScreenEmulator interface", because it decouples the daemon from any specific emulator implementation. The interface has three methods: `Process([]byte)`, `Snapshot() Snapshot`, `Resize(cols, rows)`. Phase 1 ships with `NoneEmulator` (no-op). Phase 2 adds the xterm addon backend.

### Consequences

* Good, because integrators choose the emulator that matches their frontend
* Good, because `none` backend keeps the daemon lightweight when restore isn't needed
* Bad, because each backend has different fidelity characteristics
