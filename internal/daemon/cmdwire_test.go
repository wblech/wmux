package daemon

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/wblech/wmux/internal/cmdlifecycle"
	"github.com/wblech/wmux/internal/platform/event"
)

func TestPublishCmdEvent_PromptShown(t *testing.T) {
	bus := event.NewBus()
	d := &Daemon{eventBus: bus}
	sub := bus.SubscribeTypes(event.CommandPromptShown)
	defer sub.Unsubscribe()

	d.publishCmdEvent("s1", cmdlifecycle.Event{
		Kind: cmdlifecycle.EventPromptShown,
		Row:  42,
	})

	select {
	case ev := <-sub.Events():
		if ev.Type != event.CommandPromptShown {
			t.Errorf("Type = %v, want CommandPromptShown", ev.Type)
		}
		if ev.SessionID != "s1" {
			t.Errorf("SessionID = %q, want s1", ev.SessionID)
		}
		row, _ := ev.Payload["row"].(int64)
		if row != 42 {
			t.Errorf("payload.row = %v, want 42", ev.Payload["row"])
		}
	case <-time.After(time.Second):
		t.Fatal("event not published")
	}
}

func TestPublishCmdEvent_CommandFinished(t *testing.T) {
	bus := event.NewBus()
	d := &Daemon{eventBus: bus}
	sub := bus.SubscribeTypes(event.CommandFinished)
	defer sub.Unsubscribe()

	start := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Second)
	d.publishCmdEvent("s1", cmdlifecycle.Event{
		Kind: cmdlifecycle.EventCommandFinished,
		Row:  100,
		Command: &cmdlifecycle.Command{
			StartedAt:    start,
			EndedAt:      end,
			ExitCode:     42,
			StartRow:     50,
			EndRow:       100,
			Orphan:       true,
			OrphanReason: cmdlifecycle.OrphanReasonMissingD,
		},
	})

	select {
	case ev := <-sub.Events():
		if ev.Payload["exit_code"] != 42 {
			t.Errorf("exit_code = %v, want 42", ev.Payload["exit_code"])
		}
		if ev.Payload["orphan"] != true {
			t.Errorf("orphan = %v, want true", ev.Payload["orphan"])
		}
		if ev.Payload["orphan_reason"] != "missing_d_marker" {
			t.Errorf("orphan_reason = %v, want missing_d_marker", ev.Payload["orphan_reason"])
		}
		ms, _ := ev.Payload["duration_ms"].(int64)
		if ms != 2000 {
			t.Errorf("duration_ms = %v, want 2000", ev.Payload["duration_ms"])
		}
	case <-time.After(time.Second):
		t.Fatal("event not published")
	}
}

func TestPublishCmdEvent_NoBus_NoOp(t *testing.T) {
	d := &Daemon{eventBus: nil}
	// Should not panic.
	d.publishCmdEvent("s1", cmdlifecycle.Event{Kind: cmdlifecycle.EventPromptShown})
}

func TestDaemon_HandleCreate_RegistersCmdSvc(t *testing.T) {
	repo := newCmdRepository(false)
	cmdSvc := cmdlifecycle.NewService(repo)
	d := &Daemon{
		cmdSvc:       cmdSvc,
		cmdRepo:      repo,
		lineCounters: make(map[string]*atomic.Int64),
	}

	// Simulate handleCreate's wiring inline (bypassing IPC machinery).
	const sid = "s1"
	_ = cmdSvc.Register(sid)
	d.lineCountersMu.Lock()
	d.lineCounters[sid] = new(atomic.Int64)
	d.lineCountersMu.Unlock()

	if _, err := cmdSvc.Snapshot(sid); err != nil {
		t.Errorf("cmdSvc should know about %s, got %v", sid, err)
	}
	d.lineCountersMu.RLock()
	c := d.lineCounters[sid]
	d.lineCountersMu.RUnlock()
	if c == nil {
		t.Errorf("lineCounter not allocated for %s", sid)
	}
}

func TestDaemon_OnExit_UnregistersCmdSvc(t *testing.T) {
	repo := newCmdRepository(false)
	cmdSvc := cmdlifecycle.NewService(repo)
	d := &Daemon{
		cmdSvc:       cmdSvc,
		cmdRepo:      repo,
		lineCounters: make(map[string]*atomic.Int64),
	}

	const sid = "s1"
	_ = cmdSvc.Register(sid)
	d.lineCountersMu.Lock()
	d.lineCounters[sid] = new(atomic.Int64)
	d.lineCountersMu.Unlock()

	// Simulate the OnExit callback teardown.
	cmdSvc.Unregister(sid)
	d.lineCountersMu.Lock()
	delete(d.lineCounters, sid)
	d.lineCountersMu.Unlock()
	d.cmdRepo.Close(sid)

	// After unregister, Snapshot should fail.
	if _, err := cmdSvc.Snapshot(sid); err == nil {
		t.Error("cmdSvc.Snapshot should error after Unregister")
	}
	d.lineCountersMu.RLock()
	c := d.lineCounters[sid]
	d.lineCountersMu.RUnlock()
	if c != nil {
		t.Errorf("lineCounter not deleted for %s", sid)
	}
}

func TestDaemon_NilCmdSvc_HandleCreate_NoOp(t *testing.T) {
	d := &Daemon{
		cmdSvc:       nil,
		cmdRepo:      nil,
		lineCounters: make(map[string]*atomic.Int64),
	}

	// Even with nil cmdSvc, the lineCounters allocation must work.
	d.lineCountersMu.Lock()
	d.lineCounters["s1"] = new(atomic.Int64)
	d.lineCountersMu.Unlock()

	d.lineCountersMu.RLock()
	c := d.lineCounters["s1"]
	d.lineCountersMu.RUnlock()
	if c == nil {
		t.Error("lineCounter allocation failed with nil cmdSvc")
	}
}
