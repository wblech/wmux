package session

import "time"

// Reaper periodically checks for detached sessions that have been idle
// longer than the configured TTL and kills them.
type Reaper struct {
	service       *Service
	idleTTL       time.Duration
	checkInterval time.Duration
	// OnReap is called when a session is reaped. May be nil.
	OnReap func(id string)
	done   chan struct{}
}

// newReaper creates a Reaper that checks the service every checkInterval
// for sessions that have been detached and idle for longer than idleTTL.
func newReaper(svc *Service, idleTTL, checkInterval time.Duration) *Reaper {
	return &Reaper{
		service:       svc,
		idleTTL:       idleTTL,
		checkInterval: checkInterval,
		OnReap:        nil,
		done:          make(chan struct{}),
	}
}

// Start begins the reaper goroutine. Returns a stop function that shuts
// it down. Safe to call stop multiple times.
func (r *Reaper) Start() func() {
	go r.loop()

	return func() {
		select {
		case <-r.done:
		default:
			close(r.done)
		}
	}
}

func (r *Reaper) loop() {
	ticker := time.NewTicker(r.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.done:
			return
		case <-ticker.C:
			r.tick()
		}
	}
}

func (r *Reaper) tick() {
	sessions := r.service.List()
	now := time.Now()

	for _, sess := range sessions {
		if sess.State != StateDetached {
			continue
		}

		lastAct, err := r.service.LastActivity(sess.ID)
		if err != nil {
			continue
		}

		if now.Sub(lastAct) < r.idleTTL {
			continue
		}

		_ = r.service.Kill(sess.ID)

		if r.OnReap != nil {
			r.OnReap(sess.ID)
		}
	}
}
