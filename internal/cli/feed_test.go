package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"polis/work/internal/index"

	"github.com/spf13/cobra"
)

func TestFeedCommandExists(t *testing.T) {
	root := NewRoot("test")
	found := false
	for _, cmd := range root.Commands() {
		if cmd.Name() == "feed" {
			found = true
			break
		}
	}
	if !found {
		t.Error("feed command should be registered")
	}
}

func TestFeedCommandFlags(t *testing.T) {
	root := NewRoot("test")
	var feedCmd *cobra.Command
	for _, cmd := range root.Commands() {
		if cmd.Name() == "feed" {
			feedCmd = cmd
			break
		}
	}
	if feedCmd == nil {
		t.Fatal("feed command not found")
	}
	if feedCmd.Flags().Lookup("since") == nil {
		t.Error("expected --since flag")
	}
}

func TestParseSince(t *testing.T) {
	tests := []struct {
		input string
		ok    bool
	}{
		{"1h", true},
		{"24h", true},
		{"7d", true},
		{"30m", true},
		{"x", false},
		{"", false},
		{"24x", false},
	}

	for _, tt := range tests {
		result, err := parseSince(tt.input)
		if tt.ok && err != nil {
			t.Errorf("parseSince(%q) unexpected error: %v", tt.input, err)
		}
		if !tt.ok && err == nil {
			t.Errorf("parseSince(%q) expected error", tt.input)
		}
		if tt.ok {
			if result.After(time.Now()) {
				t.Errorf("parseSince(%q) returned future time", tt.input)
			}
		}
	}
}

func TestFeedEntryJSON(t *testing.T) {
	dur := 300
	entry := FeedEntry{
		ID:        "work-abc",
		Task:      "add auth",
		Outcome:   "success",
		DurationS: &dur,
		Timestamp: "2026-02-23T10:00:00Z",
		Agent:     "zeus",
		Metadata: map[string]any{
			"bead_id": "work-abc",
			"citizen": "zeus",
		},
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	s := string(data)
	// learning-loop required fields
	for _, field := range []string{"id", "task", "outcome"} {
		if !strings.Contains(s, `"`+field+`"`) {
			t.Errorf("JSON should contain %q", field)
		}
	}
	// learning-loop optional fields
	for _, field := range []string{"duration_seconds", "timestamp", "agent"} {
		if !strings.Contains(s, `"`+field+`"`) {
			t.Errorf("JSON should contain %q", field)
		}
	}
	// metadata with work-specific fields
	if !strings.Contains(s, "metadata") {
		t.Error("JSON should contain metadata")
	}
	if !strings.Contains(s, "bead_id") {
		t.Error("metadata should contain bead_id")
	}
	if !strings.Contains(s, "citizen") {
		t.Error("metadata should contain citizen")
	}

	// error_message should be omitted when empty
	if strings.Contains(s, "error_message") {
		t.Error("error_message should be omitted when empty")
	}
}

func TestFeedEntryWithGateData(t *testing.T) {
	dur := 120
	entry := FeedEntry{
		ID:        "pol-w123",
		Task:      "fix bug",
		Outcome:   "success",
		DurationS: &dur,
		Timestamp: "2026-02-23T10:00:00Z",
		Agent:     "poseidon",
		Metadata: map[string]any{
			"bead_id":    "pol-w123",
			"citizen":    "poseidon",
			"gate_score": 0.95,
			"gate_pass":  true,
		},
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	s := string(data)
	if !strings.Contains(s, "gate_score") {
		t.Error("metadata should contain gate_score when present")
	}
	if !strings.Contains(s, "gate_pass") {
		t.Error("metadata should contain gate_pass when present")
	}
}

func TestMapOutcome(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"success", "success"},
		{"gate_fail", "failure"},
		{"timeout", "error"},
		{"error", "error"},
		{"incomplete", "error"},
		{"unknown", "error"},
	}

	for _, tt := range tests {
		got := mapOutcome(tt.input)
		if got != tt.want {
			t.Errorf("mapOutcome(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBuildFeedEntry(t *testing.T) {
	r := index.RunRecord{
		TraceID:   "pol-t1",
		Agent:     "poseidon",
		Task:      "implement feature",
		BeadID:    "pol-t1",
		StartTime: time.Date(2026, 2, 23, 10, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 2, 23, 10, 5, 0, 0, time.UTC),
		Outcome:   "success",
		DurationS: 300,
		TracePath: "/nonexistent/trace.jsonl", // will silently skip enrichment
	}

	entry := buildFeedEntry(r)

	if entry.ID != "pol-t1" {
		t.Errorf("ID = %q, want pol-t1", entry.ID)
	}
	if entry.Task != "implement feature" {
		t.Errorf("Task = %q, want 'implement feature'", entry.Task)
	}
	if entry.Outcome != "success" {
		t.Errorf("Outcome = %q, want success", entry.Outcome)
	}
	if entry.DurationS == nil || *entry.DurationS != 300 {
		t.Errorf("DurationS = %v, want 300", entry.DurationS)
	}
	if entry.Agent != "poseidon" {
		t.Errorf("Agent = %q, want poseidon", entry.Agent)
	}
	if entry.Metadata["bead_id"] != "pol-t1" {
		t.Errorf("metadata.bead_id = %v, want pol-t1", entry.Metadata["bead_id"])
	}
	if entry.Metadata["citizen"] != "poseidon" {
		t.Errorf("metadata.citizen = %v, want poseidon", entry.Metadata["citizen"])
	}
}

func TestBuildFeedEntryGateFail(t *testing.T) {
	r := index.RunRecord{
		Outcome: "gate_fail",
	}
	entry := buildFeedEntry(r)
	if entry.Outcome != "failure" {
		t.Errorf("gate_fail should map to failure, got %q", entry.Outcome)
	}
}

func TestEnrichFromTrace(t *testing.T) {
	// Create a temporary trace file with gate_result
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "trace-test.jsonl")

	lines := []string{
		`{"ts":"2026-02-23T10:00:00Z","event":"begin","agent":"poseidon","task":"test","bead":"pol-t1"}`,
		`{"ts":"2026-02-23T10:02:00Z","event":"gate_result","pass":true,"score":0.88}`,
		`{"ts":"2026-02-23T10:03:00Z","event":"end","outcome":"success","duration_s":180,"agent":"poseidon","bead":"pol-t1"}`,
	}
	os.WriteFile(tracePath, []byte(strings.Join(lines, "\n")+"\n"), 0o644)

	entry := &FeedEntry{
		Metadata: map[string]any{},
	}
	enrichFromTrace(entry, tracePath)

	score, ok := entry.Metadata["gate_score"].(float64)
	if !ok || score != 0.88 {
		t.Errorf("gate_score = %v, want 0.88", entry.Metadata["gate_score"])
	}
	pass, ok := entry.Metadata["gate_pass"].(bool)
	if !ok || !pass {
		t.Errorf("gate_pass = %v, want true", entry.Metadata["gate_pass"])
	}
}

func TestEnrichFromTraceWithError(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "trace-err.jsonl")

	lines := []string{
		`{"ts":"2026-02-23T10:00:00Z","event":"begin","agent":"zeus","task":"test","bead":"pol-t2"}`,
		`{"ts":"2026-02-23T10:01:00Z","event":"error","error":"connection refused"}`,
		`{"ts":"2026-02-23T10:01:30Z","event":"end","outcome":"error","duration_s":90,"agent":"zeus","bead":"pol-t2"}`,
	}
	os.WriteFile(tracePath, []byte(strings.Join(lines, "\n")+"\n"), 0o644)

	entry := &FeedEntry{
		Metadata: map[string]any{},
	}
	enrichFromTrace(entry, tracePath)

	if entry.ErrorMsg != "connection refused" {
		t.Errorf("ErrorMsg = %q, want 'connection refused'", entry.ErrorMsg)
	}
}

func TestEnrichFromTraceMissingFile(t *testing.T) {
	entry := &FeedEntry{
		Metadata: map[string]any{},
	}
	// Should not panic on missing file
	enrichFromTrace(entry, "/nonexistent/trace.jsonl")
}

func TestFeedOutputIsJSONL(t *testing.T) {
	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"feed", "--since", "1h"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("feed failed: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if out == "" {
		return // empty is valid
	}
	for _, line := range strings.Split(out, "\n") {
		var entry FeedEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("line is not valid JSON: %s", line)
		}
	}
}

func TestFeedOutputLearningLoopCompatible(t *testing.T) {
	// Verify feed output can be unmarshalled into learning-loop Run schema
	dur := 300
	entry := FeedEntry{
		ID:        "pol-t1",
		Task:      "fix bug",
		Outcome:   "success",
		DurationS: &dur,
		Timestamp: "2026-02-23T10:00:00Z",
		Agent:     "poseidon",
		Metadata: map[string]any{
			"bead_id":    "pol-t1",
			"citizen":    "poseidon",
			"gate_score": 0.95,
		},
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Simulate learning-loop ingestion: unmarshal into a generic map
	// to verify required fields are present
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// learning-loop required: id, task, outcome
	for _, field := range []string{"id", "task", "outcome"} {
		if _, ok := m[field]; !ok {
			t.Errorf("missing learning-loop required field: %s", field)
		}
	}

	// outcome must be valid for learning-loop
	outcome := m["outcome"].(string)
	validOutcomes := map[string]bool{"success": true, "partial": true, "failure": true, "error": true}
	if !validOutcomes[outcome] {
		t.Errorf("outcome %q is not valid for learning-loop", outcome)
	}
}
