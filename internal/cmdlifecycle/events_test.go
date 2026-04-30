package cmdlifecycle

import "testing"

func TestEventKind_Constants(t *testing.T) {
	if EventPromptShown == 0 {
		t.Error("EventPromptShown should not be the zero value (iota _ skipped)")
	}
	if EventCommandStarted == EventPromptShown {
		t.Error("EventCommandStarted should differ from EventPromptShown")
	}
	if EventCommandFinished == EventCommandStarted {
		t.Error("EventCommandFinished should differ from EventCommandStarted")
	}
}

func TestEvent_PromptShownHasNilCommand(t *testing.T) {
	ev := Event{Kind: EventPromptShown, Command: nil, Row: 42}
	if ev.Command != nil {
		t.Error("EventPromptShown should be safe to construct with nil Command")
	}
	if ev.Row != 42 {
		t.Errorf("Row = %d, want 42", ev.Row)
	}
}

func TestEventHandler_TypeAlias(t *testing.T) {
	var h EventHandler = func(sessID string, ev Event) {
		_ = sessID
		_ = ev
	}
	h("test", Event{Kind: EventPromptShown})
}
