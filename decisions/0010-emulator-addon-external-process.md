---
status: superseded
date: 2026-04-13
decision-makers: wblech
superseded-by: 0027
---

# Emulator Addon as External Process with Binary Protocol

## Context and Problem Statement

The xterm headless emulator requires Node.js runtime. Embedding Node in the Go daemon would add a mandatory dependency. We need a way to keep the daemon pure Go while supporting xterm headless.

## Decision Drivers

* Daemon must remain pure Go with zero Node dependency
* Emulator backend must be pluggable (not just xterm)
* Performance overhead must be minimal

## Considered Options

* Embed Node via cgo bindings
* External process with JSON-RPC over stdin/stdout
* External process with binary protocol over stdin/stdout

## Decision Outcome

Chosen option: "External process with binary protocol", because it keeps the daemon pure Go, the binary protocol avoids JSON/base64 encoding overhead (~33% data inflation eliminated vs JSON), and the addon architecture is generic enough for any emulator backend.

### Consequences

* Good, because daemon has zero Node dependency
* Good, because binary protocol matches the daemon's existing wire format style
* Good, because addons can be written in any language
* Bad, because IPC adds latency vs in-process calls (mitigated by fire-and-forget for process, only snapshot is synchronous)

## Superseded by 0027

xterm addon removed; charmvt is the sole maintained backend.
