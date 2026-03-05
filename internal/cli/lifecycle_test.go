package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	workctx "polis/work/internal/context"
	"polis/work/internal/ecosystem"
	"polis/work/internal/index"
	"polis/work/internal/testutil"
	"polis/work/internal/trace"
)

// Integration test: full lifecycle from context to trace to index to close reason.
// Exercises the complete pipeline without tmux worker.
func TestFullPipelineIntegration(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"bv": `
case "$1" in
  --robot-search)
    printf '%s\n' '{"results":[{"issue_id":"proj-001","score":0.9,"title":"Similar task"}]}'
    ;;
  --robot-related)
    printf '%s\n' '{"target_bead_id":"proj-001","target_title":"Similar task","concurrent":[],"total_related":0}'
    ;;
  --robot-plan)
    printf '%s\n' '{"plan":{"tracks":[],"total_actionable":0,"total_blocked":0,"summary":{"highest_impact":"","impact_reason":"","unblocks_count":0}}}'
    ;;
  *)
    exit 1
    ;;
esac`,
	})

	workDir := t.TempDir()
	repo := t.TempDir()
	beadID := "pipeline-test-001"

	// Write PRD.md in repo
	os.WriteFile(filepath.Join(repo, "PRD.md"), []byte("# Test Project\n\nA test."), 0o644)

	// Write citizen experience
	citizenDir := filepath.Join(workDir, "citizens")
	os.MkdirAll(citizenDir, 0o755)
	os.WriteFile(filepath.Join(citizenDir, "pipeline-agent.md"), []byte("Experienced with pipelines."), 0o644)

	// 1. Gather context
	ctx, err := workctx.Gather(workctx.Config{
		BeadID:    beadID,
		Citizen:   "pipeline-agent",
		Task:      "build pipeline test",
		Repo:      repo,
		WorkDir:   workDir,
		BeadsRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if ctx.BvSearch == nil {
		t.Error("expected bv search results")
	}
	if ctx.CitizenExperience != "Experienced with pipelines." {
		t.Errorf("citizen = %q", ctx.CitizenExperience)
	}
	if !strings.Contains(ctx.Markdown, "Test Project") {
		t.Error("PRD should be in context")
	}

	// 2. Assemble prompt
	prompt := assemblePrompt("build pipeline test", "pipeline-agent", beadID, repo, ctx)
	if !strings.Contains(prompt, "CONTEXT FROM PAST WORK") {
		t.Error("prompt should include context section")
	}
	if !strings.Contains(prompt, "Similar task") {
		t.Error("prompt should include bv search context")
	}

	// 3. Open trace and write events
	tr, err := trace.Open(workDir, beadID, "pipeline-agent", "build pipeline test")
	if err != nil {
		t.Fatalf("open trace: %v", err)
	}
	tr.EmitToolCall("bash", "go build ./...", 1200)
	tr.EmitToolCall("bash", "go test ./...", 3400)
	tr.EmitFileWrite("internal/pipeline.go", 85)
	tr.EmitFileWrite("internal/pipeline_test.go", 62)
	tr.EmitWorkerOutput("All 5 tests passed.")

	// Simulate gate result
	pass := true
	score := 0.95
	tr.Emit(trace.Event{
		EventType: "gate_result",
		Pass:      &pass,
		Score:     &score,
	})

	meta := tr.GetMetadata("success")
	tr.Close("success", nil)

	// 4. Index the run
	idx, err := index.Open(workDir)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	if err := idx.Record(meta); err != nil {
		t.Fatalf("record: %v", err)
	}

	// Verify index queries
	runs, err := idx.Recent(10)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].Outcome != "success" {
		t.Errorf("outcome = %q", runs[0].Outcome)
	}

	byBead, err := idx.ByBead(beadID)
	if err != nil {
		t.Fatalf("by bead: %v", err)
	}
	if len(byBead) != 1 {
		t.Errorf("expected 1 by bead, got %d", len(byBead))
	}
	idx.Close()

	// 5. Derive close reason
	cr, err := ecosystem.DeriveCloseReason(tr.FilePath(), repo)
	if err != nil {
		t.Fatalf("derive close reason: %v", err)
	}
	if cr.Outcome != "success" {
		t.Errorf("close reason outcome = %q", cr.Outcome)
	}
	if len(cr.FilesWritten) != 2 {
		t.Errorf("files written = %d, want 2", len(cr.FilesWritten))
	}
	if cr.ToolCalls != 2 {
		t.Errorf("tool calls = %d, want 2", cr.ToolCalls)
	}
	if cr.GatePass == nil || !*cr.GatePass {
		t.Error("gate should be pass")
	}

	formatted := ecosystem.FormatCloseReason(cr)
	if !strings.Contains(formatted, "success") {
		t.Error("formatted should contain success")
	}
	if !strings.Contains(formatted, "pipeline.go") {
		t.Error("formatted should mention file")
	}
	if !strings.Contains(formatted, "gate:pass") {
		t.Error("formatted should mention gate pass")
	}

	// 6. Record citizen experience
	if err := workctx.AppendCitizenExperience(workDir, "pipeline-agent", "build pipeline test", "success", beadID); err != nil {
		t.Fatalf("append experience: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(citizenDir, "pipeline-agent.md"))
	if !strings.Contains(string(data), "build pipeline test") {
		t.Error("citizen file should include task")
	}
	if !strings.Contains(string(data), "success") {
		t.Error("citizen file should include outcome")
	}

	// 7. Build run record
	gateResult := &ecosystem.GateResult{Pass: true, Score: 0.95}
	rec := buildRunRecord(beadID, "pipeline-agent", "codex", "success", meta.DurationS, gateResult, ctx)
	if rec.Status != "done" {
		t.Errorf("status = %q", rec.Status)
	}
	if rec.Verification.Tests != "pass" {
		t.Errorf("tests = %q", rec.Verification.Tests)
	}

	// 8. Trace lookup works
	tracePath, err := findTracePath(workDir, beadID)
	if err != nil {
		t.Fatalf("find trace: %v", err)
	}
	if tracePath == "" {
		t.Error("trace path should not be empty")
	}

	// 9. Read trace back
	events, err := trace.ReadTrace(tracePath)
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	// begin + 2 tool_call + 2 file_write + worker_output + gate_result + end = 8
	if len(events) != 8 {
		t.Errorf("expected 8 events, got %d", len(events))
	}
}

// Test degradation integration: nothing available, lifecycle still works.
func TestDegradedPipelineIntegration(t *testing.T) {
	testutil.SandboxPATH(t, nil)
	t.Setenv("LEARNING_LOOP_DIR", t.TempDir())

	workDir := t.TempDir()

	// Check tools reports all degraded
	report := checkTools()
	if len(report) < 3 {
		t.Fatalf("expected at least 3 degradations, got %d", len(report))
	}

	// Context still gathers (returns fresh-start message)
	ctx, err := workctx.Gather(workctx.Config{
		WorkDir:   workDir,
		Citizen:   "degraded",
		Task:      "degraded task",
		BeadsRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("gather should not fail: %v", err)
	}
	if ctx.Markdown != "No prior context available. This is a fresh start." {
		t.Errorf("unexpected markdown: %q", ctx.Markdown)
	}

	// Prompt still assembles (no context section)
	prompt := assemblePrompt("degraded task", "degraded", "work-degraded", "/tmp", ctx)
	if strings.Contains(prompt, "CONTEXT FROM PAST WORK") {
		t.Error("should not include context section with fresh-start context")
	}
	if !strings.Contains(prompt, "TASK: degraded task") {
		t.Error("prompt must still contain task")
	}

	// Trace still works
	tr, err := trace.Open(workDir, "degraded-test", "degraded", "degraded task")
	if err != nil {
		t.Fatalf("trace: %v", err)
	}
	tr.EmitError("gate_skipped: result is unverified")
	meta := tr.GetMetadata("success")
	tr.Close("success", nil)

	// Index still works
	idx, err := index.Open(workDir)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	idx.Record(meta)

	runs, _ := idx.Recent(10)
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	idx.Close()

	// Run record still builds correctly
	rec := buildRunRecord("degraded-test", "degraded", "codex", "success", 60, nil, nil)
	if rec.Status != "done" {
		t.Errorf("status = %q, want done", rec.Status)
	}
	if rec.Verification.Tests != "skipped" {
		t.Errorf("tests = %q, want skipped (no gate)", rec.Verification.Tests)
	}
	if rec.TemplateName != "custom" {
		t.Errorf("template = %q, want custom (no context)", rec.TemplateName)
	}
}

// Test index rebuild from JSONL traces.
func TestIndexRebuildFromTraces(t *testing.T) {
	workDir := t.TempDir()

	// Create several traces
	for _, tc := range []struct {
		bead    string
		outcome string
	}{
		{"rebuild-001", "success"},
		{"rebuild-002", "error"},
		{"rebuild-003", "success"},
	} {
		tr, err := trace.Open(workDir, tc.bead, "agent", "task")
		if err != nil {
			t.Fatalf("open trace %s: %v", tc.bead, err)
		}
		tr.EmitToolCall("bash", "echo", 10)
		tr.Close(tc.outcome, nil)
	}

	// Open index (should auto-rebuild from traces)
	idx, err := index.Open(workDir)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	defer idx.Close()

	runs, err := idx.Recent(10)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(runs) != 3 {
		t.Errorf("expected 3 runs from rebuild, got %d", len(runs))
	}
}

// Test multiple traces indexed and queried by bead.
func TestMultipleRunsSameBead(t *testing.T) {
	workDir := t.TempDir()

	idx, err := index.Open(workDir)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}

	now := time.Now()
	// Two different runs for the same bead (retry scenario)
	for i, outcome := range []string{"error", "success"} {
		meta := trace.Metadata{
			BeadID:    "retry-bead",
			Agent:     "zeus",
			Task:      "fix bug",
			Outcome:   outcome,
			DurationS: int64(30 + i*60),
			StartTime: now.Add(time.Duration(-2+i) * time.Hour),
			EndTime:   now.Add(time.Duration(-2+i)*time.Hour + 30*time.Second),
			FilePath:  filepath.Join(workDir, "traces", outcome+".jsonl"),
		}
		idx.Record(meta)
	}
	idx.Close()

	// Reopen and query
	idx2, err := index.Open(workDir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer idx2.Close()

	runs, err := idx2.ByBead("retry-bead")
	if err != nil {
		t.Fatalf("by bead: %v", err)
	}
	// INSERT OR REPLACE means only 1 entry (last one wins)
	if len(runs) != 1 {
		t.Errorf("expected 1 run (upsert), got %d", len(runs))
	}
}
