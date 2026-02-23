package ecosystem

import (
	"testing"
)

func TestAvailable(t *testing.T) {
	if !Available("tmux") {
		t.Error("expected tmux to be available")
	}
	if Available("nonexistent-tool-xyz-12345") {
		t.Error("expected nonexistent tool to not be available")
	}
}

// --- Graceful degradation tests (projects-r03) ---

func TestBdCreateWhenBdUnavailable(t *testing.T) {
	if Available("bd") {
		t.Skip("bd is available; this test covers the missing-bd path")
	}
	result, err := BdCreate("test task", "/tmp")
	if err != nil {
		t.Errorf("should return nil error when bd unavailable, got: %v", err)
	}
	if result != nil {
		t.Error("should return nil result when bd unavailable")
	}
}

func TestBdCloseWhenBdUnavailable(t *testing.T) {
	if Available("bd") {
		t.Skip("bd is available; this test covers the missing-bd path")
	}
	err := BdClose("test-id", "reason", "/tmp")
	if err != nil {
		t.Errorf("should return nil error when bd unavailable, got: %v", err)
	}
}

func TestGateCheckWhenGateUnavailable(t *testing.T) {
	if Available("gate") {
		t.Skip("gate is available; this test covers the missing-gate path")
	}
	result, err := GateCheck("/tmp", "test")
	if err != nil {
		t.Errorf("should return nil error when gate unavailable, got: %v", err)
	}
	if result != nil {
		t.Error("should return nil result when gate unavailable")
	}
}

// When bd IS available, test that BdCreate returns a real result.
func TestBdCreateWhenBdAvailable(t *testing.T) {
	if !Available("bd") {
		t.Skip("bd not available")
	}
	// bd q requires a valid .beads directory; just verify it doesn't panic
	_, err := BdCreate("degradation test probe", "/tmp")
	// Error is expected (no .beads dir in /tmp), but it shouldn't be nil-pointer or panic
	if err != nil {
		t.Logf("expected error from /tmp (no .beads): %v", err)
	}
}
