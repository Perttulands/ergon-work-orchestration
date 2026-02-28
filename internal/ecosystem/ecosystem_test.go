package ecosystem

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"polis/work/internal/testutil"
)

func TestAvailable(t *testing.T) {
	if !Available("sh") {
		t.Error("expected sh to be available")
	}
	if Available("nonexistent-tool-xyz-12345") {
		t.Error("expected nonexistent tool to not be available")
	}
}

// --- Graceful degradation tests (projects-r03) ---

func TestBrCreateWhenBrUnavailable(t *testing.T) {
	testutil.SandboxPATH(t, nil)

	result, err := BrCreate("test task", "/tmp")
	if err != nil {
		t.Errorf("should return nil error when br unavailable, got: %v", err)
	}
	if result != nil {
		t.Error("should return nil result when br unavailable")
	}
}

func TestBrCloseWhenBrUnavailable(t *testing.T) {
	testutil.SandboxPATH(t, nil)

	err := BrClose("test-id", "reason", "/tmp")
	if err != nil {
		t.Errorf("should return nil error when br unavailable, got: %v", err)
	}
}

func TestGateCheckWhenGateUnavailable(t *testing.T) {
	testutil.SandboxPATH(t, nil)

	result, err := GateCheck("/tmp", "test")
	if err != nil {
		t.Errorf("should return nil error when gate unavailable, got: %v", err)
	}
	if result != nil {
		t.Error("should return nil result when gate unavailable")
	}
}

// When br IS available, test that BrCreate returns a real result.
func TestBrCreateWhenBrAvailable(t *testing.T) {
	if !Available("br") {
		t.Skip("br not available")
	}
	// br create requires a valid .beads directory; just verify it doesn't panic
	_, err := BrCreate("degradation test probe", "/tmp")
	// Error is expected (no .beads dir in /tmp), but it shouldn't be nil-pointer or panic
	if err != nil {
		t.Logf("expected error from /tmp (no .beads): %v", err)
	}
}

// --- bv graceful degradation tests ---

func TestBvSearchWhenBvUnavailable(t *testing.T) {
	testutil.SandboxPATH(t, nil)

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
	testutil.SandboxPATH(t, nil)

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
	testutil.SandboxPATH(t, nil)

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
	testutil.SandboxPATH(t, map[string]string{
		"bv": `
case "$1" in
  --robot-search)
    printf '%s\n' '{"results":[{"issue_id":"projects-i01","score":0.85,"title":"Add JWT auth"}]}'
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

	result, err := BvSearch("test", t.TempDir())
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
	testutil.SandboxPATH(t, map[string]string{
		"bv": `
case "$1" in
  --robot-search)
    printf '%s\n' '{"results":[{"issue_id":"projects-i01","score":0.85,"title":"Add JWT auth"}]}'
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

	result, err := BvRelated("projects-i01", t.TempDir())
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
	testutil.SandboxPATH(t, map[string]string{
		"bv": `
case "$1" in
  --robot-search)
    printf '%s\n' '{"results":[{"issue_id":"projects-i01","score":0.85,"title":"Add JWT auth"}]}'
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

	result, err := BvPlan(t.TempDir())
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

// --- Relay + agent state degradation tests ---

func TestBrAgentStateWhenBrUnavailable(t *testing.T) {
	testutil.SandboxPATH(t, nil)

	if err := BrAgentState("test-agent", "working"); err != nil {
		t.Errorf("should return nil when br unavailable, got: %v", err)
	}
}

func TestRelayHeartbeatWhenRelayUnavailable(t *testing.T) {
	testutil.SandboxPATH(t, nil)

	if err := RelayHeartbeat("test-agent"); err != nil {
		t.Errorf("should return nil when relay unavailable, got: %v", err)
	}
}

func TestRelaySendWhenRelayUnavailable(t *testing.T) {
	testutil.SandboxPATH(t, nil)

	if err := RelaySend("zeus", "athena", "test", "thread-1", "", ""); err != nil {
		t.Errorf("should return nil when relay unavailable, got: %v", err)
	}
}

func TestRelaySendTypedWhenRelayUnavailable(t *testing.T) {
	testutil.SandboxPATH(t, nil)

	if err := RelaySend("zeus", "athena", "task done", "bead-1", "task_result", `{"bead_id":"bead-1","outcome":"success"}`); err != nil {
		t.Errorf("should return nil when relay unavailable, got: %v", err)
	}
}

// --- loop binary integration tests ---

func TestQueryLearningLoopWhenUnavailable(t *testing.T) {
	testutil.SandboxPATH(t, nil)

	result, err := QueryLearningLoop("fix a bug")
	if err != nil {
		t.Errorf("should return nil error when loop unavailable, got: %v", err)
	}
	if result != nil {
		t.Error("should return nil result when loop unavailable")
	}
}

func TestQueryLearningLoopEmptyTask(t *testing.T) {
	result, err := QueryLearningLoop("")
	if err != nil {
		t.Errorf("empty task should return nil error, got: %v", err)
	}
	if result != nil {
		t.Error("empty task should return nil result")
	}
}

func TestIngestRunWhenUnavailable(t *testing.T) {
	testutil.SandboxPATH(t, nil)

	err := IngestRun("bead-1", "fix a bug", "success", "worker", 120, true, true, nil, "")
	if err != nil {
		t.Errorf("should return nil error when loop unavailable, got: %v", err)
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

// --- Mock tool happy-path tests ---

func TestBrCloseWithMockBr(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"br": `echo "closed $2"`,
	})

	err := BrClose("test-bead-1", "task complete", t.TempDir())
	if err != nil {
		t.Errorf("BrClose should succeed with mock br, got: %v", err)
	}
}

func TestBrCloseError(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"br": `echo "error: bead not found" >&2; exit 1`,
	})

	err := BrClose("nonexistent", "reason", t.TempDir())
	if err == nil {
		t.Error("BrClose should return error when br fails")
	}
}

func TestBrAgentStateWithMock(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"br": `echo "state set"`,
	})

	err := BrAgentState("zeus", "working")
	if err != nil {
		t.Errorf("BrAgentState should succeed with mock: %v", err)
	}
}

func TestBrAgentStateError(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"br": `echo "error" >&2; exit 1`,
	})

	err := BrAgentState("zeus", "working")
	if err == nil {
		t.Error("BrAgentState should return error when br fails")
	}
}

func TestRelayHeartbeatWithMock(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"relay": `echo "heartbeat ok"`,
	})

	err := RelayHeartbeat("zeus")
	if err != nil {
		t.Errorf("RelayHeartbeat should succeed with mock: %v", err)
	}
}

func TestRelayHeartbeatError(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"relay": `echo "error" >&2; exit 1`,
	})

	err := RelayHeartbeat("zeus")
	if err == nil {
		t.Error("RelayHeartbeat should return error when relay fails")
	}
}

func TestRelaySendWithMock(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"relay": `echo "sent"`,
	})

	err := RelaySend("zeus", "athena", "task done", "", "", "")
	if err != nil {
		t.Errorf("RelaySend basic should succeed: %v", err)
	}
}

func TestRelaySendWithAllOptions(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"relay": `echo "sent with options"`,
	})

	err := RelaySend("zeus", "athena", "task done", "thread-1", "task_result", `{"outcome":"success"}`)
	if err != nil {
		t.Errorf("RelaySend with all options should succeed: %v", err)
	}
}

func TestRelaySendError(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"relay": `echo "error: connection refused" >&2; exit 1`,
	})

	err := RelaySend("zeus", "athena", "test", "", "", "")
	if err == nil {
		t.Error("RelaySend should return error when relay fails")
	}
}

func TestGateCheckWithMockPass(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"gate": `printf '{"pass":true,"score":0.95}'`,
	})

	result, err := GateCheck(t.TempDir(), "zeus")
	if err != nil {
		t.Fatalf("GateCheck should not error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Pass {
		t.Error("expected pass=true")
	}
	if result.Score != 0.95 {
		t.Errorf("score = %f, want 0.95", result.Score)
	}
}

func TestGateCheckWithMockFail(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"gate": `printf '{"pass":false,"score":0.30}'; exit 1`,
	})

	result, err := GateCheck(t.TempDir(), "")
	if err != nil {
		t.Fatalf("GateCheck should not error (even with non-zero exit): %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Pass {
		t.Error("expected pass=false")
	}
}

func TestGateCheckInvalidJSON(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"gate": `echo "not json"; exit 1`,
	})

	result, err := GateCheck(t.TempDir(), "")
	if err != nil {
		t.Fatalf("GateCheck should not error even with invalid JSON: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result from fallback")
	}
	// With invalid JSON + non-zero exit, pass should be false
	if result.Pass {
		t.Error("expected pass=false for invalid JSON with error exit")
	}
}

func TestGateCheckInvalidJSONPassExit(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"gate": `echo "all good but not json"`,
	})

	result, err := GateCheck(t.TempDir(), "")
	if err != nil {
		t.Fatalf("GateCheck should not error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Invalid JSON but exit 0 → pass=true
	if !result.Pass {
		t.Error("expected pass=true for zero exit with invalid JSON")
	}
}

func TestIngestRunWithMock(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"loop": `cat >/dev/null`, // consume stdin
	})

	err := IngestRun("bead-1", "fix bug", "success", "zeus", 120, true, true, []string{"main.go"}, "")
	if err != nil {
		t.Errorf("IngestRun should succeed with mock: %v", err)
	}
}

func TestIngestRunWithErrorMessage(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"loop": `cat >/dev/null`,
	})

	err := IngestRun("bead-2", "fix bug", "failure", "zeus", 60, false, false, nil, "tests failed")
	if err != nil {
		t.Errorf("IngestRun with error msg should succeed: %v", err)
	}
}

func TestIngestRunError(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"loop": `exit 1`,
	})

	err := IngestRun("bead-3", "fix bug", "success", "zeus", 120, true, true, nil, "")
	if err == nil {
		t.Error("IngestRun should return error when loop fails")
	}
}

func TestQueryLearningLoopWithMock(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"loop": `printf '{"patterns":[{"name":"test","score":0.9}]}'`,
	})

	result, err := QueryLearningLoop("fix a bug")
	if err != nil {
		t.Errorf("QueryLearningLoop should succeed: %v", err)
	}
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestQueryLearningLoopError(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"loop": `exit 1`,
	})

	// loop errors degrade gracefully — return nil, nil
	result, err := QueryLearningLoop("fix a bug")
	if err != nil {
		t.Errorf("QueryLearningLoop should degrade gracefully, got: %v", err)
	}
	if result != nil {
		t.Error("expected nil result on graceful degradation")
	}
}

func TestSelectTemplateWithMockScript(t *testing.T) {
	dir := t.TempDir()
	scriptsDir := filepath.Join(dir, "scripts")
	os.MkdirAll(scriptsDir, 0o755)

	script := filepath.Join(scriptsDir, "select-template.sh")
	os.WriteFile(script, []byte(`#!/bin/sh
printf '{"template":"bug-fix","variant":null,"agent":"zeus","model":"sonnet","task_type":"bug-fix","score":0.9,"confidence":"high","reasoning":"test","warnings":[]}'
`), 0o755)

	t.Setenv("LEARNING_LOOP_DIR", dir)

	result, err := SelectTemplate("fix a broken test")
	if err != nil {
		t.Fatalf("SelectTemplate should succeed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Template != "bug-fix" {
		t.Errorf("template = %q, want bug-fix", result.Template)
	}
	if result.TaskType != "bug-fix" {
		t.Errorf("task_type = %q, want bug-fix", result.TaskType)
	}
}

func TestSelectTemplateScriptError(t *testing.T) {
	dir := t.TempDir()
	scriptsDir := filepath.Join(dir, "scripts")
	os.MkdirAll(scriptsDir, 0o755)

	script := filepath.Join(scriptsDir, "select-template.sh")
	os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0o755)

	t.Setenv("LEARNING_LOOP_DIR", dir)

	_, err := SelectTemplate("fix a bug")
	if err == nil {
		t.Error("SelectTemplate should return error when script fails")
	}
}

func TestSelectTemplateInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	scriptsDir := filepath.Join(dir, "scripts")
	os.MkdirAll(scriptsDir, 0o755)

	script := filepath.Join(scriptsDir, "select-template.sh")
	os.WriteFile(script, []byte("#!/bin/sh\necho 'not json'\n"), 0o755)

	t.Setenv("LEARNING_LOOP_DIR", dir)

	_, err := SelectTemplate("fix a bug")
	if err == nil {
		t.Error("SelectTemplate should return error for invalid JSON output")
	}
}

func TestCollectFeedbackWithMockScript(t *testing.T) {
	dir := t.TempDir()
	scriptsDir := filepath.Join(dir, "scripts")
	os.MkdirAll(scriptsDir, 0o755)

	script := filepath.Join(scriptsDir, "feedback-collector.sh")
	// Write a minimal feedback file so we can verify it ran
	os.WriteFile(script, []byte(`#!/bin/sh
RECORD_PATH="$1"
BEAD=$(cat "$RECORD_PATH" | sed -n 's/.*"bead":"\([^"]*\)".*/\1/p')
mkdir -p "$FEEDBACK_DIR"
echo '{"outcome":"full_pass"}' > "$FEEDBACK_DIR/$BEAD.json"
`), 0o755)

	t.Setenv("LEARNING_LOOP_DIR", dir)

	workDir := t.TempDir()
	rec := RunRecord{
		Bead:   "mock-bead-1",
		Agent:  "test-agent",
		Status: "done",
	}

	err := CollectFeedback(rec, workDir)
	if err != nil {
		t.Fatalf("CollectFeedback should succeed: %v", err)
	}

	// Verify run record was written
	recordPath := filepath.Join(workDir, "run-records", "mock-bead-1.json")
	data, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("run record not written: %v", err)
	}

	var written RunRecord
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatalf("invalid run record JSON: %v", err)
	}
	if written.Bead != "mock-bead-1" {
		t.Errorf("bead = %q, want mock-bead-1", written.Bead)
	}
}

func TestCollectFeedbackScriptError(t *testing.T) {
	dir := t.TempDir()
	scriptsDir := filepath.Join(dir, "scripts")
	os.MkdirAll(scriptsDir, 0o755)

	script := filepath.Join(scriptsDir, "feedback-collector.sh")
	os.WriteFile(script, []byte("#!/bin/sh\necho 'script failed' >&2\nexit 1\n"), 0o755)

	t.Setenv("LEARNING_LOOP_DIR", dir)

	workDir := t.TempDir()
	rec := RunRecord{Bead: "fail-bead", Agent: "test", Status: "done"}

	err := CollectFeedback(rec, workDir)
	if err == nil {
		t.Error("CollectFeedback should return error when script fails")
	}
}

func TestLearningLoopDirNotFound(t *testing.T) {
	t.Setenv("LEARNING_LOOP_DIR", "")
	t.Setenv("HOME", t.TempDir()) // no tools/learning-loop here
	dir := LearningLoopDir()
	if dir != "" {
		t.Errorf("expected empty dir when not found, got %q", dir)
	}
}

func TestGitDiffStatInGitRepo(t *testing.T) {
	// gitDiffStat with empty repo returns "" — just verifying no crash
	stat := gitDiffStat(t.TempDir())
	_ = stat // may be empty, that's fine
}

func TestGitDiffStatEmptyRepo(t *testing.T) {
	stat := gitDiffStat("")
	if stat != "" {
		t.Errorf("expected empty string for empty repo arg, got %q", stat)
	}
}

func TestBvSearchInvalidJSON(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"bv": `echo "not valid json"`,
	})

	_, err := BvSearch("test", t.TempDir())
	if err == nil {
		t.Error("BvSearch should return error for invalid JSON")
	}
}

func TestBvRelatedInvalidJSON(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"bv": `echo "not valid json"`,
	})

	_, err := BvRelated("bead-1", t.TempDir())
	if err == nil {
		t.Error("BvRelated should return error for invalid JSON")
	}
}

func TestBvPlanInvalidJSON(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"bv": `echo "not valid json"`,
	})

	_, err := BvPlan(t.TempDir())
	if err == nil {
		t.Error("BvPlan should return error for invalid JSON")
	}
}

func TestBvSearchError(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"bv": `exit 1`,
	})

	_, err := BvSearch("test", t.TempDir())
	if err == nil {
		t.Error("BvSearch should return error when bv fails")
	}
}

func TestBvRelatedError(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"bv": `exit 1`,
	})

	_, err := BvRelated("bead-1", t.TempDir())
	if err == nil {
		t.Error("BvRelated should return error when bv fails")
	}
}

func TestBvPlanError(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"bv": `exit 1`,
	})

	_, err := BvPlan(t.TempDir())
	if err == nil {
		t.Error("BvPlan should return error when bv fails")
	}
}

func TestBrCreateError(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"br": `echo "error: no .beads" >&2; exit 1`,
	})

	_, err := BrCreate("test title", t.TempDir())
	if err == nil {
		t.Error("BrCreate should return error when br fails")
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
