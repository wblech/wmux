package debug

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Tracer writes structured JSON Lines debug events to a rotating log file.
// A nil *Tracer is safe to use — all methods are no-ops.
type Tracer struct {
	logger *slog.Logger
	level  Level
	seqs   sync.Map
	file   *lumberjack.Logger
}

// NewTracer creates a Tracer that writes to the given path at the given level.
// Parent directories are created if they do not exist. Returns an error if
// directory creation fails. Runtime write errors are silently dropped by slog.
func NewTracer(path string, level Level, opts ...TracerOption) (*Tracer, error) {
	cfg := newTracerConfig(opts...)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	lj := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    cfg.maxSizeMB,
		MaxBackups: cfg.maxFiles,
		LocalTime:  false,
		Compress:   false,
	}

	handler := slog.NewJSONHandler(lj, &slog.HandlerOptions{
		Level:     slog.LevelInfo,
		AddSource: false,
	})

	return &Tracer{
		logger: slog.New(handler),
		level:  level,
		file:   lj,
	}, nil
}

// Enabled reports whether tracing is active. Nil-safe.
func (t *Tracer) Enabled() bool {
	return t != nil && t.level > LevelOff
}

// Level returns the configured trace level. Returns LevelOff for a nil Tracer.
func (t *Tracer) Level() Level {
	if t == nil {
		return LevelOff
	}
	return t.level
}

// Emit writes a single event to the log file. No-op if the tracer is disabled.
// The Stage value is used as the slog message ("msg" field in JSON).
// Typed extras are only included when their value is non-zero.
func (t *Tracer) Emit(ev Event) {
	if !t.Enabled() {
		return
	}

	if ev.Time.IsZero() {
		ev.Time = time.Now()
	}

	attrs := []slog.Attr{
		slog.String("session_id", ev.SessionID),
		slog.Int64("seq", ev.Seq),
	}

	// Level >= Chunk: include chunk metadata.
	if t.level >= LevelChunk && ev.ByteLen > 0 {
		attrs = append(attrs,
			slog.Int("byte_len", ev.ByteLen),
			slog.String("sha1", ev.Sha1),
			slog.String("head_hex", ev.HeadHex),
			slog.String("tail_hex", ev.TailHex),
		)
	}

	// Level >= Full: include full hex payload.
	if t.level >= LevelFull && ev.FullHex != "" {
		attrs = append(attrs, slog.String("full_hex", ev.FullHex))
	}

	// Typed extras — only include non-zero values.
	if ev.BufferSize > 0 {
		attrs = append(attrs, slog.Int("buffer_size", ev.BufferSize))
	}
	if ev.BufferHWM > 0 {
		attrs = append(attrs, slog.Int("buffer_hwm", ev.BufferHWM))
	}
	if ev.BufferLWM > 0 {
		attrs = append(attrs, slog.Int("buffer_lwm", ev.BufferLWM))
	}
	if ev.Paused {
		attrs = append(attrs, slog.Bool("paused", true))
	}
	if ev.Cols > 0 {
		attrs = append(attrs, slog.Int("cols", ev.Cols))
	}
	if ev.Rows > 0 {
		attrs = append(attrs, slog.Int("rows", ev.Rows))
	}
	if ev.ExitCode != 0 {
		attrs = append(attrs, slog.Int("exit_code", ev.ExitCode))
	}
	if ev.Error != "" {
		attrs = append(attrs, slog.String("error", ev.Error))
	}

	t.logger.LogAttrs(context.Background(), slog.LevelInfo, string(ev.Stage), attrs...)
}

// NextSeq atomically increments and returns the next sequence number for the
// given session. Each session has its own independent counter starting at 1.
func (t *Tracer) NextSeq(sessionID string) int64 {
	v, _ := t.seqs.LoadOrStore(sessionID, &atomic.Int64{})
	return v.(*atomic.Int64).Add(1)
}

// ResetSeq removes the sequence counter for the given session.
// Call this when a session closes.
func (t *Tracer) ResetSeq(sessionID string) {
	t.seqs.Delete(sessionID)
}

// Close flushes and closes the underlying log file.
func (t *Tracer) Close() error {
	if t == nil || t.file == nil {
		return nil
	}
	return t.file.Close()
}
