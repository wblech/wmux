# Phase 2: Watchtower Integration — Overview

> **For agentic workers:** This is the overview plan. Each sub-plan has its own file. Execute sub-plans sequentially — each depends on the previous one. After all 4 sub-plans, write ADRs (sub-plan 5).

**Goal:** Enable Watchtower to use wmux as its terminal backend with instant warm attach via xterm headless emulator, opt-in cold restore, and a public Go client library.

**Architecture:** Phase 2 extends the Phase 1 daemon with: (1) a pluggable emulator addon protocol with an xterm headless backend running as a single Node process, (2) a public Go client library at `pkg/client/` that abstracts the binary wire protocol, (3) two-phase warm attach delivering scrollback + viewport snapshots, (4) extensible session metadata, full event system, environment forwarding, and (5) opt-in cold restore for post-reboot session recovery.

**Tech Stack:** Go 1.25+, Node.js + @xterm/headless (addon only), existing Phase 1 packages

**Spec:** [Phase 2 Design Spec](../specs/2026-04-13-phase2-watchtower-integration-design.md)

---

## Sub-plan Sequence

| # | Sub-plan | File | Status |
|---|---|---|---|
| 1 | Emulator Addon Protocol + xterm Addon | [sub1-emulator-addon](./2026-04-13-phase2-sub1-emulator-addon.md) | [ ] |
| 2 | Go Client Library + Warm Attach + Integration Doc | [sub2-client-library](./2026-04-13-phase2-sub2-client-library.md) | [ ] |
| 3 | Session Metadata + Full Events + Env Forwarding | [sub3-metadata-events-env](./2026-04-13-phase2-sub3-metadata-events-env.md) | [ ] |
| 4 | Cold Restore | [sub4-cold-restore](./2026-04-13-phase2-sub4-cold-restore.md) | [ ] |
| 5 | Architecture Decision Records | [sub5-adrs](./2026-04-13-phase2-sub5-adrs.md) | [ ] |

## Dependencies Between Sub-plans

```
Sub-plan 1 (Emulator Addon) ──► Sub-plan 2 (Client Library + Warm Attach)
                                        │
                                        ▼
                                Sub-plan 3 (Metadata + Events + Env)
                                        │
                                        ▼
                                Sub-plan 4 (Cold Restore)
                                        │
                                        ▼
                                Sub-plan 5 (ADRs)
```

## Packages Modified/Created Per Sub-plan

| Sub-plan | Creates | Modifies |
|---|---|---|
| 1 | `internal/session/addon_emulator.go`, `addons/xterm/` | `internal/session/entity.go`, `internal/platform/config/config.go` |
| 2 | `pkg/client/` (5 files), `docs/integration-guide.md` | `internal/daemon/service.go`, `internal/daemon/entity.go`, `internal/platform/protocol/entity.go` |
| 3 | `internal/daemon/environment.go`, `internal/daemon/osc.go`, `pkg/client/metadata.go`, `pkg/client/environment.go` | `internal/session/entity.go`, `internal/platform/event/entity.go`, `internal/platform/protocol/entity.go`, `internal/daemon/service.go`, `internal/daemon/entity.go` |
| 4 | `pkg/client/restore.go` | `internal/platform/config/config.go`, `internal/daemon/service.go`, `pkg/client/entity.go` |
| 5 | `decisions/0000-*.md` through `decisions/0016-*.md` | — |
