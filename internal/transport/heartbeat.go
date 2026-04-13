package transport

import (
	"sync"
	"time"

	"github.com/wblech/wmux/internal/platform/protocol"
)

// HeartbeatManager periodically sends heartbeat frames on clients' stream
// connections and tracks acknowledgments. If a client misses maxMissed
// consecutive heartbeats, the onDead callback is invoked.
type HeartbeatManager struct {
	mu        sync.RWMutex
	clients   map[string]*Client
	interval  time.Duration
	maxMissed int
	onDead    func(clientID string)
}

// NewHeartbeatManager creates a HeartbeatManager that sends heartbeats at the
// given interval and considers a client dead after maxMissed misses.
func NewHeartbeatManager(interval time.Duration, maxMissed int) *HeartbeatManager {
	return &HeartbeatManager{
		mu:        sync.RWMutex{},
		clients:   make(map[string]*Client),
		interval:  interval,
		maxMissed: maxMissed,
		onDead:    nil,
	}
}

// OnDead sets the callback invoked when a client is detected as dead.
func (h *HeartbeatManager) OnDead(fn func(clientID string)) {
	h.mu.Lock()
	h.onDead = fn
	h.mu.Unlock()
}

// Track adds a client to heartbeat monitoring.
func (h *HeartbeatManager) Track(c *Client) {
	h.mu.Lock()
	h.clients[c.ID] = c
	h.mu.Unlock()
}

// Untrack removes a client from heartbeat monitoring.
func (h *HeartbeatManager) Untrack(clientID string) {
	h.mu.Lock()
	delete(h.clients, clientID)
	h.mu.Unlock()
}

// Ack records a heartbeat acknowledgment from a client, resetting the missed
// counter and updating the last ack timestamp.
func (h *HeartbeatManager) Ack(clientID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	c, ok := h.clients[clientID]
	if !ok {
		return
	}

	c.MissedHeartbeats = 0
	c.LastHeartbeatAck = time.Now()
}

// Start begins the heartbeat ticker loop. Returns a stop function that
// cancels the loop. Safe to call stop multiple times.
func (h *HeartbeatManager) Start() (stop func()) {
	done := make(chan struct{})

	var once sync.Once
	stop = func() {
		once.Do(func() { close(done) })
	}

	go h.loop(done)

	return stop
}

// loop runs the heartbeat ticker.
func (h *HeartbeatManager) loop(done <-chan struct{}) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			h.tick()
		}
	}
}

// tick sends a heartbeat to each tracked client and increments missed counters.
func (h *HeartbeatManager) tick() {
	hbFrame := protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgHeartbeat,
		Payload: nil,
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	var dead []string

	for id, c := range h.clients {
		if c.Stream == nil {
			continue
		}

		c.MissedHeartbeats++

		if c.MissedHeartbeats > h.maxMissed {
			dead = append(dead, id)
			continue
		}

		if err := c.Stream.WriteFrame(hbFrame); err != nil {
			c.MissedHeartbeats = h.maxMissed + 1
			dead = append(dead, id)
		}
	}

	for _, id := range dead {
		delete(h.clients, id)
	}

	if h.onDead != nil {
		for _, id := range dead {
			h.onDead(id)
		}
	}
}
