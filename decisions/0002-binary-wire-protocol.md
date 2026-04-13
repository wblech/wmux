---
status: accepted
date: 2026-04-11
decision-makers: wblech
---

# Binary Length-Prefixed Wire Protocol

## Context and Problem Statement

wmux needs a wire protocol between daemon and clients for control messages and data streaming. It must be fast for high-throughput PTY output while supporting structured control messages.

## Decision Drivers

* Zero perceptible latency (PRD: "the daemon must feel like it doesn't exist")
* Support both structured control messages and raw binary data streams
* Simple to implement and debug

## Considered Options

* JSON over newline-delimited streams
* gRPC with protobuf
* Custom binary: `[version:1][type:1][length:4][payload:N]`

## Decision Outcome

Chosen option: "Custom binary frames", because the 6-byte header adds minimal overhead for high-throughput PTY data, the format is trivial to parse in any language, and we avoid external dependencies (gRPC) or encoding overhead (JSON for binary data).

### Consequences

* Good, because sub-millisecond framing overhead for PTY output
* Good, because version byte enables future protocol evolution
* Bad, because debugging requires hex inspection (mitigated by CLI adapter that outputs JSON)
