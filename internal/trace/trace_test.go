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

	if _, err := os.Stat(filepath.Dir(tr.FilePath())); os.IsNotExist(err) {
		t.Error("trace directory should exist")
	}

	tr.Emit(Event{
		EventType: "tool_call",
		Tool:      "bash",
		Cmd:       "go test ./...",
	})

	if err := tr.Close("success", nil); err != nil {
		t.Fatalf("close trace: %v", err)
	}

	data, err := os.ReadFile(tr.FilePath())
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 trace events, got %d: %s", len(lines), string(data))
	}

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

	var tool Event
	if err := json.Unmarshal([]byte(lines[1]), &tool); err != nil {
		t.Fatalf("parse tool: %v", err)
	}
	if tool.EventType != "tool_call" {
		t.Errorf("second event should be tool_call, got: %s", tool.EventType)
	}

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

func TestEmitToolCall(t *testing.T) {
	workDir := t.TempDir()
	tr, err := Open(workDir, "tool-test", "zeus", "task")
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if err := tr.EmitToolCall("bash", "npm install", 1500); err != nil {
		t.Fatalf("emit tool call: %v", err)
	}
	tr.Close("success", nil)

	events, err := ReadTrace(tr.FilePath())
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}

	// begin, tool_call, end
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[1].EventType != "tool_call" {
		t.Errorf("expected tool_call, got %s", events[1].EventType)
	}
	if events[1].Tool != "bash" {
		t.Errorf("expected tool=bash, got %s", events[1].Tool)
	}
	if events[1].DurationMs == nil || *events[1].DurationMs != 1500 {
		t.Error("expected duration_ms=1500")
	}
}

func TestEmitFileWrite(t *testing.T) {
	workDir := t.TempDir()
	tr, err := Open(workDir, "fw-test", "zeus", "task")
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if err := tr.EmitFileWrite("src/auth.ts", 45); err != nil {
		t.Fatalf("emit file write: %v", err)
	}
	tr.Close("success", nil)

	events, err := ReadTrace(tr.FilePath())
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}

	if events[1].EventType != "file_write" {
		t.Errorf("expected file_write, got %s", events[1].EventType)
	}
	if events[1].Path != "src/auth.ts" {
		t.Errorf("expected path=src/auth.ts, got %s", events[1].Path)
	}
	if events[1].Lines == nil || *events[1].Lines != 45 {
		t.Error("expected lines=45")
	}
}

func TestEmitWorkerOutput(t *testing.T) {
	workDir := t.TempDir()
	tr, err := Open(workDir, "wo-test", "ares", "task")
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if err := tr.EmitWorkerOutput("compiling... done"); err != nil {
		t.Fatalf("emit: %v", err)
	}
	tr.Close("success", nil)

	events, err := ReadTrace(tr.FilePath())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if events[1].Output != "compiling... done" {
		t.Errorf("expected output content, got: %s", events[1].Output)
	}
}

func TestEmitError(t *testing.T) {
	workDir := t.TempDir()
	tr, err := Open(workDir, "err-test", "zeus", "task")
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if err := tr.EmitError("connection timeout"); err != nil {
		t.Fatalf("emit: %v", err)
	}
	tr.Close("error", nil)

	events, err := ReadTrace(tr.FilePath())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if events[1].EventType != "error" {
		t.Errorf("expected error event, got %s", events[1].EventType)
	}
	if events[1].Error != "connection timeout" {
		t.Errorf("expected error msg, got: %s", events[1].Error)
	}
}

func TestReadTrace(t *testing.T) {
	workDir := t.TempDir()
	tr, err := Open(workDir, "read-test", "zeus", "full task")
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	tr.EmitToolCall("bash", "go build", 500)
	tr.EmitFileWrite("main.go", 20)
	tr.EmitWorkerOutput("built ok")
	tr.Close("success", nil)

	events, err := ReadTrace(tr.FilePath())
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// begin, tool_call, file_write, worker_output, end = 5
	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}

	types := make([]string, len(events))
	for i, e := range events {
		types[i] = e.EventType
	}
	expected := []string{"begin", "tool_call", "file_write", "worker_output", "end"}
	for i, want := range expected {
		if types[i] != want {
			t.Errorf("event %d: got %s, want %s", i, types[i], want)
		}
	}
}

// TestBeginTwoEventsEnd validates the core lifecycle per spec:
// begin + 2 events + end produces correct JSONL records.
func TestBeginTwoEventsEnd(t *testing.T) {
	workDir := t.TempDir()
	tr, err := Open(workDir, "lifecycle-test", "mercury", "implement feature")
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	// Event 1: tool call
	if err := tr.EmitToolCall("bash", "go build ./...", 200); err != nil {
		t.Fatalf("emit tool call: %v", err)
	}
	// Event 2: file write
	if err := tr.EmitFileWrite("internal/auth.go", 30); err != nil {
		t.Fatalf("emit file write: %v", err)
	}
	if err := tr.Close("success", nil); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Read back and verify
	events, err := ReadTrace(tr.FilePath())
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	if len(events) != 4 {
		t.Fatalf("expected 4 JSONL records (begin + 2 events + end), got %d", len(events))
	}

	// Record 1: begin
	if events[0].EventType != "begin" {
		t.Errorf("record 0: event=%s, want begin", events[0].EventType)
	}
	if events[0].Bead != "lifecycle-test" {
		t.Errorf("begin: bead=%s, want lifecycle-test", events[0].Bead)
	}
	if events[0].Agent != "mercury" {
		t.Errorf("begin: agent=%s, want mercury", events[0].Agent)
	}
	if events[0].Task != "implement feature" {
		t.Errorf("begin: task=%s, want implement feature", events[0].Task)
	}
	if events[0].Timestamp == "" {
		t.Error("begin: timestamp should be set")
	}

	// Record 2: tool_call
	if events[1].EventType != "tool_call" {
		t.Errorf("record 1: event=%s, want tool_call", events[1].EventType)
	}
	if events[1].Cmd != "go build ./..." {
		t.Errorf("tool_call: cmd=%s", events[1].Cmd)
	}

	// Record 3: file_write
	if events[2].EventType != "file_write" {
		t.Errorf("record 2: event=%s, want file_write", events[2].EventType)
	}
	if events[2].Path != "internal/auth.go" {
		t.Errorf("file_write: path=%s", events[2].Path)
	}

	// Record 4: end
	if events[3].EventType != "end" {
		t.Errorf("record 3: event=%s, want end", events[3].EventType)
	}
	if events[3].Outcome != "success" {
		t.Errorf("end: outcome=%s, want success", events[3].Outcome)
	}
	if events[3].DurationS == nil {
		t.Error("end: duration_s should be set")
	}

	// Verify each line is valid JSON
	data, err := os.ReadFile(tr.FilePath())
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 JSONL lines, got %d", len(lines))
	}
	for i, line := range lines {
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			t.Errorf("line %d is not valid JSON: %v", i, err)
		}
		if _, ok := raw["ts"]; !ok {
			t.Errorf("line %d missing 'ts' field", i)
		}
		if _, ok := raw["event"]; !ok {
			t.Errorf("line %d missing 'event' field", i)
		}
	}
}

func TestReadTraceNonExistent(t *testing.T) {
	_, err := ReadTrace("/tmp/nonexistent-trace-file.jsonl")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestGetMetadata(t *testing.T) {
	workDir := t.TempDir()
	tr, err := Open(workDir, "meta-test", "apollo", "build UI")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer tr.Close("success", nil)

	meta := tr.GetMetadata("success")
	if meta.BeadID != "meta-test" {
		t.Errorf("bead=%s, want meta-test", meta.BeadID)
	}
	if meta.Agent != "apollo" {
		t.Errorf("agent=%s, want apollo", meta.Agent)
	}
	if meta.Task != "build UI" {
		t.Errorf("task=%s, want build UI", meta.Task)
	}
	if meta.Outcome != "success" {
		t.Errorf("outcome=%s, want success", meta.Outcome)
	}
	if meta.FilePath != tr.FilePath() {
		t.Errorf("path mismatch")
	}
}

func TestBeadID(t *testing.T) {
	workDir := t.TempDir()
	tr, err := Open(workDir, "bead-id-test", "zeus", "task")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer tr.Close("success", nil)

	if tr.BeadID() != "bead-id-test" {
		t.Errorf("BeadID()=%s, want bead-id-test", tr.BeadID())
	}
}
