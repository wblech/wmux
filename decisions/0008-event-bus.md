---
status: accepted
date: 2026-04-12
decision-makers: wblech
---

# In-Process Event Bus with Fan-Out and Type Filtering

## Context and Problem Statement

The daemon needs to emit lifecycle events (session created, exited, etc.) that clients can subscribe to. The event system must be non-blocking for the publisher and support multiple subscribers with type-based filtering.

## Considered Options

* Direct callback registration
* Channel-based fan-out event bus
* External message broker (NATS, Redis pub/sub)

## Decision Outcome

Chosen option: "Channel-based fan-out event bus", because it's in-process (no external dependencies), non-blocking for publishers (events are sent to buffered channels per subscriber, 256 buffer), and supports multiple subscribers. Subscribers receive events on a channel and can filter by type.

### Consequences

* Good, because publishing never blocks the daemon's main loop
* Good, because subscriber disconnection is handled via Unsubscribe()
* Bad, because if a subscriber's buffer fills, events are dropped (acceptable trade-off for non-blocking)
