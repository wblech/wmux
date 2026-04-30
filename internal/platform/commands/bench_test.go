package commands

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/wblech/wmux/internal/cmdlifecycle"
)

func BenchmarkWriter_Flush_500Commands(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "commands.json")
	w, err := NewWriter(path, "s1", WithDebounceInterval(1*time.Hour))
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = w.Close() }()

	history := make([]cmdlifecycle.Command, 500)
	for i := range history {
		history[i] = cmdlifecycle.Command{ExitCode: i, StartRow: int64(i * 10), EndRow: int64(i*10 + 5)}
	}
	w.Update(cmdlifecycle.SessionState{History: history})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = w.Flush()
	}
}
