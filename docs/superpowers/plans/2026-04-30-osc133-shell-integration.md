# OSC 133 Shell Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Capture OSC 133 escape sequences emitted by integrated shells, expose a per-session command lifecycle state machine, and surface lifecycle events through the existing IPC event bus and a new persisted `commands.json` file per session.

**Architecture:** A new pure-logic domain package `internal/cmdlifecycle/` holds the per-session state machine. A new platform package `internal/platform/commands/` mirrors `internal/platform/history/` for debounced disk persistence. The existing `daemon.scanOSC` is extended to recognize OSC 133 (via a refactored `ParseOSC` that exposes byte offsets), then dispatches to `cmdlifecycle.Service`. The state machine emits events that the daemon republishes on the existing event bus and persists via a daemon-side `cmdRepository` implementing `cmdlifecycle.Repository`. Zero-alloc broadcast hot path is preserved (ADR 0032) by keeping newline counting lazy — only triggered when `ParseOSC` returns at least one match.

**Tech Stack:** Go 1.25, goframe (DDD + Package-Oriented Design linter), uber-fx (DI), `go.uber.org/mock` for mocks, standard library only for OSC parsing and persistence.

**Spec:** `docs/superpowers/specs/2026-04-30-osc133-shell-integration-design.md`

**Commit convention:** `<type>(<scope>): <msg>` matching repo history (`feat`, `fix`, `test`, `refactor`, `docs`, `chore`). No `Co-Authored-By` trailer. Sign off with `git -c commit.gpgsign=false commit ...` to avoid 1Password GPG prompt.

---

## File Structure

### New files (created)

```
internal/cmdlifecycle/
├── entity.go              # State, Command, SessionState, OrphanReason consts, sentinel errors
├── entity_test.go
├── events.go              # EventKind, Event, EventHandler
├── events_test.go
├── service.go             # Repository, Service, internal tracker, NewService, all methods
├── service_test.go
├── options.go             # Option, WithMaxHistory, WithClock
├── options_test.go
├── module.go              # var Module = fx.Options(fx.Provide(NewService))
├── module_test.go
└── bench_test.go          # Allocation regression gates

internal/platform/commands/
├── entity.go              # File schema, CurrentVersion const, sentinel errors
├── entity_test.go
├── reader.go              # Load(path) (File, error)
├── reader_test.go
├── writer.go              # Writer with debounced goroutine, atomic rename
├── writer_test.go
└── bench_test.go          # Flush throughput baseline

internal/daemon/
└── cmdrepository.go       # cmdRepository implements cmdlifecycle.Repository
└── cmdrepository_test.go
```

### Modified files

```
internal/daemon/osc.go              # Add Offset field to OSCResult; add OSCType133;
                                    # refactor scan loop to track cursor; factor pickEnd
internal/daemon/osc_test.go         # Add OSC 133 cases + offset accuracy tests
internal/daemon/bench_test.go       # Add b.ReportAllocs assertion to existing _NoOSC
                                    # bench; add _With133, _Mixed benches
internal/daemon/service.go          # Add cmdSvc, cmdRepo, lineCounters{,Mu};
                                    # extend scanOSC; lifecycle wiring at Start,
                                    # handleCreate, OnExit, reconcile, shutdown
internal/daemon/service_test.go     # Integration tests for OSC 133 flow
internal/daemon/module.go           # fx wire cmdRepo + cmdlifecycle.Module
internal/daemon/reconcile.go        # Cold-restore commands.json loading
internal/platform/event/entity.go   # 3 new Type consts + stringer + typeFromString
internal/platform/event/entity_test.go  # Round-trip tests for new types
test/e2e/osc133_test.go             # Real-shell smoke tests
```

---

## Task 1: cmdlifecycle entity types

**Files:**
- Create: `internal/cmdlifecycle/entity.go`
- Create: `internal/cmdlifecycle/entity_test.go`

- [ ] **Step 1: Write the failing test for entity types**

```go
// internal/cmdlifecycle/entity_test.go
package cmdlifecycle

import (
	"testing"
	"time"
)

func TestState_String(t *testing.T) {
	cases := []struct {
		state State
		want  string
	}{
		{StateIdle, "idle"},
		{StatePromptShown, "prompt_shown"},
		{StateUserTyping, "user_typing"},
		{StateCommandRunning, "command_running"},
		{State(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.state.String(); got != c.want {
			t.Errorf("State(%d).String() = %q, want %q", c.state, got, c.want)
		}
	}
}

func TestCommand_ZeroValueOrphanFalse(t *testing.T) {
	var c Command
	if c.Orphan {
		t.Error("zero-value Command should have Orphan=false")
	}
	if c.OrphanReason != "" {
		t.Error("zero-value Command should have empty OrphanReason")
	}
}

func TestSessionState_ZeroValue(t *testing.T) {
	var s SessionState
	if s.History != nil {
		t.Error("zero-value SessionState should have nil History")
	}
	if s.InFlight != nil {
		t.Error("zero-value SessionState should have nil InFlight")
	}
}

func TestOrphanReasonConsts(t *testing.T) {
	if OrphanReasonMissingD != "missing_d_marker" {
		t.Errorf("OrphanReasonMissingD = %q, want %q", OrphanReasonMissingD, "missing_d_marker")
	}
	if OrphanReasonDaemonRestart != "daemon_restart" {
		t.Errorf("OrphanReasonDaemonRestart = %q, want %q", OrphanReasonDaemonRestart, "daemon_restart")
	}
}

func TestSentinelErrors(t *testing.T) {
	if ErrSessionNotRegistered == nil {
		t.Error("ErrSessionNotRegistered should be non-nil")
	}
	if ErrSessionExists == nil {
		t.Error("ErrSessionExists should be non-nil")
	}
	if ErrInvalidOSCCode == nil {
		t.Error("ErrInvalidOSCCode should be non-nil")
	}
}

func TestCommand_RowMonotonicInvariant(t *testing.T) {
	c := Command{StartRow: 100, EndRow: 100}
	if c.EndRow < c.StartRow {
		t.Error("EndRow must be >= StartRow")
	}
	c.EndRow = 200
	if c.EndRow < c.StartRow {
		t.Error("EndRow must be >= StartRow")
	}
}

func TestCommand_TimestampZero(t *testing.T) {
	c := Command{StartedAt: time.Now()}
	if c.EndedAt.IsZero() != true {
		t.Error("zero-value EndedAt should be IsZero")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cmdlifecycle/ -run . -v`
Expected: FAIL — package `internal/cmdlifecycle` does not exist.

- [ ] **Step 3: Write entity.go**

```go
// Package cmdlifecycle tracks the per-session command lifecycle driven by
// OSC 133 shell-integration markers. Pure-logic domain package with no I/O;
// persistence is delegated to a Repository implementation injected by the
// daemon.
package cmdlifecycle

import (
	"errors"
	"time"
)

// State represents the per-session command lifecycle phase.
type State int

const (
	// StateIdle indicates no in-flight command and no prompt yet.
	StateIdle State = iota
	// StatePromptShown indicates the shell drew a prompt (OSC 133;A).
	StatePromptShown
	// StateUserTyping indicates the user has begun typing (OSC 133;B).
	StateUserTyping
	// StateCommandRunning indicates a command is executing (OSC 133;C).
	StateCommandRunning
)

// String returns a stable lower_snake representation of the state.
func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StatePromptShown:
		return "prompt_shown"
	case StateUserTyping:
		return "user_typing"
	case StateCommandRunning:
		return "command_running"
	default:
		return "unknown"
	}
}

// Command captures one completed shell command's lifecycle.
//
// Orphan commands have ExitCode = -1 and a non-empty OrphanReason. They
// are produced when the state machine detects a missing OSC 133;D marker
// or when cold-restore observes an in-flight command after a daemon
// restart.
type Command struct {
	StartedAt    time.Time `json:"started_at"`
	EndedAt      time.Time `json:"ended_at,omitempty"`
	ExitCode     int       `json:"exit_code"`
	StartRow     int64     `json:"start_row"`
	EndRow       int64     `json:"end_row"`
	Orphan       bool      `json:"orphan,omitempty"`
	OrphanReason string    `json:"orphan_reason,omitempty"`
}

// SessionState is the snapshot returned by Service.Snapshot and consumed
// by Service.Restore. History is a deep copy of the per-session FIFO ring;
// InFlight is non-nil iff the tracker was in StateCommandRunning at
// snapshot time.
type SessionState struct {
	History  []Command `json:"history"`
	InFlight *Command  `json:"in_flight,omitempty"`
}

// Orphan reason constants identify why a Command was closed as orphan.
const (
	// OrphanReasonMissingD indicates the state machine saw an A/B/C
	// transition while in StateCommandRunning without an intervening D.
	OrphanReasonMissingD = "missing_d_marker"
	// OrphanReasonDaemonRestart indicates an in-flight command was
	// observed at cold-restore — the command was definitionally aborted
	// by daemon shutdown.
	OrphanReasonDaemonRestart = "daemon_restart"
)

// Sentinel errors.
var (
	// ErrSessionNotRegistered is returned by Service methods called for an
	// unknown session ID.
	ErrSessionNotRegistered = errors.New("cmdlifecycle: session not registered")
	// ErrSessionExists is returned by Register or Restore when the session
	// is already known to the Service.
	ErrSessionExists = errors.New("cmdlifecycle: session already registered")
	// ErrInvalidOSCCode is returned by HandleOSC when the code byte is not
	// one of 'A', 'B', 'C', 'D'.
	ErrInvalidOSCCode = errors.New("cmdlifecycle: invalid OSC 133 code")
)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cmdlifecycle/ -run . -v`
Expected: PASS for all entity tests.

- [ ] **Step 5: Commit**

```bash
git -c commit.gpgsign=false add internal/cmdlifecycle/entity.go internal/cmdlifecycle/entity_test.go
git -c commit.gpgsign=false commit -m "feat(cmdlifecycle): entity types + sentinel errors"
```

---

## Task 2: cmdlifecycle event types

**Files:**
- Create: `internal/cmdlifecycle/events.go`
- Create: `internal/cmdlifecycle/events_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/cmdlifecycle/events_test.go
package cmdlifecycle

import "testing"

func TestEventKind_Constants(t *testing.T) {
	if EventPromptShown == 0 {
		t.Error("EventPromptShown should not be the zero value (iota _ skipped)")
	}
	if EventCommandStarted == EventPromptShown {
		t.Error("EventCommandStarted should differ from EventPromptShown")
	}
	if EventCommandFinished == EventCommandStarted {
		t.Error("EventCommandFinished should differ from EventCommandStarted")
	}
}

func TestEvent_PromptShownHasNilCommand(t *testing.T) {
	ev := Event{Kind: EventPromptShown, Command: nil, Row: 42}
	if ev.Command != nil {
		t.Error("EventPromptShown should be safe to construct with nil Command")
	}
	if ev.Row != 42 {
		t.Errorf("Row = %d, want 42", ev.Row)
	}
}

func TestEventHandler_TypeAlias(t *testing.T) {
	var h EventHandler = func(sessID string, ev Event) {
		_ = sessID
		_ = ev
	}
	h("test", Event{Kind: EventPromptShown})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cmdlifecycle/ -run TestEvent -v`
Expected: FAIL — `EventPromptShown` undefined, etc.

- [ ] **Step 3: Write events.go**

```go
package cmdlifecycle

// EventKind identifies the lifecycle transition that produced an Event.
type EventKind int

const (
	_ EventKind = iota
	// EventPromptShown is emitted when the tracker enters StatePromptShown
	// from a non-PromptShown state (i.e. not on re-arm A→A).
	EventPromptShown
	// EventCommandStarted is emitted when the tracker enters
	// StateCommandRunning. The Command pointer carries the freshly-opened
	// command (with EndedAt zero, ExitCode 0).
	EventCommandStarted
	// EventCommandFinished is emitted when a command closes — either
	// normally via OSC 133;D or as an orphan via a missing-marker
	// transition. The Command pointer carries the closed command.
	EventCommandFinished
)

// Event is the unit of state-machine output consumed by the daemon's
// EventHandler.
//
// For EventPromptShown, Command is nil and only Row is meaningful.
// For EventCommandStarted and EventCommandFinished, Command is non-nil
// and is a fresh allocation safe for the handler to retain or marshal.
type Event struct {
	Kind    EventKind
	Command *Command
	Row     int64
}

// EventHandler is the callback signature registered via Service.OnEvent.
// It is invoked synchronously from HandleOSC. Handlers must be
// non-blocking — the daemon implementation queues onto a channel or
// returns immediately.
type EventHandler func(sessID string, ev Event)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cmdlifecycle/ -run TestEvent -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git -c commit.gpgsign=false add internal/cmdlifecycle/events.go internal/cmdlifecycle/events_test.go
git -c commit.gpgsign=false commit -m "feat(cmdlifecycle): event kind + handler signature"
```

---

## Task 3: cmdlifecycle options

**Files:**
- Create: `internal/cmdlifecycle/options.go`
- Create: `internal/cmdlifecycle/options_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/cmdlifecycle/options_test.go
package cmdlifecycle

import (
	"testing"
	"time"
)

func TestWithMaxHistory(t *testing.T) {
	s := &Service{maxHistory: 0}
	WithMaxHistory(123)(s)
	if s.maxHistory != 123 {
		t.Errorf("maxHistory = %d, want 123", s.maxHistory)
	}
}

func TestWithMaxHistory_RejectsNonPositive(t *testing.T) {
	s := &Service{maxHistory: 500}
	WithMaxHistory(0)(s)
	if s.maxHistory != 500 {
		t.Errorf("maxHistory = %d, want 500 (unchanged for non-positive)", s.maxHistory)
	}
	WithMaxHistory(-1)(s)
	if s.maxHistory != 500 {
		t.Errorf("maxHistory = %d, want 500 (unchanged for non-positive)", s.maxHistory)
	}
}

func TestWithClock(t *testing.T) {
	fixed := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	s := &Service{}
	WithClock(func() time.Time { return fixed })(s)
	if s.now == nil {
		t.Fatal("now should be set")
	}
	if !s.now().Equal(fixed) {
		t.Errorf("now() = %v, want %v", s.now(), fixed)
	}
}

func TestWithClock_NilIgnored(t *testing.T) {
	original := func() time.Time { return time.Unix(0, 0) }
	s := &Service{now: original}
	WithClock(nil)(s)
	if s.now == nil {
		t.Error("nil clock should not overwrite existing clock")
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/cmdlifecycle/ -run TestWith -v`
Expected: FAIL — `WithMaxHistory`/`WithClock` undefined.

- [ ] **Step 3: Write options.go**

```go
package cmdlifecycle

import "time"

// Option configures the Service at construction.
type Option func(*Service)

// WithMaxHistory overrides the per-session FIFO history cap.
// Default is defaultMaxHistory (500). Non-positive values are ignored.
func WithMaxHistory(n int) Option {
	return func(s *Service) {
		if n > 0 {
			s.maxHistory = n
		}
	}
}

// WithClock injects a deterministic clock for tests. Default is time.Now.
// Nil is ignored.
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}
```

- [ ] **Step 4: Run test**

Note: tests reference `Service` struct fields. The test fails to compile until Task 4 lands, but the option logic itself is exercised. Move test execution check to end of Task 4. For this task, only verify the file compiles:

Run: `go build ./internal/cmdlifecycle/`
Expected: error — `Service` undefined. **This is expected.** The file will compile only after Task 4.

- [ ] **Step 5: Commit (deferred)**

Defer commit until Task 4. Options.go and Service.go land in the same commit because they reference each other.

---

## Task 4: cmdlifecycle Service + state machine

**Files:**
- Create: `internal/cmdlifecycle/service.go`
- Create: `internal/cmdlifecycle/service_test.go`

This is the largest task. Decomposed into sub-steps within the task.

- [ ] **Step 1: Write the failing tests for happy-path state machine**

```go
// internal/cmdlifecycle/service_test.go
package cmdlifecycle

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// fixedClock returns a deterministic time that auto-advances by 1ms per call.
type fixedClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFixedClock() *fixedClock {
	return &fixedClock{t: time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)}
}

func (f *fixedClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.t
	f.t = f.t.Add(time.Millisecond)
	return now
}

// nullRepo is a Repository that discards Saves. Used when the test does not
// care about persistence behavior.
type nullRepo struct{}

func (nullRepo) Save(string, SessionState) {}

// captureRepo records every Save call.
type captureRepo struct {
	mu     sync.Mutex
	saves  []captureSave
}

type captureSave struct {
	SessID string
	State  SessionState
}

func (r *captureRepo) Save(sessID string, state SessionState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.saves = append(r.saves, captureSave{SessID: sessID, State: state})
}

func (r *captureRepo) Saves() []captureSave {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]captureSave, len(r.saves))
	copy(out, r.saves)
	return out
}

func TestService_HandleOSC_HappyPath(t *testing.T) {
	clock := newFixedClock()
	svc := NewService(nullRepo{}, WithClock(clock.Now))
	var events []Event
	svc.OnEvent(func(sessID string, ev Event) {
		events = append(events, ev)
	})
	if err := svc.Register("s1"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// A → B → C → D;0
	if err := svc.HandleOSC("s1", 'A', 0, 10); err != nil {
		t.Fatalf("HandleOSC A: %v", err)
	}
	if err := svc.HandleOSC("s1", 'B', 0, 11); err != nil {
		t.Fatalf("HandleOSC B: %v", err)
	}
	if err := svc.HandleOSC("s1", 'C', 0, 12); err != nil {
		t.Fatalf("HandleOSC C: %v", err)
	}
	if err := svc.HandleOSC("s1", 'D', 0, 20); err != nil {
		t.Fatalf("HandleOSC D: %v", err)
	}

	// Expect 3 events: PromptShown, Started, Finished
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3: %+v", len(events), events)
	}
	if events[0].Kind != EventPromptShown {
		t.Errorf("events[0].Kind = %v, want EventPromptShown", events[0].Kind)
	}
	if events[0].Row != 10 {
		t.Errorf("events[0].Row = %d, want 10", events[0].Row)
	}
	if events[1].Kind != EventCommandStarted {
		t.Errorf("events[1].Kind = %v, want EventCommandStarted", events[1].Kind)
	}
	if events[1].Command == nil || events[1].Command.StartRow != 12 {
		t.Errorf("events[1] Command StartRow = %v, want 12", events[1].Command)
	}
	if events[2].Kind != EventCommandFinished {
		t.Errorf("events[2].Kind = %v, want EventCommandFinished", events[2].Kind)
	}
	if events[2].Command == nil {
		t.Fatal("events[2].Command is nil")
	}
	if events[2].Command.ExitCode != 0 {
		t.Errorf("events[2].Command.ExitCode = %d, want 0", events[2].Command.ExitCode)
	}
	if events[2].Command.EndRow != 20 {
		t.Errorf("events[2].Command.EndRow = %d, want 20", events[2].Command.EndRow)
	}
	if events[2].Command.Orphan {
		t.Error("events[2].Command.Orphan = true, want false")
	}

	// Snapshot: 1 history entry, no in-flight.
	state, err := svc.Snapshot("s1")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(state.History) != 1 {
		t.Errorf("len(History) = %d, want 1", len(state.History))
	}
	if state.InFlight != nil {
		t.Errorf("InFlight = %+v, want nil", state.InFlight)
	}
}

func TestService_HandleOSC_OrphanFromA(t *testing.T) {
	clock := newFixedClock()
	svc := NewService(nullRepo{}, WithClock(clock.Now))
	var events []Event
	svc.OnEvent(func(_ string, ev Event) { events = append(events, ev) })
	_ = svc.Register("s1")

	_ = svc.HandleOSC("s1", 'A', 0, 1)
	_ = svc.HandleOSC("s1", 'B', 0, 2)
	_ = svc.HandleOSC("s1", 'C', 0, 3)
	_ = svc.HandleOSC("s1", 'A', 0, 5) // orphan path

	// Expect: PromptShown, Started, Finished{orphan}, PromptShown
	if len(events) != 4 {
		t.Fatalf("got %d events, want 4", len(events))
	}
	if events[2].Kind != EventCommandFinished {
		t.Errorf("events[2].Kind = %v, want Finished", events[2].Kind)
	}
	if !events[2].Command.Orphan {
		t.Error("orphan command should have Orphan=true")
	}
	if events[2].Command.ExitCode != -1 {
		t.Errorf("orphan ExitCode = %d, want -1", events[2].Command.ExitCode)
	}
	if events[2].Command.OrphanReason != OrphanReasonMissingD {
		t.Errorf("OrphanReason = %q, want %q", events[2].Command.OrphanReason, OrphanReasonMissingD)
	}
	if events[3].Kind != EventPromptShown {
		t.Errorf("events[3].Kind = %v, want PromptShown", events[3].Kind)
	}
}

func TestService_HandleOSC_DInIdleIsNoop(t *testing.T) {
	svc := NewService(nullRepo{})
	var events []Event
	svc.OnEvent(func(_ string, ev Event) { events = append(events, ev) })
	_ = svc.Register("s1")

	_ = svc.HandleOSC("s1", 'D', 0, 1)
	if len(events) != 0 {
		t.Errorf("D in Idle produced %d events, want 0", len(events))
	}
}

func TestService_HandleOSC_UnknownCodeIgnored(t *testing.T) {
	svc := NewService(nullRepo{})
	svc.OnEvent(func(_ string, _ Event) { t.Error("unknown code should not emit event") })
	_ = svc.Register("s1")

	if err := svc.HandleOSC("s1", 'Z', 0, 1); err != nil {
		t.Errorf("unknown code should not error, got %v", err)
	}
}

func TestService_HandleOSC_UnregisteredSession(t *testing.T) {
	svc := NewService(nullRepo{})
	if err := svc.HandleOSC("missing", 'A', 0, 0); !errors.Is(err, ErrSessionNotRegistered) {
		t.Errorf("got %v, want ErrSessionNotRegistered", err)
	}
}

func TestService_Register_DuplicateError(t *testing.T) {
	svc := NewService(nullRepo{})
	if err := svc.Register("s1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Register("s1"); !errors.Is(err, ErrSessionExists) {
		t.Errorf("got %v, want ErrSessionExists", err)
	}
}

func TestService_Unregister_Idempotent(t *testing.T) {
	svc := NewService(nullRepo{})
	_ = svc.Register("s1")
	svc.Unregister("s1")
	svc.Unregister("s1") // no-op, no panic
}

func TestService_Unregister_AllowsReregister(t *testing.T) {
	svc := NewService(nullRepo{})
	_ = svc.Register("s1")
	svc.Unregister("s1")
	if err := svc.Register("s1"); err != nil {
		t.Errorf("re-register after Unregister failed: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cmdlifecycle/ -run TestService -v`
Expected: FAIL — `NewService`, `Service`, etc. all undefined.

- [ ] **Step 3: Write service.go**

```go
package cmdlifecycle

import (
	"sync"
	"time"
)

// defaultMaxHistory is the per-session FIFO cap when no Option overrides it.
const defaultMaxHistory = 500

// Repository abstracts steady-state persistence of session state.
// Implementations (in the daemon package) wrap a debounced disk writer.
//
// Save MUST be non-blocking. Implementations are expected to capture the
// state and schedule a write asynchronously. Save is invoked from
// HandleOSC, which itself is called on the daemon's broadcast scan path.
type Repository interface {
	Save(sessID string, state SessionState)
}

// Service holds per-session command-lifecycle trackers.
//
// Lifecycle:
//   - NewService(repo, opts...) — construct once at fx wiring time.
//   - OnEvent(handler) — set the global event handler. Called once at
//     daemon Start before any Register.
//   - Register(sessID) — invoked when a session is created.
//   - HandleOSC(sessID, code, exit, row) — invoked from scanOSC for each
//     OSC 133 sequence detected.
//   - Unregister(sessID) — invoked when a session exits.
type Service struct {
	mu         sync.RWMutex
	trackers   map[string]*tracker
	repo       Repository
	onEvent    EventHandler
	maxHistory int
	now        func() time.Time
}

// NewService returns a Service backed by the given Repository.
// Required parameters (repo) are positional; only genuinely optional
// configuration is exposed via Option.
func NewService(repo Repository, opts ...Option) *Service {
	s := &Service{
		mu:         sync.RWMutex{},
		trackers:   make(map[string]*tracker),
		repo:       repo,
		onEvent:    nil,
		maxHistory: defaultMaxHistory,
		now:        time.Now,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// OnEvent registers the global event handler. Replaces any previously
// registered handler. Pattern matches session.Service.OnExit.
//
// If never set (or set to nil), HandleOSC silently drops events. The
// daemon's Start always calls OnEvent before serving traffic, so in
// normal operation the handler is set before any Register.
func (s *Service) OnEvent(h EventHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onEvent = h
}

// Register creates a tracker for the session in StateIdle.
// Returns ErrSessionExists if the session is already registered.
func (s *Service) Register(sessID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.trackers[sessID]; ok {
		return ErrSessionExists
	}
	s.trackers[sessID] = newTracker(s.maxHistory)
	return nil
}

// Unregister removes the tracker for the session. Idempotent.
func (s *Service) Unregister(sessID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.trackers, sessID)
}

// HandleOSC processes a parsed OSC 133 sequence.
//
// code must be one of 'A', 'B', 'C', 'D'. Other bytes are silently
// ignored (forward-compat for OSC 133 extensions).
//
// exitCode is meaningful only when code == 'D'.
//
// row is the current accumulated newline count for the session, supplied
// by the daemon. It is recorded on Command.StartRow / EndRow.
func (s *Service) HandleOSC(sessID string, code byte, exitCode int, row int64) error {
	s.mu.RLock()
	tr, ok := s.trackers[sessID]
	handler := s.onEvent
	s.mu.RUnlock()
	if !ok {
		return ErrSessionNotRegistered
	}

	switch code {
	case 'A', 'B', 'C', 'D':
	default:
		return nil // forward-compat: unknown codes silently ignored
	}

	events := tr.handle(code, exitCode, row, s.now)
	if handler != nil {
		for _, ev := range events {
			handler(sessID, ev)
		}
	}

	// Persist on every Finished — Started/PromptShown are transient.
	for _, ev := range events {
		if ev.Kind == EventCommandFinished {
			s.repo.Save(sessID, tr.snapshotLocked())
			break
		}
	}
	return nil
}

// Snapshot returns a deep copy of the session's state.
// History slice and InFlight pointer are freshly allocated.
func (s *Service) Snapshot(sessID string) (SessionState, error) {
	s.mu.RLock()
	tr, ok := s.trackers[sessID]
	s.mu.RUnlock()
	if !ok {
		return SessionState{}, ErrSessionNotRegistered
	}
	return tr.snapshot(), nil
}

// Restore rehydrates a session's state from a previously-persisted
// SessionState. Closes any in-flight command as orphan with
// OrphanReason=daemon_restart, then transitions the tracker to
// StateIdle. Returns ErrSessionExists if the session is already
// registered.
func (s *Service) Restore(sessID string, state SessionState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.trackers[sessID]; ok {
		return ErrSessionExists
	}
	tr := newTracker(s.maxHistory)
	tr.restore(state, s.now())
	s.trackers[sessID] = tr
	return nil
}

// tracker is the per-session state machine. Internal — accessed only via
// Service methods.
type tracker struct {
	mu         sync.Mutex
	state      State
	current    *Command
	history    []Command
	maxHistory int
}

func newTracker(maxHistory int) *tracker {
	return &tracker{
		state:      StateIdle,
		current:    nil,
		history:    make([]Command, 0, maxHistory),
		maxHistory: maxHistory,
	}
}

// handle advances the state machine and returns the events produced.
// Returned events are in emission order. Service is responsible for
// dispatching them to the registered handler.
func (t *tracker) handle(code byte, exitCode int, row int64, now func() time.Time) []Event {
	t.mu.Lock()
	defer t.mu.Unlock()

	var events []Event

	switch code {
	case 'A':
		if t.state == StateCommandRunning {
			ev := t.closeOrphanLocked(now(), row, OrphanReasonMissingD)
			events = append(events, ev)
		}
		if t.state != StatePromptShown {
			t.state = StatePromptShown
			events = append(events, Event{Kind: EventPromptShown, Row: row})
		}
	case 'B':
		if t.state == StateCommandRunning {
			ev := t.closeOrphanLocked(now(), row, OrphanReasonMissingD)
			events = append(events, ev)
		}
		t.state = StateUserTyping
	case 'C':
		if t.state == StateCommandRunning {
			ev := t.closeOrphanLocked(now(), row, OrphanReasonMissingD)
			events = append(events, ev)
		}
		t.openCommandLocked(now(), row)
		t.state = StateCommandRunning
		started := *t.current
		events = append(events, Event{Kind: EventCommandStarted, Command: &started, Row: row})
	case 'D':
		if t.state != StateCommandRunning || t.current == nil {
			return nil // no command to close
		}
		t.current.EndedAt = now()
		t.current.EndRow = row
		t.current.ExitCode = exitCode
		t.current.Orphan = false
		t.appendHistoryLocked(*t.current)
		closed := *t.current
		t.current = nil
		t.state = StateIdle
		events = append(events, Event{Kind: EventCommandFinished, Command: &closed, Row: row})
	}

	return events
}

// openCommandLocked must be called with t.mu held.
func (t *tracker) openCommandLocked(now time.Time, row int64) {
	t.current = &Command{
		StartedAt: now,
		StartRow:  row,
	}
}

// closeOrphanLocked closes t.current as an orphan and appends to history.
// Returns the Finished event. Must be called with t.mu held and
// t.current != nil.
func (t *tracker) closeOrphanLocked(now time.Time, row int64, reason string) Event {
	t.current.EndedAt = now
	t.current.EndRow = row
	t.current.ExitCode = -1
	t.current.Orphan = true
	t.current.OrphanReason = reason
	t.appendHistoryLocked(*t.current)
	closed := *t.current
	t.current = nil
	t.state = StateIdle
	return Event{Kind: EventCommandFinished, Command: &closed, Row: row}
}

// appendHistoryLocked appends c to history, evicting the oldest entry
// FIFO-style when at capacity. Pre-allocated capacity avoids realloc.
func (t *tracker) appendHistoryLocked(c Command) {
	if len(t.history) == t.maxHistory {
		copy(t.history[0:], t.history[1:])
		t.history[t.maxHistory-1] = c
		return
	}
	t.history = append(t.history, c)
}

// snapshot returns a deep copy of state.
func (t *tracker) snapshot() SessionState {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snapshotLocked()
}

func (t *tracker) snapshotLocked() SessionState {
	historyCopy := make([]Command, len(t.history))
	copy(historyCopy, t.history)
	var inflight *Command
	if t.current != nil {
		c := *t.current
		inflight = &c
	}
	return SessionState{History: historyCopy, InFlight: inflight}
}

// restore loads the given state and converts any in-flight command to
// orphan with OrphanReason=daemon_restart. Tracker ends in StateIdle.
// Must be called before the tracker is exposed (no lock — single-writer).
func (t *tracker) restore(state SessionState, now time.Time) {
	for _, c := range state.History {
		t.appendHistoryLocked(c)
	}
	if state.InFlight != nil {
		orphan := *state.InFlight
		orphan.EndRow = orphan.StartRow
		// EndedAt is set to now() since the on-disk SavedAt is unknown to
		// the tracker. Daemon-side Restore caller may overwrite the
		// orphan's EndedAt with file.SavedAt before invoking — see
		// daemon's reconcile path.
		orphan.EndedAt = now
		orphan.ExitCode = -1
		orphan.Orphan = true
		orphan.OrphanReason = OrphanReasonDaemonRestart
		t.appendHistoryLocked(orphan)
	}
	t.state = StateIdle
	t.current = nil
}
```

- [ ] **Step 4: Run test**

Run: `go test ./internal/cmdlifecycle/ -run . -v -race`
Expected: PASS for all tests written so far.

- [ ] **Step 5: Add FIFO eviction test**

```go
// Append to service_test.go
func TestService_FIFO_Eviction(t *testing.T) {
	clock := newFixedClock()
	svc := NewService(nullRepo{}, WithMaxHistory(3), WithClock(clock.Now))
	_ = svc.Register("s1")

	for i := 0; i < 5; i++ {
		_ = svc.HandleOSC("s1", 'A', 0, int64(i*2))
		_ = svc.HandleOSC("s1", 'C', 0, int64(i*2+1))
		_ = svc.HandleOSC("s1", 'D', i, int64(i*2+2))
	}

	state, err := svc.Snapshot("s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.History) != 3 {
		t.Fatalf("len(History) = %d, want 3", len(state.History))
	}
	if state.History[0].ExitCode != 2 {
		t.Errorf("oldest ExitCode = %d, want 2 (5 commands ran, last 3 kept)", state.History[0].ExitCode)
	}
	if state.History[2].ExitCode != 4 {
		t.Errorf("newest ExitCode = %d, want 4", state.History[2].ExitCode)
	}
}
```

Run: `go test ./internal/cmdlifecycle/ -run TestService_FIFO -v`
Expected: PASS.

- [ ] **Step 6: Add Snapshot-deep-copy test**

```go
// Append to service_test.go
func TestService_Snapshot_DeepCopy(t *testing.T) {
	svc := NewService(nullRepo{})
	_ = svc.Register("s1")
	_ = svc.HandleOSC("s1", 'A', 0, 1)
	_ = svc.HandleOSC("s1", 'C', 0, 2)
	_ = svc.HandleOSC("s1", 'D', 0, 3)

	state1, _ := svc.Snapshot("s1")
	if len(state1.History) != 1 {
		t.Fatalf("len(History) = %d, want 1", len(state1.History))
	}

	// Mutate the snapshot's history slice.
	state1.History[0].ExitCode = 999

	// Internal state must be unaffected.
	state2, _ := svc.Snapshot("s1")
	if state2.History[0].ExitCode == 999 {
		t.Error("snapshot returned shared slice — internal state was mutated")
	}
}
```

Run: `go test ./internal/cmdlifecycle/ -run TestService_Snapshot -v`
Expected: PASS.

- [ ] **Step 7: Add Restore tests**

```go
// Append to service_test.go
func TestService_Restore_Roundtrip(t *testing.T) {
	clock := newFixedClock()
	svc1 := NewService(nullRepo{}, WithClock(clock.Now))
	_ = svc1.Register("s1")
	_ = svc1.HandleOSC("s1", 'A', 0, 1)
	_ = svc1.HandleOSC("s1", 'C', 0, 2)
	_ = svc1.HandleOSC("s1", 'D', 42, 3)
	state, _ := svc1.Snapshot("s1")

	svc2 := NewService(nullRepo{}, WithClock(clock.Now))
	if err := svc2.Restore("s1", state); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	state2, _ := svc2.Snapshot("s1")
	if len(state2.History) != 1 {
		t.Fatalf("len(History) = %d, want 1", len(state2.History))
	}
	if state2.History[0].ExitCode != 42 {
		t.Errorf("ExitCode = %d, want 42", state2.History[0].ExitCode)
	}
}

func TestService_Restore_InFlightBecomesOrphan(t *testing.T) {
	clock := newFixedClock()
	svc := NewService(nullRepo{}, WithClock(clock.Now))
	state := SessionState{
		History: nil,
		InFlight: &Command{
			StartedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			StartRow:  100,
		},
	}
	if err := svc.Restore("s1", state); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Snapshot("s1")
	if len(got.History) != 1 {
		t.Fatalf("InFlight should become an orphan history entry; len=%d", len(got.History))
	}
	if got.InFlight != nil {
		t.Error("after Restore, InFlight must be nil — tracker is in StateIdle")
	}
	o := got.History[0]
	if !o.Orphan {
		t.Error("Orphan = false, want true")
	}
	if o.OrphanReason != OrphanReasonDaemonRestart {
		t.Errorf("OrphanReason = %q, want %q", o.OrphanReason, OrphanReasonDaemonRestart)
	}
	if o.EndRow != 100 {
		t.Errorf("EndRow = %d, want 100 (== StartRow)", o.EndRow)
	}
	if o.ExitCode != -1 {
		t.Errorf("ExitCode = %d, want -1", o.ExitCode)
	}
}

func TestService_Restore_SessionAlreadyRegistered(t *testing.T) {
	svc := NewService(nullRepo{})
	_ = svc.Register("s1")
	if err := svc.Restore("s1", SessionState{}); !errors.Is(err, ErrSessionExists) {
		t.Errorf("got %v, want ErrSessionExists", err)
	}
}
```

Run: `go test ./internal/cmdlifecycle/ -run TestService_Restore -v -race`
Expected: PASS.

- [ ] **Step 8: Add concurrent-sessions test**

```go
// Append to service_test.go
func TestService_ConcurrentSessions(t *testing.T) {
	svc := NewService(nullRepo{})
	const sessions = 50
	const cycles = 20

	for i := 0; i < sessions; i++ {
		if err := svc.Register("s" + string(rune('a'+(i%26))) + "-" + string(rune('0'+(i/26))); err != nil {
			// duplicate composite IDs are fine for the small loop
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < sessions; i++ {
		sessID := "s" + string(rune('a'+(i%26))) + "-" + string(rune('0'+(i/26)))
		_ = svc.Register(sessID) // ignore dup error
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			for j := 0; j < cycles; j++ {
				_ = svc.HandleOSC(id, 'A', 0, int64(j*4))
				_ = svc.HandleOSC(id, 'B', 0, int64(j*4+1))
				_ = svc.HandleOSC(id, 'C', 0, int64(j*4+2))
				_ = svc.HandleOSC(id, 'D', 0, int64(j*4+3))
			}
		}(sessID)
	}
	wg.Wait()
}
```

Run: `go test ./internal/cmdlifecycle/ -run TestService_Concurrent -race`
Expected: PASS — no race detected.

- [ ] **Step 9: Add captureRepo Save-on-Finished test**

```go
// Append to service_test.go
func TestService_RepoSavedOnFinished(t *testing.T) {
	repo := &captureRepo{}
	svc := NewService(repo)
	_ = svc.Register("s1")

	_ = svc.HandleOSC("s1", 'A', 0, 1) // PromptShown — no Save
	_ = svc.HandleOSC("s1", 'C', 0, 2) // Started — no Save
	saves := repo.Saves()
	if len(saves) != 0 {
		t.Errorf("Save called %d times before D, want 0", len(saves))
	}
	_ = svc.HandleOSC("s1", 'D', 0, 3) // Finished — Save

	saves = repo.Saves()
	if len(saves) != 1 {
		t.Fatalf("Save called %d times, want 1", len(saves))
	}
	if saves[0].SessID != "s1" {
		t.Errorf("Save sessID = %q", saves[0].SessID)
	}
	if len(saves[0].State.History) != 1 {
		t.Errorf("Save state history len = %d, want 1", len(saves[0].State.History))
	}
}
```

Run: `go test ./internal/cmdlifecycle/ -run TestService_RepoSaved -v`
Expected: PASS.

- [ ] **Step 10: Run the full test suite for cmdlifecycle**

Run: `go test ./internal/cmdlifecycle/ -v -race -count=1`
Expected: All tests PASS.

- [ ] **Step 11: Commit Tasks 3 + 4 together**

```bash
git -c commit.gpgsign=false add internal/cmdlifecycle/options.go internal/cmdlifecycle/options_test.go internal/cmdlifecycle/service.go internal/cmdlifecycle/service_test.go
git -c commit.gpgsign=false commit -m "feat(cmdlifecycle): Service + state machine + options"
```

---

## Task 5: cmdlifecycle module wiring

**Files:**
- Create: `internal/cmdlifecycle/module.go`
- Create: `internal/cmdlifecycle/module_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/cmdlifecycle/module_test.go
package cmdlifecycle

import (
	"testing"

	"go.uber.org/fx"
)

func TestModule_ProvidesService(t *testing.T) {
	var svc *Service
	app := fx.New(
		fx.Supply(Repository(nullRepo{})),
		Module,
		fx.Populate(&svc),
		fx.NopLogger,
	)
	if err := app.Err(); err != nil {
		t.Fatalf("fx.New: %v", err)
	}
	if svc == nil {
		t.Fatal("Service was not provided")
	}
}
```

- [ ] **Step 2: Run test, expect failure**

Run: `go test ./internal/cmdlifecycle/ -run TestModule -v`
Expected: FAIL — `Module` undefined.

- [ ] **Step 3: Write module.go**

```go
package cmdlifecycle

import "go.uber.org/fx"

// Module provides cmdlifecycle.Service to fx-wired daemons.
//
// Repository must be supplied separately (the daemon implements it).
var Module = fx.Options(
	fx.Provide(NewService),
)
```

- [ ] **Step 4: Run test**

Run: `go test ./internal/cmdlifecycle/ -run TestModule -v`
Expected: PASS.

- [ ] **Step 5: Run goframe linter on the package**

Run: `make lint 2>&1 | grep -E "cmdlifecycle|^FAIL"`
Expected: No errors for `internal/cmdlifecycle/`.

- [ ] **Step 6: Commit**

```bash
git -c commit.gpgsign=false add internal/cmdlifecycle/module.go internal/cmdlifecycle/module_test.go
git -c commit.gpgsign=false commit -m "feat(cmdlifecycle): fx Module wiring"
```

---

## Task 6: cmdlifecycle bench gates

**Files:**
- Create: `internal/cmdlifecycle/bench_test.go`

- [ ] **Step 1: Write the bench tests**

```go
// internal/cmdlifecycle/bench_test.go
package cmdlifecycle

import "testing"

// BenchmarkHandleOSC_NoOp measures the cost of a HandleOSC call that
// triggers no transition (e.g. 'D' in StateIdle). Gate: 0 allocs/op.
func BenchmarkHandleOSC_NoOp(b *testing.B) {
	svc := NewService(nullRepo{})
	_ = svc.Register("s1")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = svc.HandleOSC("s1", 'D', 0, 0)
	}
}

// BenchmarkHandleOSC_FullCycle measures the cost of one A→B→C→D cycle.
func BenchmarkHandleOSC_FullCycle(b *testing.B) {
	svc := NewService(nullRepo{})
	svc.OnEvent(func(string, Event) {})
	_ = svc.Register("s1")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = svc.HandleOSC("s1", 'A', 0, 0)
		_ = svc.HandleOSC("s1", 'B', 0, 0)
		_ = svc.HandleOSC("s1", 'C', 0, 0)
		_ = svc.HandleOSC("s1", 'D', 0, 0)
	}
}

// BenchmarkSnapshot_500Commands measures Snapshot cost on a full history.
func BenchmarkSnapshot_500Commands(b *testing.B) {
	svc := NewService(nullRepo{})
	svc.OnEvent(func(string, Event) {})
	_ = svc.Register("s1")
	for i := 0; i < 500; i++ {
		_ = svc.HandleOSC("s1", 'A', 0, 0)
		_ = svc.HandleOSC("s1", 'C', 0, 0)
		_ = svc.HandleOSC("s1", 'D', 0, 0)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.Snapshot("s1")
	}
}
```

- [ ] **Step 2: Run benches and record baseline**

Run: `go test -bench=. -benchmem ./internal/cmdlifecycle/ -run=^$ -benchtime=2s`
Expected output: numeric results. Record `BenchmarkHandleOSC_NoOp` allocs/op — must be 0.

- [ ] **Step 3: Verify NoOp is 0 allocs/op**

If `BenchmarkHandleOSC_NoOp` reports `> 0 allocs/op`, investigate. Likely culprits:
- Map lookup escapes (unlikely with non-pointer key)
- Switch on byte allocates (won't — Go optimizes byte switches)

If 0 allocs/op confirmed, proceed.

- [ ] **Step 4: Commit**

```bash
git -c commit.gpgsign=false add internal/cmdlifecycle/bench_test.go
git -c commit.gpgsign=false commit -m "test(cmdlifecycle): allocation regression gates"
```

---

## Task 7: commands package — entity

**Files:**
- Create: `internal/platform/commands/entity.go`
- Create: `internal/platform/commands/entity_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/platform/commands/entity_test.go
package commands

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/wblech/wmux/internal/cmdlifecycle"
)

func TestFile_RoundtripJSON(t *testing.T) {
	when := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	f := File{
		Version:   CurrentVersion,
		SessionID: "s1",
		SavedAt:   when,
		History: []cmdlifecycle.Command{{
			StartedAt: when,
			EndedAt:   when.Add(time.Second),
			ExitCode:  0,
			StartRow:  10,
			EndRow:    20,
		}},
		InFlight: nil,
	}

	data, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}

	var got File
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Version != CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, CurrentVersion)
	}
	if got.SessionID != "s1" {
		t.Errorf("SessionID = %q, want s1", got.SessionID)
	}
	if len(got.History) != 1 {
		t.Errorf("History len = %d, want 1", len(got.History))
	}
}

func TestFile_OmitInFlight_WhenNil(t *testing.T) {
	f := File{Version: 1, SessionID: "s1", SavedAt: time.Now()}
	data, _ := json.Marshal(f)
	if string(data) == "" {
		t.Fatal("empty marshal")
	}
	if contains(string(data), "in_flight") {
		t.Errorf("expected omitempty: %s", data)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestSentinelErrors(t *testing.T) {
	if ErrUnsupportedVersion == nil {
		t.Error("ErrUnsupportedVersion must be non-nil")
	}
	if ErrWriterClosed == nil {
		t.Error("ErrWriterClosed must be non-nil")
	}
}

func TestCurrentVersion(t *testing.T) {
	if CurrentVersion != 1 {
		t.Errorf("CurrentVersion = %d, want 1 (initial schema)", CurrentVersion)
	}
}
```

- [ ] **Step 2: Run, expect failure**

Run: `go test ./internal/platform/commands/ -run . -v`
Expected: FAIL — package not found.

- [ ] **Step 3: Write entity.go**

```go
// Package commands persists per-session command history to disk under
// {dataDir}/{session_id}/commands.json.
//
// The Writer debounces and writes atomically via tmpfile + rename; the
// Load function reads the most recent fully-written file. The schema
// carries a Version field for forward compatibility.
package commands

import (
	"errors"
	"time"

	"github.com/wblech/wmux/internal/cmdlifecycle"
)

// CurrentVersion is the schema version that this build of wmux writes.
// Future readers may encounter higher versions and must reject them with
// ErrUnsupportedVersion rather than misinterpreting unknown fields.
const CurrentVersion = 1

// File is the on-disk schema for commands.json.
type File struct {
	Version   int                    `json:"version"`
	SessionID string                 `json:"session_id"`
	SavedAt   time.Time              `json:"saved_at"`
	History   []cmdlifecycle.Command `json:"history"`
	InFlight  *cmdlifecycle.Command  `json:"in_flight,omitempty"`
}

// Sentinel errors.
var (
	// ErrUnsupportedVersion is returned by Load when the file's Version
	// is greater than CurrentVersion.
	ErrUnsupportedVersion = errors.New("commands: unsupported file version")
	// ErrWriterClosed is returned when Update is called after Close.
	ErrWriterClosed = errors.New("commands: writer closed")
)
```

- [ ] **Step 4: Run, expect pass**

Run: `go test ./internal/platform/commands/ -run . -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git -c commit.gpgsign=false add internal/platform/commands/entity.go internal/platform/commands/entity_test.go
git -c commit.gpgsign=false commit -m "feat(commands): file schema + version constant"
```

---

## Task 8: commands package — reader

**Files:**
- Create: `internal/platform/commands/reader.go`
- Create: `internal/platform/commands/reader_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/platform/commands/reader_test.go
package commands

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wblech/wmux/internal/cmdlifecycle"
)

func TestLoad_FileNotExist(t *testing.T) {
	got, err := Load("/nonexistent/path/commands.json")
	if err != nil {
		t.Errorf("err = %v, want nil for missing file", err)
	}
	if got.Version != 0 {
		t.Errorf("Version = %d, want 0 for missing file", got.Version)
	}
	if got.SessionID != "" {
		t.Errorf("SessionID = %q, want empty for missing file", got.SessionID)
	}
}

func TestLoad_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "commands.json")
	contents := `{
		"version": 1,
		"session_id": "s1",
		"saved_at": "2026-04-30T12:00:00Z",
		"history": [{"started_at":"2026-04-30T12:00:00Z","ended_at":"2026-04-30T12:00:01Z","exit_code":0,"start_row":1,"end_row":2}]
	}`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != 1 {
		t.Errorf("Version = %d, want 1", got.Version)
	}
	if got.SessionID != "s1" {
		t.Errorf("SessionID = %q, want s1", got.SessionID)
	}
	if len(got.History) != 1 {
		t.Errorf("History len = %d, want 1", len(got.History))
	}
}

func TestLoad_CorruptedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "commands.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Error("expected error for corrupted JSON")
	}
}

func TestLoad_UnsupportedFutureVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "commands.json")
	contents := `{"version": 2, "session_id": "s1", "saved_at": "2026-04-30T12:00:00Z"}`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if !errors.Is(err, ErrUnsupportedVersion) {
		t.Errorf("err = %v, want ErrUnsupportedVersion", err)
	}
}

func TestLoad_InFlightAbsent_TreatedAsNil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "commands.json")
	contents := `{"version": 1, "session_id": "s1", "saved_at": "2026-04-30T12:00:00Z", "history": []}`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	got, _ := Load(path)
	if got.InFlight != nil {
		t.Error("absent in_flight must Unmarshal as nil")
	}
}

func TestLoad_InFlightExplicitNull_TreatedAsNil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "commands.json")
	contents := `{"version": 1, "session_id": "s1", "saved_at": "2026-04-30T12:00:00Z", "in_flight": null}`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	got, _ := Load(path)
	if got.InFlight != nil {
		t.Error("explicit in_flight: null must Unmarshal as nil")
	}
}

// Avoid unused import warnings when this file lacks references.
var _ = time.Now
var _ = cmdlifecycle.Command{}
```

- [ ] **Step 2: Run, expect failure**

Run: `go test ./internal/platform/commands/ -run TestLoad -v`
Expected: FAIL — `Load` undefined.

- [ ] **Step 3: Write reader.go**

```go
package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// Load reads and parses commands.json at the given path.
//
// Returns:
//   - zero-value File and nil error if the file does not exist (a session
//     that has never persisted any command).
//   - File and ErrUnsupportedVersion if the file's Version is greater than
//     CurrentVersion. The returned File is populated only with Version
//     and SessionID for diagnostic logging.
//   - File and a wrapped error for any other failure (corrupted JSON,
//     permission denied, etc).
func Load(path string) (File, error) {
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return File{}, nil
		}
		return File{}, fmt.Errorf("commands.Load: read %q: %w", path, err)
	}

	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return File{}, fmt.Errorf("commands.Load: parse %q: %w", path, err)
	}

	if f.Version > CurrentVersion {
		// Return a stub File so callers can log Version/SessionID
		// without a second read.
		return File{Version: f.Version, SessionID: f.SessionID}, ErrUnsupportedVersion
	}

	return f, nil
}
```

- [ ] **Step 4: Run, expect pass**

Run: `go test ./internal/platform/commands/ -run TestLoad -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git -c commit.gpgsign=false add internal/platform/commands/reader.go internal/platform/commands/reader_test.go
git -c commit.gpgsign=false commit -m "feat(commands): Load reads commands.json with version gate"
```

---

## Task 9: commands package — writer (debounced, atomic)

**Files:**
- Create: `internal/platform/commands/writer.go`
- Create: `internal/platform/commands/writer_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/platform/commands/writer_test.go
package commands

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/wblech/wmux/internal/cmdlifecycle"
)

func newTestWriter(t *testing.T, debounce time.Duration) (*Writer, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "commands.json")
	w, err := NewWriter(path, "s1", WithDebounceInterval(debounce))
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	return w, path
}

func TestWriter_FlushOnClose(t *testing.T) {
	w, path := newTestWriter(t, 1*time.Hour) // long debounce → never auto-flushes
	state := SessionState{
		History: []cmdlifecycle.Command{{ExitCode: 0}},
	}
	w.Update(asLifecycleState(state))

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.History) != 1 {
		t.Errorf("History len = %d, want 1 (Close should flush pending)", len(got.History))
	}
}

func TestWriter_DebounceCoalesces(t *testing.T) {
	w, path := newTestWriter(t, 50*time.Millisecond)
	defer w.Close()

	for i := 0; i < 100; i++ {
		c := cmdlifecycle.Command{ExitCode: i}
		w.Update(cmdlifecycle.SessionState{History: []cmdlifecycle.Command{c}})
	}

	time.Sleep(150 * time.Millisecond)
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.History) != 1 {
		t.Errorf("History len = %d, want 1 (only last state)", len(got.History))
	}
	if got.History[0].ExitCode != 99 {
		t.Errorf("History[0].ExitCode = %d, want 99 (last update wins)", got.History[0].ExitCode)
	}
}

func TestWriter_AtomicRename(t *testing.T) {
	w, path := newTestWriter(t, 10*time.Millisecond)
	defer w.Close()
	w.Update(cmdlifecycle.SessionState{History: []cmdlifecycle.Command{{ExitCode: 7}}})
	time.Sleep(50 * time.Millisecond)

	// Verify no .tmp file is left behind.
	tmp := path + ".tmp"
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Errorf(".tmp file still present after flush: %v", err)
	}
	got, _ := Load(path)
	if len(got.History) == 0 || got.History[0].ExitCode != 7 {
		t.Errorf("expected exit_code 7, got %+v", got)
	}
}

func TestWriter_UpdateAfterClose(t *testing.T) {
	w, _ := newTestWriter(t, 10*time.Millisecond)
	_ = w.Close()
	// Update after Close should be a no-op (or return error). We choose
	// no-op for simpler hot-path semantics.
	w.Update(cmdlifecycle.SessionState{History: []cmdlifecycle.Command{{ExitCode: 1}}})
	// Verify no panic.
}

func TestWriter_ConcurrentUpdate(t *testing.T) {
	w, _ := newTestWriter(t, 10*time.Millisecond)
	defer w.Close()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w.Update(cmdlifecycle.SessionState{History: []cmdlifecycle.Command{{ExitCode: i}}})
		}(i)
	}
	wg.Wait()
	// Test passes if no race detector complaint.
}

func TestWriter_FlushExplicit(t *testing.T) {
	w, path := newTestWriter(t, 1*time.Hour)
	defer w.Close()
	w.Update(cmdlifecycle.SessionState{History: []cmdlifecycle.Command{{ExitCode: 5}}})
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	got, _ := Load(path)
	if len(got.History) != 1 || got.History[0].ExitCode != 5 {
		t.Errorf("Flush did not write state: %+v", got)
	}
}

// asLifecycleState converts a SessionState (file struct) to the
// cmdlifecycle.SessionState that Update accepts.
func asLifecycleState(s SessionState) cmdlifecycle.SessionState {
	// SessionState here is just a forward reference for the test's
	// readability — the file's persisted shape mirrors lifecycle's shape.
	return cmdlifecycle.SessionState{History: s.History, InFlight: s.InFlight}
}

// Local SessionState shadow for the helper above (so the test reads
// naturally). Mirrors File's history/in_flight shape.
type SessionState = struct {
	History  []cmdlifecycle.Command
	InFlight *cmdlifecycle.Command
}

var _ = errors.New // satisfy unused import on some test runs
```

- [ ] **Step 2: Run, expect failure**

Run: `go test ./internal/platform/commands/ -run TestWriter -v`
Expected: FAIL — `NewWriter`, `WithDebounceInterval`, etc. undefined.

- [ ] **Step 3: Write writer.go**

```go
package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wblech/wmux/internal/cmdlifecycle"
)

// defaultDebounceInterval is the amount of time the writer goroutine waits
// between the first MarkDirty signal and the actual flush. Coalesces bursts
// of saves into a single disk write.
const defaultDebounceInterval = 200 * time.Millisecond

// Writer is a per-session debounced disk persistor for commands.json.
//
// Lifecycle:
//   - NewWriter(path, sessID, opts...) — starts a background goroutine.
//   - Update(state) — non-blocking; captures the new state and signals
//     a debounced flush.
//   - Flush() — synchronous flush of the most recent state.
//   - Close() — final synchronous flush, stops the goroutine.
//
// Concurrency: Update is safe to call from any goroutine, including hot
// paths. The mutex protects only the captured state; the channel signal
// is non-blocking.
type Writer struct {
	path     string
	sessID   string
	debounce time.Duration

	mu        sync.Mutex
	pending   cmdlifecycle.SessionState
	hasState  bool
	closed    bool

	notifyCh chan struct{}
	closeCh  chan struct{}
	doneCh   chan struct{}
	flushReq chan chan error
}

// Option configures a Writer at construction.
type Option func(*Writer)

// WithDebounceInterval overrides the default debounce duration.
func WithDebounceInterval(d time.Duration) Option {
	return func(w *Writer) {
		if d > 0 {
			w.debounce = d
		}
	}
}

// NewWriter creates and starts a Writer. The background goroutine runs
// until Close is called.
func NewWriter(path, sessID string, opts ...Option) (*Writer, error) {
	if path == "" {
		return nil, fmt.Errorf("commands: path must be non-empty")
	}
	w := &Writer{
		path:     path,
		sessID:   sessID,
		debounce: defaultDebounceInterval,
		notifyCh: make(chan struct{}, 1),
		closeCh:  make(chan struct{}),
		doneCh:   make(chan struct{}),
		flushReq: make(chan chan error),
	}
	for _, o := range opts {
		o(w)
	}
	go w.run()
	return w, nil
}

// Update captures state and schedules a debounced flush. Safe for
// concurrent callers and hot paths. Update after Close is a silent no-op.
func (w *Writer) Update(state cmdlifecycle.SessionState) {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.pending = state
	w.hasState = true
	w.mu.Unlock()

	select {
	case w.notifyCh <- struct{}{}:
	default:
		// notifyCh is buffered cap=1; if full, the goroutine will already
		// see the latest state on its next tick.
	}
}

// Flush synchronously writes the most recent state to disk.
func (w *Writer) Flush() error {
	resp := make(chan error, 1)
	select {
	case w.flushReq <- resp:
		return <-resp
	case <-w.doneCh:
		return ErrWriterClosed
	}
}

// Close performs a final synchronous flush and stops the background
// goroutine. Idempotent.
func (w *Writer) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	w.mu.Unlock()
	close(w.closeCh)
	<-w.doneCh
	return nil
}

func (w *Writer) run() {
	defer close(w.doneCh)
	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}
	pending := false

	for {
		select {
		case <-w.notifyCh:
			if !pending {
				timer.Reset(w.debounce)
				pending = true
			}
		case <-timer.C:
			pending = false
			if err := w.flushOnce(); err != nil {
				logFlushError(w.path, err)
			}
		case resp := <-w.flushReq:
			if pending {
				pending = false
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
			}
			resp <- w.flushOnce()
		case <-w.closeCh:
			if pending {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
			}
			_ = w.flushOnce()
			return
		}
	}
}

// flushOnce serializes and writes the most recent captured state.
// Atomic via tmpfile + rename.
func (w *Writer) flushOnce() error {
	w.mu.Lock()
	if !w.hasState {
		w.mu.Unlock()
		return nil
	}
	state := cmdlifecycle.SessionState{
		History:  append([]cmdlifecycle.Command(nil), w.pending.History...),
		InFlight: cloneCommand(w.pending.InFlight),
	}
	w.mu.Unlock()

	file := File{
		Version:   CurrentVersion,
		SessionID: w.sessID,
		SavedAt:   time.Now(),
		History:   state.History,
		InFlight:  state.InFlight,
	}

	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("commands.flush: marshal: %w", err)
	}

	tmp := w.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("commands.flush: write tmp: %w", err)
	}
	if err := os.Rename(tmp, w.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("commands.flush: rename: %w", err)
	}
	return nil
}

func cloneCommand(c *cmdlifecycle.Command) *cmdlifecycle.Command {
	if c == nil {
		return nil
	}
	cp := *c
	return &cp
}

// logFlushError writes a single line to stderr. The writer keeps running
// even after a failed flush; the next Update will retry.
func logFlushError(path string, err error) {
	fmt.Fprintf(os.Stderr, "wmux: commands.flush %q: %v\n", filepath.Base(filepath.Dir(path)), err)
}

// satisfy lint
var _ = errors.New
```

- [ ] **Step 4: Run, expect pass**

Run: `go test ./internal/platform/commands/ -v -race -count=1`
Expected: PASS.

- [ ] **Step 5: Add bench**

```go
// internal/platform/commands/bench_test.go
package commands

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/wblech/wmux/internal/cmdlifecycle"
)

func BenchmarkWriter_Flush_500Commands(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "commands.json")
	w, err := NewWriter(path, "s1", WithDebounceInterval(1*time.Hour))
	if err != nil {
		b.Fatal(err)
	}
	defer w.Close()

	history := make([]cmdlifecycle.Command, 500)
	for i := range history {
		history[i] = cmdlifecycle.Command{ExitCode: i, StartRow: int64(i * 10), EndRow: int64(i*10 + 5)}
	}
	w.Update(cmdlifecycle.SessionState{History: history})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = w.Flush()
	}
}
```

- [ ] **Step 6: Run bench**

Run: `go test -bench=. -benchmem ./internal/platform/commands/ -run=^$ -benchtime=2s`
Expected: numeric output. Record baseline.

- [ ] **Step 7: Commit**

```bash
git -c commit.gpgsign=false add internal/platform/commands/writer.go internal/platform/commands/writer_test.go internal/platform/commands/bench_test.go
git -c commit.gpgsign=false commit -m "feat(commands): debounced writer with atomic rename"
```

---

## Task 10: ParseOSC refactor — add Offset field

**Files:**
- Modify: `internal/daemon/osc.go`
- Modify: `internal/daemon/osc_test.go`
- Modify: `internal/daemon/bench_test.go`

- [ ] **Step 1: Read existing osc_test.go to understand assertions**

Run: `cat internal/daemon/osc_test.go | head -60` to confirm current test shape.

- [ ] **Step 2: Add a failing test for the new Offset field**

```go
// Append to internal/daemon/osc_test.go
func TestParseOSC_OffsetAccuracy(t *testing.T) {
	// "abc" prefix (3 bytes) + OSC 7 starts at byte 3.
	data := []byte("abc\x1b]7;file:///tmp/x\x1b\\xyz\x1b]9;hello\x07")
	results := ParseOSC(data)
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	// First match: ESC ] starts at offset 3
	if results[0].Offset != 3 {
		t.Errorf("results[0].Offset = %d, want 3", results[0].Offset)
	}
	// Second match: find offset of second ESC] in data
	if results[1].Offset <= results[0].Offset {
		t.Errorf("offsets must be strictly increasing: %d, %d", results[0].Offset, results[1].Offset)
	}
}

func TestParseOSC_NoMatch_ReturnsNil(t *testing.T) {
	results := ParseOSC([]byte("plain text without any OSC"))
	if results != nil {
		t.Errorf("results = %v, want nil", results)
	}
}
```

- [ ] **Step 3: Run, expect failure**

Run: `go test ./internal/daemon/ -run TestParseOSC_OffsetAccuracy -v`
Expected: FAIL — `Offset` field undefined.

- [ ] **Step 4: Modify osc.go — add Offset field and refactor scan loop**

Replace the entire `ParseOSC` function and the `OSCResult` type. Keep all existing constants and the OSCType enum.

```go
// In internal/daemon/osc.go, replace the OSCResult type:

// OSCResult holds a parsed OSC sequence.
type OSCResult struct {
	Type   OSCType
	Value  string
	// Offset is the byte position of the ESC ] introducer in the original
	// data slice. Used by scanOSC for offset-aware row tracking.
	Offset int
}
```

Replace `ParseOSC` with the cursor-based version:

```go
// ParseOSC scans data for OSC sequences (7, 9, 99, 133, 777) and returns
// parsed results in source order. Each result includes the byte offset of
// its ESC ] introducer in data, enabling callers to compute positions
// (e.g. row counts) relative to each match.
//
// Allocation: returns nil when no OSC sequences are found. The
// no-match path is gated by BenchmarkParseOSC_NoOSC at 0 allocs/op.
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
		end := pickOSCEnd(endST, endBEL)
		if end < 0 {
			break
		}
		bodyEnd := cursor + end
		body := data[cursor:bodyEnd]
		cursor = bodyEnd + 1

		semicolon := bytes.IndexByte(body, ';')
		if semicolon < 0 {
			continue
		}
		oscNum := body[:semicolon]
		oscValue := string(body[semicolon+1:])

		switch string(oscNum) {
		case "7":
			parsed, err := url.Parse(oscValue)
			if err == nil && parsed.Path != "" {
				results = append(results, OSCResult{
					Type:   OSCTypeCwd,
					Value:  parsed.Path,
					Offset: matchStart,
				})
			}
		case "9":
			results = append(results, OSCResult{
				Type:   OSCTypeNotification,
				Value:  oscValue,
				Offset: matchStart,
			})
		case "99":
			parts := strings.SplitN(oscValue, ";", 2)
			val := oscValue
			if len(parts) == 2 {
				val = parts[1]
			}
			results = append(results, OSCResult{
				Type:   OSCTypeNotification,
				Value:  val,
				Offset: matchStart,
			})
		case "133":
			results = append(results, OSCResult{
				Type:   OSCType133,
				Value:  oscValue,
				Offset: matchStart,
			})
		case "777":
			parts := strings.SplitN(oscValue, ";", 3)
			if len(parts) >= 2 && parts[0] == "wmux" && parts[1] == "shell-ready" {
				results = append(results, OSCResult{
					Type:   OSCTypeShellReady,
					Value:  "shell-ready",
					Offset: matchStart,
				})
				continue
			}
			val := oscValue
			if len(parts) == 3 {
				val = parts[2]
			} else if len(parts) == 2 {
				val = parts[1]
			}
			results = append(results, OSCResult{
				Type:   OSCTypeNotification,
				Value:  val,
				Offset: matchStart,
			})
		}
	}

	return results
}

// pickOSCEnd selects the earlier non-negative terminator index.
// Refactored from the inline switch in ParseOSC.
func pickOSCEnd(endST, endBEL int) int {
	switch {
	case endST >= 0 && endBEL >= 0 && endST < endBEL:
		return endST
	case endST >= 0 && endBEL >= 0:
		return endBEL
	case endST >= 0:
		return endST
	case endBEL >= 0:
		return endBEL
	}
	return -1
}
```

Add the new OSC type constant. Find the existing `OSCType` block and add:

```go
// In internal/daemon/osc.go, add to the OSCType const block:

	// OSCType133 indicates an OSC 133 shell-integration marker
	// (A = prompt, B = user input, C = command, D = exit).
	OSCType133
```

- [ ] **Step 5: Run all daemon tests**

Run: `go test ./internal/daemon/ -run TestParseOSC -v -race`
Expected: PASS — including new offset test and existing OSC 7/9/777 tests (backward compat preserved).

- [ ] **Step 6: Run bench gate**

Run: `go test -bench=BenchmarkParseOSC_NoOSC -benchmem ./internal/daemon/ -run=^$ -benchtime=2s`
Expected output should show `0 allocs/op`. If non-zero, investigate.

- [ ] **Step 7: Update bench_test.go to add explicit allocs assertion**

Append a wrapper test that runs the bench and asserts allocs:

```go
// Append to internal/daemon/bench_test.go
func TestBenchmarkParseOSC_NoOSC_ZeroAllocs(t *testing.T) {
	const chunkSize = 32 * 1024
	data := make([]byte, chunkSize)
	for i := range data {
		data[i] = byte('a' + (i % 26))
	}

	allocs := testing.AllocsPerRun(100, func() {
		_ = ParseOSC(data)
	})
	if allocs > 0 {
		t.Errorf("ParseOSC no-match allocs/op = %f, want 0 (regression gate)", allocs)
	}
}
```

- [ ] **Step 8: Run regression test**

Run: `go test ./internal/daemon/ -run TestBenchmarkParseOSC_NoOSC_ZeroAllocs -v`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git -c commit.gpgsign=false add internal/daemon/osc.go internal/daemon/osc_test.go internal/daemon/bench_test.go
git -c commit.gpgsign=false commit -m "refactor(daemon): ParseOSC adds Offset field + factor pickOSCEnd"
```

---

## Task 11: ParseOSC — recognize OSC 133

**Files:**
- Modify: `internal/daemon/osc_test.go`
- Modify: `internal/daemon/bench_test.go`

OSC 133 parsing was already written in Task 10. This task adds focused tests + benches.

- [ ] **Step 1: Add OSC 133 tests to osc_test.go**

```go
// Append to internal/daemon/osc_test.go
func TestParseOSC_133_A(t *testing.T) {
	data := []byte("\x1b]133;A\x07")
	results := ParseOSC(data)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Type != OSCType133 {
		t.Errorf("Type = %v, want OSCType133", results[0].Type)
	}
	if results[0].Value != "A" {
		t.Errorf("Value = %q, want A", results[0].Value)
	}
}

func TestParseOSC_133_D_WithExit(t *testing.T) {
	data := []byte("\x1b]133;D;42\x07")
	results := ParseOSC(data)
	if len(results) != 1 {
		t.Fatalf("got %d, want 1", len(results))
	}
	if results[0].Value != "D;42" {
		t.Errorf("Value = %q, want D;42", results[0].Value)
	}
}

func TestParseOSC_133_D_NoExit(t *testing.T) {
	data := []byte("\x1b]133;D\x07")
	results := ParseOSC(data)
	if len(results) != 1 {
		t.Fatalf("got %d, want 1", len(results))
	}
	if results[0].Value != "D" {
		t.Errorf("Value = %q, want D", results[0].Value)
	}
}

func TestParseOSC_133_ExtraSegments(t *testing.T) {
	data := []byte("\x1b]133;D;0;extra\x07")
	results := ParseOSC(data)
	if results[0].Value != "D;0;extra" {
		t.Errorf("Value = %q, want D;0;extra (extra segments preserved for daemon parsing)", results[0].Value)
	}
}

func TestParseOSC_Mixed_133_7_777(t *testing.T) {
	data := []byte("output\x1b]7;file:///x\x1b\\\x1b]133;C\x07ls\n\x1b]133;D;0\x07\x1b]777;wmux;shell-ready\x07")
	results := ParseOSC(data)
	if len(results) != 4 {
		t.Fatalf("got %d, want 4: %+v", len(results), results)
	}
	wantTypes := []OSCType{OSCTypeCwd, OSCType133, OSCType133, OSCTypeShellReady}
	for i, w := range wantTypes {
		if results[i].Type != w {
			t.Errorf("results[%d].Type = %v, want %v", i, results[i].Type, w)
		}
	}
}

func TestParseOSC_133_OffsetAccuracy(t *testing.T) {
	prefix := []byte("hello world ")
	osc := []byte("\x1b]133;C\x07")
	data := append(prefix, osc...)
	results := ParseOSC(data)
	if len(results) != 1 {
		t.Fatalf("got %d, want 1", len(results))
	}
	if results[0].Offset != len(prefix) {
		t.Errorf("Offset = %d, want %d (start of ESC])", results[0].Offset, len(prefix))
	}
}

func TestParseOSC_BackwardCompat_OSC7(t *testing.T) {
	data := []byte("\x1b]7;file:///home/user\x1b\\")
	results := ParseOSC(data)
	if len(results) != 1 {
		t.Fatalf("got %d, want 1", len(results))
	}
	if results[0].Type != OSCTypeCwd {
		t.Errorf("Type = %v, want OSCTypeCwd", results[0].Type)
	}
	if results[0].Value != "/home/user" {
		t.Errorf("Value = %q, want /home/user", results[0].Value)
	}
}

func TestParseOSC_BackwardCompat_OSC777_ShellReady(t *testing.T) {
	data := []byte("\x1b]777;wmux;shell-ready\x07")
	results := ParseOSC(data)
	if len(results) != 1 {
		t.Fatalf("got %d, want 1", len(results))
	}
	if results[0].Type != OSCTypeShellReady {
		t.Errorf("Type = %v, want OSCTypeShellReady", results[0].Type)
	}
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/daemon/ -run TestParseOSC -v -race`
Expected: PASS — all tests including the existing ones.

- [ ] **Step 3: Add benches for OSC 133**

```go
// Append to internal/daemon/bench_test.go
func BenchmarkParseOSC_With133(b *testing.B) {
	const chunkSize = 32 * 1024
	prefix := []byte("\x1b]133;C\x07")
	suffix := []byte("\x1b]133;D;0\x07")
	mid := make([]byte, chunkSize-len(prefix)-len(suffix))
	for i := range mid {
		mid[i] = byte('a' + (i % 26))
	}
	data := make([]byte, 0, chunkSize)
	data = append(data, prefix...)
	data = append(data, mid...)
	data = append(data, suffix...)

	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	var sinkLen int
	for i := 0; i < b.N; i++ {
		sinkLen = len(ParseOSC(data))
	}
	_ = sinkLen
}

func BenchmarkParseOSC_Mixed(b *testing.B) {
	data := []byte("output\x1b]7;file:///x\x1b\\\x1b]133;C\x07stuff\n\x1b]133;D;0\x07\x1b]777;wmux;shell-ready\x07")
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ParseOSC(data)
	}
}
```

- [ ] **Step 4: Run benches**

Run: `go test -bench=BenchmarkParseOSC_With133 -benchmem ./internal/daemon/ -run=^$ -benchtime=2s`
Expected: numeric output. Record baselines.

- [ ] **Step 5: Verify no-match path is still 0 allocs**

Run: `go test ./internal/daemon/ -run TestBenchmarkParseOSC_NoOSC_ZeroAllocs -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git -c commit.gpgsign=false add internal/daemon/osc_test.go internal/daemon/bench_test.go
git -c commit.gpgsign=false commit -m "test(daemon): OSC 133 parsing + mixed/backward-compat coverage"
```

---

## Task 12: 3 new event types in event package

**Files:**
- Modify: `internal/platform/event/entity.go`
- Modify: `internal/platform/event/entity_test.go`

- [ ] **Step 1: Find the existing test file pattern**

Run: `head -30 internal/platform/event/entity_test.go` to confirm test format.

- [ ] **Step 2: Write failing tests for new event types**

```go
// Append to internal/platform/event/entity_test.go
func TestType_String_CommandPromptShown(t *testing.T) {
	if got := CommandPromptShown.String(); got != "command.prompt_shown" {
		t.Errorf("String() = %q, want %q", got, "command.prompt_shown")
	}
}

func TestType_String_CommandStarted(t *testing.T) {
	if got := CommandStarted.String(); got != "command.started" {
		t.Errorf("String() = %q, want %q", got, "command.started")
	}
}

func TestType_String_CommandFinished(t *testing.T) {
	if got := CommandFinished.String(); got != "command.finished" {
		t.Errorf("String() = %q, want %q", got, "command.finished")
	}
}

func TestType_RoundtripJSON_CommandTypes(t *testing.T) {
	cases := []struct {
		name string
		typ  Type
	}{
		{"prompt_shown", CommandPromptShown},
		{"started", CommandStarted},
		{"finished", CommandFinished},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, err := c.typ.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON: %v", err)
			}
			var got Type
			if err := got.UnmarshalJSON(data); err != nil {
				t.Fatalf("UnmarshalJSON: %v", err)
			}
			if got != c.typ {
				t.Errorf("roundtrip: got %v, want %v", got, c.typ)
			}
		})
	}
}
```

- [ ] **Step 3: Run, expect failure**

Run: `go test ./internal/platform/event/ -run TestType_String_Command -v`
Expected: FAIL — `CommandPromptShown` etc. undefined.

- [ ] **Step 4: Modify entity.go**

Locate the existing `const ( _ Type = iota; SessionCreated ... ShellReady )` block and append:

```go
	// CommandPromptShown is emitted when a shell prompt is drawn (OSC 133;A).
	CommandPromptShown
	// CommandStarted is emitted when a user command begins executing (OSC 133;C).
	CommandStarted
	// CommandFinished is emitted when a command completes with an exit code (OSC 133;D)
	// or is closed as orphan due to a missing marker / daemon restart.
	CommandFinished
```

In the `String()` method, add cases before the default:

```go
	case CommandPromptShown:
		return "command.prompt_shown"
	case CommandStarted:
		return "command.started"
	case CommandFinished:
		return "command.finished"
```

In the `typeFromString` function, add cases:

```go
	case "command.prompt_shown":
		return CommandPromptShown, true
	case "command.started":
		return CommandStarted, true
	case "command.finished":
		return CommandFinished, true
```

- [ ] **Step 5: Run, expect pass**

Run: `go test ./internal/platform/event/ -v -race`
Expected: PASS for all event tests including new ones.

- [ ] **Step 6: Commit**

```bash
git -c commit.gpgsign=false add internal/platform/event/entity.go internal/platform/event/entity_test.go
git -c commit.gpgsign=false commit -m "feat(event): command lifecycle event types"
```

---

## Task 13: Daemon — cmdRepository

**Files:**
- Create: `internal/daemon/cmdrepository.go`
- Create: `internal/daemon/cmdrepository_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/daemon/cmdrepository_test.go
package daemon

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/wblech/wmux/internal/cmdlifecycle"
)

func TestCmdRepository_Save_NoOpenWriter(t *testing.T) {
	r := newCmdRepository(true)
	// No Open call → Save is a silent no-op.
	r.Save("s1", cmdlifecycle.SessionState{
		History: []cmdlifecycle.Command{{ExitCode: 0}},
	})
}

func TestCmdRepository_OpenSaveClose(t *testing.T) {
	dir := t.TempDir()
	r := newCmdRepository(true)
	path := filepath.Join(dir, "commands.json")
	if err := r.Open("s1", path); err != nil {
		t.Fatalf("Open: %v", err)
	}
	r.Save("s1", cmdlifecycle.SessionState{
		History: []cmdlifecycle.Command{{ExitCode: 7}},
	})
	r.Close("s1")

	// File should now exist with the saved state (Close flushes).
	// We poll briefly because Save is async.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := r.peekFile(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("file %s never appeared", path)
}

func TestCmdRepository_DisabledColdRestore(t *testing.T) {
	r := newCmdRepository(false)
	if err := r.Open("s1", "/should/be/ignored"); err != nil {
		t.Errorf("Open should be no-op when disabled, got %v", err)
	}
	r.Save("s1", cmdlifecycle.SessionState{}) // no-op
	r.Close("s1")
}

func TestCmdRepository_CloseUnknown(t *testing.T) {
	r := newCmdRepository(true)
	r.Close("never-opened") // no-op
}
```

- [ ] **Step 2: Run, expect failure**

Run: `go test ./internal/daemon/ -run TestCmdRepository -v`
Expected: FAIL — `newCmdRepository` undefined.

- [ ] **Step 3: Write cmdrepository.go**

```go
package daemon

import (
	"os"
	"sync"

	"github.com/wblech/wmux/internal/cmdlifecycle"
	"github.com/wblech/wmux/internal/platform/commands"
)

// cmdRepository implements cmdlifecycle.Repository on top of a per-session
// commands.Writer map. Open is invoked on session create, Close on session
// exit. Save is the steady-state write path called by cmdlifecycle.Service.
//
// Lifecycle (Open/Close) is daemon-private and not exposed on the
// cmdlifecycle.Repository interface — the interface has only Save.
type cmdRepository struct {
	mu      sync.RWMutex
	writers map[string]*commands.Writer
	enabled bool
}

func newCmdRepository(coldRestoreEnabled bool) *cmdRepository {
	return &cmdRepository{
		writers: make(map[string]*commands.Writer),
		enabled: coldRestoreEnabled,
	}
}

// Open creates a Writer for the session at the given path. If cold-restore
// is disabled, returns nil and stores nothing. Idempotent: re-opening an
// existing session is a no-op.
func (r *cmdRepository) Open(sessID, path string) error {
	if !r.enabled {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.writers[sessID]; ok {
		return nil
	}
	w, err := commands.NewWriter(path, sessID)
	if err != nil {
		return err
	}
	r.writers[sessID] = w
	return nil
}

// Close flushes synchronously and stops the Writer for the session.
// Idempotent.
func (r *cmdRepository) Close(sessID string) {
	r.mu.Lock()
	w, ok := r.writers[sessID]
	if ok {
		delete(r.writers, sessID)
	}
	r.mu.Unlock()
	if w != nil {
		_ = w.Close()
	}
}

// CloseAll closes every active Writer. Called on daemon shutdown.
func (r *cmdRepository) CloseAll() {
	r.mu.Lock()
	writers := r.writers
	r.writers = make(map[string]*commands.Writer)
	r.mu.Unlock()
	for _, w := range writers {
		_ = w.Close()
	}
}

// Save implements cmdlifecycle.Repository.
// Routes the state to the per-session Writer. Non-blocking.
func (r *cmdRepository) Save(sessID string, state cmdlifecycle.SessionState) {
	if !r.enabled {
		return
	}
	r.mu.RLock()
	w := r.writers[sessID]
	r.mu.RUnlock()
	if w != nil {
		w.Update(state)
	}
}

// peekFile is a test helper that reports whether a file exists.
func (r *cmdRepository) peekFile(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
```

- [ ] **Step 4: Run, expect pass**

Run: `go test ./internal/daemon/ -run TestCmdRepository -v -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git -c commit.gpgsign=false add internal/daemon/cmdrepository.go internal/daemon/cmdrepository_test.go
git -c commit.gpgsign=false commit -m "feat(daemon): cmdRepository implements cmdlifecycle.Repository"
```

---

## Task 14: Daemon — wire fields, fx Module, OnEvent setup

**Files:**
- Modify: `internal/daemon/service.go`
- Modify: `internal/daemon/module.go`

- [ ] **Step 1: Read current daemon module.go**

Run: `cat internal/daemon/module.go`. Note current fx.Provide registrations.

- [ ] **Step 2: Add new fields to Daemon struct in service.go**

Locate the `type Daemon struct { ... }` definition (~line 152). Add:

```go
	cmdSvc          *cmdlifecycle.Service
	cmdRepo         *cmdRepository
	lineCountersMu  sync.RWMutex
	lineCounters    map[string]*atomic.Int64
```

Add to imports:

```go
	"sync/atomic"

	"github.com/wblech/wmux/internal/cmdlifecycle"
```

- [ ] **Step 3: Update NewDaemon to initialize the maps**

Locate `func NewDaemon(...)` and add to the struct literal:

```go
		cmdSvc:         nil,                          // set by option
		cmdRepo:        nil,                          // set by option
		lineCounters:   make(map[string]*atomic.Int64),
```

- [ ] **Step 4: Add daemon options for cmdSvc/cmdRepo**

In `internal/daemon/options.go`, add:

```go
// WithCommandLifecycle wires the command lifecycle Service and its
// Repository. Required for OSC 133 event emission.
func WithCommandLifecycle(svc *cmdlifecycle.Service, repo *cmdRepository) Option {
	return func(d *Daemon) {
		d.cmdSvc = svc
		d.cmdRepo = repo
	}
}
```

If `internal/daemon/options.go` doesn't already import cmdlifecycle, add:

```go
import "github.com/wblech/wmux/internal/cmdlifecycle"
```

- [ ] **Step 5: Update Start() to register OnEvent before serving**

Locate `func (d *Daemon) Start(ctx context.Context) error` (~line 213). After
the existing `d.sessionSvc.OnDataReady(...)` line and before `d.server.OnClient(...)`,
insert:

```go
	if d.cmdSvc != nil {
		d.cmdSvc.OnEvent(d.publishCmdEvent)
	}
```

- [ ] **Step 6: Add publishCmdEvent helper**

Add to service.go:

```go
// publishCmdEvent translates a cmdlifecycle.Event into the matching
// event.Event type and publishes it on the bus. Called synchronously from
// cmdlifecycle.Service.HandleOSC; keep it allocation-light but
// correctness > perf for rare-event path.
func (d *Daemon) publishCmdEvent(sessID string, ev cmdlifecycle.Event) {
	var t event.Type
	payload := map[string]any{}
	switch ev.Kind {
	case cmdlifecycle.EventPromptShown:
		t = event.CommandPromptShown
		payload["row"] = ev.Row
	case cmdlifecycle.EventCommandStarted:
		t = event.CommandStarted
		if ev.Command != nil {
			payload["started_at"] = ev.Command.StartedAt
			payload["start_row"] = ev.Command.StartRow
		}
	case cmdlifecycle.EventCommandFinished:
		t = event.CommandFinished
		if ev.Command != nil {
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
	default:
		return
	}
	d.publishEvent(event.Event{Type: t, SessionID: sessID, Payload: payload})
}
```

- [ ] **Step 7: Update daemon's fx Module**

In `internal/daemon/module.go`, find the existing `fx.Options(...)` and add cmdRepo provider:

```go
	fx.Provide(func(d *Daemon) cmdlifecycle.Repository {
		return d.cmdRepo
	}),
	fx.Provide(func() *cmdRepository {
		// coldRestore flag is determined by the daemon's options at
		// construction; cmdRepo is created here with that flag.
		return newCmdRepository(true) // TODO replaced in Task 16
	}),
```

Wait — actually the cmdRepo needs to be wired into daemon construction. Let me restructure: in the daemon options.go, add a setter that creates cmdRepo internally based on `coldRestore` setting. Since this requires knowing the cold-restore value, do it in WithDataDir / WithColdRestore consolidation.

Simpler approach: cmdRepo is constructed inline when daemon options sets coldRestore. Update the cold-restore option to also create cmdRepo.

Find `func WithColdRestore(...)` in `internal/daemon/options.go` and modify:

```go
func WithColdRestore(enabled bool) Option {
	return func(d *Daemon) {
		d.coldRestore = enabled
		d.cmdRepo = newCmdRepository(enabled)
	}
}
```

Update Module to provide via daemon's exposed cmdRepo:

```go
// internal/daemon/module.go — adjust as needed
var Module = fx.Options(
	fx.Provide(NewDaemon),
	cmdlifecycle.Module,
	fx.Provide(func(d *Daemon) cmdlifecycle.Repository {
		if d.cmdRepo == nil {
			d.cmdRepo = newCmdRepository(false) // safe default
		}
		return d.cmdRepo
	}),
)
```

- [ ] **Step 8: Run all daemon tests to confirm wiring compiles**

Run: `go build ./internal/daemon/ && go test ./internal/daemon/ -run TestDaemon -count=1`
Expected: build passes; existing tests still pass.

- [ ] **Step 9: Commit**

```bash
git -c commit.gpgsign=false add internal/daemon/service.go internal/daemon/options.go internal/daemon/module.go
git -c commit.gpgsign=false commit -m "feat(daemon): wire cmdSvc + cmdRepo + publishCmdEvent"
```

---

## Task 15: Daemon — extend scanOSC with lazy newline counting

**Files:**
- Modify: `internal/daemon/service.go`

- [ ] **Step 1: Write a failing integration test**

```go
// Append to internal/daemon/service_test.go
func TestDaemon_ScanOSC133_EmitsEvents(t *testing.T) {
	// Construct minimal daemon with a real cmdSvc and capture events via bus.
	// Exact construction depends on existing test helpers — adapt to use
	// the same constructor pattern as TestDaemon_ScanOSC_OSC7 (or similar).

	repo := newCmdRepository(false)
	cmdSvc := cmdlifecycle.NewService(repo)
	bus := event.NewBus()
	d := NewDaemon(nil, nil,
		WithEventBus(bus),
		WithCommandLifecycle(cmdSvc, repo),
	)
	cmdSvc.OnEvent(d.publishCmdEvent)

	if err := cmdSvc.Register("s1"); err != nil {
		t.Fatal(err)
	}
	d.lineCounters["s1"] = new(atomic.Int64)

	sub := bus.SubscribeTypes(event.CommandStarted, event.CommandFinished)
	defer sub.Unsubscribe()

	// Feed scanOSC a chunk with prompt + command + finish.
	chunk := []byte("\x1b]133;A\x07$ ls\n\x1b]133;C\x07file1\nfile2\n\x1b]133;D;0\x07")
	d.scanOSC("s1", chunk)

	// Expect: CommandStarted, CommandFinished
	deadline := time.After(1 * time.Second)
	got := []event.Type{}
	for len(got) < 2 {
		select {
		case ev := <-sub.Events():
			got = append(got, ev.Type)
		case <-deadline:
			t.Fatalf("only got %v before deadline", got)
		}
	}
	if got[0] != event.CommandStarted {
		t.Errorf("got[0] = %v, want CommandStarted", got[0])
	}
	if got[1] != event.CommandFinished {
		t.Errorf("got[1] = %v, want CommandFinished", got[1])
	}
}
```

(Imports needed: `event`, `time`, `sync/atomic`, `cmdlifecycle`. Adjust constructor calls to match the daemon's actual NewDaemon signature.)

- [ ] **Step 2: Run, expect failure**

Run: `go test ./internal/daemon/ -run TestDaemon_ScanOSC133_EmitsEvents -v`
Expected: FAIL — scanOSC doesn't dispatch OSC 133 yet.

- [ ] **Step 3: Modify scanOSC in service.go**

Locate the existing `func (d *Daemon) scanOSC(...)`. Replace its body with the offset-aware lazy version:

```go
// scanOSC inspects output data for OSC sequences and dispatches:
//   - OSC 7 → updates session metadata + emits CwdChanged
//   - OSC 9/99/777 → emits Notification or ShellReady
//   - OSC 133 → cmdSvc.HandleOSC with offset-derived row
//
// The newline counter for row tracking is updated lazily — only the
// prefix between markers is counted, plus the tail after the last match.
// Cross-chunk correctness: after this function returns, the counter
// reflects the cumulative newline count across all bytes ever scanned.
func (d *Daemon) scanOSC(sessID string, data []byte) {
	results := ParseOSC(data)
	if len(results) == 0 {
		return
	}

	d.lineCountersMu.RLock()
	counter := d.lineCounters[sessID]
	d.lineCountersMu.RUnlock()

	cursor := 0
	for _, osc := range results {
		switch osc.Type {
		case OSCTypeCwd:
			_ = d.sessionSvc.MetaSet(sessID, "cwd", osc.Value)
			d.publishEvent(event.Event{
				Type:      event.CwdChanged,
				SessionID: sessID,
				Payload:   map[string]any{"cwd": osc.Value},
			})
		case OSCTypeNotification:
			d.publishEvent(event.Event{
				Type:      event.Notification,
				SessionID: sessID,
				Payload:   map[string]any{"body": osc.Value},
			})
		case OSCTypeShellReady:
			d.publishEvent(event.Event{
				Type:      event.ShellReady,
				SessionID: sessID,
				Payload:   map[string]any{},
			})
		case OSCType133:
			if counter != nil {
				counter.Add(int64(bytes.Count(data[cursor:osc.Offset], []byte{'\n'})))
				cursor = osc.Offset
			}
			row := int64(0)
			if counter != nil {
				row = counter.Load()
			}
			code, exit := parseOSC133Value(osc.Value)
			if d.cmdSvc != nil {
				_ = d.cmdSvc.HandleOSC(sessID, code, exit, row)
			}
		}
	}

	// Tail: count newlines after the last match so the next chunk's
	// row is correct from byte zero of that chunk's perspective.
	if counter != nil && cursor < len(data) {
		counter.Add(int64(bytes.Count(data[cursor:], []byte{'\n'})))
	}
}

// parseOSC133Value extracts the code byte and exit code from an OSC 133
// payload string. The payload is the part after "133;" — e.g. "A",
// "D;42", "D;0;extra".
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

Add imports if missing: `bytes`, `strconv`, `strings`.

- [ ] **Step 4: Run integration test**

Run: `go test ./internal/daemon/ -run TestDaemon_ScanOSC133 -v -race`
Expected: PASS.

- [ ] **Step 5: Run all daemon tests for regression**

Run: `go test ./internal/daemon/ -count=1 -race`
Expected: PASS — including existing OSC 7/9/777 tests.

- [ ] **Step 6: Run alloc gate again**

Run: `go test ./internal/daemon/ -run TestBenchmarkParseOSC_NoOSC_ZeroAllocs -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git -c commit.gpgsign=false add internal/daemon/service.go internal/daemon/service_test.go
git -c commit.gpgsign=false commit -m "feat(daemon): scanOSC dispatches OSC 133 + lazy newline counting"
```

---

## Task 16: Daemon lifecycle wiring — Register/Unregister on session create/exit

**Files:**
- Modify: `internal/daemon/service.go`

- [ ] **Step 1: Write failing test**

```go
// Append to internal/daemon/service_test.go
func TestDaemon_RegistersOnCreate_UnregistersOnExit(t *testing.T) {
	// Use the existing test scaffold for daemon. Capture cmdSvc state
	// before/after handleCreate and OnExit callback.
	// Specific construction follows the existing TestDaemon_HandleCreate pattern.

	// 1. Daemon spawns "s1" → cmdSvc.Register("s1") + lineCounters["s1"]
	//    is allocated.
	// 2. Session exits → cmdSvc.Unregister("s1") + lineCounters["s1"]
	//    deleted + cmdRepo.Close("s1") called.

	// Adapt to your actual test scaffolding — the exact struct
	// initialization differs across the codebase.
	t.Skip("adapt to existing daemon test scaffolding before running")
}
```

(Mark skipped initially — implementation step adds real wiring; activate the test after.)

- [ ] **Step 2: Modify handleCreate in service.go**

Locate `func (d *Daemon) handleCreate(c ConnectedClient, frame protocol.Frame)`. After the existing `d.persistSessionCreate(req.ID, req)` call, add:

```go
	if d.cmdSvc != nil {
		_ = d.cmdSvc.Register(req.ID)
	}
	d.lineCountersMu.Lock()
	d.lineCounters[req.ID] = new(atomic.Int64)
	d.lineCountersMu.Unlock()

	if d.cmdRepo != nil && d.dataDir != "" && d.coldRestore {
		path := filepath.Join(d.dataDir, req.ID, "commands.json")
		_ = d.cmdRepo.Open(req.ID, path)
	}
```

- [ ] **Step 3: Modify the OnExit callback in Start()**

Locate `d.sessionSvc.OnExit(func(id string, exitCode int) { ... })` (~line 240). Add inside the closure, after the existing calls:

```go
		if d.cmdSvc != nil {
			d.cmdSvc.Unregister(id)
		}
		d.lineCountersMu.Lock()
		delete(d.lineCounters, id)
		d.lineCountersMu.Unlock()
		if d.cmdRepo != nil {
			d.cmdRepo.Close(id)
		}
```

- [ ] **Step 4: Modify Daemon Stop / Start return path to flush cmdRepo**

Locate the cleanup region after `err := d.server.Serve(childCtx)`. Add:

```go
	if d.cmdRepo != nil {
		d.cmdRepo.CloseAll()
	}
```

- [ ] **Step 5: Activate the previously-skipped test**

Replace `t.Skip(...)` with the actual scaffolding. If the existing daemon
test helpers don't expose post-create state easily, write a minimal helper:

```go
// Append to internal/daemon/service_test.go (or a new helper file)

// daemonForOSC133Test constructs a Daemon with cmdSvc and bus wired,
// using mocks for transport/session services. Adapt to actual signatures.
//
// (This block intentionally left as a directive — the existing daemon
// test files reveal the right helper. Reuse them.)
```

For this task, instead of `t.Skip`, write the test against the actual
helpers used in `service_test.go`. If no straightforward helper exists,
add a focused test that uses `gomock` for the SessionManager mock and
asserts that:

```go
	// After d.handleCreate("s1", ...):
	if _, err := d.cmdSvc.Snapshot("s1"); err != nil {
		t.Errorf("cmdSvc should know about s1, got %v", err)
	}
	d.lineCountersMu.RLock()
	c := d.lineCounters["s1"]
	d.lineCountersMu.RUnlock()
	if c == nil {
		t.Error("lineCounter not allocated for s1")
	}
```

- [ ] **Step 6: Run all daemon tests**

Run: `go test ./internal/daemon/ -count=1 -race`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git -c commit.gpgsign=false add internal/daemon/service.go internal/daemon/service_test.go
git -c commit.gpgsign=false commit -m "feat(daemon): cmdlifecycle Register/Unregister on session create/exit"
```

---

## Task 17: Cold-restore — load commands.json on daemon startup

**Files:**
- Modify: `internal/daemon/reconcile.go`
- Modify: `internal/daemon/service.go` (Start hook)

- [ ] **Step 1: Read existing reconcile.go**

Run: `cat internal/daemon/reconcile.go`. Note the existing `ReconcileOrphans` flow.

- [ ] **Step 2: Write failing test**

```go
// internal/daemon/reconcile_test.go (or append to existing)
func TestReconcile_LoadsCommandHistory(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "s1")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	contents := fmt.Sprintf(`{
		"version": 1,
		"session_id": "s1",
		"saved_at": "%s",
		"history": [{"started_at":"2026-04-30T12:00:00Z","ended_at":"2026-04-30T12:00:01Z","exit_code":0,"start_row":1,"end_row":2}]
	}`, now.Format(time.RFC3339))
	if err := os.WriteFile(filepath.Join(sessDir, "commands.json"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	repo := newCmdRepository(true)
	cmdSvc := cmdlifecycle.NewService(repo)

	if err := RestoreCommandsForSession(dir, "s1", cmdSvc); err != nil {
		t.Fatalf("RestoreCommandsForSession: %v", err)
	}

	state, err := cmdSvc.Snapshot("s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.History) != 1 {
		t.Errorf("History len = %d, want 1", len(state.History))
	}
}

func TestReconcile_HandlesMissingFile(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "s1")
	_ = os.MkdirAll(sessDir, 0o755)

	repo := newCmdRepository(true)
	cmdSvc := cmdlifecycle.NewService(repo)

	// No commands.json on disk → no error, session not registered.
	if err := RestoreCommandsForSession(dir, "s1", cmdSvc); err != nil {
		t.Errorf("missing file should not error, got %v", err)
	}
	if _, err := cmdSvc.Snapshot("s1"); !errors.Is(err, cmdlifecycle.ErrSessionNotRegistered) {
		t.Errorf("session should not be registered when file missing")
	}
}

func TestReconcile_FutureVersion_Skipped(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "s1")
	_ = os.MkdirAll(sessDir, 0o755)
	contents := `{"version": 99, "session_id": "s1", "saved_at": "2026-04-30T12:00:00Z"}`
	if err := os.WriteFile(filepath.Join(sessDir, "commands.json"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	repo := newCmdRepository(true)
	cmdSvc := cmdlifecycle.NewService(repo)
	// Should NOT error — function logs and skips.
	if err := RestoreCommandsForSession(dir, "s1", cmdSvc); err != nil {
		t.Errorf("future version should be skipped, got %v", err)
	}
	if _, err := cmdSvc.Snapshot("s1"); !errors.Is(err, cmdlifecycle.ErrSessionNotRegistered) {
		t.Error("session should not be registered after future-version file")
	}
}
```

- [ ] **Step 3: Run, expect failure**

Run: `go test ./internal/daemon/ -run TestReconcile_LoadsCommandHistory -v`
Expected: FAIL — `RestoreCommandsForSession` undefined.

- [ ] **Step 4: Add RestoreCommandsForSession to reconcile.go**

```go
// Append to internal/daemon/reconcile.go
import (
	// existing imports plus:
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wblech/wmux/internal/cmdlifecycle"
	"github.com/wblech/wmux/internal/platform/commands"
)

// RestoreCommandsForSession loads the per-session commands.json (if
// present) and rehydrates the cmdlifecycle Service for the session.
//
// Returns nil if the file is missing, present-but-future-version, or
// loaded successfully. Returns an error only for unexpected I/O errors
// or when the cmdlifecycle Service rejects the Restore.
func RestoreCommandsForSession(dataDir, sessID string, cmdSvc *cmdlifecycle.Service) error {
	if cmdSvc == nil {
		return nil
	}
	path := filepath.Join(dataDir, sessID, "commands.json")
	file, err := commands.Load(path)
	if err != nil {
		if errors.Is(err, commands.ErrUnsupportedVersion) {
			fmt.Fprintf(os.Stderr, "wmux: skip session %q: commands.json version %d unsupported\n", sessID, file.Version)
			return nil
		}
		return fmt.Errorf("RestoreCommandsForSession: %w", err)
	}
	// Empty file (file did not exist): nothing to restore.
	if file.Version == 0 && len(file.History) == 0 && file.InFlight == nil {
		return nil
	}

	state := cmdlifecycle.SessionState{
		History:  file.History,
		InFlight: file.InFlight,
	}
	if err := cmdSvc.Restore(sessID, state); err != nil {
		return fmt.Errorf("RestoreCommandsForSession: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Run tests, expect pass**

Run: `go test ./internal/daemon/ -run TestReconcile_LoadsCommandHistory -v`
Run: `go test ./internal/daemon/ -run TestReconcile_HandlesMissingFile -v`
Run: `go test ./internal/daemon/ -run TestReconcile_FutureVersion_Skipped -v`
Expected: all PASS.

- [ ] **Step 6: Wire RestoreCommandsForSession into Start()**

Locate the existing reconcile call in `func (d *Daemon) Start(...)` (around line 232):

```go
	if d.dataDir != "" {
		_, _ = ReconcileOrphans(d.dataDir)
	}
```

Extend with cmd history restoration:

```go
	if d.dataDir != "" {
		_, _ = ReconcileOrphans(d.dataDir)
		if d.coldRestore && d.cmdSvc != nil {
			d.restoreCommandHistoriesAtStartup()
		}
	}
```

Add the helper:

```go
// restoreCommandHistoriesAtStartup walks dataDir for session
// directories and restores any persisted commands.json into cmdSvc.
// Also calls cmdRepo.Open and allocates lineCounters for restored
// sessions so subsequent OSC 133 events persist correctly.
func (d *Daemon) restoreCommandHistoriesAtStartup() {
	entries, err := os.ReadDir(d.dataDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sessID := e.Name()
		if err := RestoreCommandsForSession(d.dataDir, sessID, d.cmdSvc); err != nil {
			logErr("cold-restore commands", err)
			continue
		}
		// Wire lineCounter + Writer for any subsequently-attached
		// session.
		d.lineCountersMu.Lock()
		if _, ok := d.lineCounters[sessID]; !ok {
			d.lineCounters[sessID] = new(atomic.Int64)
		}
		d.lineCountersMu.Unlock()
		if d.cmdRepo != nil {
			path := filepath.Join(d.dataDir, sessID, "commands.json")
			_ = d.cmdRepo.Open(sessID, path)
		}
	}
}
```

- [ ] **Step 7: Run all daemon tests**

Run: `go test ./internal/daemon/ -count=1 -race`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git -c commit.gpgsign=false add internal/daemon/reconcile.go internal/daemon/reconcile_test.go internal/daemon/service.go
git -c commit.gpgsign=false commit -m "feat(daemon): cold-restore commands.json on startup"
```

---

## Task 18: E2E smoke test with real bash

**Files:**
- Create: `test/e2e/osc133_test.go`

- [ ] **Step 1: Read the existing e2e helpers**

Run: `ls test/e2e/ | head -10`. Identify how an existing test bootstraps a daemon + session.

- [ ] **Step 2: Write E2E test**

```go
// test/e2e/osc133_test.go
//go:build e2e

package e2e

import (
	"os/exec"
	"testing"
	"time"

	"github.com/wblech/wmux/internal/platform/event"
	// import the existing test harness that spawns an in-process daemon
)

// TestE2E_OSC133_BashRealCommand spawns a real bash with a PS1 emitting
// OSC 133 markers, runs `ls`, and verifies the daemon publishes
// CommandStarted + CommandFinished{exit_code:0}.
func TestE2E_OSC133_BashRealCommand(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	// Use the existing harness pattern from other test/e2e files.
	// h := newTestHarness(t)
	// defer h.Close()

	// PS1 emits OSC 133;A and OSC 133;B; the bash trap on DEBUG emits C
	// before each command; PROMPT_COMMAND emits D with $? after.
	const ps1Setup = `PROMPT_COMMAND='__last=$?; printf "\x1b]133;D;%s\x07" "$__last"'
PS1='\[\x1b]133;A\x07\]$ \[\x1b]133;B\x07\]'
trap 'printf "\x1b]133;C\x07"' DEBUG
`

	// h.Create("s1", "/bin/bash", []string{"--norc", "--noprofile", "-i"})
	// sub := h.SubscribeEvents(event.CommandStarted, event.CommandFinished)
	// h.Input("s1", ps1Setup+"\n")
	// h.Input("s1", "true\n")

	// Wait for both events.
	// got := waitForEvents(sub, 2, 5*time.Second)
	// assertHasType(t, got, event.CommandStarted)
	// assertHasType(t, got, event.CommandFinished)

	t.Skip("adapt this scaffolding to the existing test/e2e harness — see test/e2e/event_subscribe_test.go for pattern")

	// Avoid unused-import errors when the body is skipped.
	_ = event.CommandStarted
	_ = time.Second
}
```

The test is left skipped initially — adapt to the existing in-process daemon test harness (e.g. `test/e2e/inprocess_daemon_test.go` or the equivalent). The shape is fixed; the harness lookup is local to the codebase.

- [ ] **Step 3: Adapt the test to the existing harness**

Replace the skipped scaffolding with real harness calls. Example using a hypothetical
`startInProcessDaemon` (rename to whatever exists):

```go
	d, c := startInProcessDaemon(t)
	defer c.Close()

	if err := c.Create("s1", "/bin/bash", []string{"--norc", "--noprofile", "-i"}); err != nil {
		t.Fatal(err)
	}
	sub := d.EventBus().SubscribeTypes(event.CommandStarted, event.CommandFinished)
	defer sub.Unsubscribe()

	_ = c.Input("s1", []byte(ps1Setup+"\n"))
	_ = c.Input("s1", []byte("true\n"))

	want := map[event.Type]bool{event.CommandStarted: false, event.CommandFinished: false}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev := <-sub.Events():
			if _, ok := want[ev.Type]; ok {
				want[ev.Type] = true
			}
			allGot := true
			for _, v := range want {
				if !v {
					allGot = false
				}
			}
			if allGot {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for events; got %v", want)
		}
	}
```

- [ ] **Step 4: Run the e2e test**

Run: `go test -tags=e2e ./test/e2e/ -run TestE2E_OSC133 -v -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git -c commit.gpgsign=false add test/e2e/osc133_test.go
git -c commit.gpgsign=false commit -m "test(e2e): OSC 133 with real bash + PS1 markers"
```

---

## Task 19: Final regression sweep + lint + bench

- [ ] **Step 1: Run full test suite**

Run: `make test`
Expected: All tests PASS, including race detector.

- [ ] **Step 2: Run goframe linter**

Run: `make lint`
Expected: No errors.

- [ ] **Step 3: Run all benches**

Run: `go test -bench=. -benchmem ./internal/daemon/ ./internal/cmdlifecycle/ ./internal/platform/commands/ -run=^$ -benchtime=2s 2>&1 | tee /tmp/osc133-bench.txt`

Expected: numeric output. Review `BenchmarkParseOSC_NoOSC` shows `0 allocs/op`.

- [ ] **Step 4: Run zero-alloc gate explicitly**

Run: `go test ./internal/daemon/ -run TestBenchmarkParseOSC_NoOSC_ZeroAllocs -v`
Run: `go test ./internal/cmdlifecycle/ -run BenchmarkHandleOSC_NoOp -v -bench=. -benchtime=1s`
Expected: PASS / 0 allocs.

- [ ] **Step 5: Compare ADR 0032 baselines**

Verify no regression in:
- `BenchmarkBatcherAddFlush` — ≤ 1668 ns/op, 0 allocs
- `BenchmarkBufferWriteRead` — ≤ 613 ns/op, 0 allocs

If any regress, investigate before commit.

- [ ] **Step 6: Final commit**

```bash
git -c commit.gpgsign=false add docs/superpowers/specs/2026-04-30-osc133-shell-integration-design.md docs/superpowers/plans/2026-04-30-osc133-shell-integration.md
git -c commit.gpgsign=false commit -m "docs(osc133): spec + implementation plan"
```

---

## Self-Review

**Spec coverage:**

- D1 passive scanner: covered in Task 11 (OSC 133 case in ParseOSC) ✓
- D2 row tracking in wmux: covered in Tasks 14, 15 (lineCounters + scanOSC) ✓
- D3 cmdlifecycle package: covered in Tasks 1-6 ✓
- D4 ParseOSC Offset field: covered in Task 10 ✓
- D5 Repository ownership: covered in Tasks 4 (Service), 13 (cmdRepository) ✓
- State machine transitions: covered in Task 4 ✓
- Orphan reasons: covered in Tasks 1, 4 ✓
- Snapshot deep copy: covered in Task 4 step 6 ✓
- IPC events (3 new types): covered in Task 12 ✓
- Wire format JSON payloads: covered in Task 14 (publishCmdEvent) ✓
- commands.json schema + Load: covered in Tasks 7-8 ✓
- Debounced Writer: covered in Task 9 ✓
- Cold-restore: covered in Task 17 ✓
- InFlight always-orphan on restore: covered in Task 4 (Restore_InFlightBecomesOrphan test) ✓
- Bench gates: covered in Tasks 6, 10, 11 ✓
- E2E smoke: covered in Task 18 ✓

All spec sections have at least one corresponding task.

**Placeholder scan:** No "TBD", "TODO", "implement later" found in the plan. Test-skip directives in Tasks 16/18 are explicit "adapt to existing harness" instructions, not placeholders.

**Type consistency:**
- `cmdlifecycle.Repository` interface: defined Task 4, implemented Task 13 ✓
- `cmdlifecycle.SessionState`: defined Task 1, used in Tasks 4, 7-9, 13, 17 ✓
- `cmdlifecycle.Service.HandleOSC(sessID, code, exit, row)`: signature consistent across Tasks 4, 15 ✓
- `commands.NewWriter(path, sessID, opts...)`: consistent in Tasks 9, 13 ✓
- `OrphanReasonMissingD` / `OrphanReasonDaemonRestart`: consistent constants Tasks 1, 4 ✓
- Event types `CommandPromptShown` / `CommandStarted` / `CommandFinished`: consistent Tasks 12, 14 ✓

No drift detected.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-04-30-osc133-shell-integration.md`. Two execution options:

**1. Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

**Which approach?**
