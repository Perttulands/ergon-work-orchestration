package ecosystem

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAvailable(t *testing.T) {
	if !Available("tmux") {
		t.Error("expected tmux to be available")
	}
	if Available("nonexistent-tool-xyz-12345") {
		t.Error("expected nonexistent tool to not be available")
	}
}

// --- Graceful degradation tests (projects-r03) ---

func TestBdCreateWhenBdUnavailable(t *testing.T) {
	if Available("bd") {
		t.Skip("bd is available; this test covers the missing-bd path")
	}
	result, err := BdCreate("test task", "/tmp")
	if err != nil {
		t.Errorf("should return nil error when bd unavailable, got: %v", err)
	}
	if result != nil {
		t.Error("should return nil result when bd unavailable")
	}
}

func TestBdCloseWhenBdUnavailable(t *testing.T) {
	if Available("bd") {
		t.Skip("bd is available; this test covers the missing-bd path")
	}
	err := BdClose("test-id", "reason", "/tmp")
	if err != nil {
		t.Errorf("should return nil error when bd unavailable, got: %v", err)
	}
}

func TestGateCheckWhenGateUnavailable(t *testing.T) {
	if Available("gate") {
		t.Skip("gate is available; this test covers the missing-gate path")
	}
	result, err := GateCheck("/tmp", "test")
	if err != nil {
		t.Errorf("should return nil error when gate unavailable, got: %v", err)
	}
	if result != nil {
		t.Error("should return nil result when gate unavailable")
	}
}

// When bd IS available, test that BdCreate returns a real result.
func TestBdCreateWhenBdAvailable(t *testing.T) {
	if !Available("bd") {
		t.Skip("bd not available")
	}
	// bd q requires a valid .beads directory; just verify it doesn't panic
	_, err := BdCreate("degradation test probe", "/tmp")
	// Error is expected (no .beads dir in /tmp), but it shouldn't be nil-pointer or panic
	if err != nil {
		t.Logf("expected error from /tmp (no .beads): %v", err)
	}
}

// --- bv graceful degradation tests ---

func TestBvSearchWhenBvUnavailable(t *testing.T) {
	if Available("bv") {
		t.Skip("bv is available; this test covers the missing-bv path")
	}
	result, err := BvSearch("test query", "/tmp")
	if err != nil {
		t.Errorf("should return nil error when bv unavailable, got: %v", err)
	}
	if result != nil {
		t.Error("should return nil result when bv unavailable")
	}
}

func TestBvSearchEmptyQuery(t *testing.T) {
	result, err := BvSearch("", "/tmp")
	if err != nil {
		t.Errorf("empty query should return nil error, got: %v", err)
	}
	if result != nil {
		t.Error("empty query should return nil result")
	}
}

func TestBvRelatedWhenBvUnavailable(t *testing.T) {
	if Available("bv") {
		t.Skip("bv is available; this test covers the missing-bv path")
	}
	result, err := BvRelated("test-bead", "/tmp")
	if err != nil {
		t.Errorf("should return nil error when bv unavailable, got: %v", err)
	}
	if result != nil {
		t.Error("should return nil result when bv unavailable")
	}
}

func TestBvRelatedEmptyBead(t *testing.T) {
	result, err := BvRelated("", "/tmp")
	if err != nil {
		t.Errorf("empty bead should return nil error, got: %v", err)
	}
	if result != nil {
		t.Error("empty bead should return nil result")
	}
}

func TestBvPlanWhenBvUnavailable(t *testing.T) {
	if Available("bv") {
		t.Skip("bv is available; this test covers the missing-bv path")
	}
	result, err := BvPlan("/tmp")
	if err != nil {
		t.Errorf("should return nil error when bv unavailable, got: %v", err)
	}
	if result != nil {
		t.Error("should return nil result when bv unavailable")
	}
}

// When bv IS available, test real calls against the beads root.
func TestBvSearchWhenBvAvailable(t *testing.T) {
	if !Available("bv") {
		t.Skip("bv not available")
	}
	result, err := BvSearch("test", "/home/polis/projects")
	if err != nil {
		t.Fatalf("bv search should not error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result from real bv")
	}
	if len(result.Results) == 0 {
		t.Log("bv search returned no results (possible if no matching beads)")
	}
}

func TestBvRelatedWhenBvAvailable(t *testing.T) {
	if !Available("bv") {
		t.Skip("bv not available")
	}
	result, err := BvRelated("projects-i01", "/home/polis/projects")
	if err != nil {
		t.Fatalf("bv related should not error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result from real bv")
	}
	if result.TargetBeadID != "projects-i01" {
		t.Errorf("target bead = %q, want projects-i01", result.TargetBeadID)
	}
}

func TestBvPlanWhenBvAvailable(t *testing.T) {
	if !Available("bv") {
		t.Skip("bv not available")
	}
	result, err := BvPlan("/home/polis/projects")
	if err != nil {
		t.Fatalf("bv plan should not error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result from real bv")
	}
	if result.Plan.TotalActionable == 0 {
		t.Log("bv plan shows 0 actionable items")
	}
}

// --- Learning-loop integration tests ---

func TestLearningLoopDirFromEnv(t *testing.T) {
	t.Setenv("LEARNING_LOOP_DIR", "/custom/path")
	dir := LearningLoopDir()
	if dir != "/custom/path" {
		t.Errorf("expected /custom/path, got %q", dir)
	}
}

func TestLearningLoopDirFallback(t *testing.T) {
	t.Setenv("LEARNING_LOOP_DIR", "")
	dir := LearningLoopDir()
	// On this system the scripts exist at ~/tools/learning-loop/
	if dir == "" {
		t.Log("learning-loop scripts not found at well-known path")
	}
}

func TestSelectTemplateWhenUnavailable(t *testing.T) {
	t.Setenv("LEARNING_LOOP_DIR", t.TempDir()) // no scripts here
	result, err := SelectTemplate("fix a bug")
	if err != nil {
		t.Errorf("should return nil error when unavailable, got: %v", err)
	}
	if result != nil {
		t.Error("should return nil result when unavailable")
	}
}

func TestSelectTemplateWhenAvailable(t *testing.T) {
	dir := LearningLoopDir()
	if dir == "" {
		t.Skip("learning-loop scripts not available")
	}

	result, err := SelectTemplate("fix a broken test in the auth module")
	if err != nil {
		t.Fatalf("select-template should not error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.TaskType == "" {
		t.Error("task_type should not be empty")
	}
	if result.Template == "" {
		t.Error("template should not be empty")
	}
	// "fix" + "broken" + "test" should classify as bug-fix
	if result.TaskType != "bug-fix" {
		t.Errorf("expected task_type bug-fix, got %q", result.TaskType)
	}
}

func TestCollectFeedbackWhenUnavailable(t *testing.T) {
	t.Setenv("LEARNING_LOOP_DIR", t.TempDir()) // no scripts here
	rec := RunRecord{
		Bead:   "test-bead",
		Agent:  "test-agent",
		Status: "done",
	}
	err := CollectFeedback(rec, t.TempDir())
	if err != nil {
		t.Errorf("should return nil error when unavailable, got: %v", err)
	}
}

func TestCollectFeedbackWritesRunRecord(t *testing.T) {
	dir := LearningLoopDir()
	if dir == "" {
		t.Skip("learning-loop scripts not available")
	}

	workDir := t.TempDir()
	rec := RunRecord{
		Bead:            "test-collect-fb",
		Agent:           "test-agent",
		Model:           "claude-sonnet",
		TemplateName:    "bug-fix",
		Status:          "done",
		ExitCode:        0,
		DurationSeconds: 120,
		Attempt:         1,
		Verification: Verification{
			Tests:      "pass",
			Lint:       "pass",
			UBS:        "skipped",
			Truthsayer: "skipped",
		},
	}

	err := CollectFeedback(rec, workDir)
	if err != nil {
		t.Fatalf("collect feedback failed: %v", err)
	}

	// Verify run record was written
	recordPath := filepath.Join(workDir, "run-records", "test-collect-fb.json")
	data, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("run record not written: %v", err)
	}

	var written RunRecord
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatalf("invalid run record JSON: %v", err)
	}
	if written.Bead != "test-collect-fb" {
		t.Errorf("bead = %q, want test-collect-fb", written.Bead)
	}

	// Verify feedback was collected
	feedbackPath := filepath.Join(workDir, "feedback", "test-collect-fb.json")
	fbData, err := os.ReadFile(feedbackPath)
	if err != nil {
		t.Fatalf("feedback not written: %v", err)
	}
	var fb map[string]interface{}
	if err := json.Unmarshal(fbData, &fb); err != nil {
		t.Fatalf("invalid feedback JSON: %v", err)
	}
	// outcome depends on verification signals: "full_pass" if all pass,
	// "partial_pass" if some are skipped. Both are valid.
	outcome, _ := fb["outcome"].(string)
	if outcome != "full_pass" && outcome != "partial_pass" {
		t.Errorf("outcome = %v, want full_pass or partial_pass", fb["outcome"])
	}
}
