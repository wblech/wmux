package session

import (
	"encoding/json"
	"sync"
)

// ProcessStarter creates a new addon child process.
type ProcessStarter interface {
	Start() (AddonProcess, error)
}

// AddonManager manages a singleton addon process and provides
// AddonEmulator instances for individual sessions.
type AddonManager struct {
	mu      sync.Mutex
	starter ProcessStarter
	process AddonProcess
}

// NewAddonManager creates an AddonManager with the given process starter.
func NewAddonManager(starter ProcessStarter) *AddonManager {
	return &AddonManager{
		mu:      sync.Mutex{},
		starter: starter,
		process: nil,
	}
}

// EmulatorFor returns an AddonEmulator for the given session, starting the
// addon process if it is not already running.
func (m *AddonManager) EmulatorFor(sessionID string, cols, rows int) *AddonEmulator {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.process == nil {
		proc, err := m.starter.Start()
		if err != nil {
			return newAddonEmulatorWithProcess(nil, sessionID)
		}
		m.process = proc
	}

	em := &AddonEmulator{
		mu:        sync.Mutex{},
		process:   m.process,
		sessionID: sessionID,
	}

	payload, _ := json.Marshal(struct {
		Cols int `json:"cols"`
		Rows int `json:"rows"`
	}{Cols: cols, Rows: rows})
	em.sendRequest(AddonMethodCreate, payload)

	return em
}

// Shutdown sends a shutdown command to the addon process.
func (m *AddonManager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.process == nil {
		return
	}

	shutdownEm := &AddonEmulator{
		mu:        sync.Mutex{},
		process:   m.process,
		sessionID: "",
	}
	shutdownEm.sendRequest(AddonMethodShutdown, nil)
	_ = m.process.Kill()
	m.process = nil
}
