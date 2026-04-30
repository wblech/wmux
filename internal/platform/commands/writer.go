package commands

import (
	"encoding/json"
	"fmt"
	"os"
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

	mu       sync.Mutex
	pending  cmdlifecycle.SessionState
	hasState bool
	closed   bool

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
	if sessID == "" {
		return nil, fmt.Errorf("commands: sessID must be non-empty")
	}
	w := &Writer{ //nolint:exhaustruct
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
			_ = w.flushOnce() // best-effort; next Update/Flush retries on failure
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
