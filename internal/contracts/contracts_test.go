package contracts

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// runCLI executes a command with a 10s timeout and returns stdout, stderr, exit code.
func runCLI(t *testing.T, name string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	return
}

// --- work → senate ---

func TestContract_Work_Senate_Health(t *testing.T) { // integration test
	if testing.Short() {
		t.Skip("integration test: skipped in short mode")
	}
	if _, err := exec.LookPath("senate"); err != nil {
		t.Skip("senate not on PATH")
	}

	stdout, _, exitCode := runCLI(t, "senate", "health")
	if exitCode != 0 {
		t.Fatalf("senate health exit code = %d, want 0\nstdout: %s", exitCode, stdout)
	}
}

// --- work → gate ---

func TestContract_Work_Gate_Check(t *testing.T) { // integration test
	if testing.Short() {
		t.Skip("integration test: skipped in short mode")
	}
	if _, err := exec.LookPath("gate"); err != nil {
		t.Skip("gate not on PATH")
	}

	stdout, _, exitCode := runCLI(t, "gate", "check", ".")
	if exitCode != 0 {
		t.Fatalf("gate check exit code = %d, want 0\nstdout: %s", exitCode, stdout)
	}
	// Gate output should contain PASS or FAIL
	if !strings.Contains(stdout, "PASS") && !strings.Contains(stdout, "FAIL") {
		t.Fatalf("gate check output should contain PASS or FAIL, got: %s", stdout)
	}
}

// --- work → relay ---

func TestContract_Work_Relay_Read(t *testing.T) { // integration test
	if testing.Short() {
		t.Skip("integration test: skipped in short mode")
	}
	if _, err := exec.LookPath("relay"); err != nil {
		t.Skip("relay not on PATH")
	}

	// relay read with --json should exit 0 and produce parseable JSON or empty
	stdout, _, exitCode := runCLI(t, "relay", "read", "--json")
	if exitCode != 0 {
		t.Fatalf("relay read --json exit code = %d, want 0\nstdout: %s", exitCode, stdout)
	}
	// Output should be valid JSON (array) or empty
	trimmed := strings.TrimSpace(stdout)
	if trimmed != "" {
		var msgs []json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &msgs); err != nil {
			// Could be newline-delimited JSON
			for _, line := range strings.Split(trimmed, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				var obj json.RawMessage
				if err := json.Unmarshal([]byte(line), &obj); err != nil {
					t.Fatalf("relay read --json output not valid JSON: %s", line)
				}
			}
		}
	}
}

// --- work → loop ---

func TestContract_Work_Loop_Status(t *testing.T) { // integration test
	if testing.Short() {
		t.Skip("integration test: skipped in short mode")
	}
	if _, err := exec.LookPath("loop"); err != nil {
		t.Skip("loop not on PATH")
	}

	stdout, _, exitCode := runCLI(t, "loop", "status")
	if exitCode != 0 {
		t.Fatalf("loop status exit code = %d, want 0\nstdout: %s", exitCode, stdout)
	}
}

// --- gate → truthsayer ---

func TestContract_Gate_Truthsayer_Scan(t *testing.T) { // integration test
	if testing.Short() {
		t.Skip("integration test: skipped in short mode")
	}
	if _, err := exec.LookPath("truthsayer"); err != nil {
		t.Skip("truthsayer not on PATH")
	}

	stdout, _, exitCode := runCLI(t, "truthsayer", "scan", ".")
	// truthsayer scan exits 0 on success (may find findings)
	if exitCode != 0 {
		t.Fatalf("truthsayer scan exit code = %d, want 0\nstdout: %s", exitCode, stdout)
	}
}

// --- gate → ubs ---

func TestContract_Gate_UBS_Report(t *testing.T) { // integration test
	if testing.Short() {
		t.Skip("integration test: skipped in short mode")
	}
	if _, err := exec.LookPath("ubs"); err != nil {
		t.Skip("ubs not on PATH")
	}

	stdout, _, exitCode := runCLI(t, "ubs", "--format=json", ".")
	// ubs may exit 0 or 1 (warnings), but should produce JSON output
	if exitCode > 1 {
		t.Fatalf("ubs exit code = %d, want 0 or 1\nstdout: %s", exitCode, stdout)
	}
	// Verify JSON output — ubs --format=json emits a single JSON object (pretty or compact).
	// Must be non-empty and parseable as JSON.
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		t.Fatalf("ubs --format=json produced no output")
	}
	var result json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		// Fallback: try newline-delimited JSON (future ubs versions may change format)
		lines := strings.Split(trimmed, "\n")
		jsonCount := 0
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var obj json.RawMessage
			if err2 := json.Unmarshal([]byte(line), &obj); err2 == nil {
				jsonCount++
			}
		}
		if jsonCount == 0 {
			t.Fatalf("ubs --format=json produced no valid JSON (tried full-doc and NDJSON)\nstdout: %s", stdout)
		}
	}
}

// --- orbit → gate health (orbit not installed, skip) ---

func TestContract_Orbit_Gate_Health(t *testing.T) { // integration test
	if testing.Short() {
		t.Skip("integration test: skipped in short mode")
	}
	if _, err := exec.LookPath("orbit"); err != nil {
		t.Skip("orbit not on PATH")
	}
	if _, err := exec.LookPath("gate"); err != nil {
		t.Skip("gate not on PATH")
	}

	// orbit uses gate health internally
	stdout, _, exitCode := runCLI(t, "gate", "health")
	if exitCode != 0 {
		t.Fatalf("gate health exit code = %d, want 0\nstdout: %s", exitCode, stdout)
	}
}

// --- pol-120g.4: Failure-reproduction logging ---

// TestContractTestResult_JSONReport verifies that a failing ContractTestResult
// produces valid JSON output containing all diagnostic fields.
func TestContractTestResult_JSONReport(t *testing.T) {
	result := ContractTestResult{
		Tool:             "gate",
		Args:             []string{"check", "."},
		ExitCode:         1,
		Stdout:           "FAIL: some check",
		Stderr:           "error details here",
		ExpectedExitCode: 0,
		ExpectedFields:   []string{"PASS"},
		Passed:           false,
		DiagMessage:      "exit code mismatch: got 1, want 0",
	}

	var buf bytes.Buffer
	if err := result.ReportJSON(&buf); err != nil {
		t.Fatalf("ReportJSON: %v", err)
	}

	// Parse the output and verify all fields are present
	var parsed map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output not valid JSON: %v\nraw: %s", err, buf.String())
	}

	requiredFields := []string{"tool", "args", "exit_code", "stdout", "stderr", "expected_exit_code", "passed", "diag_message"}
	for _, field := range requiredFields {
		if _, ok := parsed[field]; !ok {
			t.Errorf("missing required field %q in JSON output", field)
		}
	}
	if parsed["tool"] != "gate" {
		t.Errorf("tool = %v, want gate", parsed["tool"])
	}
	if parsed["passed"] != false {
		t.Errorf("passed = %v, want false", parsed["passed"])
	}
}

// TestContractTestResult_HumanReport verifies the human-readable output
// contains the command, exit code, and diagnostic message.
func TestContractTestResult_HumanReport(t *testing.T) {
	result := ContractTestResult{
		Tool:             "senate",
		Args:             []string{"health"},
		ExitCode:         127,
		Stdout:           "",
		Stderr:           "command not found",
		ExpectedExitCode: 0,
		Passed:           false,
		DiagMessage:      "senate binary not found or not executable",
	}

	var buf bytes.Buffer
	result.ReportHuman(&buf)
	output := buf.String()

	if !strings.Contains(output, "CONTRACT FAILURE") {
		t.Error("human report should contain CONTRACT FAILURE header")
	}
	if !strings.Contains(output, "senate health") {
		t.Error("human report should contain the command")
	}
	if !strings.Contains(output, "127") {
		t.Error("human report should contain actual exit code")
	}
	if !strings.Contains(output, "not found or not executable") {
		t.Error("human report should contain diag message")
	}
}

// TestContractTestResult_PassingSkipsHumanReport verifies that passing
// tests produce no human-readable output.
func TestContractTestResult_PassingSkipsHumanReport(t *testing.T) {
	result := ContractTestResult{
		Tool:   "gate",
		Args:   []string{"check", "."},
		Passed: true,
	}

	var buf bytes.Buffer
	result.ReportHuman(&buf)
	if buf.Len() != 0 {
		t.Errorf("passing test should produce no human output, got: %s", buf.String())
	}
}

// TestContractTestResult_FixtureDeliberateFailure is a fixture test that
// constructs a deliberately wrong expected exit code to verify the diagnostic
// output contains all required fields. Only activated with -run flag.
func TestContractTestResult_FixtureDeliberateFailure(t *testing.T) {
	// This test always passes — it validates the diagnostic struct completeness
	// by constructing a "would-fail" scenario and checking the report.
	result := ContractTestResult{
		Tool:             "echo",
		Args:             []string{"hello"},
		ExitCode:         0,
		Stdout:           "hello\n",
		Stderr:           "",
		ExpectedExitCode: 42, // deliberately wrong
		ExpectedFields:   []string{"goodbye"},
		Passed:           false,
		DiagMessage:      "fixture: exit code mismatch: got 0, want 42",
	}

	// Verify JSON contains all diagnostic fields
	var jsonBuf bytes.Buffer
	if err := result.ReportJSON(&jsonBuf); err != nil {
		t.Fatalf("ReportJSON: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(jsonBuf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	diagnosticFields := []string{"tool", "args", "exit_code", "stdout", "stderr", "expected_exit_code", "expected_fields", "passed", "diag_message"}
	for _, f := range diagnosticFields {
		if _, ok := parsed[f]; !ok {
			t.Errorf("fixture failure report missing field %q", f)
		}
	}

	// Verify human report has key diagnostic info
	var humanBuf bytes.Buffer
	result.ReportHuman(&humanBuf)
	human := humanBuf.String()
	if !strings.Contains(human, "echo hello") {
		t.Error("human report should show the command")
	}
	if !strings.Contains(human, "fixture") {
		t.Error("human report should contain diagnostic message")
	}
}
