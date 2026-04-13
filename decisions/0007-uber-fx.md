---
status: accepted
date: 2026-04-11
decision-makers: wblech
---

# Use uber/fx for Dependency Injection

## Context and Problem Statement

wmux has multiple packages that need to be wired together (config, session, transport, daemon). Manual wiring in main() gets complex as the project grows.

## Considered Options

* Manual wiring in main()
* uber/fx dependency injection
* google/wire compile-time DI

## Decision Outcome

Chosen option: "uber/fx", because each domain package exposes a `Module = fx.Options(fx.Provide(...))` that declares its dependencies, and fx resolves the graph at startup. This integrates well with the goframe convention of `module.go` per package.

### Consequences

* Good, because adding new packages is declarative — just add to Module
* Good, because fx detects missing dependencies at startup, not runtime
* Bad, because fx uses reflection, which can make debugging harder
