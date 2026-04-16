---
status: accepted
date: 2026-04-16
decision-makers: wblech
---

# Runtime Scrollback Configuration via Optional Interface

## Context and Problem Statement

Emulator addon configuration (scrollback size, callbacks, logger) was fixed at daemon startup time. Changing the scrollback buffer size of a running session required stopping the daemon and creating a new one. This prevented integrators from offering per-session or dynamic scrollback controls.

## Decision Drivers

* Integrators need to change scrollback size on live sessions without restart
* Not all emulator backends support runtime configuration (e.g., NoneEmulator)
* The ScreenEmulator interface should remain minimal and stable
* New capabilities should not break existing addon implementations

## Considered Options

* Add SetScrollbackSize to the ScreenEmulator interface
* Optional interface via type assertion (ScrollbackConfigurable)
* Configuration method on EmulatorFactory

## Decision Outcome

Chosen option: "Optional interface via type assertion", because it keeps ScreenEmulator minimal, does not force addons to implement methods they cannot support, and follows the Go idiom of capability detection (like io.ReadCloser vs io.Reader).

The session service checks at runtime whether the emulator implements ScrollbackConfigurable:

```go
type ScrollbackConfigurable interface {
    SetScrollbackSize(lines int)
}

cfg, ok := ms.emulator.(ScrollbackConfigurable)
if !ok {
    return ErrScrollbackNotConfigurable
}
cfg.SetScrollbackSize(scrollbackLines)
```

A new RPC (MsgUpdateEmulatorScrollback) exposes this through the full stack: client -> daemon -> session service -> emulator.

### Consequences

* Good, because existing addons (NoneEmulator, AddonEmulator) are unaffected
* Good, because the pattern is extensible to future runtime configuration (colors, callbacks)
* Good, because the error is explicit when the emulator does not support it
* Bad, because type assertions bypass compile-time interface checks
* Bad, because the adapter layer must forward the optional interface (screenEmulatorAdapter delegates SetScrollbackSize to the inner emulator)
