package cli

import (
	"strings"
	"testing"

	workctx "polis/work/internal/context"

	"github.com/spf13/cobra"
)

func TestAssemblePrompt(t *testing.T) {
	ctx := &workctx.Result{
		Markdown: "## Past Work\n\n- **work-old** [closed] Previous auth work",
	}

	prompt := assemblePrompt("add JWT auth", "zeus", "work-abc", "/home/polis/projects/test", ctx)

	checks := []string{
		"BEAD: work-abc",
		"CITIZEN: zeus",
		"REPO: /home/polis/projects/test",
		"TASK: add JWT auth",
		"QUALITY EXPECTATIONS:",
		"Write tests",
		"CONTEXT FROM PAST WORK:",
		"Previous auth work",
		"DONE WHEN:",
	}

	for _, check := range checks {
		if !strings.Contains(prompt, check) {
			t.Errorf("prompt should contain %q", check)
		}
	}
}

func TestAssemblePromptNoContext(t *testing.T) {
	ctx := &workctx.Result{
		Markdown: "No prior context available. This is a fresh start.",
	}

	prompt := assemblePrompt("new task", "worker", "work-123", "/tmp", ctx)

	// Should NOT include context section when there's nothing useful
	if strings.Contains(prompt, "CONTEXT FROM PAST WORK") {
		t.Error("should not include context section when there's no useful context")
	}
}

func TestAssemblePromptNilContext(t *testing.T) {
	prompt := assemblePrompt("task", "worker", "work-123", "/tmp", nil)
	if !strings.Contains(prompt, "TASK: task") {
		t.Error("should still contain task even with nil context")
	}
}

func TestRandomID(t *testing.T) {
	id1 := randomID()
	id2 := randomID()
	if id1 == id2 {
		t.Error("random IDs should be different")
	}
	if len(id1) != 8 {
		t.Errorf("expected 8-char hex ID, got %d chars: %s", len(id1), id1)
	}
}

func TestRunCommandExists(t *testing.T) {
	root := NewRoot("test")
	// Verify run command is registered
	found := false
	for _, cmd := range root.Commands() {
		if cmd.Name() == "run" {
			found = true
			break
		}
	}
	if !found {
		t.Error("run command should be registered")
	}
}

func TestRunCommandFlags(t *testing.T) {
	root := NewRoot("test")
	var runCmd *cobra.Command
	for _, cmd := range root.Commands() {
		if cmd.Name() == "run" {
			runCmd = cmd
			break
		}
	}
	if runCmd == nil {
		t.Fatal("run command not found")
	}

	// Check flags exist
	flags := []string{"repo", "citizen", "deadline"}
	for _, name := range flags {
		if runCmd.Flags().Lookup(name) == nil {
			t.Errorf("expected flag --%s to exist", name)
		}
	}
}
