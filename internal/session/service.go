package session

import (
	"fmt"
	"sync"
	"time"

	"github.com/wblech/wmux/internal/platform/pty"
)

const (
	// readChunkSize is the number of bytes read from the PTY in each iteration.
	readChunkSize = 32 * 1024

	// pausedSleepInterval is how long the read loop sleeps when backpressure is active.
	pausedSleepInterval = 10 * time.Millisecond

	// defaultHighWatermark is the default high watermark for the output buffer (4 MiB).
	defaultHighWatermark = 4 * 1024 * 1024

	// defaultLowWatermark is the default low watermark for the output buffer (2 MiB).
	defaultLowWatermark = 2 * 1024 * 1024

	// defaultBatchInterval is the default flush interval for the output batcher.
	defaultBatchInterval = 16 * time.Millisecond
)

// Repository defines the persistence operations for sessions.
type Repository interface {
	// Get returns the session with the given id or ErrSessionNotFound.
	Get(id string) (*Session, error)
	// List returns all persisted sessions.
	List() []*Session
	// Save persists or updates a session.
	Save(sess *Session) error
	// Delete removes a session by id.
	Delete(id string) error
}

// CreateOptions carries the parameters for creating a new session.
type CreateOptions struct {
	// Shell is the path to the shell binary to run.
	Shell string
	// Args contains additional arguments passed to the shell.
	Args []string
	// Cols is the initial terminal width in columns.
	Cols int
	// Rows is the initial terminal height in rows.
	Rows int
	// Cwd is the initial working directory for the shell process.
	Cwd string
	// Env is the environment variable list for the shell process.
	Env []string
	// HighWatermark is the buffer size at which backpressure is applied.
	HighWatermark int
	// LowWatermark is the buffer size at which backpressure is released.
	LowWatermark int
	// BatchInterval is how often the batcher flushes output to the buffer.
	BatchInterval time.Duration
}

// managedSession groups a Session with its runtime resources.
type managedSession struct {
	session   *Session
	process   *pty.Process
	emulator  ScreenEmulator
	buffer    *Buffer
	batcher   *Batcher
	closeOnce sync.Once
}

// Service manages the lifecycle of terminal sessions.
type Service struct {
	mu          sync.RWMutex
	sessions    map[string]*managedSession
	spawner     pty.Spawner
	maxSessions int
}

// NewService creates a new Service backed by the given Spawner.
// Options are applied in order after the defaults are set.
func NewService(spawner pty.Spawner, opts ...Option) *Service {
	s := &Service{
		mu:          sync.RWMutex{},
		sessions:    make(map[string]*managedSession),
		spawner:     spawner,
		maxSessions: 0,
	}

	for _, o := range opts {
		o(s)
	}

	return s
}

// Create starts a new terminal session with the given id and options.
// It validates the id, checks for duplicates, enforces the session cap,
// spawns the PTY, and starts the internal read and wait goroutines.
func (s *Service) Create(id string, opts CreateOptions) (*Session, error) {
	if err := ValidateSessionID(id); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.sessions[id]; exists {
		return nil, ErrSessionExists
	}

	if s.maxSessions > 0 && len(s.sessions) >= s.maxSessions {
		return nil, ErrMaxSessions
	}

	cols := opts.Cols
	if cols <= 0 {
		cols = 80
	}

	rows := opts.Rows
	if rows <= 0 {
		rows = 24
	}

	highWM := opts.HighWatermark
	if highWM <= 0 {
		highWM = defaultHighWatermark
	}

	lowWM := opts.LowWatermark
	if lowWM <= 0 {
		lowWM = defaultLowWatermark
	}

	batchInterval := opts.BatchInterval
	if batchInterval <= 0 {
		batchInterval = defaultBatchInterval
	}

	proc, err := s.spawner.Spawn(pty.SpawnOptions{
		Command: opts.Shell,
		Args:    opts.Args,
		Cols:    cols,
		Rows:    rows,
		Cwd:     opts.Cwd,
		Env:     opts.Env,
	})
	if err != nil {
		return nil, fmt.Errorf("spawn pty: %w", err)
	}

	buf := NewBuffer(highWM, lowWM)
	batcher := NewBatcher(batchInterval, func(data []byte) {
		buf.Write(data) //nolint:errcheck
	})

	sess := &Session{
		ID:        id,
		Shell:     opts.Shell,
		Cwd:       opts.Cwd,
		State:     StateAlive,
		Pid:       proc.Pid(),
		Cols:      cols,
		Rows:      rows,
		ExitCode:  0,
		StartedAt: time.Now(),
		EndedAt:   time.Time{},
	}

	ms := &managedSession{
		session:   sess,
		process:   proc,
		emulator:  NoneEmulator{},
		buffer:    buf,
		batcher:   batcher,
		closeOnce: sync.Once{},
	}

	s.sessions[id] = ms

	go s.readLoop(ms)
	go s.waitLoop(ms)

	return sess, nil
}

// Get returns the Session for the given id.
// Returns ErrSessionNotFound if no session with that id exists.
func (s *Service) Get(id string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ms, ok := s.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}

	return ms.session, nil
}

// List returns a snapshot of all currently managed sessions.
func (s *Service) List() []*Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*Session, 0, len(s.sessions))
	for _, ms := range s.sessions {
		out = append(out, ms.session)
	}

	return out
}

// Kill stops the session with the given id: it removes the session from the
// map, stops the batcher, and closes the PTY master (which delivers SIGHUP to
// the child process). The waitLoop goroutine handles final cleanup once the
// process exits.
// Returns ErrSessionNotFound if the session does not exist.
func (s *Service) Kill(id string) error {
	s.mu.Lock()
	ms, ok := s.sessions[id]
	if !ok {
		s.mu.Unlock()
		return ErrSessionNotFound
	}

	delete(s.sessions, id)
	s.mu.Unlock()

	ms.batcher.Stop()
	ms.closeOnce.Do(func() {
		ms.process.Close() //nolint:errcheck
	})

	return nil
}

// Resize updates the terminal dimensions of the session identified by id.
// It resizes both the PTY and the emulator, then updates the session metadata.
// Returns ErrSessionNotFound if the session does not exist.
func (s *Service) Resize(id string, cols, rows int) error {
	s.mu.RLock()
	ms, ok := s.sessions[id]
	s.mu.RUnlock()

	if !ok {
		return ErrSessionNotFound
	}

	if err := ms.process.Resize(cols, rows); err != nil {
		return fmt.Errorf("resize pty: %w", err)
	}

	ms.emulator.Resize(cols, rows)

	s.mu.Lock()
	ms.session.Cols = cols
	ms.session.Rows = rows
	s.mu.Unlock()

	return nil
}

// WriteInput sends data as input to the PTY process of the session.
// Returns ErrSessionNotFound if the session does not exist.
func (s *Service) WriteInput(id string, data []byte) error {
	s.mu.RLock()
	ms, ok := s.sessions[id]
	s.mu.RUnlock()

	if !ok {
		return ErrSessionNotFound
	}

	if _, err := ms.process.Write(data); err != nil {
		return fmt.Errorf("write pty input: %w", err)
	}

	return nil
}

// Snapshot returns the current screen state captured by the session's emulator.
// Returns ErrSessionNotFound if the session does not exist.
func (s *Service) Snapshot(id string) (Snapshot, error) {
	s.mu.RLock()
	ms, ok := s.sessions[id]
	s.mu.RUnlock()

	if !ok {
		return Snapshot{}, ErrSessionNotFound
	}

	return ms.emulator.Snapshot(), nil
}

// ReadOutput drains all buffered output from the session.
// Returns ErrSessionNotFound if the session does not exist.
func (s *Service) ReadOutput(id string) ([]byte, error) {
	s.mu.RLock()
	ms, ok := s.sessions[id]
	s.mu.RUnlock()

	if !ok {
		return nil, ErrSessionNotFound
	}

	return ms.buffer.Read(), nil
}

// readLoop continuously reads from the PTY and feeds data to the batcher
// and emulator. It applies backpressure by sleeping when the buffer is paused.
// The loop exits when the PTY read returns an error (e.g. EOF after process exit).
func (s *Service) readLoop(ms *managedSession) {
	buf := make([]byte, readChunkSize)

	for {
		if ms.buffer.Paused() {
			time.Sleep(pausedSleepInterval)
			continue
		}

		n, err := ms.process.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			ms.batcher.Add(chunk)
			ms.emulator.Process(chunk)
		}

		if err != nil {
			return
		}
	}
}

// waitLoop waits for the PTY process to exit, then records the exit code,
// updates the session state, stops the batcher, closes the PTY, and removes
// the session from the map.
func (s *Service) waitLoop(ms *managedSession) {
	exitCode, _ := ms.process.Wait()

	s.mu.Lock()
	ms.session.ExitCode = exitCode
	ms.session.EndedAt = time.Now()
	ms.session.State = StateExited
	delete(s.sessions, ms.session.ID)
	s.mu.Unlock()

	ms.batcher.Stop()
	ms.closeOnce.Do(func() {
		ms.process.Close() //nolint:errcheck
	})
}
