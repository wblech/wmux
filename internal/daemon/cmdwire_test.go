package daemon

import (
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
