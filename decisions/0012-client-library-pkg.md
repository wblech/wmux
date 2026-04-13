---
status: accepted
date: 2026-04-13
decision-makers: wblech
---

# Go Client Library in pkg/client/

## Context and Problem Statement

Watchtower and other Go applications need a convenient way to interact with the wmux daemon without speaking the binary protocol directly. We need a client library that abstracts the protocol.

## Considered Options

* Client library in a separate repository
* Client library inside wmux repo at `pkg/client/`
* No client library (consumers speak raw protocol)

## Decision Outcome

Chosen option: "Inside wmux repo at `pkg/client/`", because it evolves alongside the daemon (protocol changes are updated atomically), consumers import it as a Go module dependency, and `pkg/` is the standard Go convention for public packages.

### Consequences

* Good, because protocol and client stay in sync
* Good, because single `go get` adds the dependency
* Bad, because consumers pull the entire wmux module (acceptable for Go applications)
