---
status: accepted
date: 2026-04-15
decision-makers: wblech
amends: [0003](0003-two-channel-ipc.md)
---

# Events Flow on Control Channel, Not Stream

## Context and Problem Statement

ADR 0003 states that "data and events flow on the stream channel." During implementation of the client stream channel, we found that daemon events (`MsgEvent`) are actually sent on the **control channel**, not the stream channel. The stream channel carries only `MsgData` (PTY output). This amendment corrects the record.

## Decision Drivers

* Events are low-frequency, request-initiated (client subscribes via `MsgEvent` on control, daemon pushes events back on the same channel)
* Stream channel is optimized for high-throughput unidirectional PTY data
* Mixing events into the stream channel would require the client to demux two frame types on stream, adding complexity without benefit

## Decision Outcome

Events (`MsgEvent`) flow on the **control channel**. The stream channel carries only `MsgData` frames. This amends ADR 0003's description.

The channel responsibilities are:

* **Control channel**: request-response RPCs + server-pushed events (`MsgEvent`)
* **Stream channel**: unidirectional PTY output (`MsgData`)

The client demuxes the control channel: `MsgEvent` frames are dispatched to the event handler, all other frames are treated as RPC responses.

### Consequences

* Good, because the stream reader loop is trivial (only `MsgData`)
* Good, because event subscription is a natural RPC on the control channel
* Neutral, because the control channel now carries two traffic types (RPC responses + pushed events), requiring a demux goroutine in the client
