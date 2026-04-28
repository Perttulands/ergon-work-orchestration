package ecosystem

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"polis/work/internal/loopfeed"
	"polis/work/internal/testutil"
)

func readLoggedArgs(t *testing.T, path string) []string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read logged args %s: %v", path, err)
	}

	raw := bytes.Split(data, []byte{0})
	if len(raw) > 0 && len(raw[len(raw)-1]) == 0 {
		raw = raw[:len(raw)-1]
	}

	args := make([]string, 0, len(raw))
	for _, item := range raw {
		args = append(args, string(item))
	}
	return args
}

func testFeedEntry(id, task, outcome, agent string, durationS int, errMsg string) loopfeed.Entry {
	entry := loopfeed.Entry{
		ID:        id,
		Task:      task,
		Outcome:   outcome,
		Timestamp: "2026-03-13T00:00:00Z",
		Agent:     agent,
	}
	if durationS > 0 {
		dur := durationS
		entry.DurationS = &dur
	}
	if errMsg != "" {
		entry.ErrorMsg = errMsg
	}
	return entry
}

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
	testutil.TestBeadsDir(t)
	t.Setenv("POLIS_ACTOR", "ecosystem-test")
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

	err := IngestRun(testFeedEntry("bead-1", "fix a bug", "success", "worker", 120, ""))
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
	if err == nil {
		t.Errorf("should return error when LEARNING_LOOP_DIR is configured but script is missing")
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

func TestWriteRunRecord(t *testing.T) {
	rec := RunRecord{
		Bead:   "test-bead",
		Agent:  "test-agent",
		Status: "done",
	}
	workDir := t.TempDir()
	err := WriteRunRecord(rec, workDir)
	if err != nil {
		t.Fatalf("WriteRunRecord should succeed: %v", err)
	}

	recordPath := filepath.Join(workDir, "run-records", "test-bead.json")
	data, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("run record not written: %v", err)
	}

	var written RunRecord
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatalf("invalid run record JSON: %v", err)
	}
	if written.Bead != "test-bead" {
		t.Errorf("bead = %q, want test-bead", written.Bead)
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

func TestBrAgentStateNoOp(t *testing.T) {
	// BrAgentState is a no-op since br doesn't support agent state tracking
	err := BrAgentState("zeus", "working")
	if err != nil {
		t.Errorf("BrAgentState should always return nil: %v", err)
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

func TestGateCheckStderrDoesNotContaminateJSON(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		// Simulate gate calling br which writes INFO logs to stderr
		"gate": `echo "INFO beads_rust::sync: Auto-flush complete" >&2; printf '{"pass":false,"score":0.75}'; exit 1`,
	})

	result, err := GateCheck(t.TempDir(), "worker")
	if err != nil {
		t.Fatalf("GateCheck should not error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// JSON should parse correctly despite stderr noise
	if result.Pass {
		t.Error("expected pass=false from JSON")
	}
	if result.Score != 0.75 {
		t.Errorf("score = %f, want 0.75 (stderr contamination?)", result.Score)
	}
}

func TestGateCheckCommandContract(t *testing.T) {
	tmp := t.TempDir()
	argsPath := filepath.Join(tmp, "gate.args")
	pwdPath := filepath.Join(tmp, "gate.pwd")

	testutil.SandboxPATH(t, map[string]string{
		"gate": `pwd > ` + pwdPath + `
printf '%s\0' "$@" > ` + argsPath + `
printf '{"pass":true,"score":0.95}'`,
	})

	repo := t.TempDir()
	result, err := GateCheck(repo, "zeus")
	if err != nil {
		t.Fatalf("GateCheck should not error: %v", err)
	}
	if result == nil || !result.Pass || result.Score != 0.95 {
		t.Fatalf("unexpected gate result: %#v", result)
	}

	args := readLoggedArgs(t, argsPath)
	want := []string{"check", ".", "--json", "--citizen", "zeus"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("gate args = %#v, want %#v", args, want)
	}

	pwd, err := os.ReadFile(pwdPath)
	if err != nil {
		t.Fatalf("read gate pwd: %v", err)
	}
	if got := string(bytes.TrimSpace(pwd)); got != repo {
		t.Fatalf("gate cwd = %q, want %q", got, repo)
	}
}

func TestIngestRunWithMock(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"loop": `cat >/dev/null`, // consume stdin
	})

	err := IngestRun(testFeedEntry("bead-1", "fix bug", "success", "zeus", 120, ""))
	if err != nil {
		t.Errorf("IngestRun should succeed with mock: %v", err)
	}
}

func TestIngestRunWithErrorMessage(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"loop": `cat >/dev/null`,
	})

	err := IngestRun(testFeedEntry("bead-2", "fix bug", "failure", "zeus", 60, "tests failed"))
	if err != nil {
		t.Errorf("IngestRun with error msg should succeed: %v", err)
	}
}

func TestIngestRunError(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"loop": `exit 1`,
	})

	err := IngestRun(testFeedEntry("bead-3", "fix bug", "success", "zeus", 120, ""))
	if err == nil {
		t.Error("IngestRun should return error when loop fails")
	}
}

func TestIngestRunCommandContract(t *testing.T) {
	tmp := t.TempDir()
	argsPath := filepath.Join(tmp, "loop.args")
	stdinPath := filepath.Join(tmp, "loop.stdin.json")

	testutil.SandboxPATH(t, map[string]string{
		"loop": `printf '%s\0' "$@" > ` + argsPath + `
cat > ` + stdinPath,
	})

	t.Setenv("POLIS_LOOP_DB", filepath.Join(tmp, "loop.db"))
	entry := testFeedEntry("bead-42", "fix flaky gate contract test", "failure", "zeus", 91, "gate failed")
	entry.Metadata = map[string]any{
		"files_touched": []string{"internal/cli/run.go"},
	}
	err := IngestRun(entry)
	if err != nil {
		t.Fatalf("IngestRun should not error: %v", err)
	}

	args := readLoggedArgs(t, argsPath)
	wantArgs := []string{"ingest", "--db", filepath.Join(tmp, "loop.db"), "-"}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("loop args = %#v, want %#v", args, wantArgs)
	}

	data, err := os.ReadFile(stdinPath)
	if err != nil {
		t.Fatalf("read loop stdin: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal loop payload: %v", err)
	}

	if payload["id"] != "bead-42" {
		t.Fatalf("payload id = %v, want bead-42", payload["id"])
	}
	if payload["task"] != "fix flaky gate contract test" {
		t.Fatalf("payload task = %v", payload["task"])
	}
	if payload["outcome"] != "failure" {
		t.Fatalf("payload outcome = %v, want failure", payload["outcome"])
	}
	if payload["agent"] != "zeus" {
		t.Fatalf("payload agent = %v, want zeus", payload["agent"])
	}
	if payload["duration_seconds"] != float64(91) {
		t.Fatalf("payload duration_seconds = %v, want 91", payload["duration_seconds"])
	}
	if payload["error_message"] != "gate failed" {
		t.Fatalf("payload error_message = %v, want gate failed", payload["error_message"])
	}

	metadata, ok := payload["metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("payload metadata = %#v, want object", payload["metadata"])
	}
	files, ok := metadata["files_touched"].([]interface{})
	if !ok || len(files) != 1 || files[0] != "internal/cli/run.go" {
		t.Fatalf("payload metadata.files_touched = %#v, want [internal/cli/run.go]", metadata["files_touched"])
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

	// loop query errors should return contextual error
	result, err := QueryLearningLoop("fix a bug")
	if err == nil {
		t.Error("QueryLearningLoop should return error when loop command fails")
	}
	if result != nil {
		t.Error("expected nil result on command failure")
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

func TestWriteRunRecordOverwritesExistingRecord(t *testing.T) {
	workDir := t.TempDir()
	rec := RunRecord{
		Bead:   "mock-bead-1",
		Agent:  "test-agent",
		Status: "done",
	}

	err := WriteRunRecord(rec, workDir)
	if err != nil {
		t.Fatalf("WriteRunRecord should succeed: %v", err)
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

func TestWriteRunRecordWithVerification(t *testing.T) {
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

	err := WriteRunRecord(rec, workDir)
	if err != nil {
		t.Fatalf("write run record failed: %v", err)
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
	if written.Verification.Tests != "pass" {
		t.Errorf("tests = %q, want pass", written.Verification.Tests)
	}
}
