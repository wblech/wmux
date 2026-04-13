---
status: accepted
date: 2026-04-12
decision-makers: wblech
---

# Autodaemonize with PID File and Heartbeat

## Context and Problem Statement

CLI commands like `wmux create` need a running daemon. Users shouldn't have to manually start/stop the daemon. The daemon needs to be auto-started on first use and discovered by subsequent CLI commands.

## Considered Options

* Require manual `wmux daemon start`
* Auto-fork daemon on first CLI command
* Systemd/launchd service (Phase 4)

## Decision Outcome

Chosen option: "Auto-fork daemon", because it provides zero-config startup. The CLI checks if a daemon is running (via PID file at `~/.wmux/daemon.pid`), and if not, forks a background daemon process. The PID file contains JSON with PID, version, and start time. A heartbeat file is updated periodically so clients can detect stale daemons.

### Consequences

* Good, because zero user configuration needed
* Good, because PID file enables discovery and version checking
* Bad, because orphaned PID files need cleanup (handled by reconciliation on startup)
