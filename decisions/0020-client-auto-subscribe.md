---
status: accepted
date: 2026-04-15
decision-makers: wblech
amends: [0012](0012-client-library-pkg.md)
---

# Client Auto-Subscribes to Events on Connect

## Context and Problem Statement

ADR 0012 established the Go client library in `pkg/client/` but did not specify how event subscription works. The daemon requires clients to send a `MsgEvent` frame to activate event forwarding (ADR 0008). Without this, `OnEvent` handlers register locally but never receive events — a bug reported by a DSK integrator.

The question is when and how the client should subscribe: eagerly on connect, lazily on first `OnEvent` call, or explicitly via a public method.

## Decision Drivers

* Events should work out of the box — consumers shouldn't need to know about the subscription protocol
* Subscribe should use the same RPC pattern as other client operations (consistency)
* No events should be lost between connect and the first `OnEvent` registration

## Decision Outcome

The client auto-subscribes to all daemon events during `connect()`, after spawning reader goroutines. The subscribe is a standard RPC: send `MsgEvent` frame, receive `MsgOK` via the `responses` channel (demuxed by `readControl`).

The connection lifecycle is now:

1. Dial control channel, authenticate (`MsgAuth` → `MsgOK`)
2. Dial stream channel, authenticate
3. Spawn `readControl()` and `readStream()` goroutines
4. **Subscribe to events (`MsgEvent` → `MsgOK` via `sendRequest`)** ← new
5. Return ready client

If the subscribe fails, `connect()` tears down and returns an error.

### Consequences

* Good, because events work immediately — no opt-in step for consumers
* Good, because subscribe uses `sendRequest`, consistent with Create/Kill/List RPCs
* Good, because subscribe happens after readers, so the response flows through the standard demux path (no ordering dependency)
* Neutral, because the client subscribes to all events (no server-side filtering by session ID) — volume is low enough that this is not a concern
* Bad, because connect is slightly slower (one extra round-trip) — negligible for a Unix socket
