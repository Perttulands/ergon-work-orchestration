package ecosystem

import (
	"os"
	"os/exec"
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

// TestFormatCloseReasonShortDuration exercises the seconds-only formatting
// path (< 60s). This is the common case for fast tasks like lint-only runs.
func TestFormatCloseReasonShortDuration(t *testing.T) {
	cr := &CloseReason{Outcome: "success", DurationS: 45}
	result := FormatCloseReason(cr)
	if !strings.Contains(result, "45s") {
		t.Errorf("should format short duration as seconds: %s", result)
	}
	// Must NOT contain "0m45s" — only "45s"
	if strings.Contains(result, "m") {
		t.Errorf("short duration should not show minutes: %s", result)
	}
}

// TestFormatCloseReasonGateFail verifies the gate:fail formatting path.
// This is how agents learn that quality checks didn't pass.
func TestCaptureGitDiffEmpty(t *testing.T) {
	// Empty repo path returns empty string
	if got := CaptureGitDiff(""); got != "" {
		t.Errorf("expected empty string for empty repo, got %q", got)
	}
}

func TestCaptureGitDiffCleanRepo(t *testing.T) {
	dir := t.TempDir()
	// Init a git repo with a commit so HEAD exists
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "test"},
		{"commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}

	got := CaptureGitDiff(dir)
	if got != "" {
		t.Errorf("clean repo should have empty diff, got %q", got)
	}
}

func TestCaptureGitDiffDirtyRepo(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
	// Create and commit a file, then modify it
	os.WriteFile(dir+"/hello.txt", []byte("hello\n"), 0o644)
	for _, args := range [][]string{
		{"add", "."},
		{"commit", "-m", "add hello"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
	os.WriteFile(dir+"/hello.txt", []byte("hello world\n"), 0o644)

	got := CaptureGitDiff(dir)
	if !strings.Contains(got, "hello world") {
		t.Errorf("diff should contain the change, got %q", got)
	}
}

func TestCaptureGitDiffNoGit(t *testing.T) {
	// A directory that is not a git repo
	dir := t.TempDir()
	got := CaptureGitDiff(dir)
	if got != "" {
		t.Errorf("non-git dir should return empty, got %q", got)
	}
}

func TestFormatCloseReasonGateFail(t *testing.T) {
	pass := false
	score := 0.30
	cr := &CloseReason{
		Outcome:   "gate_fail",
		DurationS: 90,
		GatePass:  &pass,
		GateScore: &score,
	}
	result := FormatCloseReason(cr)
	if !strings.Contains(result, "gate:fail(0.30)") {
		t.Errorf("should contain gate:fail with score: %s", result)
	}
	if !strings.Contains(result, "1m30s") {
		t.Errorf("should format duration: %s", result)
	}
}
