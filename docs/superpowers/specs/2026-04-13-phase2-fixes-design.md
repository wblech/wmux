# Phase 2 Fixes — Post-Review Corrections

**Date:** 2026-04-13
**Source:** Code review of Phase 2 implementation against design spec

---

## Problem Statement

A comprehensive code review of Phase 2 identified 6 issues (1 critical, 3 major, 2 minor) that need correction before Phase 2 can be considered complete.

---

## Fix 1: Double "sessions" path in `handleEnvForward` (Critical)

**Bug:** `handleEnvForward` (`internal/daemon/service.go:741`) builds the session directory as:
```go
sessionDir := filepath.Join(d.dataDir, "sessions", req.SessionID)
```

But `dataDir` is already `~/.wmux/sessions` (set in `cmd/wmux/daemon.go:23`), producing `~/.wmux/sessions/sessions/{id}/` — a double "sessions" segment.

Meanwhile, cold restore uses `filepath.Join(d.dataDir, sessionID)` → `~/.wmux/sessions/{id}/` — correct.

**Fix:** Remove the extra `"sessions"` segment:
```go
sessionDir := filepath.Join(d.dataDir, req.SessionID)
```

This aligns env forwarding with cold restore — both write to `~/.wmux/sessions/{id}/`.

**Files:** `internal/daemon/service.go` (1 line change), `internal/daemon/service_test.go` (update test expectations if any assert the path)

---

## Fix 2: Wire `WithColdRestore` in `cmd/wmux/daemon.go` (Major)

**Bug:** The daemon is constructed without `WithColdRestore`, so cold restore is always disabled even when `cold_restore = true` in config.

**Root cause:** `cmd/wmux/daemon.go` does not load the TOML config file at all — it only parses CLI flags.

**Fix:** Load config in `cmdDaemon`:
1. Derive config path: `configPath := filepath.Join(baseDir, "config.toml")`
2. Attempt `config.Load(configPath)` — if file doesn't exist, use `config.defaults()` (need to export or add a `LoadOrDefault` helper)
3. Pass `daemon.WithColdRestore(cfg.History.ColdRestore)` to `NewDaemon`

Config file is optional — daemon works without it using defaults. Only load if file exists.

**Files:** `cmd/wmux/daemon.go`, possibly `internal/platform/config/config.go` (export defaults or add helper)

---

## Fix 3: Pass `MaxPerSession` to `history.NewWriter` (Major)

**Bug:** `persistSessionCreate` (`service.go:786`) passes `maxSize: 0` (unlimited) to `history.NewWriter`, ignoring config.

**Fix:** Add a `WithMaxScrollbackSize(int64)` option to the daemon. Wire it from config in `cmd/`:
1. Parse `cfg.History.MaxPerSession` string (e.g. `"10MB"`) to int64 bytes — reuse or add a `parseSize` helper
2. Add `daemon.WithMaxScrollbackSize(maxBytes)` option
3. In `persistSessionCreate`, use `d.maxScrollbackSize` instead of `0`

**Files:**
- `internal/daemon/options.go` — new `WithMaxScrollbackSize` option
- `internal/daemon/entity.go` — add `maxScrollbackSize int64` field to Daemon
- `internal/daemon/service.go` — use field in `persistSessionCreate`
- `cmd/wmux/daemon.go` — parse and pass option

---

## Fix 4: Add `pkg/client/environment.go` (Major)

**Bug:** The design spec says the client library sends env vars automatically on attach. No `environment.go` exists in `pkg/client/`.

**Fix:** Create `pkg/client/environment.go` with:
```go
func (c *Client) ForwardEnv(sessionID string, env map[string]string) error
```

This sends a `MsgEnvForward` frame with JSON payload `{"session_id": sessionID, "env": env}` and reads the OK/error response.

Also create `pkg/client/environment_test.go` with tests for:
- Successful forwarding
- Empty env map (should still succeed)
- Daemon error response

**Note:** Automatic env-on-attach (calling `ForwardEnv` inside `Attach`) is deferred. The method is exposed for explicit use by integrators like Watchtower. This avoids changing the `Attach` signature and keeps the auto-forward behavior configurable at the integrator level.

**Files:** `pkg/client/environment.go`, `pkg/client/environment_test.go`

---

## Fix 5: Add `addons/xterm/.gitignore` (Minor)

**Bug:** `node_modules/` and `dist/` are untracked and could be accidentally committed.

**Fix:** Create `addons/xterm/.gitignore`:
```
node_modules/
dist/
```

**Files:** `addons/xterm/.gitignore`

---

## Fix 6: Log errors in `handleEnvForward` (Minor)

**Bug:** `ForwardEnv` and `WriteEnvFile` errors are silently discarded (lines 747, 753).

**Fix:** Log errors using `fmt.Fprintf(os.Stderr, ...)` consistent with the daemon's existing error logging pattern. The OK response to the client is still correct — env forwarding is best-effort — but failures should be observable.

**Files:** `internal/daemon/service.go`

---

## Deferred: Stream reader for `OnData`/`OnEvent` (Note)

The client's `OnData` and `OnEvent` register callbacks but no goroutine reads from the stream channel to dispatch them. This requires significant architectural changes (background reader goroutine, frame demuxing, connection lifecycle management). Deferred to when Watchtower actually needs real-time event streaming. The current polling-based approach via control channel is sufficient for Phase 2.

---

## Testing Strategy

- Fix 1: Existing env forwarding tests should pass with corrected path (verify)
- Fix 2: Integration-level — verify cold restore works end-to-end with config loaded
- Fix 3: Unit test for `WithMaxScrollbackSize` option; verify `persistSessionCreate` uses the value
- Fix 4: Full test suite for `ForwardEnv` method (mock server pattern from existing client tests)
- Fix 5: No tests needed
- Fix 6: No tests needed (logging)

All fixes must maintain ≥90% coverage in affected packages.
