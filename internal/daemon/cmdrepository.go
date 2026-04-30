package daemon

import (
	"fmt"
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
		mu:      sync.RWMutex{},
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
		return fmt.Errorf("cmdRepository.Open: %w", err)
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
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("peekFile: %w", err)
	}
	return info, nil
}
