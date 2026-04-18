// Package debug provides opt-in structured tracing for the PTY data flow.
// When enabled, it writes JSON Lines via slog to a rotating file (lumberjack).
// When disabled (the default), all operations are no-ops with zero overhead.
package debug

import "time"

// Level controls the verbosity of debug tracing.
type Level int

const (
	// LevelOff disables tracing entirely (zero overhead).
	LevelOff Level = 0
	// LevelLifecycle traces session lifecycle and backpressure transitions only.
	LevelLifecycle Level = 1
	// LevelChunk traces every data chunk with size, sha1, and head/tail hex.
	LevelChunk Level = 2
	// LevelFull traces every chunk with the full hex payload.
	LevelFull Level = 3
)

// Stage identifies where in the PTY data pipeline an event was emitted.
type Stage string

const (
	StagePtyRead       Stage = "pty.read"
	StageBufferAppend  Stage = "buffer.append"
	StageBufferFlush   Stage = "buffer.flush"
	StageBufferPause   Stage = "buffer.pause"
	StageBufferResume  Stage = "buffer.resume"
	StageEmulatorIn    Stage = "emulator.in"
	StageEmulatorOut   Stage = "emulator.out"
	StageEmulatorDrop  Stage = "emulator.drop"
	StageFrameSend     Stage = "frame.send"
	StageSnapshotStart Stage = "snapshot.start"
	StageSnapshotDone  Stage = "snapshot.done"
	StageResize        Stage = "resize"
	StageAttach        Stage = "attach"
	StageDetach        Stage = "detach"
	StageSessionCreate Stage = "session.create"
	StageSessionClose  Stage = "session.close"
)

// Event represents a single debug trace point in the PTY data pipeline.
// Fields are populated according to the configured Level — higher levels
// include more fields. Zero-value fields are omitted from the JSON output.
type Event struct {
	// Time is the wall-clock time of the event. Populated by Emit if zero.
	Time time.Time
	// SessionID identifies the session that produced the event.
	SessionID string
	// Stage identifies the pipeline position.
	Stage Stage
	// Seq is a per-session monotonic counter. -1 for lifecycle events.
	Seq int64
	// ByteLen is the size of the data chunk in bytes.
	ByteLen int
	// HeadHex is the hex encoding of the first 64 bytes (level >= 2).
	HeadHex string
	// TailHex is the hex encoding of the last 64 bytes (level >= 2).
	TailHex string
	// FullHex is the hex encoding of the entire chunk (level == 3).
	FullHex string
	// Sha1 is the first 8 characters of the SHA-1 hex digest (level >= 2).
	Sha1 string

	// Typed extras — zero-value means not applicable for this stage.
	// Paused is only meaningful for buffer.pause (true) and buffer.resume (false).
	BufferSize int
	BufferHWM  int
	BufferLWM  int
	Paused     bool
	Cols       int
	Rows       int
	ExitCode   int
	Error      string
}
