package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"polis/work/internal/testutil"

	"github.com/spf13/cobra"
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

func TestTruncate(t *testing.T) {
	if truncate("short", 10) != "short" {
		t.Error("should not truncate short strings")
	}
	result := truncate("this is a longer string", 10)
	if !strings.HasSuffix(result, "...") {
		t.Error("truncated string should end with ...")
	}
	if len(result) != 13 { // 10 + "..."
		t.Errorf("expected length 13, got %d", len(result))
	}
}

func TestSenateCaseJSON(t *testing.T) {
	c := SenateCase{
		ID:       "senate-test-1",
		Type:     "general",
		Summary:  "Should we use Go or Rust?",
		Question: "Should we use Go or Rust for the new CLI?",
		Evidence: []string{"bead:proj-abc"},
		FiledAt:  "2026-02-23T12:00:00Z",
		FiledBy:  "work",
	}

	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var parsed SenateCase
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if parsed.ID != "senate-test-1" {
		t.Errorf("id = %q, want senate-test-1", parsed.ID)
	}
	if parsed.Type != "general" {
		t.Errorf("type = %q, want general", parsed.Type)
	}
	if len(parsed.Evidence) != 1 || parsed.Evidence[0] != "bead:proj-abc" {
		t.Errorf("evidence = %v, want [bead:proj-abc]", parsed.Evidence)
	}
}

func TestSenateVerdictJSON(t *testing.T) {
	v := SenateVerdict{
		CaseID:         "senate-test-1",
		Verdict:        "approved",
		Reasoning:      "Clear benefit, manageable risks.",
		Implementation: "1. Do X\n2. Do Y",
		Binding:        true,
	}

	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var parsed SenateVerdict
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if parsed.Verdict != "approved" {
		t.Errorf("verdict = %q, want approved", parsed.Verdict)
	}
	if !parsed.Binding {
		t.Error("expected binding = true")
	}
}

func TestDeliberateCommandExists(t *testing.T) {
	root := NewRoot("test")
	found := false
	for _, cmd := range root.Commands() {
		if cmd.Name() == "deliberate" {
			found = true
			break
		}
	}
	if !found {
		t.Error("deliberate command should be registered")
	}
}

func TestDeliberateCommandFlags(t *testing.T) {
	root := NewRoot("test")
	var delCmd *cobra.Command
	for _, cmd := range root.Commands() {
		if cmd.Name() == "deliberate" {
			delCmd = cmd
			break
		}
	}
	if delCmd == nil {
		t.Fatal("deliberate command not found")
	}

	flags := []string{"type", "participants", "evidence", "filed-by", "state-dir", "no-handoff"}
	for _, name := range flags {
		if delCmd.Flags().Lookup(name) == nil {
			t.Errorf("expected flag --%s to exist", name)
		}
	}
}

// --- functional tests for runDeliberate ---

// TestRunDeliberateHappyPath exercises runDeliberate with a mock senate
// that returns an approved verdict JSON, and a mock br for bead lifecycle.
func TestRunDeliberateHappyPath(t *testing.T) {
	verdict := SenateVerdict{
		CaseID:         "senate-work-test",
		Verdict:        "approved",
		Reasoning:      "Makes sense to proceed.",
		Implementation: "Do X then Y",
		Binding:        true,
	}
	verdictJSON, _ := json.Marshal(verdict)

	// Write verdict to a temp file so the mock can cat it (avoids shell escaping issues).
	verdictFile := filepath.Join(t.TempDir(), "verdict.json")
	os.WriteFile(verdictFile, verdictJSON, 0o644)

	testutil.SandboxPATH(t, map[string]string{
		"senate": `cat ` + verdictFile,
		"br":     `echo "test-bead-001"`,
	})

	// Set HOME to a temp dir so case files are written to a temp .work dir
	home := t.TempDir()
	t.Setenv("HOME", home)

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)

	cmd := &cobra.Command{}
	cmd.SetOut(buf)

	err := runDeliberate(cmd, "Should we adopt Go modules?", "architecture", 3, nil, "tester", "", true)
	if err != nil {
		t.Fatalf("runDeliberate returned error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Deliberating:") {
		t.Error("output should contain 'Deliberating:'")
	}
	if !strings.Contains(out, "Verdict: approved") {
		t.Error("output should contain 'Verdict: approved'")
	}
	if !strings.Contains(out, "Reasoning:") {
		t.Error("output should contain reasoning")
	}

	// Case file should have been written
	caseDir := filepath.Join(home, ".work", "senate-cases")
	entries, err := os.ReadDir(caseDir)
	if err != nil {
		t.Fatalf("read case dir: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected at least one case file")
	}

	// Verify case file is valid JSON
	caseData, err := os.ReadFile(filepath.Join(caseDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("read case file: %v", err)
	}
	var sc SenateCase
	if err := json.Unmarshal(caseData, &sc); err != nil {
		t.Fatalf("invalid case JSON: %v", err)
	}
	if sc.Type != "architecture" {
		t.Errorf("case type = %q, want architecture", sc.Type)
	}
	if sc.FiledBy != "tester" {
		t.Errorf("filed_by = %q, want tester", sc.FiledBy)
	}
}

// TestRunDeliberateSenateError exercises runDeliberate when senate exits
// with a non-zero status.
func TestRunDeliberateSenateError(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"senate": `echo "deliberation failed: quorum not reached" >&2; exit 1`,
		"br":     `echo "test-bead-err"`,
	})

	home := t.TempDir()
	t.Setenv("HOME", home)

	cmd := &cobra.Command{}
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	err := runDeliberate(cmd, "Should we refactor?", "general", 3, nil, "", "", false)
	if err == nil {
		t.Fatal("expected error when senate fails")
	}
	if !strings.Contains(err.Error(), "senate ask failed") {
		t.Errorf("error should mention senate failure, got: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Senate error:") {
		t.Error("output should report senate error")
	}
}

// TestRunDeliberateWithEvidence verifies that evidence flags are passed through
// and included in the case file.
func TestRunDeliberateWithEvidence(t *testing.T) {
	verdict := SenateVerdict{
		CaseID:    "senate-ev-test",
		Verdict:   "rejected",
		Reasoning: "Insufficient evidence.",
	}
	verdictJSON, _ := json.Marshal(verdict)

	verdictFile := filepath.Join(t.TempDir(), "verdict.json")
	os.WriteFile(verdictFile, verdictJSON, 0o644)

	testutil.SandboxPATH(t, map[string]string{
		"senate": `cat ` + verdictFile,
		"br":     `echo "test-bead-ev"`,
	})

	home := t.TempDir()
	t.Setenv("HOME", home)

	cmd := &cobra.Command{}
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	evidence := []string{"bead:evidence-1", "bead:evidence-2"}
	err := runDeliberate(cmd, "Adopt new framework?", "general", 5, evidence, "zeus", "", true)
	if err != nil {
		t.Fatalf("runDeliberate returned error: %v", err)
	}

	// Verify evidence landed in case file
	caseDir := filepath.Join(home, ".work", "senate-cases")
	entries, _ := os.ReadDir(caseDir)
	if len(entries) == 0 {
		t.Fatal("expected case file")
	}
	caseData, _ := os.ReadFile(filepath.Join(caseDir, entries[0].Name()))
	var sc SenateCase
	json.Unmarshal(caseData, &sc)
	if len(sc.Evidence) != 2 {
		t.Errorf("expected 2 evidence items, got %d", len(sc.Evidence))
	}

	out := buf.String()
	if !strings.Contains(out, "Verdict: rejected") {
		t.Error("output should show rejected verdict")
	}
}

func TestRunDeliberateSenateCommandContract(t *testing.T) {
	tmp := t.TempDir()
	askArgsPath := filepath.Join(tmp, "senate-ask.args")
	askPwdPath := filepath.Join(tmp, "senate-ask.pwd")
	handoffArgsPath := filepath.Join(tmp, "senate-handoff.args")
	handoffPwdPath := filepath.Join(tmp, "senate-handoff.pwd")
	stateDir := filepath.Join(tmp, "state")

	verdict := SenateVerdict{
		CaseID:         "senate-case-42",
		Verdict:        "approved",
		Reasoning:      "Ship it.",
		Implementation: "Open the follow-up bead.",
		Binding:        true,
	}
	verdictJSON, err := json.Marshal(verdict)
	if err != nil {
		t.Fatalf("marshal verdict: %v", err)
	}

	verdictFile := filepath.Join(tmp, "verdict.json")
	if err := os.WriteFile(verdictFile, verdictJSON, 0o644); err != nil {
		t.Fatalf("write verdict file: %v", err)
	}

	testutil.SandboxPATH(t, map[string]string{
		"senate": `
case "$1" in
  ask)
    pwd > ` + askPwdPath + `
    printf '%s\0' "$@" > ` + askArgsPath + `
    cat ` + verdictFile + `
    ;;
  handoff)
    pwd > ` + handoffPwdPath + `
    printf '%s\0' "$@" > ` + handoffArgsPath + `
    printf '{"beads":["impl-1"]}'
    ;;
  *)
    exit 1
    ;;
esac`,
		"br": `echo "test-bead-contract"`,
	})

	home := t.TempDir()
	t.Setenv("HOME", home)

	repo, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	cmd := &cobra.Command{}
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	if err := runDeliberate(cmd, "Should we formalize the senate CLI contract?", "architecture", 5, []string{"bead:work-1"}, "tester", stateDir, false); err != nil {
		t.Fatalf("runDeliberate returned error: %v\noutput: %s", err, buf.String())
	}

	caseDir := filepath.Join(home, ".work", "senate-cases")
	entries, err := os.ReadDir(caseDir)
	if err != nil {
		t.Fatalf("read case dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 case file, got %d", len(entries))
	}

	casePath := filepath.Join(caseDir, entries[0].Name())
	caseData, err := os.ReadFile(casePath)
	if err != nil {
		t.Fatalf("read case file: %v", err)
	}

	var sc SenateCase
	if err := json.Unmarshal(caseData, &sc); err != nil {
		t.Fatalf("unmarshal case file: %v", err)
	}

	askArgs := readLoggedArgs(t, askArgsPath)
	wantAsk := []string{"ask", "Should we formalize the senate CLI contract?", "--json", "--agents", "5", "--type", "architecture", "--filed-by", "tester", "--state-dir", stateDir}
	if !reflect.DeepEqual(askArgs, wantAsk) {
		t.Fatalf("ask args = %#v, want %#v", askArgs, wantAsk)
	}

	handoffArgs := readLoggedArgs(t, handoffArgsPath)
	wantHandoff := []string{"handoff", "--case-id", verdict.CaseID, "--json", "--state-dir", stateDir}
	if !reflect.DeepEqual(handoffArgs, wantHandoff) {
		t.Fatalf("handoff args = %#v, want %#v", handoffArgs, wantHandoff)
	}

	askPwd, err := os.ReadFile(askPwdPath)
	if err != nil {
		t.Fatalf("read ask pwd: %v", err)
	}
	if strings.TrimSpace(string(askPwd)) != repo {
		t.Fatalf("ask ran in %q, want %q", strings.TrimSpace(string(askPwd)), repo)
	}

	handoffPwd, err := os.ReadFile(handoffPwdPath)
	if err != nil {
		t.Fatalf("read handoff pwd: %v", err)
	}
	if strings.TrimSpace(string(handoffPwd)) != repo {
		t.Fatalf("handoff ran in %q, want %q", strings.TrimSpace(string(handoffPwd)), repo)
	}
}
