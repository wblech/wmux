---
status: accepted
date: 2026-04-17
decision-makers: wblech
supersedes: 0010
---

# charmvt as the Sole Maintained Emulator Backend

## Context and Problem Statement

ADR 0010 defined an external-process addon framework so the wmux daemon could
stay pure Go while supporting an xterm.js-based headless emulator (Node.js).
Since then, charmvt (an in-process Go VT emulator) was implemented and is the
only backend in production use. Maintaining the addon process framework adds
code, tests, and IPC edge cases without a current consumer.

## Decision

Remove the xterm Node.js addon and the `internal/session/addon_*` framework.
Keep charmvt as the sole maintained emulator backend. Keep the `ScreenEmulator`
interface so future in-process backends can still plug in via
`client.WithEmulatorFactory`.

## Consequences

* Good, because ~1.5k LOC of TypeScript + Go IPC code is deleted.
* Good, because wmux no longer needs Node.js for any optional feature.
* Good, because the binary protocol from ADR 0010 is eliminated (along with
  its duplication-prone two-field snapshot contract — see ADR 0026).
* Bad, because adding a non-Go emulator backend in the future requires
  reintroducing an IPC layer. We accept this; there are no such consumers.
