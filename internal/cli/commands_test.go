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
	"polis/work/internal/trace"
	"polis/work/internal/testutil"
)

// --- context command tests ---

func TestContextCommandExists(t *testing.T) {
	root := NewRoot("test")
	var found bool
	for _, cmd := range root.Commands() {
		if cmd.Name() == "context" {
			found = true
			break
		}
	}
	if !found {
		t.Error("context command should be registered")
	}
}

func TestContextCommandRuns(t *testing.T) {
	testutil.SandboxPATH(t, nil)

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"context", "--citizen", "test-agent", "--repo", t.TempDir()})

	if err := root.Execute(); err != nil {
		t.Fatalf("context failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("expected output from context command")
	}
}

func TestContextCommandWithCitizenFile(t *testing.T) {
	testutil.SandboxPATH(t, nil)

	workDir := t.TempDir()
	citizenDir := filepath.Join(workDir, "citizens")
	os.MkdirAll(citizenDir, 0o755)
	os.WriteFile(filepath.Join(citizenDir, "tester.md"), []byte("I test everything."), 0o644)

	// Set HOME so workDir is discovered
	t.Setenv("HOME", filepath.Dir(workDir))

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"context", "--citizen", "tester", "--repo", t.TempDir()})

	if err := root.Execute(); err != nil {
		t.Fatalf("context failed: %v", err)
	}
}

func TestContextCommandWithBeadID(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"git": `echo "abc1234 initial commit"`,
	})

	workDir := t.TempDir()
	citizenDir := filepath.Join(workDir, "citizens")
	os.MkdirAll(citizenDir, 0o755)
	os.WriteFile(filepath.Join(citizenDir, "zeus.md"),
		[]byte("Zeus has deep experience with auth systems."), 0o644)

	// Point HOME so ~/.work resolves to our temp dir
	homeDir := filepath.Dir(workDir)
	dotWork := filepath.Join(homeDir, ".work")
	os.Symlink(workDir, dotWork)
	t.Cleanup(func() { os.Remove(dotWork) })
	t.Setenv("HOME", homeDir)

	repoDir := t.TempDir()

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{
		"context", "pol-test-123",
		"--citizen", "zeus",
		"--repo", repoDir,
		"--task", "add authentication",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("context with bead ID failed: %v", err)
	}

	out := buf.String()
	if out == "" {
		t.Fatal("expected non-empty output from context command")
	}

	// Verify output contains citizen experience
	if !strings.Contains(out, "zeus") || !strings.Contains(out, "auth") {
		t.Error("output should contain citizen experience mentioning zeus and auth")
	}
}

func TestContextCommandOutputStructure(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"git": `echo "deadbeef some commit message"`,
	})

	workDir := t.TempDir()
	citizenDir := filepath.Join(workDir, "citizens")
	os.MkdirAll(citizenDir, 0o755)
	os.WriteFile(filepath.Join(citizenDir, "mercury.md"),
		[]byte("Experienced with CLI tools and testing."), 0o644)

	homeDir := filepath.Dir(workDir)
	dotWork := filepath.Join(homeDir, ".work")
	os.Symlink(workDir, dotWork)
	t.Cleanup(func() { os.Remove(dotWork) })
	t.Setenv("HOME", homeDir)

	repoDir := t.TempDir()

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{
		"context", "pol-test-456",
		"--citizen", "mercury",
		"--repo", repoDir,
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("context failed: %v", err)
	}

	out := buf.String()
	// With git mock and citizen file, we should get structured markdown sections
	if !strings.Contains(out, "Git History") && !strings.Contains(out, "Experience") {
		t.Error("output should contain markdown section headers (Git History or Experience)")
	}
}

func TestContextCommandNoArgs(t *testing.T) {
	testutil.SandboxPATH(t, nil)

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"context", "--repo", t.TempDir()})

	if err := root.Execute(); err != nil {
		t.Fatalf("context with no bead ID failed: %v", err)
	}
	// Should still produce output (fresh start message or gathered context)
	if buf.Len() == 0 {
		t.Error("expected output even without bead ID")
	}
}

// --- history command with seeded index ---

func TestHistoryWithSeededIndex(t *testing.T) {
	workDir := t.TempDir()

	// Create and seed index
	idx, err := index.Open(workDir)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}

	now := time.Now()
	for i, tc := range []struct {
		bead    string
		agent   string
		task    string
		outcome string
		dur     int64
	}{
		{"bead-001", "zeus", "add auth", "success", 120},
		{"bead-002", "mercury", "fix bug", "error", 30},
		{"bead-003", "apollo", "write tests", "success", 250},
	} {
		meta := trace.Metadata{
			BeadID:    tc.bead,
			Agent:     tc.agent,
			Task:      tc.task,
			Outcome:   tc.outcome,
			DurationS: tc.dur,
			StartTime: now.Add(-time.Duration(3-i) * time.Hour),
			EndTime:   now.Add(-time.Duration(3-i)*time.Hour + time.Duration(tc.dur)*time.Second),
			FilePath:  filepath.Join(workDir, "traces", tc.bead+".jsonl"),
		}
		if err := idx.Record(meta); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	idx.Close()

	// Test table output
	t.Setenv("HOME", filepath.Dir(workDir))
	// Mock workDir by creating a symlink from ~/.work to our temp dir
	homeDir := filepath.Dir(workDir)
	dotWork := filepath.Join(homeDir, ".work")
	os.Symlink(workDir, dotWork)
	t.Cleanup(func() { os.Remove(dotWork) })

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"history", "-n", "10"})
	t.Setenv("HOME", homeDir)

	if err := root.Execute(); err != nil {
		t.Fatalf("history failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Recent runs") {
		t.Errorf("expected 'Recent runs' header, got: %s", out)
	}
	if !strings.Contains(out, "bead-001") {
		t.Error("should contain bead-001")
	}
	if !strings.Contains(out, "success") {
		t.Error("should contain success outcome")
	}

	// Test JSON output
	buf.Reset()
	root = NewRoot("test")
	root.SetOut(buf)
	root.SetArgs([]string{"history", "--json", "-n", "2"})

	if err := root.Execute(); err != nil {
		t.Fatalf("history --json failed: %v", err)
	}
	var runs []index.RunRecord
	if err := json.Unmarshal(buf.Bytes(), &runs); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, buf.String())
	}
	if len(runs) != 2 {
		t.Errorf("expected 2 runs, got %d", len(runs))
	}
}

func TestHistoryEmpty(t *testing.T) {
	workDir := t.TempDir()
	homeDir := filepath.Dir(workDir)
	dotWork := filepath.Join(homeDir, ".work")
	os.Symlink(workDir, dotWork)
	t.Cleanup(func() { os.Remove(dotWork) })
	t.Setenv("HOME", homeDir)

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"history"})

	if err := root.Execute(); err != nil {
		t.Fatalf("history failed: %v", err)
	}
	if !strings.Contains(buf.String(), "No completed runs") {
		t.Error("expected 'No completed runs' for empty index")
	}
}

// --- trace command with seeded trace file ---

func TestTraceWithSeededData(t *testing.T) {
	workDir := t.TempDir()

	// Create a real trace
	tr, err := trace.Open(workDir, "test-trace-001", "zeus", "build feature")
	if err != nil {
		t.Fatalf("open trace: %v", err)
	}
	tr.EmitToolCall("bash", "go build", 500)
	tr.EmitFileWrite("main.go", 25)
	tr.EmitError("lint warning: unused var")
	meta := tr.GetMetadata("success")
	tr.Close("success", nil)

	// Index it
	idx, err := index.Open(workDir)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	idx.Record(meta)
	idx.Close()

	homeDir := filepath.Dir(workDir)
	dotWork := filepath.Join(homeDir, ".work")
	os.Symlink(workDir, dotWork)
	t.Cleanup(func() { os.Remove(dotWork) })
	t.Setenv("HOME", homeDir)

	// Test table output
	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"trace", "test-trace-001"})

	if err := root.Execute(); err != nil {
		t.Fatalf("trace failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Trace: test-trace-001") {
		t.Error("should contain trace header")
	}
	if !strings.Contains(out, "BEGIN") {
		t.Error("should contain BEGIN event")
	}
	if !strings.Contains(out, "TOOL") {
		t.Error("should contain TOOL event")
	}
	if !strings.Contains(out, "WRITE") {
		t.Error("should contain WRITE event")
	}
	if !strings.Contains(out, "ERROR") {
		t.Error("should contain ERROR event")
	}
	if !strings.Contains(out, "END") {
		t.Error("should contain END event")
	}

	// Test JSON output
	buf.Reset()
	root = NewRoot("test")
	root.SetOut(buf)
	root.SetArgs([]string{"trace", "test-trace-001", "--json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("trace --json failed: %v", err)
	}
	var events []trace.Event
	if err := json.Unmarshal(buf.Bytes(), &events); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(events) < 4 {
		t.Errorf("expected at least 4 events, got %d", len(events))
	}
}

func TestTraceNotFound(t *testing.T) {
	workDir := t.TempDir()
	os.MkdirAll(filepath.Join(workDir, "traces"), 0o755)

	homeDir := filepath.Dir(workDir)
	dotWork := filepath.Join(homeDir, ".work")
	os.Symlink(workDir, dotWork)
	t.Cleanup(func() { os.Remove(dotWork) })
	t.Setenv("HOME", homeDir)

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"trace", "nonexistent-bead"})

	err := root.Execute()
	if err == nil {
		t.Error("expected error for nonexistent trace")
	}
}

// --- feed command with seeded index ---

func TestFeedWithSeededIndex(t *testing.T) {
	workDir := t.TempDir()

	idx, err := index.Open(workDir)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	now := time.Now()
	meta := trace.Metadata{
		BeadID:    "feed-001",
		Agent:     "zeus",
		Task:      "add feature",
		Outcome:   "success",
		DurationS: 90,
		StartTime: now.Add(-30 * time.Minute),
		EndTime:   now.Add(-28 * time.Minute),
		FilePath:  "/tmp/trace.jsonl",
	}
	idx.Record(meta)
	idx.Close()

	homeDir := filepath.Dir(workDir)
	dotWork := filepath.Join(homeDir, ".work")
	os.Symlink(workDir, dotWork)
	t.Cleanup(func() { os.Remove(dotWork) })
	t.Setenv("HOME", homeDir)

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"feed", "--since", "1h"})

	if err := root.Execute(); err != nil {
		t.Fatalf("feed failed: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if out == "" {
		t.Fatal("expected feed output")
	}

	// Verify it's valid JSONL
	var entry FeedEntry
	if err := json.Unmarshal([]byte(out), &entry); err != nil {
		t.Fatalf("invalid JSONL: %v", err)
	}
	if entry.ID != "feed-001" {
		t.Errorf("id = %q, want feed-001", entry.ID)
	}
	if entry.Outcome != "success" {
		t.Errorf("outcome = %q, want success", entry.Outcome)
	}
}

func TestFeedFiltersBySince(t *testing.T) {
	workDir := t.TempDir()

	idx, err := index.Open(workDir)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	now := time.Now()
	// Recent run (30 minutes ago)
	idx.Record(trace.Metadata{
		BeadID: "recent", Agent: "zeus", Task: "new",
		Outcome: "success", DurationS: 60,
		StartTime: now.Add(-30 * time.Minute), EndTime: now.Add(-29 * time.Minute),
		FilePath: "/tmp/t.jsonl",
	})
	// Old run (3 hours ago)
	idx.Record(trace.Metadata{
		BeadID: "old", Agent: "zeus", Task: "old",
		Outcome: "success", DurationS: 60,
		StartTime: now.Add(-3 * time.Hour), EndTime: now.Add(-179 * time.Minute),
		FilePath: "/tmp/t2.jsonl",
	})
	idx.Close()

	homeDir := filepath.Dir(workDir)
	dotWork := filepath.Join(homeDir, ".work")
	os.Symlink(workDir, dotWork)
	t.Cleanup(func() { os.Remove(dotWork) })
	t.Setenv("HOME", homeDir)

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"feed", "--since", "1h"})

	if err := root.Execute(); err != nil {
		t.Fatalf("feed failed: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	lines := strings.Split(out, "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 entry (recent only), got %d", len(lines))
	}
	if !strings.Contains(out, "recent") {
		t.Error("should contain recent entry")
	}
	if strings.Contains(out, "old") {
		t.Error("should not contain old entry")
	}
}

// --- deliberate command degradation ---

func TestDeliberateRequiresSenate(t *testing.T) {
	testutil.SandboxPATH(t, nil) // no senate

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"deliberate", "should we use Go?"})

	err := root.Execute()
	if err == nil {
		t.Error("expected error when senate not on PATH")
	}
	if err != nil && !strings.Contains(err.Error(), "senate not on PATH") {
		t.Errorf("error should mention senate not on PATH, got: %v", err)
	}
}

// --- decide command tests ---

func TestDecideWithoutRelay(t *testing.T) {
	testutil.SandboxPATH(t, nil) // no relay, no br

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"decide", "which strategy?"})

	if err := root.Execute(); err != nil {
		t.Fatalf("decide failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Decision needed") {
		t.Error("should contain 'Decision needed'")
	}
	if !strings.Contains(out, "relay not available") {
		t.Error("should mention relay not available")
	}
	if !strings.Contains(out, "Gate bead") {
		t.Error("should output gate bead ID")
	}
}

func TestDecideWithRelay(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"relay": `echo "sent"`,
	})

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"decide", "which strategy?", "--decider", "athena"})

	if err := root.Execute(); err != nil {
		t.Fatalf("decide failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Notified: athena") {
		t.Error("should confirm notification sent")
	}
}

// --- version command tests ---

func TestVersionOutput(t *testing.T) {
	root := NewRoot("1.2.3")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("version failed: %v", err)
	}
	if !strings.Contains(buf.String(), "work 1.2.3") {
		t.Errorf("expected 'work 1.2.3', got %q", buf.String())
	}
}

// --- prettyPrintTrace edge cases ---

func TestPrettyPrintTraceAllEventTypes(t *testing.T) {
	dur100ms := int64(100)
	dur5s := int64(5)
	lines := 42
	passTrue := true
	score := 0.92

	events := []trace.Event{
		{Timestamp: "2026-02-27T10:00:00Z", EventType: "begin", Agent: "zeus", Task: "test", Bead: "b-123"},
		{Timestamp: "2026-02-27T10:00:01Z", EventType: "tool_call", Tool: "bash", Cmd: "go build", DurationMs: &dur100ms},
		{Timestamp: "2026-02-27T10:00:02Z", EventType: "file_write", Path: "main.go", Lines: &lines},
		{Timestamp: "2026-02-27T10:00:03Z", EventType: "gate_result", Pass: &passTrue, Score: &score},
		{Timestamp: "2026-02-27T10:00:04Z", EventType: "worker_output", Output: "all tests pass"},
		{Timestamp: "2026-02-27T10:00:05Z", EventType: "error", Error: "lint warning"},
		{Timestamp: "2026-02-27T10:00:06Z", EventType: "end", Outcome: "success", DurationS: &dur5s},
		{Timestamp: "2026-02-27T10:00:07Z", EventType: "custom_unknown"},
	}

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)

	err := prettyPrintTrace(root, "b-123", events)
	if err != nil {
		t.Fatalf("pretty print failed: %v", err)
	}

	out := buf.String()
	checks := []string{"BEGIN", "TOOL", "100ms", "WRITE", "42 lines", "GATE", "PASS", "0.92",
		"OUTPUT", "all tests pass", "ERROR", "lint warning", "END", "success", "5s", "custom_unknown"}
	for _, c := range checks {
		if !strings.Contains(out, c) {
			t.Errorf("output should contain %q", c)
		}
	}
}

func TestPrettyPrintTraceWorkerOutputTruncation(t *testing.T) {
	longOutput := strings.Repeat("x", 300)
	events := []trace.Event{
		{Timestamp: "2026-02-27T10:00:00Z", EventType: "worker_output", Output: longOutput},
	}

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)

	prettyPrintTrace(root, "test", events)
	out := buf.String()
	if strings.Contains(out, strings.Repeat("x", 250)) {
		t.Error("long output should be truncated")
	}
	if !strings.Contains(out, "...") {
		t.Error("truncated output should end with ...")
	}
}

func TestPrettyPrintTraceEndWithError(t *testing.T) {
	events := []trace.Event{
		{Timestamp: "2026-02-27T10:00:00Z", EventType: "end", Outcome: "error", Error: "spawn failed"},
	}

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)

	prettyPrintTrace(root, "test", events)
	out := buf.String()
	if !strings.Contains(out, "spawn failed") {
		t.Error("should show error in END event")
	}
}

func TestPrettyPrintTraceGateFail(t *testing.T) {
	passFalse := false
	score := 0.3

	events := []trace.Event{
		{Timestamp: "2026-02-27T10:00:00Z", EventType: "gate_result", Pass: &passFalse, Score: &score},
	}

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)

	prettyPrintTrace(root, "test", events)
	out := buf.String()
	if !strings.Contains(out, "FAIL") {
		t.Error("should show FAIL for gate")
	}
	if !strings.Contains(out, "0.30") {
		t.Error("should show score")
	}
}

// --- formatDuration edge cases ---

func TestFormatDurationEdgeCases(t *testing.T) {
	tests := []struct {
		sec  int64
		want string
	}{
		{0, "0s"},
		{1, "1s"},
		{59, "59s"},
		{60, "1m0s"},
		{61, "1m1s"},
		{3599, "59m59s"},
		{3600, "1h0m"},
		{7261, "2h1m"},
	}
	for _, tt := range tests {
		got := formatDuration(tt.sec)
		if got != tt.want {
			t.Errorf("formatDuration(%d) = %q, want %q", tt.sec, got, tt.want)
		}
	}
}
