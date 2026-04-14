package session

import (
	"syscall"
	"time"
)

// Watchdog periodically checks that each managed session's process is still alive.
// If a process has died but the session was not cleaned up by waitLoop (e.g.,
// due to a stuck goroutine), the watchdog force-kills the session.
type Watchdog struct {
	service  *Service
	interval time.Duration
	// OnKill is called when the watchdog force-kills a session. May be nil.
	OnKill func(id string)
	done   chan struct{}
}

// newWatchdog creates a Watchdog that checks process liveness at the given interval.
func newWatchdog(svc *Service, interval time.Duration) *Watchdog {
	return &Watchdog{
		service:  svc,
		interval: interval,
		OnKill:   nil,
		done:     make(chan struct{}),
	}
}

// Start begins the watchdog goroutine. Returns a stop function.
func (w *Watchdog) Start() func() {
	go w.loop()

	return func() {
		select {
		case <-w.done:
		default:
			close(w.done)
		}
	}
}

func (w *Watchdog) loop() {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.done:
			return
		case <-ticker.C:
			w.tick()
		}
	}
}

func (w *Watchdog) tick() {
	sessions := w.service.List()

	for _, sess := range sessions {
		if sess.State.IsTerminal() {
			continue
		}

		if !processAlive(sess.Pid) {
			_ = w.service.Kill(sess.ID)

			if w.OnKill != nil {
				w.OnKill(sess.ID)
			}
		}
	}
}

// processAlive checks if a process with the given PID is still running
// by sending signal 0.
func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil
}
