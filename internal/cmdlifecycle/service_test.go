package cmdlifecycle

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// fixedClock returns a deterministic time that auto-advances by 1ms per call.
type fixedClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFixedClock() *fixedClock {
	return &fixedClock{t: time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)}
}

func (f *fixedClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.t
	f.t = f.t.Add(time.Millisecond)
	return now
}

// nullRepo is a Repository that discards Saves.
type nullRepo struct{}

func (nullRepo) Save(string, SessionState) {}

// captureRepo records every Save call.
type captureRepo struct {
	mu    sync.Mutex
	saves []captureSave
}

type captureSave struct {
	SessID string
	State  SessionState
}

func (r *captureRepo) Save(sessID string, state SessionState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.saves = append(r.saves, captureSave{SessID: sessID, State: state})
}

func (r *captureRepo) Saves() []captureSave {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]captureSave, len(r.saves))
	copy(out, r.saves)
	return out
}

func TestService_HandleOSC_HappyPath(t *testing.T) {
	clock := newFixedClock()
	svc := NewService(nullRepo{}, WithClock(clock.Now))
	var events []Event
	svc.OnEvent(func(sessID string, ev Event) {
		events = append(events, ev)
	})
	if err := svc.Register("s1"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := svc.HandleOSC("s1", 'A', 0, 10); err != nil {
		t.Fatalf("HandleOSC A: %v", err)
	}
	if err := svc.HandleOSC("s1", 'B', 0, 11); err != nil {
		t.Fatalf("HandleOSC B: %v", err)
	}
	if err := svc.HandleOSC("s1", 'C', 0, 12); err != nil {
		t.Fatalf("HandleOSC C: %v", err)
	}
	if err := svc.HandleOSC("s1", 'D', 0, 20); err != nil {
		t.Fatalf("HandleOSC D: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("got %d events, want 3: %+v", len(events), events)
	}
	if events[0].Kind != EventPromptShown {
		t.Errorf("events[0].Kind = %v, want EventPromptShown", events[0].Kind)
	}
	if events[0].Row != 10 {
		t.Errorf("events[0].Row = %d, want 10", events[0].Row)
	}
	if events[1].Kind != EventCommandStarted {
		t.Errorf("events[1].Kind = %v, want EventCommandStarted", events[1].Kind)
	}
	if events[1].Command == nil || events[1].Command.StartRow != 12 {
		t.Errorf("events[1] Command StartRow = %v, want 12", events[1].Command)
	}
	if events[2].Kind != EventCommandFinished {
		t.Errorf("events[2].Kind = %v, want EventCommandFinished", events[2].Kind)
	}
	if events[2].Command == nil {
		t.Fatal("events[2].Command is nil")
	}
	if events[2].Command.ExitCode != 0 {
		t.Errorf("events[2].Command.ExitCode = %d, want 0", events[2].Command.ExitCode)
	}
	if events[2].Command.EndRow != 20 {
		t.Errorf("events[2].Command.EndRow = %d, want 20", events[2].Command.EndRow)
	}
	if events[2].Command.Orphan {
		t.Error("events[2].Command.Orphan = true, want false")
	}

	state, err := svc.Snapshot("s1")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(state.History) != 1 {
		t.Errorf("len(History) = %d, want 1", len(state.History))
	}
	if state.InFlight != nil {
		t.Errorf("InFlight = %+v, want nil", state.InFlight)
	}
}

func TestService_HandleOSC_OrphanFromA(t *testing.T) {
	clock := newFixedClock()
	svc := NewService(nullRepo{}, WithClock(clock.Now))
	var events []Event
	svc.OnEvent(func(_ string, ev Event) { events = append(events, ev) })
	_ = svc.Register("s1")

	_ = svc.HandleOSC("s1", 'A', 0, 1)
	_ = svc.HandleOSC("s1", 'B', 0, 2)
	_ = svc.HandleOSC("s1", 'C', 0, 3)
	_ = svc.HandleOSC("s1", 'A', 0, 5)

	if len(events) != 4 {
		t.Fatalf("got %d events, want 4", len(events))
	}
	if events[2].Kind != EventCommandFinished {
		t.Errorf("events[2].Kind = %v, want Finished", events[2].Kind)
	}
	if !events[2].Command.Orphan {
		t.Error("orphan command should have Orphan=true")
	}
	if events[2].Command.ExitCode != -1 {
		t.Errorf("orphan ExitCode = %d, want -1", events[2].Command.ExitCode)
	}
	if events[2].Command.OrphanReason != OrphanReasonMissingD {
		t.Errorf("OrphanReason = %q, want %q", events[2].Command.OrphanReason, OrphanReasonMissingD)
	}
	if events[3].Kind != EventPromptShown {
		t.Errorf("events[3].Kind = %v, want PromptShown", events[3].Kind)
	}
}

func TestService_HandleOSC_DInIdleIsNoop(t *testing.T) {
	svc := NewService(nullRepo{})
	var events []Event
	svc.OnEvent(func(_ string, ev Event) { events = append(events, ev) })
	_ = svc.Register("s1")

	_ = svc.HandleOSC("s1", 'D', 0, 1)
	if len(events) != 0 {
		t.Errorf("D in Idle produced %d events, want 0", len(events))
	}
}

func TestService_HandleOSC_UnknownCodeIgnored(t *testing.T) {
	svc := NewService(nullRepo{})
	svc.OnEvent(func(_ string, _ Event) { t.Error("unknown code should not emit event") })
	_ = svc.Register("s1")

	if err := svc.HandleOSC("s1", 'Z', 0, 1); err != nil {
		t.Errorf("unknown code should not error, got %v", err)
	}
}

func TestService_HandleOSC_UnregisteredSession(t *testing.T) {
	svc := NewService(nullRepo{})
	if err := svc.HandleOSC("missing", 'A', 0, 0); !errors.Is(err, ErrSessionNotRegistered) {
		t.Errorf("got %v, want ErrSessionNotRegistered", err)
	}
}

func TestService_Register_DuplicateError(t *testing.T) {
	svc := NewService(nullRepo{})
	if err := svc.Register("s1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Register("s1"); !errors.Is(err, ErrSessionExists) {
		t.Errorf("got %v, want ErrSessionExists", err)
	}
}

func TestService_Unregister_Idempotent(t *testing.T) {
	svc := NewService(nullRepo{})
	_ = svc.Register("s1")
	svc.Unregister("s1")
	svc.Unregister("s1")
}

func TestService_Unregister_AllowsReregister(t *testing.T) {
	svc := NewService(nullRepo{})
	_ = svc.Register("s1")
	svc.Unregister("s1")
	if err := svc.Register("s1"); err != nil {
		t.Errorf("re-register after Unregister failed: %v", err)
	}
}

func TestService_FIFO_Eviction(t *testing.T) {
	clock := newFixedClock()
	svc := NewService(nullRepo{}, WithMaxHistory(3), WithClock(clock.Now))
	_ = svc.Register("s1")

	for i := 0; i < 5; i++ {
		_ = svc.HandleOSC("s1", 'A', 0, int64(i*2))
		_ = svc.HandleOSC("s1", 'C', 0, int64(i*2+1))
		_ = svc.HandleOSC("s1", 'D', i, int64(i*2+2))
	}

	state, err := svc.Snapshot("s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.History) != 3 {
		t.Fatalf("len(History) = %d, want 3", len(state.History))
	}
	if state.History[0].ExitCode != 2 {
		t.Errorf("oldest ExitCode = %d, want 2", state.History[0].ExitCode)
	}
	if state.History[2].ExitCode != 4 {
		t.Errorf("newest ExitCode = %d, want 4", state.History[2].ExitCode)
	}
}

func TestService_Snapshot_DeepCopy(t *testing.T) {
	svc := NewService(nullRepo{})
	_ = svc.Register("s1")
	_ = svc.HandleOSC("s1", 'A', 0, 1)
	_ = svc.HandleOSC("s1", 'C', 0, 2)
	_ = svc.HandleOSC("s1", 'D', 0, 3)

	state1, _ := svc.Snapshot("s1")
	if len(state1.History) != 1 {
		t.Fatalf("len(History) = %d, want 1", len(state1.History))
	}
	state1.History[0].ExitCode = 999

	state2, _ := svc.Snapshot("s1")
	if state2.History[0].ExitCode == 999 {
		t.Error("snapshot returned shared slice — internal state was mutated")
	}
}

func TestService_Restore_Roundtrip(t *testing.T) {
	clock := newFixedClock()
	svc1 := NewService(nullRepo{}, WithClock(clock.Now))
	_ = svc1.Register("s1")
	_ = svc1.HandleOSC("s1", 'A', 0, 1)
	_ = svc1.HandleOSC("s1", 'C', 0, 2)
	_ = svc1.HandleOSC("s1", 'D', 42, 3)
	state, _ := svc1.Snapshot("s1")

	svc2 := NewService(nullRepo{}, WithClock(clock.Now))
	if err := svc2.Restore("s1", state); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	state2, _ := svc2.Snapshot("s1")
	if len(state2.History) != 1 {
		t.Fatalf("len(History) = %d, want 1", len(state2.History))
	}
	if state2.History[0].ExitCode != 42 {
		t.Errorf("ExitCode = %d, want 42", state2.History[0].ExitCode)
	}
}

func TestService_Restore_InFlightBecomesOrphan(t *testing.T) {
	clock := newFixedClock()
	svc := NewService(nullRepo{}, WithClock(clock.Now))
	state := SessionState{
		History: nil,
		InFlight: &Command{
			StartedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			StartRow:  100,
		},
	}
	if err := svc.Restore("s1", state); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Snapshot("s1")
	if len(got.History) != 1 {
		t.Fatalf("InFlight should become an orphan history entry; len=%d", len(got.History))
	}
	if got.InFlight != nil {
		t.Error("after Restore, InFlight must be nil — tracker is in StateIdle")
	}
	o := got.History[0]
	if !o.Orphan {
		t.Error("Orphan = false, want true")
	}
	if o.OrphanReason != OrphanReasonDaemonRestart {
		t.Errorf("OrphanReason = %q, want %q", o.OrphanReason, OrphanReasonDaemonRestart)
	}
	if o.EndRow != 100 {
		t.Errorf("EndRow = %d, want 100 (== StartRow)", o.EndRow)
	}
	if o.ExitCode != -1 {
		t.Errorf("ExitCode = %d, want -1", o.ExitCode)
	}
}

func TestService_Restore_SessionAlreadyRegistered(t *testing.T) {
	svc := NewService(nullRepo{})
	_ = svc.Register("s1")
	if err := svc.Restore("s1", SessionState{}); !errors.Is(err, ErrSessionExists) {
		t.Errorf("got %v, want ErrSessionExists", err)
	}
}

func TestService_ConcurrentSessions(t *testing.T) {
	svc := NewService(nullRepo{})
	const sessions = 50
	const cycles = 20

	var wg sync.WaitGroup
	for i := 0; i < sessions; i++ {
		sessID := "session-" + string(rune('a'+(i%26))) + "-" + string(rune('0'+(i/26)))
		if err := svc.Register(sessID); err != nil {
			continue
		}
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			for j := 0; j < cycles; j++ {
				_ = svc.HandleOSC(id, 'A', 0, int64(j*4))
				_ = svc.HandleOSC(id, 'B', 0, int64(j*4+1))
				_ = svc.HandleOSC(id, 'C', 0, int64(j*4+2))
				_ = svc.HandleOSC(id, 'D', 0, int64(j*4+3))
			}
		}(sessID)
	}
	wg.Wait()
}

func TestService_RepoSavedOnFinished(t *testing.T) {
	repo := &captureRepo{}
	svc := NewService(repo)
	_ = svc.Register("s1")

	_ = svc.HandleOSC("s1", 'A', 0, 1)
	_ = svc.HandleOSC("s1", 'C', 0, 2)
	saves := repo.Saves()
	if len(saves) != 0 {
		t.Errorf("Save called %d times before D, want 0", len(saves))
	}
	_ = svc.HandleOSC("s1", 'D', 0, 3)

	saves = repo.Saves()
	if len(saves) != 1 {
		t.Fatalf("Save called %d times, want 1", len(saves))
	}
	if saves[0].SessID != "s1" {
		t.Errorf("Save sessID = %q", saves[0].SessID)
	}
	if len(saves[0].State.History) != 1 {
		t.Errorf("Save state history len = %d, want 1", len(saves[0].State.History))
	}
}
