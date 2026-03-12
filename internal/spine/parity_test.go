package spine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCompareMatchesLegacyAndSpine(t *testing.T) {
	workDir := t.TempDir()
	traceDir := filepath.Join(workDir, "traces", "2026", "03", "13")
	if err := os.MkdirAll(traceDir, 0o755); err != nil {
		t.Fatalf("mkdir trace dir: %v", err)
	}

	tracePath := filepath.Join(traceDir, "trace-pol-abc.jsonl")
	traceLines := []map[string]any{
		{"ts": "2026-03-13T00:00:00Z", "event": "begin", "bead": "pol-abc", "trace_id": "01ARZ3NDEKTSV4RRFFQ69G5FAV", "session_id": "7d444840-9dc0-11d1-b245-5ffdce74fad2", "run_id": "run-01ARZ3NDEKTSV4RRFFQ69G5FAV", "agent": "worker", "task": "task"},
		{"ts": "2026-03-13T00:00:01Z", "event": "tool_call", "tool": "bash", "cmd": "go test", "run_id": "run-01ARZ3NDEKTSV4RRFFQ69G5FAV"},
		{"ts": "2026-03-13T00:00:02Z", "event": "gate_result", "run_id": "run-01ARZ3NDEKTSV4RRFFQ69G5FAV", "pass": true},
		{"ts": "2026-03-13T00:00:03Z", "event": "end", "run_id": "run-01ARZ3NDEKTSV4RRFFQ69G5FAV", "outcome": "success"},
	}
	writeJSONLLines(t, tracePath, traceLines)

	spineDir := t.TempDir()
	writer := NewWriter(spineDir)
	bead := "pol-abc"
	agent := "worker"
	model := "openai/gpt"
	runID := "run-01ARZ3NDEKTSV4RRFFQ69G5FAV"
	traceID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	sessionID := "7d444840-9dc0-11d1-b245-5ffdce74fad2"
	for _, env := range []RawEventEnvelope{
		{ID: MintULID(), TS: "2026-03-13T00:00:00Z", Kind: "session.start", TraceID: traceID, SessionID: sessionID, RunID: runID, BeadID: &bead, AgentID: &agent, Model: &model, Data: map[string]any{"cwd": "/tmp/repo", "mode": "orchestrated", "model": model}},
		{ID: MintULID(), TS: "2026-03-13T00:00:00Z", Kind: "agent.start", TraceID: traceID, SessionID: sessionID, RunID: runID, BeadID: &bead, AgentID: &agent, Model: &model, Data: map[string]any{"agent_name": "worker", "role": nil}},
		{ID: MintULID(), TS: "2026-03-13T00:00:01Z", Kind: "bash.run", TraceID: traceID, SessionID: sessionID, RunID: runID, BeadID: &bead, AgentID: &agent, Model: &model, Data: map[string]any{"command": "go test", "exit_code": 0, "duration_ms": 0, "stdout": "", "stderr": "", "truncated": false}},
		{ID: MintULID(), TS: "2026-03-13T00:00:02Z", Kind: "gate.result", TraceID: traceID, SessionID: sessionID, RunID: runID, BeadID: &bead, AgentID: &agent, Model: &model, Data: map[string]any{"verdict": "pass", "checks": []map[string]any{{"name": "gate", "passed": true, "message": "score unavailable"}}, "duration_ms": 0}},
		{ID: MintULID(), TS: "2026-03-13T00:00:03Z", Kind: "session.end", TraceID: traceID, SessionID: sessionID, RunID: runID, BeadID: &bead, AgentID: &agent, Model: &model, Data: map[string]any{"exit_reason": "completed", "turn_count": 0}},
		{ID: MintULID(), TS: "2026-03-13T00:00:03Z", Kind: "agent.end", TraceID: traceID, SessionID: sessionID, RunID: runID, BeadID: &bead, AgentID: &agent, Model: &model, Data: map[string]any{"exit_reason": "completed"}},
	} {
		if err := writer.Write(env); err != nil {
			t.Fatalf("writer.Write: %v", err)
		}
	}

	report, err := Compare(workDir, spineDir)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if len(report.MissingInSpine) != 0 || len(report.MissingInLegacy) != 0 || len(report.OutcomeMismatches) != 0 || len(report.OrderingMismatches) != 0 || len(report.GateMismatches) != 0 {
		t.Fatalf("unexpected mismatches: %+v", report)
	}
}

func TestCompareDetectsOutcomeMismatch(t *testing.T) {
	workDir := t.TempDir()
	traceDir := filepath.Join(workDir, "traces", "2026", "03", "13")
	if err := os.MkdirAll(traceDir, 0o755); err != nil {
		t.Fatalf("mkdir trace dir: %v", err)
	}
	tracePath := filepath.Join(traceDir, "trace-pol-outcome.jsonl")
	writeJSONLLines(t, tracePath, []map[string]any{
		{"ts": "2026-03-13T00:00:00Z", "event": "begin", "bead": "pol-outcome", "run_id": "run-01ARZ3NDEKTSV4RRFFQ69G5FAA"},
		{"ts": "2026-03-13T00:00:01Z", "event": "end", "run_id": "run-01ARZ3NDEKTSV4RRFFQ69G5FAA", "outcome": "timeout"},
	})
	spineDir := t.TempDir()
	writer := NewWriter(spineDir)
	runID := "run-01ARZ3NDEKTSV4RRFFQ69G5FAA"
	if err := writer.Write(RawEventEnvelope{
		ID: MintULID(), TS: "2026-03-13T00:00:01Z", Kind: "session.end",
		TraceID: "01ARZ3NDEKTSV4RRFFQ69G5FAA", SessionID: "7d444840-9dc0-11d1-b245-5ffdce74fad2", RunID: runID,
		Data: map[string]any{"exit_reason": "completed", "turn_count": 0},
	}); err != nil {
		t.Fatalf("writer.Write: %v", err)
	}

	report, err := Compare(workDir, spineDir)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if len(report.OutcomeMismatches) != 1 {
		t.Fatalf("OutcomeMismatches = %d, want 1", len(report.OutcomeMismatches))
	}
}

func writeJSONLLines(t *testing.T, path string, lines []map[string]any) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	for _, line := range lines {
		data, err := json.Marshal(line)
		if err != nil {
			t.Fatalf("marshal line: %v", err)
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			t.Fatalf("write line: %v", err)
		}
	}
}
