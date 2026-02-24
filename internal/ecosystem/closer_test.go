package ecosystem

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeriveCloseReasonFromTrace(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "trace-test.jsonl")
	traceContent := `{"ts":"2026-02-23T14:00:00Z","event":"begin","agent":"zeus","task":"add auth","bead":"work-abc"}
{"ts":"2026-02-23T14:00:05Z","event":"tool_call","tool":"bash","cmd":"go test"}
{"ts":"2026-02-23T14:00:10Z","event":"file_write","path":"auth.go","lines":45}
{"ts":"2026-02-23T14:00:15Z","event":"file_write","path":"auth_test.go","lines":30}
{"ts":"2026-02-23T14:00:20Z","event":"gate_result","pass":true,"score":0.87}
{"ts":"2026-02-23T14:00:25Z","event":"end","outcome":"success","duration_s":25}
`
	os.WriteFile(tracePath, []byte(traceContent), 0o644)

	cr, err := DeriveCloseReason(tracePath, t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cr.Outcome != "success" {
		t.Errorf("outcome = %q, want success", cr.Outcome)
	}
	if cr.DurationS != 25 {
		t.Errorf("duration = %d, want 25", cr.DurationS)
	}
	if len(cr.FilesWritten) != 2 {
		t.Errorf("files written = %d, want 2", len(cr.FilesWritten))
	}
	if cr.ToolCalls != 1 {
		t.Errorf("tool calls = %d, want 1", cr.ToolCalls)
	}
	if cr.GatePass == nil || !*cr.GatePass {
		t.Error("gate should be pass")
	}
	if cr.GateScore == nil || *cr.GateScore != 0.87 {
		t.Errorf("gate score = %v, want 0.87", cr.GateScore)
	}
}

func TestDeriveCloseReasonError(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "trace-err.jsonl")
	traceContent := `{"ts":"2026-02-23T14:00:00Z","event":"begin","agent":"zeus","task":"add auth","bead":"work-abc"}
{"ts":"2026-02-23T14:00:05Z","event":"end","outcome":"error","duration_s":5,"error":"spawn failed"}
`
	os.WriteFile(tracePath, []byte(traceContent), 0o644)

	cr, err := DeriveCloseReason(tracePath, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cr.Outcome != "error" {
		t.Errorf("outcome = %q, want error", cr.Outcome)
	}
	if cr.Error != "spawn failed" {
		t.Errorf("error = %q, want 'spawn failed'", cr.Error)
	}
}

func TestDeriveCloseReasonMissingTrace(t *testing.T) {
	_, err := DeriveCloseReason("/nonexistent/trace.jsonl", "")
	if err == nil {
		t.Error("expected error for missing trace file")
	}
}

func TestFormatCloseReason(t *testing.T) {
	pass := true
	score := 0.87
	cr := &CloseReason{
		Outcome:      "success",
		DurationS:    125,
		FilesWritten: []string{"auth.go", "auth_test.go"},
		ToolCalls:    3,
		GatePass:     &pass,
		GateScore:    &score,
		DiffStat:     "2 files changed, 75 insertions(+), 3 deletions(-)",
	}
	result := FormatCloseReason(cr)
	if !strings.Contains(result, "success") {
		t.Error("missing outcome")
	}
	if !strings.Contains(result, "2m5s") {
		t.Error("missing duration")
	}
	if !strings.Contains(result, "auth.go") {
		t.Error("missing file name")
	}
	if !strings.Contains(result, "gate:pass(0.87)") {
		t.Error("missing gate result")
	}
	if !strings.Contains(result, "2 files changed") {
		t.Error("missing diff stat")
	}
}

func TestFormatCloseReasonManyFiles(t *testing.T) {
	cr := &CloseReason{
		Outcome:      "success",
		FilesWritten: []string{"a.go", "b.go", "c.go", "d.go", "e.go"},
	}
	result := FormatCloseReason(cr)
	if !strings.Contains(result, "wrote 5 files") {
		t.Errorf("should truncate file list: %s", result)
	}
	if !strings.Contains(result, "...") {
		t.Error("should have ellipsis for truncated files")
	}
}

func TestFormatCloseReasonMinimal(t *testing.T) {
	cr := &CloseReason{Outcome: "timeout"}
	result := FormatCloseReason(cr)
	if result != "timeout" {
		t.Errorf("minimal close reason = %q, want 'timeout'", result)
	}
}

func TestFormatCloseReasonWithError(t *testing.T) {
	cr := &CloseReason{Outcome: "error", Error: "spawn failed"}
	result := FormatCloseReason(cr)
	if !strings.Contains(result, "error: spawn failed") {
		t.Errorf("should contain error: %s", result)
	}
}
