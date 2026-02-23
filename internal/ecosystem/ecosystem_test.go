package ecosystem

import (
	"testing"
)

func TestAvailable(t *testing.T) {
	// tmux should be available in our environment
	if !Available("tmux") {
		t.Error("expected tmux to be available")
	}
	// something definitely not available
	if Available("nonexistent-tool-xyz") {
		t.Error("expected nonexistent tool to not be available")
	}
}

func TestBdCreateGracefulDegradation(t *testing.T) {
	// When bd is not available (or returns error), should degrade gracefully
	// We test the Available check path
	if !Available("bd") {
		result, err := BdCreate("test task", "/tmp")
		if err != nil {
			t.Errorf("expected nil error when bd not available, got: %v", err)
		}
		if result != nil {
			t.Error("expected nil result when bd not available")
		}
	}
}

func TestGateCheckGracefulDegradation(t *testing.T) {
	if !Available("gate") {
		result, err := GateCheck("/tmp", "test")
		if err != nil {
			t.Errorf("expected nil error when gate not available, got: %v", err)
		}
		if result != nil {
			t.Error("expected nil result when gate not available")
		}
	}
}
