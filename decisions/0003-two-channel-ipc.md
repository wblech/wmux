---
status: amended by [0019](0019-control-channel-events.md)
date: 2026-04-11
decision-makers: wblech
---

# Two-Channel IPC Design (Control + Stream)

## Context and Problem Statement

Mixing control messages (create, kill, resize) with data streams (PTY output) on a single connection causes head-of-line blocking — a large data payload delays time-sensitive control responses. Superset encountered this exact problem.

## Decision Drivers

* Avoid head-of-line blocking between control and data
* Keep the protocol simple (single socket)
* Support independent backpressure on stream channel

## Considered Options

* Single channel with priority flags
* Two separate Unix sockets
* Single socket, two logical channels (control + stream) multiplexed by message type

## Decision Outcome

Chosen option: "Single socket, two logical channels", because it avoids the complexity of managing two sockets while eliminating head-of-line blocking. Control messages are request-response on the control channel; data and events flow on the stream channel with independent backpressure.

### Consequences

* Good, because control latency is unaffected by data throughput
* Good, because single socket simplifies connection management
* Bad, because the transport layer must demux frames to the correct channel
