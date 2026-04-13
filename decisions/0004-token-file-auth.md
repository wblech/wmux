---
status: accepted
date: 2026-04-11
decision-makers: wblech
---

# Token File Authentication

## Context and Problem Statement

Clients connecting to the daemon need authentication to prevent unauthorized access. The mechanism must work without user interaction (no passwords) and integrate with automation security modes (open, same-user, children).

## Considered Options

* Unix socket peer credentials only (SO_PEERCRED)
* Token file at `~/.wmux/daemon.token`
* TLS client certificates

## Decision Outcome

Chosen option: "Token file", because it's simple (32 random bytes, file-permission protected), works across all platforms (including future Windows named pipes), and combines well with peer credential checks for defense in depth.

### Consequences

* Good, because zero user interaction — token is auto-generated on first daemon spawn
* Good, because file permissions restrict access to the owning user
* Bad, because any process that can read the token file can connect (mitigated by automation_mode setting)
