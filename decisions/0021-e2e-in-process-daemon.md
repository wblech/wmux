---
status: accepted
date: 2026-04-15
decision-makers: wblech
---

# E2E Tests with In-Process Daemon

## Context and Problem Statement

Unit tests for the client use mock servers that simulate the daemon protocol. This missed a real bug: the client never sent the `MsgEvent` subscribe frame, but mock-based tests never caught it because they bypassed the real daemon dispatch loop. We need an E2E testing strategy that exercises the full client→daemon flow.

## Considered Options

* In-process daemon (real daemon in goroutine, Unix socket in temp dir)
* Subprocess daemon (compile binary, spawn as child process)
* Hybrid (in-process for most, subprocess for smoke tests)

## Decision Outcome

Chosen option: "In-process daemon", because it's fast, debuggable, requires no subprocess management, and covers the real client→daemon flow that caught this bug.

The harness lives in `test/e2e/` and provides two helpers:

* `startTestDaemon(t)` — creates a real `Daemon` with event bus, `transport.Server`, and `session.Service` on a temporary Unix socket. Cleanup via `t.Cleanup`.
* `connectTestClient(t, env)` — creates a real `pkg/client.Client` connected to the test daemon.

The `test/e2e/` package imports both `internal/daemon` (via test adapters) and `pkg/client`, which is why it lives outside both packages. The adapters replicate the pattern from `internal/daemon/service_test.go`.

### Consequences

* Good, because tests run in-process — fast (~0.5s for 5 scenarios), no subprocess lifecycle to manage
* Good, because real daemon dispatch loop, event bus, and session manager are exercised
* Good, because test failures are debuggable with standard Go tooling (breakpoints, race detector)
* Bad, because adapters duplicate code from `internal/daemon/service_test.go` — acceptable since test adapters are intentionally unexported in the daemon package
* Neutral, because this doesn't test the compiled binary — subprocess smoke tests can be added later if needed
