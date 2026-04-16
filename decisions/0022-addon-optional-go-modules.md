---
status: accepted
date: 2026-04-16
decision-makers: wblech
---

# Emulator Addons as Optional Go Modules

## Context and Problem Statement

Using `WithEmulatorBackend("xterm")` required integrators to manually build the xterm addon (`npm install` + `tsc`), ensure Node.js was installed on the target machine, and pass `WithXtermBinPath(path)` to the daemon. This leaked internal implementation details and made the emulator backend effectively unusable without manual setup. Additionally, hardcoding addon-specific options (`WithEmulatorBackend`, `WithXtermBinPath`) in `pkg/client` coupled the SDK to specific emulator implementations.

## Decision Drivers

* Zero-config experience for integrators — one import should be enough
* Addons must be optional — integrators who don't need an emulator shouldn't pay for its dependencies
* The addon architecture must be extensible to future backends (xterm.js, ghostty-vt, etc.)
* No external runtime dependencies for the pure Go backend

## Considered Options

* Embed xterm.js in the main wmux module via go:embed
* Keep addon-specific options in pkg/client, add auto-discovery
* Emulator addons as separate Go modules with EmulatorFactory interface

## Decision Outcome

Chosen option: "Separate Go modules with EmulatorFactory", because it provides true opt-in dependencies (only pulled when imported), keeps pkg/client free of addon-specific knowledge, and enables any number of future backends through a single generic interface.

The first addon is `charmvt` — a pure Go backend wrapping `charmbracelet/x/vt`. It requires no external runtime (no Node.js), runs in-process, and provides terminal state tracking with scrollback.

The integration point is `EmulatorFactory` — an interface in `pkg/client` that addon modules implement. `pkg/client` has no knowledge of specific addons; all wiring happens through this generic interface.

### Consequences

* Good, because `go get` + one import = working emulator
* Good, because binary only includes emulator code if the addon is imported
* Good, because `pkg/client` is decoupled from addon implementations
* Good, because the pattern is extensible to future addons (xtermaddon, ghosttyvt)
* Good, because charmvt has no external runtime dependencies (pure Go)
* Bad, because multi-module repo adds versioning complexity
* Bad, because charmbracelet/x/vt is pre-release (API may change)
* Bad, because scrollback rendering requires custom ANSI serialization (viewport is provided by vt.Render())
