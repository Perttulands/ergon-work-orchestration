package cli

import (
	"bytes"
	"strings"
	"testing"

	"polis/work/internal/testutil"

	"github.com/spf13/cobra"
)

func TestDecideCommandExists(t *testing.T) {
	root := NewRoot("test")
	found := false
	for _, cmd := range root.Commands() {
		if cmd.Name() == "decide" {
			found = true
			break
		}
	}
	if !found {
		t.Error("decide command should be registered")
	}
}

func TestDecideCommandFlags(t *testing.T) {
	root := NewRoot("test")
	var decideCmd *cobra.Command
	for _, cmd := range root.Commands() {
		if cmd.Name() == "decide" {
			decideCmd = cmd
			break
		}
	}
	if decideCmd == nil {
		t.Fatal("decide command not found")
	}

	flags := []string{"evidence", "decider", "priority"}
	for _, name := range flags {
		if decideCmd.Flags().Lookup(name) == nil {
			t.Errorf("expected flag --%s to exist", name)
		}
	}
}

func TestGatherBeadEvidence(t *testing.T) {
	// When br is available, gatherBeadEvidence on a nonexistent bead returns empty
	result := gatherBeadEvidence("nonexistent-bead-xyz-12345", "/tmp")
	if result != "" {
		t.Errorf("expected empty string for nonexistent bead, got %q", result)
	}
}

// --- functional tests for runDecide ---

// TestRunDecideHappyPath exercises runDecide with mock relay and br,
// verifying the full flow: bead creation, message assembly, relay send.
func TestRunDecideHappyPath(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"relay": `echo "sent"`,
		"br":    `echo "gate-bead-001"`,
	})

	home := t.TempDir()
	t.Setenv("HOME", home)

	cmd := &cobra.Command{}
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	err := runDecide(cmd, "Should we migrate to PostgreSQL?", nil, "athena", "high")
	if err != nil {
		t.Fatalf("runDecide returned error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Decision needed:") {
		t.Error("output should contain 'Decision needed:'")
	}
	if !strings.Contains(out, "Notified: athena") {
		t.Error("output should confirm relay notification sent")
	}
	if !strings.Contains(out, "Gate bead:") {
		t.Error("output should show gate bead ID")
	}
	if !strings.Contains(out, "To rule:") {
		t.Error("output should show how to close the gate bead")
	}
}

// TestRunDecideNoRelay exercises runDecide when relay is not available,
// verifying graceful degradation with message printed to stdout.
func TestRunDecideNoRelay(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"br": `echo "gate-bead-002"`,
	})

	home := t.TempDir()
	t.Setenv("HOME", home)

	cmd := &cobra.Command{}
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	err := runDecide(cmd, "Which API version?", nil, "hermes", "normal")
	if err != nil {
		t.Fatalf("runDecide returned error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "relay not available") {
		t.Error("should mention relay not available")
	}
	// The full message should still be printed for the user to see
	if !strings.Contains(out, "DECISION REQUESTED") {
		t.Error("should print the decision message when relay is unavailable")
	}
}

// TestRunDecideWithEvidence exercises runDecide with evidence bead references,
// verifying evidence assembly into the notification message.
func TestRunDecideWithEvidence(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"relay": `echo "sent"`,
		"br": `
case "$1" in
  show) echo "Evidence: performance benchmarks show 2x improvement" ;;
  *)    echo "gate-bead-ev" ;;
esac`,
	})

	home := t.TempDir()
	t.Setenv("HOME", home)

	cmd := &cobra.Command{}
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	evidence := []string{"perf-bead-001", "perf-bead-002"}
	err := runDecide(cmd, "Approve perf optimization?", evidence, "athena", "urgent")
	if err != nil {
		t.Fatalf("runDecide returned error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Decision needed:") {
		t.Error("output should contain decision question")
	}
	if !strings.Contains(out, "Notified: athena") {
		t.Error("should confirm notification sent")
	}
}

// TestRunDecideRelayFails exercises runDecide when relay exists but fails,
// verifying the warning is printed but the function still succeeds.
func TestRunDecideRelayFails(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"relay": `echo "connection refused" >&2; exit 1`,
		"br":    `echo "gate-bead-fail"`,
	})

	home := t.TempDir()
	t.Setenv("HOME", home)

	cmd := &cobra.Command{}
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	err := runDecide(cmd, "Should we deploy?", nil, "athena", "normal")
	if err != nil {
		t.Fatalf("runDecide should not fail even when relay fails: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "relay send failed") {
		t.Error("should warn about relay send failure")
	}
	// Gate bead should still be created and reported
	if !strings.Contains(out, "Gate bead:") {
		t.Error("should still report gate bead even when relay fails")
	}
}
