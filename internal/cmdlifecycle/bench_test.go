package cmdlifecycle

import "testing"

// BenchmarkHandleOSC_NoOp measures the cost of a HandleOSC call that
// triggers no transition (e.g. 'D' in StateIdle). Gate: 0 allocs/op.
func BenchmarkHandleOSC_NoOp(b *testing.B) {
	svc := NewService(nullRepo{})
	_ = svc.Register("s1")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = svc.HandleOSC("s1", 'D', 0, 0)
	}
}

// BenchmarkHandleOSC_FullCycle measures the cost of one A→B→C→D cycle.
func BenchmarkHandleOSC_FullCycle(b *testing.B) {
	svc := NewService(nullRepo{})
	svc.OnEvent(func(string, Event) {})
	_ = svc.Register("s1")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = svc.HandleOSC("s1", 'A', 0, 0)
		_ = svc.HandleOSC("s1", 'B', 0, 0)
		_ = svc.HandleOSC("s1", 'C', 0, 0)
		_ = svc.HandleOSC("s1", 'D', 0, 0)
	}
}

// BenchmarkSnapshot_500Commands measures Snapshot cost on a full history.
func BenchmarkSnapshot_500Commands(b *testing.B) {
	svc := NewService(nullRepo{})
	svc.OnEvent(func(string, Event) {})
	_ = svc.Register("s1")
	for i := 0; i < 500; i++ {
		_ = svc.HandleOSC("s1", 'A', 0, 0)
		_ = svc.HandleOSC("s1", 'C', 0, 0)
		_ = svc.HandleOSC("s1", 'D', 0, 0)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.Snapshot("s1")
	}
}
