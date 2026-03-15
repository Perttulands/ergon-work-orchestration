package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpineParityCommandExists(t *testing.T) {
	root := NewRoot("test")
	found := false
	for _, cmd := range root.Commands() {
		if cmd.Name() == "spine-parity" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("spine-parity command should be registered")
	}
}

func TestSpineParityCommandFlags(t *testing.T) {
	root := NewRoot("test")
	var parityCmdName string
	for _, cmd := range root.Commands() {
		if cmd.Name() == "spine-parity" {
			if cmd.Flags().Lookup("work-dir") == nil {
				t.Fatal("expected --work-dir flag")
			}
			if cmd.Flags().Lookup("spine-dir") == nil {
				t.Fatal("expected --spine-dir flag")
			}
			if cmd.Flags().Lookup("json") == nil {
				t.Fatal("expected --json flag")
			}
			parityCmdName = cmd.Name()
			break
		}
	}
	if parityCmdName == "" {
		t.Fatal("spine-parity command should be present")
	}
}

func TestSpineParityJSONSuccess(t *testing.T) {
	workDir, spineDir := writeParityFixtures(t, "success", "pass")

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"spine-parity", "--work-dir", workDir, "--spine-dir", spineDir, "--json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("spine-parity failed: %v\noutput: %s", err, buf.String())
	}

	var report map[string]any
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("parse parity output: %v\noutput: %s", err, buf.String())
	}
	if got := int(report["legacy_runs"].(float64)); got != 1 {
		t.Fatalf("legacy_runs = %d, want 1", got)
	}
	if got := int(report["spine_runs"].(float64)); got != 1 {
		t.Fatalf("spine_runs = %d, want 1", got)
	}
}

func TestSpineParityMismatchReturnsError(t *testing.T) {
	workDir, spineDir := writeParityFixtures(t, "timeout", "pass")

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"spine-parity", "--work-dir", workDir, "--spine-dir", spineDir})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected parity mismatch to return an error")
	}
	if !strings.Contains(buf.String(), "outcome_mismatch") {
		t.Fatalf("expected mismatch details in output, got: %s", buf.String())
	}
}

func writeParityFixtures(t *testing.T, legacyOutcome, gateVerdict string) (string, string) {
	t.Helper()

	workDir := t.TempDir()
	traceDir := filepath.Join(workDir, "traces", "2026", "03", "13")
	if err := os.MkdirAll(traceDir, 0o755); err != nil {
		t.Fatalf("mkdir trace dir: %v", err)
	}
	tracePath := filepath.Join(traceDir, "trace-pol-parity.jsonl")
	traceContent := strings.Join([]string{
		`{"ts":"2026-03-13T00:00:00Z","event":"begin","bead":"pol-parity","trace_id":"01ARZ3NDEKTSV4RRFFQ69G5FAV","session_id":"7d444840-9dc0-11d1-b245-5ffdce74fad2","run_id":"run-01ARZ3NDEKTSV4RRFFQ69G5FAV","agent":"worker","task":"parity test"}`,
		`{"ts":"2026-03-13T00:00:01Z","event":"tool_call","tool":"bash","cmd":"go test","run_id":"run-01ARZ3NDEKTSV4RRFFQ69G5FAV"}`,
		`{"ts":"2026-03-13T00:00:02Z","event":"gate_result","run_id":"run-01ARZ3NDEKTSV4RRFFQ69G5FAV","pass":true}`,
		`{"ts":"2026-03-13T00:00:03Z","event":"end","run_id":"run-01ARZ3NDEKTSV4RRFFQ69G5FAV","outcome":"` + legacyOutcome + `"}`,
		"",
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(traceContent), 0o644); err != nil {
		t.Fatalf("write trace: %v", err)
	}

	spineDir := t.TempDir()
	spinePath := filepath.Join(spineDir, "2026-03-13.jsonl")
	spineContent := strings.Join([]string{
		`{"id":"01ARZ3NDEKTSV4RRFFQ69G5FAA","ts":"2026-03-13T00:00:00Z","kind":"session.start","trace_id":"01ARZ3NDEKTSV4RRFFQ69G5FAV","session_id":"7d444840-9dc0-11d1-b245-5ffdce74fad2","run_id":"run-01ARZ3NDEKTSV4RRFFQ69G5FAV","bead_id":"pol-parity","agent_id":"worker","model":"openai/gpt","data":{"cwd":"/tmp/repo","mode":"orchestrated","model":"openai/gpt"}}`,
		`{"id":"01ARZ3NDEKTSV4RRFFQ69G5FAB","ts":"2026-03-13T00:00:00Z","kind":"agent.start","trace_id":"01ARZ3NDEKTSV4RRFFQ69G5FAV","session_id":"7d444840-9dc0-11d1-b245-5ffdce74fad2","run_id":"run-01ARZ3NDEKTSV4RRFFQ69G5FAV","bead_id":"pol-parity","agent_id":"worker","model":"openai/gpt","data":{"agent_name":"worker","role":null}}`,
		`{"id":"01ARZ3NDEKTSV4RRFFQ69G5FAC","ts":"2026-03-13T00:00:01Z","kind":"bash.run","trace_id":"01ARZ3NDEKTSV4RRFFQ69G5FAV","session_id":"7d444840-9dc0-11d1-b245-5ffdce74fad2","run_id":"run-01ARZ3NDEKTSV4RRFFQ69G5FAV","bead_id":"pol-parity","agent_id":"worker","model":"openai/gpt","data":{"command":"go test","exit_code":0,"duration_ms":0,"stdout":"","stderr":"","truncated":false}}`,
		`{"id":"01ARZ3NDEKTSV4RRFFQ69G5FAD","ts":"2026-03-13T00:00:02Z","kind":"gate.result","trace_id":"01ARZ3NDEKTSV4RRFFQ69G5FAV","session_id":"7d444840-9dc0-11d1-b245-5ffdce74fad2","run_id":"run-01ARZ3NDEKTSV4RRFFQ69G5FAV","bead_id":"pol-parity","agent_id":"worker","model":"openai/gpt","data":{"verdict":"` + gateVerdict + `","checks":[{"name":"gate","passed":true,"message":"score unavailable"}],"duration_ms":0}}`,
		`{"id":"01ARZ3NDEKTSV4RRFFQ69G5FAE","ts":"2026-03-13T00:00:03Z","kind":"session.end","trace_id":"01ARZ3NDEKTSV4RRFFQ69G5FAV","session_id":"7d444840-9dc0-11d1-b245-5ffdce74fad2","run_id":"run-01ARZ3NDEKTSV4RRFFQ69G5FAV","bead_id":"pol-parity","agent_id":"worker","model":"openai/gpt","data":{"exit_reason":"completed","turn_count":0}}`,
		`{"id":"01ARZ3NDEKTSV4RRFFQ69G5FAF","ts":"2026-03-13T00:00:03Z","kind":"agent.end","trace_id":"01ARZ3NDEKTSV4RRFFQ69G5FAV","session_id":"7d444840-9dc0-11d1-b245-5ffdce74fad2","run_id":"run-01ARZ3NDEKTSV4RRFFQ69G5FAV","bead_id":"pol-parity","agent_id":"worker","model":"openai/gpt","data":{"exit_reason":"completed"}}`,
		"",
	}, "\n")
	if err := os.WriteFile(spinePath, []byte(spineContent), 0o644); err != nil {
		t.Fatalf("write spine: %v", err)
	}

	return workDir, spineDir
}
