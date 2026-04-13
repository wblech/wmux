---
status: accepted
date: 2026-04-13
decision-makers: wblech
---

# Stable Symlinks for Environment Forwarding

## Context and Problem Statement

When a client re-attaches after an SSH reconnection, environment variables like SSH_AUTH_SOCK point to a new socket path. The shell inside the session still uses the old path. We need to update the session's environment on re-attach.

## Considered Options

* Inject environment variables into the shell process (not portable)
* Write an env file for the shell to source manually
* Create stable symlinks that always point to the current path

## Decision Outcome

Chosen option: "Stable symlinks", because the session shell sees a constant path (`~/.wmux/sessions/{id}/SSH_AUTH_SOCK`) that is a symlink to the real socket. On re-attach, the daemon updates the symlink target without the shell needing to do anything. Non-path values fall back to an env file.

### Consequences

* Good, because transparent to the shell — no sourcing required for socket paths
* Good, because works for SSH agent forwarding, X11, etc.
* Bad, because only works for file/socket paths — other env vars require manual sourcing
