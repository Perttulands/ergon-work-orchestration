package cli

import (
	"testing"

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
	// When bd is available, gatherBeadEvidence on a nonexistent bead returns empty
	result := gatherBeadEvidence("nonexistent-bead-xyz-12345", "/tmp")
	if result != "" {
		t.Errorf("expected empty string for nonexistent bead, got %q", result)
	}
}
