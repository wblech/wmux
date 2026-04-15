package buildinfo

import "testing"

func TestDefaultVersion(t *testing.T) {
	if Version != "dev" {
		t.Errorf("expected default Version %q, got %q", "dev", Version)
	}
}

func TestDefaultCommit(t *testing.T) {
	if Commit != "unknown" {
		t.Errorf("expected default Commit %q, got %q", "unknown", Commit)
	}
}
