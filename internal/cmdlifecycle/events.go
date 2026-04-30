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
