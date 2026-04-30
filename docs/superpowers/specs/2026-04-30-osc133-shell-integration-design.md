# OSC 133 Shell Integration — Command lifecycle tracking in wmux

## Goal

Capture OSC 133 escape sequences emitted by integrated shells, expose a
per-session command lifecycle state machine, and surface command lifecycle
events through the existing IPC event bus and a new persisted history file.

The Watchtower client (and any other integrator) will be able to:
- Show command exit-code badges
- Jump between prompts by row
- Restore command history after daemon restart
- Tag selections by command boundary

Watchtower-side work is out of scope; this spec covers wmux only.

## Background: OSC 133

OSC 133 is the de facto standard for shell integration markers, originally
introduced by FinalTerm and adopted by VS Code, iTerm2, WezTerm, Kitty, etc.
Shells emit invisible escape sequences:

| Sequence | Meaning |
|---|---|
| `OSC 133 ; A ST` | Prompt has been drawn |
| `OSC 133 ; B ST` | User input has begun |
| `OSC 133 ; C ST` | Command has started executing |
| `OSC 133 ; D ; <exit> ST` | Command finished with exit code |

`ST` is the string terminator: either `BEL` (`\x07`) or `ESC \` (`\x1b\\`).

Today wmux receives these as opaque bytes in the PTY broadcast stream. The goal
is to recognize them and expose structured semantics.

## Decisions made during design

### D1. Use the existing passive scanner, not charmvt's `RegisterOscHandler`

wmux's OSC 7/9/777 handling lives in `internal/daemon/osc.go::ParseOSC`, a
passive byte scanner called from `daemon.scanOSC`. The `charmvt` emulator
does expose `RegisterOscHandler(133, fn)`, but routing OSC 133 through the
emulator path is wrong:

- The emulator channel (`emulatorCh`) is **lossy** by design — `select { default }`
  drops chunks under load (see ADR 0024).
- When `EmulatorFactory == nil`, sessions use `NoneEmulator{}` which discards
  all input. OSC 133 events would never fire.
- Adding event emission inside the emulator violates layer separation.

OSC 133 detection lives where OSC 7/9/777 already lives: the passive scanner
on the broadcast path.

### D2. Row tracking is wmux's responsibility, not the client's

The "row" field on a `Command` is the line position in the persistent
scrollback. Justification:

1. **Single source of truth** — wmux owns `scrollback.bin`; rows reference it.
2. **Cold-restore consistency** — `commands.json` and `scrollback.bin` are both
   persisted by wmux and must align. Computing rows in the client breaks this.
3. **Multi-client sessions** — letting each attached client recompute rows
   independently invites drift.
4. **Cheap to compute** — incremental `bytes.Count(prefix, '\n')` is SIMD on
   amd64/arm64 and only fires on chunks that contain OSC 133 (see D3).

For shell-prompt boundaries (the OSC 133 use case), counting `\n` is
mathematically exact: prompts always come after a newline. TUIs that move the
cursor without `\n` are not relevant — jump-to-prompt has no meaning inside
vim or htop.

### D3. New domain package `internal/cmdlifecycle/`

Goframe DDD splits domains by responsibility. The state machine is its own
domain (input: OSC events + row; output: Commands + lifecycle Events). It is
not part of `daemon` (which handles wire protocol) or `session` (which handles
PTY lifecycle).

### D4. `ParseOSC` extends in place; bench gate prevents alloc regression

The existing scanner returns `[]OSCResult{Type, Value}`. We add `Offset int`
to the struct and pre-cursor-track during scan to retain absolute byte
positions. Backward-compat: existing callers ignore `.Offset`.

The existing benches `BenchmarkParseOSC_NoOSC` and `BenchmarkParseOSC_WithOSC`
(`internal/daemon/bench_test.go`) currently report 0 allocs / 422 ns and
≥1 alloc respectively. **This spec adds an explicit allocs/op assertion to
`BenchmarkParseOSC_NoOSC` as a new CI gate** (ADR 0032 itself does not pin
ParseOSC — only Batcher/Buffer/ReadLoop). The risk that adding an `Offset`
field regresses the no-match path is detected by this new gate.

### D5. Steady-state persistence is owned by `cmdlifecycle.Service` via a
`Repository` interface; per-session writer lifecycle stays daemon-private

The `Repository` interface has a single method: `Save(sessID, state)`. It
covers steady-state writes only — the state machine pushes new state to the
writer after every transition that changes persisted data.

Per-session writer lifecycle (open `commands.Writer` on session create,
close on session exit) lives on the daemon side, **not** on the
`Repository` interface. The daemon's `cmdRepository` type wraps a map of
`commands.Writer` and implements `Save` by routing to the right writer.
`Open` / `Close` are daemon-private helpers, not part of the
`cmdlifecycle.Repository` contract.

This split keeps `cmdlifecycle` itself I/O-free (testable as pure logic
with a mock `Repository`), while putting the file/dir lifecycle next to
the existing `scrollbackWriters` map in the daemon.

## Architecture

```
                    PTY bytes
                        │
               ┌────────┴─────────┐
               ▼                  ▼
       session.batcher       session.emulatorCh
       (zero-alloc path)     (lossy, charmvt — UNCHANGED)
               │
               ▼
         session.buffer
               │
               ▼
   daemon.flushSessionOutput
               │
               ├─► persistOutput → scrollback.bin
               ├─► teeRecording  → asciinema recording (if enabled)
               │
               ▼
         daemon.scanOSC
               │
               ▼
       ParseOSC(data) → []OSCResult{Type, Value, Offset}
               │
       ┌───────┴────────┬──────────────┬──────────────┐
       ▼                ▼              ▼              ▼
   OSC 7 (cwd)    OSC 9/99/777    OSC 133         (others)
                  (notif/ready)        │
                                       │ HandleOSC(sessID, code, exit, row)
                                       ▼
                          cmdlifecycle.Service
                                       │
                          ┌────────────┼─────────────┐
                          ▼            ▼             ▼
                     state machine   onEvent     repo.Save
                     (per session)   (handler)   (per session)
                                       │             │
                                       ▼             ▼
                              event.Bus.Publish   commands.Writer
                              ↓                   (debounced 200ms)
                              MsgEvent (IPC)         ↓
                                                  commands.json
                                                  (atomic rename)
```

Key properties:

- OSC 133 detection runs on the **same path** as OSC 7/9/777 — no new
  performance surface area.
- Row counting is **lazy** — only triggered when `ParseOSC` returns at least
  one match (see "Performance constraints" below).
- The emulator path is untouched; charmvt + `NoneEmulator` work identically.

## Package: `internal/cmdlifecycle/`

### File layout (goframe-compliant)

```
internal/cmdlifecycle/
├── entity.go         # State, Command, sentinel errors
├── events.go         # Event, EventKind, EventHandler
├── service.go        # Repository interface, Service, internal tracker
├── options.go        # Option, WithMaxHistory, WithClock
├── module.go         # var Module = fx.Options(fx.Provide(NewService))
├── entity_test.go
├── events_test.go
├── service_test.go
├── options_test.go
├── module_test.go
└── bench_test.go
```

### `entity.go`

```go
type State int

const (
    _ State = iota
    StateIdle           // no in-flight command
    StatePromptShown    // received OSC 133;A
    StateUserTyping     // received OSC 133;B
    StateCommandRunning // received OSC 133;C
)

func (s State) String() string

type Command struct {
    StartedAt    time.Time `json:"started_at"`
    EndedAt      time.Time `json:"ended_at,omitempty"`
    ExitCode     int       `json:"exit_code"`
    StartRow     int64     `json:"start_row"`
    EndRow       int64     `json:"end_row"`
    Orphan       bool      `json:"orphan,omitempty"`
    OrphanReason string    `json:"orphan_reason,omitempty"`
}

type SessionState struct {
    History  []Command `json:"history"`
    InFlight *Command  `json:"in_flight,omitempty"`
}

var (
    ErrSessionNotRegistered = errors.New("cmdlifecycle: session not registered")
    ErrSessionExists        = errors.New("cmdlifecycle: session already registered")
    ErrInvalidOSCCode       = errors.New("cmdlifecycle: invalid OSC 133 code")
)
```

Orphan reason values used by the daemon:

| Constant | When |
|---|---|
| `"missing_d_marker"` | CommandRunning + A/B/C without intervening D |
| `"daemon_restart"`   | InFlight present at cold-restore |

(String constants live in `entity.go` as exported `OrphanReason*` consts.)

### `events.go`

```go
type EventKind int

const (
    _ EventKind = iota
    EventPromptShown    // entered StatePromptShown (not on re-arm)
    EventCommandStarted // entered StateCommandRunning
    EventCommandFinished
)

type Event struct {
    Kind    EventKind
    Command *Command  // nil only for EventPromptShown
    Row     int64     // current row at the time of the event
}

type EventHandler func(sessID string, ev Event)
```

### `service.go`

```go
// Repository abstracts persistence. Daemon implements via commands.Writer.
type Repository interface {
    // Save is non-blocking; implementation handles debouncing/queuing.
    Save(sessID string, state SessionState)
}

type Service struct {
    mu         sync.RWMutex
    trackers   map[string]*tracker
    repo       Repository
    onEvent    EventHandler // set via OnEvent before first Register
    maxHistory int
    now        func() time.Time
}

func NewService(repo Repository, opts ...Option) *Service

// OnEvent registers the global handler. Pattern matches
// session.Service.OnExit / OnDataReady — handler-style late binding is the
// established convention in this codebase. Replaces any previously
// registered handler.
//
// If OnEvent is never called (or set to nil), events are silently dropped.
// Persistence via Repository.Save is unaffected. The daemon's Start()
// always calls OnEvent before serving any traffic, so in normal operation
// the handler is always set by the time HandleOSC fires.
func (s *Service) OnEvent(h EventHandler)

func (s *Service) Register(sessID string) error
func (s *Service) Unregister(sessID string)

// HandleOSC processes a single parsed OSC 133 sequence.
// code is one of 'A', 'B', 'C', 'D'. Unknown codes are silently ignored.
// exitCode is meaningful only for code == 'D'; ignored otherwise.
// row is the current accumulated newline count for the session.
func (s *Service) HandleOSC(sessID string, code byte, exitCode int, row int64) error

func (s *Service) Snapshot(sessID string) (SessionState, error)
func (s *Service) Restore(sessID string, state SessionState) error

// tracker is the per-session state machine. Private — exposed only via
// Service methods.
type tracker struct {
    state    State
    current  *Command
    history  []Command  // pre-allocated cap = maxHistory
    onEvent  EventHandler
    sessID   string
    now      func() time.Time
}
```

### `options.go`

```go
type Option func(*Service)

// WithMaxHistory overrides the per-session FIFO cap (default 500).
func WithMaxHistory(n int) Option

// WithClock injects a deterministic clock for tests.
func WithClock(now func() time.Time) Option
```

### `module.go`

```go
var Module = fx.Options(fx.Provide(NewService))
```

`NewService` requires a `Repository` — `fx` resolves it from the daemon-side
implementation registered in the daemon's `Module`.

## State machine

### Transition table

| State | Received | Next | Side effects |
|---|---|---|---|
| Idle | A | PromptShown | emit `EventPromptShown` |
| Idle | B | UserTyping | (defensive) |
| Idle | C | CommandRunning | open Command, emit `EventCommandStarted` |
| Idle | D | Idle | ignore (no command to close) |
| PromptShown | A | PromptShown | re-arm — no event |
| PromptShown | B | UserTyping | — |
| PromptShown | C | CommandRunning | open, emit `EventCommandStarted` |
| PromptShown | D | Idle | ignore |
| UserTyping | A | PromptShown | reset, emit `EventPromptShown` |
| UserTyping | B | UserTyping | — |
| UserTyping | C | CommandRunning | open, emit `EventCommandStarted` |
| UserTyping | D | Idle | ignore |
| CommandRunning | A | PromptShown | **orphan**: close current with `OrphanReason=missing_d_marker`, emit `EventCommandFinished`, emit `EventPromptShown` |
| CommandRunning | B | UserTyping | **orphan**: close + `EventCommandFinished` |
| CommandRunning | C | CommandRunning | **orphan + reopen**: close + `EventCommandFinished`, open new + `EventCommandStarted` |
| CommandRunning | D;n | Idle | close current with `ExitCode=n`, `Orphan=false`, emit `EventCommandFinished` |

### Invariants

- `current != nil` iff `state == StateCommandRunning`.
- `EndRow >= StartRow` for every closed Command.
- `len(history) <= maxHistory` (FIFO eviction when full).
- Every `EventCommandStarted` is paired with exactly one `EventCommandFinished`.
- Orphan commands always have `ExitCode = -1`.

### Malformed inputs

| Input | Behavior |
|---|---|
| `D` (no exit) | `ExitCode = 0` |
| `D;` (empty) | `ExitCode = 0` |
| `D;abc` (non-int) | `ExitCode = 0` |
| `D;1;extra` | `ExitCode = 1`, ignore trailing |
| Codes E, F, Z, etc. | silently ignored (forward-compat) |

### FIFO eviction

`history` is pre-allocated as `make([]Command, 0, maxHistory)`. When length
reaches `maxHistory`, eviction shifts left and overwrites:

```go
if len(t.history) == t.maxHistory {
    copy(t.history[0:], t.history[1:])
    t.history[t.maxHistory-1] = closed
} else {
    t.history = append(t.history, closed)
}
```

Cost is `O(n)` per eviction with `n = 500`. Acceptable since command
finish events are rare (≤ 100/min in normal use).

### Snapshot/Restore

Snapshot returns `SessionState{History, InFlight}` where `InFlight` is non-nil
iff `state == StateCommandRunning`.

**Snapshot returns deep copies.** The History slice is allocated fresh each
call, and `InFlight` (if non-nil) is a freshly-allocated `*Command`. This is
required because `Repository.Save` is non-blocking — the writer goroutine
may serialize the state after `HandleOSC` has already mutated the tracker's
internal slice. Without a deep copy, that would be a torn-write bug.

Restore behavior:

1. If session already registered: return `ErrSessionExists`.
2. Register the session (creates tracker in `StateIdle`).
3. Copy `state.History` into the tracker's pre-allocated history.
4. If `state.InFlight != nil`, close it as orphan **immediately** (the
   command is definitionally dead — see "Why orphan-on-restore is correct"
   in the cold-restore section):
   - `EndedAt = SavedAt` (from the on-disk `commands.json`, not `now()` —
     more honest representation of when the daemon last knew the command
     was alive)
   - `EndRow = StartRow` (no observed row movement after the save)
   - `ExitCode = -1`
   - `Orphan = true`
   - `OrphanReason = "daemon_restart"`
   - Append to history (subject to FIFO cap).
5. After Restore, tracker is always in `StateIdle`. **Restore can never
   resume `StateCommandRunning`.** The on-disk `InFlight` field is kept
   for forensic value (last-known state at save time) but is consumed-as-
   orphan on every read.

### Ordering invariant: Restore must precede Register for the same sessID

Cold-restore paths read `commands.json` and call `Restore(sessID, state)`.
Normal session-create paths call `Register(sessID)`. Both create tracker
state, and `Restore` returns `ErrSessionExists` if the session is already
registered.

The daemon ensures cold-restored sessions go through `Restore` *before*
the session-create code path can call `Register` for the same ID — the
reconcile loop runs once at startup, before the IPC server begins
accepting new `MsgCreate` frames. New sessions (never persisted) take only
the `Register` path. Sessions previously persisted but absent from the
reconcile result also take only `Register`.

## Daemon integration

### `internal/daemon/osc.go` changes

Extend `OSCResult` with the byte offset:

```go
type OSCType int

const (
    _ OSCType = iota
    OSCTypeCwd
    OSCTypeNotification
    OSCTypeShellReady
    OSCType133  // NEW
)

type OSCResult struct {
    Type   OSCType
    Value  string
    Offset int  // NEW — byte offset of ESC ] in original data
}
```

Replace the `s = s[idx+2:]` slice-walk with a `cursor` integer to retain
absolute offsets:

```go
func ParseOSC(data []byte) []OSCResult {
    var results []OSCResult
    cursor := 0
    for {
        idx := bytes.Index(data[cursor:], oscIntroducer)
        if idx < 0 {
            break
        }
        matchStart := cursor + idx
        cursor = matchStart + 2

        endST := bytes.Index(data[cursor:], oscTerminatorST)
        endBEL := bytes.IndexByte(data[cursor:], 0x07)
        end := pickEnd(endST, endBEL)  // factor existing inline switch into helper
        if end < 0 {
            break
        }
        bodyEnd := cursor + end
        body := data[cursor:bodyEnd]
        cursor = bodyEnd + 1

        // ... existing parsing ...
        switch string(oscNum) {
        case "7":
            results = append(results, OSCResult{Type: OSCTypeCwd, Value: parsed.Path, Offset: matchStart})
        case "9", "99", "777":
            // ... existing logic, with Offset: matchStart added
        case "133":
            results = append(results, OSCResult{Type: OSCType133, Value: oscValue, Offset: matchStart})
        }
    }
    return results
}

// pickEnd centralizes the "ST vs BEL terminator" choice that exists today
// inline in ParseOSC. Pure refactor — same logic, same allocations (none).
func pickEnd(endST, endBEL int) int { /* ... */ }
```

Backward compatibility: existing callers (OSC 7/9/777 paths) ignore `.Offset`.
The no-match path still returns `nil`. The new `BenchmarkParseOSC_NoOSC`
gate (with explicit `b.ReportAllocs()` and a `0 allocs/op` assertion in CI)
is the regression detector.

### `internal/daemon/service.go` changes

New per-session state in `Daemon`:

```go
type Daemon struct {
    // ... existing fields ...
    cmdSvc          *cmdlifecycle.Service
    cmdRepo         *cmdRepository
    lineCountersMu  sync.RWMutex             // guards lineCounters map
    lineCounters    map[string]*atomic.Int64 // keyed by sessID; *Int64 reads are lock-free
}
```

**Synchronization rules for `lineCounters`:**

- The map itself uses a dedicated `sync.RWMutex` (`lineCountersMu`) — *not*
  the existing `d.mu`, because `d.mu` already covers attachment/recording
  state and serializing the broadcast hot path against unrelated session
  metadata is a perf regression.
- `handleCreate` and `OnExit` callback take `lineCountersMu.Lock()` to
  insert/delete entries — both rare events.
- `scanOSC` (broadcast hot path) takes `lineCountersMu.RLock()` to look up
  the `*atomic.Int64`, then releases immediately and uses `Add` / `Load`
  on the pointer (lock-free integer ops).
- The lookup happens only when `ParseOSC` returns at least one match, so
  the RLock is *not* on every chunk — only on chunks containing OSC
  sequences.

**Always allocate the counter.** The previous draft made it nil when
cold-restore was disabled, which silently disabled row tracking. New rule:
allocate the counter for every session at create time (cost: one
`*atomic.Int64`, ~24 bytes). Cold-restore on/off only affects whether the
counter's value is *persisted*, not whether it is computed.

`scanOSC` extended (lazy newline counting).

**Cross-chunk invariant:** after `scanOSC` returns, `counter` reflects the
total newline count over **all bytes ever scanned for this session** — not
just the current chunk. This is preserved by always counting the tail
(post-last-match) before returning, so the next call's `counter.Load()` is
already correct from byte zero of that chunk's perspective.

```go
func (d *Daemon) scanOSC(sessID string, data []byte) {
    matches := ParseOSC(data)
    if len(matches) == 0 {
        return  // hot path: zero work for chunks without OSC sequences
    }

    d.lineCountersMu.RLock()
    counter := d.lineCounters[sessID]
    d.lineCountersMu.RUnlock()
    // counter is always non-nil for active sessions (allocated at create).

    cursor := 0
    for _, m := range matches {
        switch m.Type {
        case OSCTypeCwd:
            // existing
        case OSCTypeNotification:
            // existing
        case OSCTypeShellReady:
            // existing
        case OSCType133:
            // Lazy: count newlines only over the prefix between the
            // previous marker and this one. Cross-chunk correctness is
            // ensured by the tail-count after the loop.
            counter.Add(int64(bytes.Count(data[cursor:m.Offset], []byte{'\n'})))
            cursor = m.Offset
            row := counter.Load()
            code, exit := parseOSC133Value(m.Value)
            _ = d.cmdSvc.HandleOSC(sessID, code, exit, row)
        }
    }

    // Tail: count newlines after the last match. REQUIRED for cross-chunk
    // accuracy — without this, the next chunk's first OSC 133 marker would
    // see a stale row number missing this chunk's tail newlines.
    if cursor < len(data) {
        counter.Add(int64(bytes.Count(data[cursor:], []byte{'\n'})))
    }
}

func parseOSC133Value(v string) (code byte, exit int) {
    if len(v) == 0 {
        return 0, 0
    }
    code = v[0]
    if code == 'D' && len(v) > 2 {
        rest := v[2:] // skip "D;"
        if semi := strings.IndexByte(rest, ';'); semi >= 0 {
            rest = rest[:semi]
        }
        exit, _ = strconv.Atoi(rest)
    }
    return code, exit
}
```

Lifecycle wiring:

| Daemon event | Action |
|---|---|
| `Start()` | call `cmdSvc.OnEvent(d.publishCmdEvent)` before serving any traffic |
| `reconcile` cold-restore (in `Start()`, before serving) | for each persisted session: `commands.Load(path)` → `cmdSvc.Restore(sessID, state)`; `cmdRepo.Open(sessID, path)`; allocate `lineCounters[sessID]` |
| `handleCreate` (new sessions) | `cmdSvc.Register(sessID)`; if cold-restore enabled, `cmdRepo.Open(sessID, path)`; allocate `lineCounters[sessID]` |
| `OnExit` callback | `cmdSvc.Unregister(sessID)`; `cmdRepo.Close(sessID)`; delete `lineCounters[sessID]` |
| Daemon shutdown | iterate `cmdRepo` writers, call `Close()` (final flush sync) |

`publishCmdEvent` translates `cmdlifecycle.Event` to `event.Event`:

```go
func (d *Daemon) publishCmdEvent(sessID string, ev cmdlifecycle.Event) {
    var t event.Type
    payload := map[string]any{}
    switch ev.Kind {
    case cmdlifecycle.EventPromptShown:
        t = event.CommandPromptShown
        payload["row"] = ev.Row
    case cmdlifecycle.EventCommandStarted:
        t = event.CommandStarted
        payload["started_at"] = ev.Command.StartedAt
        payload["start_row"] = ev.Command.StartRow
    case cmdlifecycle.EventCommandFinished:
        t = event.CommandFinished
        payload["started_at"] = ev.Command.StartedAt
        payload["ended_at"] = ev.Command.EndedAt
        payload["exit_code"] = ev.Command.ExitCode
        payload["start_row"] = ev.Command.StartRow
        payload["end_row"] = ev.Command.EndRow
        payload["duration_ms"] = ev.Command.EndedAt.Sub(ev.Command.StartedAt).Milliseconds()
        payload["orphan"] = ev.Command.Orphan
        if ev.Command.OrphanReason != "" {
            payload["orphan_reason"] = ev.Command.OrphanReason
        }
    }
    d.publishEvent(event.Event{Type: t, SessionID: sessID, Payload: payload})
}
```

### New file: `internal/daemon/cmdrepository.go`

```go
type cmdRepository struct {
    mu      sync.RWMutex
    writers map[string]*commands.Writer
    enabled bool  // false when cold-restore is off
}

func newCmdRepository(coldRestore bool) *cmdRepository

func (r *cmdRepository) Open(sessID, path string) error  // session create
func (r *cmdRepository) Close(sessID string)              // session exit, flushes sync

// Save implements cmdlifecycle.Repository.
func (r *cmdRepository) Save(sessID string, state cmdlifecycle.SessionState) {
    if !r.enabled {
        return
    }
    r.mu.RLock()
    w := r.writers[sessID]
    r.mu.RUnlock()
    if w != nil {
        w.Update(state)  // captures + non-blocking dirty mark
    }
}
```

## IPC events

### Three new types in `internal/platform/event/entity.go`

```go
const (
    // ... existing ...
    CommandPromptShown
    CommandStarted
    CommandFinished
)
```

Stringer additions:

```go
case CommandPromptShown: return "command.prompt_shown"
case CommandStarted:     return "command.started"
case CommandFinished:    return "command.finished"
```

`typeFromString` additions (for `UnmarshalJSON`):

```go
case "command.prompt_shown": return CommandPromptShown, true
case "command.started":      return CommandStarted, true
case "command.finished":     return CommandFinished, true
```

### Wire format (over `MsgEvent`)

```jsonc
{"type": "command.prompt_shown",
 "session_id": "main/build",
 "payload": {"row": 156}}

{"type": "command.started",
 "session_id": "main/build",
 "payload": {
   "started_at": "2026-04-30T12:34:56.123Z",
   "start_row": 142}}

{"type": "command.finished",
 "session_id": "main/build",
 "payload": {
   "started_at":   "2026-04-30T12:34:56.123Z",
   "ended_at":     "2026-04-30T12:35:02.456Z",
   "exit_code":    0,
   "start_row":    142,
   "end_row":      156,
   "duration_ms":  6333,
   "orphan":       false,
   "orphan_reason": ""}}
```

### Delivery semantics (unchanged from existing event bus)

- Best-effort fan-out via `event.Bus`. Subscribers with full buffers drop events.
- **`commands.json` is the authoritative store.** Clients can re-read it to
  recover from missed events.
- Subscribers can filter via `Bus.SubscribeTypes(...)` to opt out of high-
  frequency events (e.g., subscribe only to `CommandFinished`).

## Persistence: `internal/platform/commands/`

Mirrors `internal/platform/history/` patterns.

### Canonical file path

```
{dataDir}/{session_id}/commands.json
```

Same directory as `meta.json` and `scrollback.bin` (created by
`history.EnsureSessionDir`). Per-session, never shared.

### File layout

```
internal/platform/commands/
├── entity.go     # File schema, sentinel errors
├── writer.go     # debounced disk persistor
├── reader.go     # Load (cold-restore)
├── entity_test.go
├── writer_test.go
├── reader_test.go
└── bench_test.go
```

### `entity.go`

```go
const CurrentVersion = 1

type File struct {
    Version   int                          `json:"version"`
    SessionID string                       `json:"session_id"`
    SavedAt   time.Time                    `json:"saved_at"`
    History   []cmdlifecycle.Command       `json:"history"`
    InFlight  *cmdlifecycle.Command        `json:"in_flight,omitempty"`
}

var (
    ErrUnsupportedVersion = errors.New("commands: unsupported file version")
    ErrWriterClosed       = errors.New("commands: writer closed")
)
```

**`InFlight` field semantics:**

- Written when the tracker is in `StateCommandRunning` at save time
  (forensic snapshot of the last-known active command).
- On read, `InFlight` is **always** consumed as an orphan (see Restore
  step 4). It cannot resume `StateCommandRunning` — the command was
  definitionally aborted by daemon shutdown.
- Steady-state idle case: `InFlight` is omitted from JSON via
  `omitempty`. Readers must treat absent and `null` identically.

### `writer.go`

Per-session goroutine, debounced 200ms:

```go
type Writer struct {
    path     string
    sessID   string
    debounce time.Duration

    mu       sync.Mutex
    pending  cmdlifecycle.SessionState  // captured on Update

    notifyCh chan struct{}  // cap 1
    closeCh  chan struct{}
    doneCh   chan struct{}
}

func NewWriter(path, sessID string, opts ...Option) (*Writer, error)

// Update captures the new state and schedules a flush. Non-blocking.
// Safe to call from any goroutine, including hot paths.
func (w *Writer) Update(state cmdlifecycle.SessionState)

// Flush forces a synchronous write of the most recent state.
func (w *Writer) Flush() error

// Close performs a final synchronous flush and stops the goroutine.
func (w *Writer) Close() error
```

Internal loop:

```go
func (w *Writer) run() {
    defer close(w.doneCh)
    timer := time.NewTimer(0)
    timer.Stop()
    pending := false

    for {
        select {
        case <-w.notifyCh:
            if !pending {
                timer.Reset(w.debounce)
                pending = true
            }
        case <-timer.C:
            _ = w.flushLocked()
            pending = false
        case <-w.closeCh:
            if pending {
                _ = w.flushLocked()
            }
            return
        }
    }
}
```

`flushLocked` writes via `path + ".tmp"` then `os.Rename` for atomicity.
Errors are logged but do not kill the goroutine — next `Update` retries.

### `reader.go`

```go
// Load reads commands.json. Returns zero File and nil error if the file
// does not exist (sessions without commands yet). Returns ErrUnsupportedVersion
// if the file is from a future schema.
func Load(path string) (File, error)
```

## Performance constraints

The wmux PTY broadcast hot path achieves 0 allocs/op in steady state per
ADR 0032. This design preserves all guarantees:

| Allocation site | How it stays at 0 allocs |
|---|---|
| `ParseOSC` no-match path | `var results []OSCResult` is nil; early-return when `bytes.Index` fails. Gated by `BenchmarkParseOSC_NoOSC` (existing bench, this spec adds explicit allocs/op assertion). |
| Per-chunk newline counting | **Not done unconditionally.** `bytes.Count` only runs after `ParseOSC` returns at least one match. Chunks without OSC sequences (the common case) skip newline counting entirely. |
| Per-session line counter | `*atomic.Int64`, allocated once at session create. Increments via `Add` are alloc-free. |
| `cmdSvc.HandleOSC` on no-op transitions (e.g., `D` in Idle) | Early return after switch; no allocations. Gated by `BenchmarkHandleOSC_NoOp`. |
| `commands.Writer.Update` | Map lookup + struct copy; no I/O on caller goroutine. |

Allocations that exist on the rare-event path (per OSC 133 sequence) are
acceptable: prompt transitions occur ≤ 100/min in interactive use.

### Bench gates added

```
internal/daemon/bench_test.go (extending existing):
  BenchmarkParseOSC_NoOSC            ≤ 0 allocs/op  ← regression gate (existing bench, gain assertion)
  BenchmarkParseOSC_WithOSC          baseline (existing bench)
  BenchmarkParseOSC_With133          baseline (NEW, parallel to _WithOSC)
  BenchmarkParseOSC_Mixed            baseline (NEW)

internal/cmdlifecycle/bench_test.go:
  BenchmarkHandleOSC_NoOp            ≤ 0 allocs/op
  BenchmarkHandleOSC_FullCycle       baseline (A→B→C→D)
  BenchmarkSnapshot_500Commands      baseline

internal/daemon/bench_test.go:
  BenchmarkScanOSC_NoMatch           ≤ 0 allocs/op  ← integration gate
  BenchmarkScanOSC_With133Match      baseline
```

## Cold-restore behavior

On daemon startup with `--cold-restore`:

1. Reconcile loop discovers existing session directories under `dataDir`.
2. For each, attempt `commands.Load(sessionDir + "/commands.json")`.
3. If file missing → register with empty state (no-op).
4. If file present and `Version == CurrentVersion`:
   - `cmdSvc.Restore(sessID, SessionState{History, InFlight})`
   - `InFlight` (if non-nil) is closed as orphan with `OrphanReason = "daemon_restart"`,
     `EndedAt = file.SavedAt`, `EndRow = InFlight.StartRow`, `ExitCode = -1`.
   - Tracker enters `StateIdle`, ready for new shell input.
5. If file present and `Version > CurrentVersion`:
   - Log warning, do not Restore. Session continues without history (writes
     a fresh `commands.json` overwriting the future one only if a new command
     finishes).

### Why orphan-on-restore is correct

When the daemon dies, PTY child processes die with it (PPID-tied). The shell
that emitted `OSC 133;C` is no longer running by the time we restart. The
"in-flight" command was definitionally aborted — not silently completed.
Marking it as orphan preserves an honest history: users see `exit_code=-1`
and `orphan_reason=daemon_restart` rather than a fabricated success.

## Watchtower / integrator impact

### What changes for integrators

- **3 new event types** on the IPC event bus (`command.prompt_shown`,
  `command.started`, `command.finished`). Adding handlers is additive — the
  default case in existing switches ignores them.
- **New optional file** `commands.json` per session directory under
  `dataDir`. Integrators that want command history can read it; those that
  don't can ignore it.
- **No new IPC `MsgType`** — events ride on the existing `MsgEvent` frame.
- **No changes to `MsgData`** — PTY bytes flow as before, including the OSC
  133 escape sequences themselves (clients that already render terminals can
  continue stripping them via their own emulator).

### Compatibility matrix

| wmux client | wmux server | Behavior |
|---|---|---|
| Old client + new server | New events arrive; client's switch hits `default`, ignores | OK |
| New client + old server | New events never fire; client's handlers never called | OK |
| Both new | Full feature available | Best |

No breaking changes. Integrators adopt at their own pace.

## Testing strategy

### Spike (do first, ~30min, throwaway)

Confirm the OSC 133 byte format produced by real shells before extending the
parser:

```go
// internal/daemon/osc_spike_test.go (delete after running)
func TestSpike_OSC133_Formats(t *testing.T) {
    cases := []struct {
        name string
        data []byte
    }{
        {"A-bel",     []byte("\x1b]133;A\x07")},
        {"A-st",      []byte("\x1b]133;A\x1b\\")},
        {"D-0",       []byte("\x1b]133;D;0\x07")},
        {"D-42",      []byte("\x1b]133;D;42\x07")},
        {"mixed",     []byte("\x1b]133;C\x07ls\nfile1\n\x1b]133;D;0\x07")},
    }
    for _, c := range cases {
        t.Run(c.name, func(t *testing.T) {
            results := ParseOSC(c.data)
            t.Logf("input=%q  results=%+v", c.data, results)
        })
    }
}
```

### Unit tests by package

**`internal/cmdlifecycle/`** (pure logic, target ≥95% coverage):

- Happy path A→B→C→D;0
- Non-zero exit D;42
- Orphan from CommandRunning + A
- Orphan + reopen from C→C
- Malformed D variants
- Unknown codes ignored
- D in Idle (no-op)
- Unregistered session error
- FIFO eviction at boundary (501st command evicts 1st)
- Snapshot/Restore roundtrip
- Restore with InFlight produces orphan with `OrphanReason=daemon_restart`
- Concurrent sessions (50 parallel A→B→C→D, race-clean)
- Stress 1000 commands (`runtime.MemStats` delta < 1MB)
- Row monotonicity (EndRow ≥ StartRow always)
- `WithClock` injection for deterministic timestamps

**`internal/daemon/osc_test.go`** (extend existing):

- `TestParseOSC_133_*` for each code variant
- `TestParseOSC_133_OffsetAccuracy` (offset matches `bytes.Index(data, "\x1b]")`)
- `TestParseOSC_133_Mixed` (133 + 7 + 777 in same chunk, all returned in order)
- `TestParseOSC_BackwardCompat_OSC7` (no regression)
- `TestParseOSC_BackwardCompat_OSC777_ShellReady` (no regression)

**`internal/daemon/service_test.go`** (extend):

- `TestDaemon_OSC133_EmitsEvents` (events on bus when shell emits sequences)
- `TestDaemon_OSC133_RowAccuracy` (row matches actual newline position)
- `TestDaemon_OSC133_Lifecycle` (Register/Unregister called at right moments)
- `TestDaemon_OSC133_NoColdRestore` (events still emit when persistence is off)
- `TestDaemon_OSC133_ColdRestore` (history restored from disk on startup)
- `TestDaemon_OSC133_ColdRestore_InFlight` (in-flight becomes daemon_restart orphan)

**`internal/platform/commands/`**:

- `TestWriter_DebounceCoalesces` (100 Updates in 50ms → 1 file write)
- `TestWriter_FlushOnClose`
- `TestWriter_AtomicRename` (no partial state visible to readers)
- `TestWriter_FailedWrite_GoroutineSurvives`
- `TestLoad_ValidFile`
- `TestLoad_FileNotExist` (returns zero File, nil error)
- `TestLoad_CorruptedJSON` (returns error)
- `TestLoad_VersionFuture` (returns ErrUnsupportedVersion)

**`internal/platform/event/entity_test.go`** (extend):

- `TestType_String_Command_*` for the 3 new types
- `TestType_TypeFromString_Command_*` for the 3 new types
- `TestEvent_MarshalJSON_CommandFinished` (full payload schema verified)

### Smoke / E2E tests (in `test/e2e/`)

- `TestE2E_BashRealCommand` — spawn bash with PS1 emitting OSC 133, run `ls`,
  verify CommandStarted + CommandFinished received via subscription.
- `TestE2E_NonZeroExit` — run `false`, verify `exit_code=1`.
- `TestE2E_OrphanCommand` — kill shell mid-command, verify orphan with
  `orphan_reason=missing_d_marker`.
- `TestE2E_ColdRestore` — spawn 5 commands, `kill -KILL` the daemon, restart
  with `--cold-restore`, verify history visible.

## Cost estimate

Single-track effort, includes review and CI gating:

| Phase | Estimate |
|---|---|
| Spike + verify shell formats | 0.25d |
| Refactor `ParseOSC` for offsets + bench gates | 0.5d |
| `internal/cmdlifecycle/` package + unit tests | 1.5d |
| `internal/platform/commands/` package + tests | 0.75d |
| Daemon wiring (Register, scanOSC, cmdRepository, lineCounters) | 0.75d |
| 3 new event types + JSON serialization tests | 0.25d |
| Cold-restore integration | 0.5d |
| Smoke / E2E + benchmark gates in CI | 1d |
| **Total** | **~5.5d** |

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| `ParseOSC` refactor regresses zero-alloc no-match path | `BenchmarkParseOSC_NoOSC` (existing bench, with new explicit `0 allocs/op` assertion) is the CI gate; failing build blocks merge. Rollback is a one-commit revert. |
| `bytes.Count` adds CPU on hot path | Lazy: only runs when `ParseOSC` returns at least one match. Common case (no OSC) is unchanged. |
| Per-session goroutine for `commands.Writer` scales poorly | wmux already runs ≥3 goroutines per session (readLoop, waitLoop, emulatorLoop). One more is well within Go's scheduling envelope. 1000 sessions = 4000 goroutines is fine. |
| Debounce loses up to 200ms of finished commands on crash | Acceptable. `commands.Writer.Close()` flushes synchronously on session exit and on daemon shutdown. Crash-only window is small. |
| Shell variants emit non-standard OSC 133 | Spike test verifies before code is written. Forward-compat for unknown codes (silently ignored). |
| Watchtower not yet updated | Additive design: new events ride on existing `MsgEvent`; old clients ignore via `default` case. |
| Future `commands.json` schema change | `Version` field; loaders reject unknown future versions cleanly with a logged warning. |
| Concurrent map access in `cmdSvc.trackers` | `sync.RWMutex`: `HandleOSC` holds `RLock`, Register/Unregister hold `Lock`. |

## Out of scope

- **Watchtower-side features** (exit code badges, prompt-jump hotkey,
  triple-click selection, build-failed notifications) — separate Watchtower
  issues.
- **Extended OSC 133 parameters** (`OSC 133;Pn=cmd=ls;Pn=cwd=/foo ST`) —
  not parsed; v1 only handles A/B/C/D.
- **Aggregate statistics** (success rate, avg duration, percentiles) —
  computable from `commands.json`; clients implement if needed.
- **Search / filter API on history** — clients read full history via Snapshot
  IPC or by reading `commands.json` directly.
- **Auto-injecting OSC 133 markers** for shells without integration — too
  invasive; users opt in via shell config (zsh `precmd`, bash `PROMPT_COMMAND`,
  fish `fish_prompt`, etc.).

## Future extensions (post-v1)

- `OSC 133;Pn` parameter parsing for command text capture (lets clients
  display the actual command without re-extracting from scrollback).
- IPC query method `MsgCommandHistory` to fetch `[]Command` for a session
  without going through the file system.
- Integration with `MsgWait` to wait for the next `command.finished` event.
- Configurable `maxHistory` per session via session metadata.
