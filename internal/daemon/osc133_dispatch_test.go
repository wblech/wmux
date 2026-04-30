package daemon

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/wblech/wmux/internal/cmdlifecycle"
	"github.com/wblech/wmux/internal/platform/event"
)

func TestScanOSC_133_DispatchesToCmdSvc(t *testing.T) {
	repo := newCmdRepository(false)
	cmdSvc := cmdlifecycle.NewService(repo)
	bus := event.NewBus()
	d := &Daemon{
		eventBus:     bus,
		cmdSvc:       cmdSvc,
		cmdRepo:      repo,
		lineCounters: make(map[string]*atomic.Int64),
	}
	cmdSvc.OnEvent(d.publishCmdEvent)
	if err := cmdSvc.Register("s1"); err != nil {
		t.Fatal(err)
	}
	d.lineCounters["s1"] = new(atomic.Int64)

	sub := bus.SubscribeTypes(event.CommandStarted, event.CommandFinished)
	defer sub.Unsubscribe()

	chunk := []byte("\x1b]133;A\x07$ ls\n\x1b]133;C\x07file1\nfile2\n\x1b]133;D;0\x07")
	d.scanOSC("s1", chunk)

	deadline := time.After(2 * time.Second)
	got := []event.Type{}
	for len(got) < 2 {
		select {
		case ev := <-sub.Events():
			got = append(got, ev.Type)
		case <-deadline:
			t.Fatalf("only got %v before deadline", got)
		}
	}
	if got[0] != event.CommandStarted {
		t.Errorf("got[0] = %v, want CommandStarted", got[0])
	}
	if got[1] != event.CommandFinished {
		t.Errorf("got[1] = %v, want CommandFinished", got[1])
	}
}

func TestScanOSC_133_RowAccuracy(t *testing.T) {
	repo := newCmdRepository(false)
	cmdSvc := cmdlifecycle.NewService(repo)
	bus := event.NewBus()
	d := &Daemon{
		eventBus:     bus,
		cmdSvc:       cmdSvc,
		cmdRepo:      repo,
		lineCounters: make(map[string]*atomic.Int64),
	}
	cmdSvc.OnEvent(d.publishCmdEvent)
	_ = cmdSvc.Register("s1")
	d.lineCounters["s1"] = new(atomic.Int64)

	sub := bus.SubscribeTypes(event.CommandStarted, event.CommandFinished)
	defer sub.Unsubscribe()

	// 5 newlines before C, then 3 between C and D
	chunk := []byte("\n\n\n\n\n\x1b]133;C\x07x\ny\nz\n\x1b]133;D;0\x07")
	d.scanOSC("s1", chunk)

	var startRow, endRow int64
	gotStart, gotFinish := false, false
	deadline := time.After(2 * time.Second)
	for !gotStart || !gotFinish {
		select {
		case ev := <-sub.Events():
			switch ev.Type {
			case event.CommandStarted:
				startRow = ev.Payload["start_row"].(int64)
				gotStart = true
			case event.CommandFinished:
				endRow = ev.Payload["end_row"].(int64)
				gotFinish = true
			}
		case <-deadline:
			t.Fatalf("missing events: start=%v finish=%v", gotStart, gotFinish)
		}
	}
	if startRow != 5 {
		t.Errorf("start_row = %d, want 5 (5 newlines before C)", startRow)
	}
	if endRow != 8 {
		t.Errorf("end_row = %d, want 8 (5 + 3 newlines)", endRow)
	}
}

func TestScanOSC_NoMatch_FastPath(t *testing.T) {
	d := &Daemon{
		lineCounters: make(map[string]*atomic.Int64),
	}
	// Should not panic — early return when ParseOSC returns nil.
	d.scanOSC("s1", []byte("plain output without OSC sequences"))
}

func TestParseOSC133Value(t *testing.T) {
	cases := []struct {
		v        string
		wantCode byte
		wantExit int
	}{
		{"A", 'A', 0},
		{"B", 'B', 0},
		{"C", 'C', 0},
		{"D", 'D', 0},
		{"D;0", 'D', 0},
		{"D;42", 'D', 42},
		{"D;1;extra", 'D', 1},
		{"D;abc", 'D', 0}, // non-int exit → 0
		{"D;", 'D', 0},
		{"", 0, 0},
	}
	for _, c := range cases {
		t.Run(c.v, func(t *testing.T) {
			code, exit := parseOSC133Value(c.v)
			if code != c.wantCode {
				t.Errorf("code = %q, want %q", code, c.wantCode)
			}
			if exit != c.wantExit {
				t.Errorf("exit = %d, want %d", exit, c.wantExit)
			}
		})
	}
}
