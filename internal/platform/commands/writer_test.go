package commands

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/wblech/wmux/internal/cmdlifecycle"
)

func newTestWriter(t *testing.T, debounce time.Duration) (*Writer, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "commands.json")
	w, err := NewWriter(path, "s1", WithDebounceInterval(debounce))
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	return w, path
}

func TestWriter_FlushOnClose(t *testing.T) {
	w, path := newTestWriter(t, 1*time.Hour)
	state := cmdlifecycle.SessionState{
		History: []cmdlifecycle.Command{{ExitCode: 0}},
	}
	w.Update(state)

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.History) != 1 {
		t.Errorf("History len = %d, want 1 (Close should flush pending)", len(got.History))
	}
}

func TestWriter_DebounceCoalesces(t *testing.T) {
	w, path := newTestWriter(t, 50*time.Millisecond)
	defer func() { _ = w.Close() }()

	for i := 0; i < 100; i++ {
		c := cmdlifecycle.Command{ExitCode: i}
		w.Update(cmdlifecycle.SessionState{History: []cmdlifecycle.Command{c}})
	}

	time.Sleep(150 * time.Millisecond)
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.History) != 1 {
		t.Errorf("History len = %d, want 1 (only last state)", len(got.History))
	}
	if got.History[0].ExitCode != 99 {
		t.Errorf("History[0].ExitCode = %d, want 99 (last update wins)", got.History[0].ExitCode)
	}
}

func TestWriter_AtomicRename(t *testing.T) {
	w, path := newTestWriter(t, 10*time.Millisecond)
	defer func() { _ = w.Close() }()
	w.Update(cmdlifecycle.SessionState{History: []cmdlifecycle.Command{{ExitCode: 7}}})
	time.Sleep(50 * time.Millisecond)

	tmp := path + ".tmp"
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Errorf(".tmp file still present after flush: %v", err)
	}
	got, _ := Load(path)
	if len(got.History) == 0 || got.History[0].ExitCode != 7 {
		t.Errorf("expected exit_code 7, got %+v", got)
	}
}

func TestWriter_UpdateAfterClose(t *testing.T) {
	w, _ := newTestWriter(t, 10*time.Millisecond)
	_ = w.Close()
	// Update after Close should be a silent no-op (no panic).
	w.Update(cmdlifecycle.SessionState{History: []cmdlifecycle.Command{{ExitCode: 1}}})
}

func TestWriter_ConcurrentUpdate(t *testing.T) {
	w, _ := newTestWriter(t, 10*time.Millisecond)
	defer func() { _ = w.Close() }()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w.Update(cmdlifecycle.SessionState{History: []cmdlifecycle.Command{{ExitCode: i}}})
		}(i)
	}
	wg.Wait()
	// Test passes if no race detector complaint.
}

func TestWriter_FlushExplicit(t *testing.T) {
	w, path := newTestWriter(t, 1*time.Hour)
	defer func() { _ = w.Close() }()
	w.Update(cmdlifecycle.SessionState{History: []cmdlifecycle.Command{{ExitCode: 5}}})
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	got, _ := Load(path)
	if len(got.History) != 1 || got.History[0].ExitCode != 5 {
		t.Errorf("Flush did not write state: %+v", got)
	}
}
