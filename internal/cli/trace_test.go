package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"polis/work/internal/index"
	"polis/work/internal/trace"

	"github.com/spf13/cobra"
)

func TestTraceCommandExists(t *testing.T) {
	root := NewRoot("test")
	found := false
	for _, cmd := range root.Commands() {
		if cmd.Name() == "trace" {
			found = true
			break
		}
	}
	if !found {
		t.Error("trace command should be registered")
	}
}

func TestTraceCommandFlags(t *testing.T) {
	root := NewRoot("test")
	var traceCmd *cobra.Command
	for _, cmd := range root.Commands() {
		if cmd.Name() == "trace" {
			traceCmd = cmd
			break
		}
	}
	if traceCmd == nil {
		t.Fatal("trace not found")
	}
	if traceCmd.Flags().Lookup("json") == nil {
		t.Error("expected --json flag")
	}
}

func TestPrettyPrintTrace(t *testing.T) {
	events := []trace.Event{
		{Timestamp: "2026-02-23T10:00:00Z", EventType: "begin", Agent: "zeus", Task: "test task", Bead: "test-123"},
		{Timestamp: "2026-02-23T10:00:05Z", EventType: "tool_call", Tool: "bash", Cmd: "go test"},
		{Timestamp: "2026-02-23T10:00:10Z", EventType: "end", Outcome: "success"},
	}

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)

	// Use the root cmd for output
	err := prettyPrintTrace(root, "test-123", events)
	if err != nil {
		t.Fatalf("pretty print failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Trace: test-123") {
		t.Error("should contain trace header")
	}
	if !strings.Contains(out, "BEGIN") {
		t.Error("should contain BEGIN")
	}
	if !strings.Contains(out, "TOOL") {
		t.Error("should contain TOOL")
	}
	if !strings.Contains(out, "END") {
		t.Error("should contain END")
	}
}

func TestFindTracePathByGlob(t *testing.T) {
	workDir := t.TempDir()

	// Create a trace file
	tr, err := trace.Open(workDir, "find-test", "zeus", "task")
	if err != nil {
		t.Fatalf("open trace: %v", err)
	}
	tr.Close("success", nil)

	// Should find it by glob
	path, err := findTracePath(workDir, "find-test")
	if err != nil {
		t.Fatalf("find trace: %v", err)
	}
	if !strings.Contains(path, "trace-find-test.jsonl") {
		t.Errorf("unexpected path: %s", path)
	}
}

func TestFindTracePathByIndex(t *testing.T) {
	workDir := t.TempDir()

	// Create a trace and index it
	tr, err := trace.Open(workDir, "idx-test", "zeus", "task")
	if err != nil {
		t.Fatalf("open trace: %v", err)
	}
	meta := tr.GetMetadata("success")
	tr.Close("success", nil)

	idx, err := index.Open(workDir)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	idx.Record(meta)
	idx.Close()

	// Should find via index
	path, err := findTracePath(workDir, "idx-test")
	if err != nil {
		t.Fatalf("find trace: %v", err)
	}
	if path != meta.FilePath {
		t.Errorf("expected %s, got %s", meta.FilePath, path)
	}
}

func TestFindTracePathNotFound(t *testing.T) {
	workDir := t.TempDir()
	os.MkdirAll(filepath.Join(workDir, "traces"), 0o755)

	_, err := findTracePath(workDir, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent trace")
	}
}
