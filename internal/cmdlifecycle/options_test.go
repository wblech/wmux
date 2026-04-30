package cmdlifecycle

import (
	"testing"
	"time"
)

func TestWithMaxHistory(t *testing.T) {
	s := &Service{maxHistory: 0}
	WithMaxHistory(123)(s)
	if s.maxHistory != 123 {
		t.Errorf("maxHistory = %d, want 123", s.maxHistory)
	}
}

func TestWithMaxHistory_RejectsNonPositive(t *testing.T) {
	s := &Service{maxHistory: 500}
	WithMaxHistory(0)(s)
	if s.maxHistory != 500 {
		t.Errorf("maxHistory = %d, want 500 (unchanged for non-positive)", s.maxHistory)
	}
	WithMaxHistory(-1)(s)
	if s.maxHistory != 500 {
		t.Errorf("maxHistory = %d, want 500 (unchanged for non-positive)", s.maxHistory)
	}
}

func TestWithClock(t *testing.T) {
	fixed := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	s := &Service{}
	WithClock(func() time.Time { return fixed })(s)
	if s.now == nil {
		t.Fatal("now should be set")
	}
	if !s.now().Equal(fixed) {
		t.Errorf("now() = %v, want %v", s.now(), fixed)
	}
}

func TestWithClock_NilIgnored(t *testing.T) {
	original := func() time.Time { return time.Unix(0, 0) }
	s := &Service{now: original}
	WithClock(nil)(s)
	if s.now == nil {
		t.Error("nil clock should not overwrite existing clock")
	}
}
