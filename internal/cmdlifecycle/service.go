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
			s.repo.Save(sessID, tr.snapshot())
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
		mu:         sync.Mutex{},
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
			events = append(events, Event{Kind: EventPromptShown, Command: nil, Row: row})
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
		StartedAt:    now,
		EndedAt:      time.Time{},
		ExitCode:     0,
		StartRow:     row,
		EndRow:       0,
		Orphan:       false,
		OrphanReason: "",
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
