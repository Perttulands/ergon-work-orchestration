package trace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenAndClose(t *testing.T) {
	workDir := t.TempDir()

	tr, err := Open(workDir, "test-bead-123", "zeus", "add auth")
	if err != nil {
		t.Fatalf("open trace: %v", err)
	}

	// Verify trace dir was created
	if _, err := os.Stat(filepath.Dir(tr.FilePath())); os.IsNotExist(err) {
		t.Error("trace directory should exist")
	}

	// Emit a custom event
	tr.Emit(Event{
		EventType: "tool_call",
		Tool:      "bash",
		Cmd:       "go test ./...",
	})

	// Close with success
	if err := tr.Close("success", nil); err != nil {
		t.Fatalf("close trace: %v", err)
	}

	// Read the trace file and verify JSONL
	data, err := os.ReadFile(tr.FilePath())
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 trace events, got %d: %s", len(lines), string(data))
	}

	// Verify begin event
	var begin Event
	if err := json.Unmarshal([]byte(lines[0]), &begin); err != nil {
		t.Fatalf("parse begin: %v", err)
	}
	if begin.EventType != "begin" {
		t.Errorf("first event should be begin, got: %s", begin.EventType)
	}
	if begin.Agent != "zeus" {
		t.Errorf("agent should be zeus, got: %s", begin.Agent)
	}
	if begin.Bead != "test-bead-123" {
		t.Errorf("bead should be test-bead-123, got: %s", begin.Bead)
	}

	// Verify tool_call event
	var tool Event
	if err := json.Unmarshal([]byte(lines[1]), &tool); err != nil {
		t.Fatalf("parse tool: %v", err)
	}
	if tool.EventType != "tool_call" {
		t.Errorf("second event should be tool_call, got: %s", tool.EventType)
	}
	if tool.Cmd != "go test ./..." {
		t.Errorf("cmd should be 'go test ./...', got: %s", tool.Cmd)
	}

	// Verify end event
	var end Event
	if err := json.Unmarshal([]byte(lines[2]), &end); err != nil {
		t.Fatalf("parse end: %v", err)
	}
	if end.EventType != "end" {
		t.Errorf("third event should be end, got: %s", end.EventType)
	}
	if end.Outcome != "success" {
		t.Errorf("outcome should be success, got: %s", end.Outcome)
	}
}

func TestCloseWithError(t *testing.T) {
	workDir := t.TempDir()

	tr, err := Open(workDir, "test-err", "ares", "fix bug")
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	tr.Close("error", os.ErrNotExist)

	data, err := os.ReadFile(tr.FilePath())
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var end Event
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &end); err != nil {
		t.Fatalf("parse end: %v", err)
	}
	if end.Error == "" {
		t.Error("expected error field to be set")
	}
}

func TestTraceFilePath(t *testing.T) {
	workDir := t.TempDir()
	tr, err := Open(workDir, "path-test", "apollo", "task")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer tr.Close("success", nil)

	if !strings.Contains(tr.FilePath(), "trace-path-test.jsonl") {
		t.Errorf("file path should contain bead ID: %s", tr.FilePath())
	}
}
