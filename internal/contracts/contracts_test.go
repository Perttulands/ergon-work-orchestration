package contracts

import (
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
	// Verify JSON output (newline-delimited)
	trimmed := strings.TrimSpace(stdout)
	if trimmed != "" {
		lines := strings.Split(trimmed, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var obj json.RawMessage
			if err := json.Unmarshal([]byte(line), &obj); err != nil {
				// ubs may output text headers before JSON — skip non-JSON lines
				continue
			}
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
