package recording

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// header is the asciinema v2 header line.
type header struct {
	Version   int    `json:"version"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Timestamp int64  `json:"timestamp"`
	Env       envObj `json:"env,omitempty"`
}

type envObj struct {
	Term string `json:"TERM,omitempty"`
}

// Writer records terminal output in asciinema v2 format (NDJSON).
// Thread-safe. Implements io.WriteCloser.
type Writer struct {
	mu        sync.Mutex
	file      *os.File
	path      string
	startTime time.Time
	written   int64
	maxSize   int64 // 0 = unlimited
	closed    bool
}

// NewWriter creates a new recording file at path with an asciinema v2 header.
// cols and rows define the terminal dimensions. maxSize of 0 means unlimited.
func NewWriter(path string, cols, rows int, maxSize int64) (*Writer, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("recording: open: %w", err)
	}

	now := time.Now()

	h := header{
		Version:   2,
		Width:     cols,
		Height:    rows,
		Timestamp: now.Unix(),
		Env:       envObj{Term: "xterm-256color"},
	}

	headerBytes, err := json.Marshal(h)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("recording: marshal header: %w", err)
	}
	headerBytes = append(headerBytes, '\n')

	n, err := f.Write(headerBytes)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("recording: write header: %w", err)
	}

	return &Writer{
		mu:        sync.Mutex{},
		file:      f,
		path:      path,
		startTime: now,
		written:   int64(n),
		maxSize:   maxSize,
		closed:    false,
	}, nil
}

// Write appends an output event in asciinema v2 format: [timestamp, "o", data].
func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return 0, ErrRecordingClosed
	}

	if w.maxSize > 0 && w.written >= w.maxSize {
		return 0, ErrSizeLimitReached
	}

	elapsed := time.Since(w.startTime).Seconds()

	event := [3]any{elapsed, "o", string(p)}
	line, err := json.Marshal(event)
	if err != nil {
		return 0, fmt.Errorf("recording: marshal event: %w", err)
	}
	line = append(line, '\n')

	if w.maxSize > 0 && w.written+int64(len(line)) > w.maxSize {
		w.closed = true
		_ = w.file.Close()
		return 0, ErrSizeLimitReached
	}

	n, err := w.file.Write(line)
	w.written += int64(n)
	if err != nil {
		return 0, fmt.Errorf("recording: write event: %w", err)
	}

	return len(p), nil
}

// Path returns the file path of the recording.
func (w *Writer) Path() string {
	return w.path
}

// Close flushes and closes the recording file.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}
	w.closed = true

	if err := w.file.Close(); err != nil {
		return fmt.Errorf("recording: close: %w", err)
	}

	return nil
}
