// Package logger provides structured JSON logging built on stdlib log/slog.
package logger

import (
	"io"
	"log/slog"
	"strings"
)

// New creates a JSON-formatted structured logger writing to w at the given minimum level.
func New(w io.Writer, level slog.Level) *slog.Logger {
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:       level,
		AddSource:   false,
		ReplaceAttr: nil,
	})
	return slog.New(h)
}

// WithSessionID returns a new logger derived from l with the session_id field
// pre-populated with sessionID.
func WithSessionID(l *slog.Logger, sessionID string) *slog.Logger {
	return l.With("session_id", sessionID)
}

// ParseLevel converts a string log level to slog.Level.
// Accepts "debug", "info", "warn"/"warning", "error" (case-insensitive).
// Returns slog.LevelInfo for unknown or empty values.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
