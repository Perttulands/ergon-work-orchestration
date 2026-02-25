package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"polis/work/internal/ecosystem"
	"polis/work/internal/testutil"
)

func TestReadCitizenExperience(t *testing.T) {
	// Create temp work dir with citizen file
	workDir := t.TempDir()
	citizenDir := filepath.Join(workDir, "citizens")
	if err := os.MkdirAll(citizenDir, 0o755); err != nil {
		t.Fatal(err)
	}

	content := "I work best with clear specifications.\nPrefer small, focused commits."
	if err := os.WriteFile(filepath.Join(citizenDir, "mercury.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := readCitizenExperience(workDir, "mercury")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != content {
		t.Errorf("got %q, want %q", got, content)
	}
}

func TestReadCitizenExperienceMissing(t *testing.T) {
	workDir := t.TempDir()
	_, err := readCitizenExperience(workDir, "nobody")
	if err == nil {
		t.Error("expected error for missing citizen file")
	}
}

func TestFormatBeads(t *testing.T) {
	beads := []BeadResult{
		{ID: "work-abc", Status: "closed", Title: "Add JWT auth"},
		{ID: "work-def", Status: "closed", Title: "Fix flaky test"},
	}
	result := formatBeads(beads)
	if !strings.Contains(result, "## Past Work") {
		t.Error("missing header")
	}
	if !strings.Contains(result, "work-abc") {
		t.Error("missing first bead")
	}
	if !strings.Contains(result, "Fix flaky test") {
		t.Error("missing second bead title")
	}
}

func TestFormatCitizenExperience(t *testing.T) {
	result := formatCitizenExperience("zeus", "Always runs tests first.")
	if !strings.Contains(result, "## zeus's Experience Notes") {
		t.Error("missing citizen header")
	}
	if !strings.Contains(result, "Always runs tests first.") {
		t.Error("missing experience content")
	}
}

func TestFormatPatterns(t *testing.T) {
	result := formatPatterns("Use structured logging.")
	if !strings.Contains(result, "## Learned Patterns") {
		t.Error("missing patterns header")
	}
}

func TestGatherWithCitizenOnly(t *testing.T) {
	testutil.SandboxPATH(t, nil)

	workDir := t.TempDir()
	citizenDir := filepath.Join(workDir, "citizens")
	if err := os.MkdirAll(citizenDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(citizenDir, "apollo.md"), []byte("Good with UI work."), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Citizen:   "apollo",
		WorkDir:   workDir,
		BeadsRoot: t.TempDir(), // isolate from real bv data
	}
	result, err := Gather(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.CitizenExperience != "Good with UI work." {
		t.Errorf("citizen experience = %q, want %q", result.CitizenExperience, "Good with UI work.")
	}
	if !strings.Contains(result.Markdown, "apollo's Experience Notes") {
		t.Error("markdown should contain citizen section")
	}
}

func TestGatherNoContext(t *testing.T) {
	testutil.SandboxPATH(t, nil)

	cfg := Config{
		WorkDir:   t.TempDir(),
		BeadsRoot: t.TempDir(), // isolate from real bv data
	}
	result, err := Gather(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Markdown != "No prior context available. This is a fresh start." {
		t.Errorf("unexpected markdown: %q", result.Markdown)
	}
}

// --- bv integration tests ---

func TestFormatBvSearch(t *testing.T) {
	resp := &ecosystem.BvSearchResponse{
		Results: []ecosystem.BvSearchResult{
			{IssueID: "proj-abc", Score: 0.85, Title: "Add JWT auth"},
			{IssueID: "proj-def", Score: 0.42, Title: "Fix login bug"},
		},
	}
	result := formatBvSearch(resp)
	if !strings.Contains(result, "## Similar Beads") {
		t.Error("missing header")
	}
	if !strings.Contains(result, "proj-abc") {
		t.Error("missing first result")
	}
	if !strings.Contains(result, "85%") {
		t.Error("missing score percentage")
	}
	if !strings.Contains(result, "Fix login bug") {
		t.Error("missing second result title")
	}
}

func TestFormatBvRelated(t *testing.T) {
	resp := &ecosystem.BvRelatedResponse{
		TargetBeadID: "proj-xyz",
		TargetTitle:  "Wire bv into work",
		Concurrent: []ecosystem.BvRelatedItem{
			{BeadID: "proj-aaa", Title: "Auto close reasons", Status: "open", Reason: "Active in same window"},
		},
		TotalRelated: 1,
	}
	result := formatBvRelated(resp)
	if !strings.Contains(result, "## Related Work") {
		t.Error("missing header")
	}
	if !strings.Contains(result, "proj-xyz") {
		t.Error("missing target bead ID")
	}
	if !strings.Contains(result, "proj-aaa") {
		t.Error("missing related bead")
	}
	if !strings.Contains(result, "Active in same window") {
		t.Error("missing reason")
	}
}

func TestFormatBvPlan(t *testing.T) {
	resp := &ecosystem.BvPlanResponse{}
	resp.Plan.Tracks = []ecosystem.BvPlanTrack{
		{
			TrackID: "track-A",
			Items: []ecosystem.BvPlanItem{
				{ID: "proj-111", Title: "First task", Priority: 0, Status: "open"},
				{ID: "proj-222", Title: "Done task", Priority: 1, Status: "closed"},
			},
		},
	}
	resp.Plan.TotalActionable = 2
	resp.Plan.TotalBlocked = 0
	resp.Plan.Summary = ecosystem.BvPlanSummary{
		HighestImpact: "proj-111",
		ImpactReason:  "Unblocks 3 others",
	}

	result := formatBvPlan(resp)
	if !strings.Contains(result, "## Execution Plan") {
		t.Error("missing header")
	}
	if !strings.Contains(result, "Actionable: 2") {
		t.Error("missing actionable count")
	}
	if !strings.Contains(result, "[ ] **proj-111**") {
		t.Error("missing open task checkbox")
	}
	if !strings.Contains(result, "[x] **proj-222**") {
		t.Error("missing closed task checkbox")
	}
	if !strings.Contains(result, "Highest impact: **proj-111**") {
		t.Error("missing highest impact summary")
	}
}

func TestFormatPRD(t *testing.T) {
	result := formatPRD("# My Project\n\nA cool tool.")
	if !strings.Contains(result, "## Project Context (PRD.md)") {
		t.Error("missing header")
	}
	if !strings.Contains(result, "# My Project") {
		t.Error("missing PRD content")
	}
}

func TestFormatPRDTruncation(t *testing.T) {
	long := strings.Repeat("x", 3000)
	result := formatPRD(long)
	if !strings.Contains(result, "[...truncated]") {
		t.Error("long PRD should be truncated")
	}
	// Should not contain the full 3000 chars
	if len(result) > 2200 {
		t.Errorf("truncated PRD too long: %d chars", len(result))
	}
}

func TestReadPRD(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "PRD.md"), []byte("# Test PRD\n\nDo things."), 0o644)

	got, err := readPRD(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "# Test PRD\n\nDo things." {
		t.Errorf("got %q", got)
	}
}

func TestReadPRDMissing(t *testing.T) {
	_, err := readPRD(t.TempDir())
	if err == nil {
		t.Error("expected error for missing PRD.md")
	}
}

// Test that Gather with real bv produces bv sections when BeadsRoot has data.
func TestGatherWithBv(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"bv": `
case "$1" in
  --robot-search)
    printf '%s\n' '{"results":[{"issue_id":"projects-i01","score":0.88,"title":"Wire bv into work"}]}'
    ;;
  --robot-related)
    printf '%s\n' '{"target_bead_id":"projects-i01","target_title":"Wire bv into work","concurrent":[{"bead_id":"projects-a11","title":"Context budget handoff","status":"open","relation_type":"concurrent","relevance":82,"reason":"Touches same context flow"}],"total_related":1}'
    ;;
  --robot-plan)
    printf '%s\n' '{"plan":{"tracks":[{"track_id":"A","items":[{"id":"projects-i01","title":"Wire bv into work","priority":0,"status":"open","unblocks":["projects-i02"]}],"reason":"Critical path"}],"total_actionable":1,"total_blocked":0,"summary":{"highest_impact":"projects-i01","impact_reason":"Unblocks downstream work","unblocks_count":1}}}'
    ;;
  *)
    exit 1
    ;;
esac`,
	})

	cfg := Config{
		Task:      "wire bv integration",
		BeadID:    "projects-i01",
		WorkDir:   t.TempDir(),
		BeadsRoot: t.TempDir(),
	}
	result, err := Gather(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.BvSearch == nil {
		t.Error("expected bv search results")
	}
	if result.BvRelated == nil {
		t.Error("expected bv related results")
	}
	if result.BvPlan == nil {
		t.Error("expected bv plan results")
	}
	if !strings.Contains(result.Markdown, "Similar Beads") {
		t.Error("markdown should contain bv search section")
	}
	if !strings.Contains(result.Markdown, "Related Work") {
		t.Error("markdown should contain bv related section")
	}
	if !strings.Contains(result.Markdown, "Execution Plan") {
		t.Error("markdown should contain bv plan section")
	}
}

func TestFormatTemplateSelection(t *testing.T) {
	variant := "focused"
	sel := &ecosystem.TemplateSelection{
		Template:   "code-fix",
		Variant:    &variant,
		Agent:      "mercury",
		TaskType:   "bugfix",
		Score:      0.92,
		Confidence: "high",
		Reasoning:  "Past fixes with this template scored well.",
		Warnings:   []string{"Flaky test suite detected"},
	}
	result := formatTemplateSelection(sel)
	if !strings.Contains(result, "## Recommended Approach") {
		t.Error("missing header")
	}
	if !strings.Contains(result, "code-fix") {
		t.Error("missing template name")
	}
	if !strings.Contains(result, "focused") {
		t.Error("missing variant")
	}
	if !strings.Contains(result, "0.92") {
		t.Error("missing score")
	}
	if !strings.Contains(result, "Flaky test suite") {
		t.Error("missing warning")
	}
}

func TestGatherWithLearningLoop(t *testing.T) {
	if ecosystem.LearningLoopDir() == "" {
		t.Skip("learning-loop scripts not available")
	}

	cfg := Config{
		Task:      "fix a broken test",
		WorkDir:   t.TempDir(),
		BeadsRoot: t.TempDir(),
	}
	result, err := Gather(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TemplateSelection == nil {
		t.Fatal("expected template selection")
	}
	if result.TemplateSelection.TaskType == "" {
		t.Error("task_type should not be empty")
	}
	if !strings.Contains(result.Markdown, "Recommended Approach") {
		t.Error("markdown should contain learning-loop recommendation")
	}
}

func TestFormatLearningInsights(t *testing.T) {
	raw := `{
		"query": "fix a bug",
		"total_runs": 20,
		"matched_runs": 5,
		"success_rate": 0.8,
		"insights": [
			{"text": "Always run tests before committing.", "confidence": 0.9},
			{"text": "Check error handling paths.", "confidence": 0.7}
		],
		"top_patterns": [
			{"name": "missing-tests", "count": 3, "impact": "high"}
		],
		"success_signals": ["Ran tests and they passed"]
	}`

	result := formatLearningInsights([]byte(raw))
	if !strings.Contains(result, "## Learning (from past runs)") {
		t.Error("missing header")
	}
	if !strings.Contains(result, "5 similar runs") {
		t.Error("missing matched runs count")
	}
	if !strings.Contains(result, "80% success") {
		t.Error("missing success rate")
	}
	if !strings.Contains(result, "Always run tests") {
		t.Error("missing insight text")
	}
	if !strings.Contains(result, "missing-tests") {
		t.Error("missing pattern name")
	}
	if !strings.Contains(result, "Ran tests and they passed") {
		t.Error("missing success signal")
	}
}

func TestFormatLearningInsightsEmpty(t *testing.T) {
	raw := `{"matched_runs": 0, "success_rate": 0, "insights": [], "top_patterns": [], "success_signals": []}`
	result := formatLearningInsights([]byte(raw))
	if result != "" {
		t.Errorf("expected empty string for zero matches, got %q", result)
	}
}

func TestFormatLearningInsightsInvalidJSON(t *testing.T) {
	result := formatLearningInsights([]byte("not json"))
	if result != "" {
		t.Errorf("expected empty string for invalid JSON, got %q", result)
	}
}

func TestGatherWithoutLearningLoop(t *testing.T) {
	t.Setenv("LEARNING_LOOP_DIR", t.TempDir())

	cfg := Config{
		Task:      "some task",
		WorkDir:   t.TempDir(),
		BeadsRoot: t.TempDir(),
	}
	result, err := Gather(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TemplateSelection != nil {
		t.Error("should have no template selection when learning-loop unavailable")
	}
}
