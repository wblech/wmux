---
status: accepted
date: 2026-04-13
decision-makers: wblech
---

# Single-Client Per Session (Multi-Client Deferred)

## Context and Problem Statement

The PRD defines multi-client support with broadcast, leader control, and capabilities. However, Watchtower's primary use case is single-user with one client per session. Multi-client is needed for pair programming, which is not an immediate priority.

## Decision Drivers

* Simplify Phase 2 scope
* Watchtower operates as single-client per session
* Multi-client infrastructure is significant complexity (broadcast, leader, capabilities, resize strategy)

## Considered Options

* Full multi-client in Phase 2
* Single-client in Phase 2, multi-client deferred
* Basic multi-client (broadcast only, no leader/capabilities)

## Decision Outcome

Chosen option: "Single-client, multi-client deferred", because it significantly reduces Phase 2 scope while covering Watchtower's actual use case. The existing attachment tracking in the daemon already handles one client per session. Multi-client with pair programming will be added when that use case becomes a priority.

### Consequences

* Good, because Phase 2 scope is focused and achievable
* Good, because the daemon architecture doesn't preclude multi-client later (attachment maps already support N clients)
* Bad, because pair programming use case is blocked until multi-client is implemented
