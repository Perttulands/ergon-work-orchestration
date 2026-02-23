package cli

import (
	"bytes"
	"testing"
)

func TestStatusCommandExists(t *testing.T) {
	root := NewRoot("test")
	found := false
	for _, cmd := range root.Commands() {
		if cmd.Name() == "status" {
			found = true
			break
		}
	}
	if !found {
		t.Error("status command should be registered")
	}
}

func TestStatusCommandRuns(t *testing.T) {
	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"status"})

	if err := root.Execute(); err != nil {
		t.Fatalf("status failed: %v", err)
	}
	// Should produce some output (either sessions or "no active")
	if buf.Len() == 0 {
		t.Error("expected output from status command")
	}
}

func TestStatusJSON(t *testing.T) {
	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"status", "--json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("status --json failed: %v", err)
	}
}
