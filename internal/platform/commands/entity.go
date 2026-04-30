// Package commands persists per-session command history to disk under
// {dataDir}/{session_id}/commands.json.
//
// The Writer debounces and writes atomically via tmpfile + rename; the
// Load function reads the most recent fully-written file. The schema
// carries a Version field for forward compatibility.
package commands

import (
	"errors"
	"time"

	"github.com/wblech/wmux/internal/cmdlifecycle"
)

// CurrentVersion is the schema version that this build of wmux writes.
// Future readers may encounter higher versions and must reject them with
// ErrUnsupportedVersion rather than misinterpreting unknown fields.
const CurrentVersion = 1

// File is the on-disk schema for commands.json.
type File struct {
	Version   int                    `json:"version"`
	SessionID string                 `json:"session_id"`
	SavedAt   time.Time              `json:"saved_at"`
	History   []cmdlifecycle.Command `json:"history"`
	InFlight  *cmdlifecycle.Command  `json:"in_flight,omitempty"`
}

// Sentinel errors.
var (
	// ErrUnsupportedVersion is returned by Load when the file's Version
	// is greater than CurrentVersion.
	ErrUnsupportedVersion = errors.New("commands: unsupported file version")
	// ErrWriterClosed is returned when Update is called after Close.
	ErrWriterClosed = errors.New("commands: writer closed")
)
