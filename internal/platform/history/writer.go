package history

import (
	"fmt"
	"os"
	"sync"
)

// Writer is an append-only, optionally size-capped writer for scrollback.bin.
// It implements io.WriteCloser. When maxSize > 0, writes that would exceed
// the cap are truncated (partial writes) or silently dropped (cap reached).
// Thread-safe.
type Writer struct {
	mu      sync.Mutex
	file    *os.File
	written int64
	maxSize int64 // 0 = unlimited
	closed  bool
}

// NewWriter creates a new scrollback writer at the given path.
// The file is created (or truncated) with mode 0644.
// maxSize of 0 means unlimited.
func NewWriter(path string, maxSize int64) (*Writer, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("history: open scrollback: %w", err)
	}

	return &Writer{
		mu:      sync.Mutex{},
		file:    f,
		written: 0,
		maxSize: maxSize,
		closed:  false,
	}, nil
}

// Write appends data to the scrollback file. If the writer has a size cap,
// the write is truncated to fit within the remaining space. Returns the
// number of bytes actually written. Returns ErrWriterClosed if the writer
// has been closed.
func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return 0, ErrWriterClosed
	}

	data := p

	if w.maxSize > 0 {
		remaining := w.maxSize - w.written
		if remaining <= 0 {
			return 0, nil
		}

		if int64(len(data)) > remaining {
			data = data[:remaining]
		}
	}

	n, err := w.file.Write(data)
	w.written += int64(n)

	if err != nil {
		return n, fmt.Errorf("history: write scrollback: %w", err)
	}

	return n, nil
}

// Written returns the total number of bytes written so far.
func (w *Writer) Written() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.written
}

// Close flushes and closes the underlying file. Safe to call multiple times.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}

	w.closed = true

	if err := w.file.Close(); err != nil {
		return fmt.Errorf("history: close scrollback: %w", err)
	}

	return nil
}
