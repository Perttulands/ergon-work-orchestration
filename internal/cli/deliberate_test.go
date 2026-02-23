package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

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
