package cli

import (
	"strings"
	"testing"

	workctx "polis/work/internal/context"
	"polis/work/internal/ecosystem"

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

func TestBuildRunRecordSuccess(t *testing.T) {
	gate := &ecosystem.GateResult{Pass: true, Score: 0.95}
	ctx := &workctx.Result{
		TemplateSelection: &ecosystem.TemplateSelection{TaskType: "bug-fix"},
	}
	rec := buildRunRecord("bead-1", "mercury", "success", 120, gate, ctx)

	if rec.Bead != "bead-1" {
		t.Errorf("bead = %q, want bead-1", rec.Bead)
	}
	if rec.Status != "done" {
		t.Errorf("status = %q, want done", rec.Status)
	}
	if rec.ExitCode != 0 {
		t.Errorf("exit_code = %d, want 0", rec.ExitCode)
	}
	if rec.TemplateName != "bug-fix" {
		t.Errorf("template = %q, want bug-fix", rec.TemplateName)
	}
	if rec.Verification.Tests != "pass" {
		t.Errorf("tests = %q, want pass", rec.Verification.Tests)
	}
	if rec.Verification.Lint != "pass" {
		t.Errorf("lint = %q, want pass", rec.Verification.Lint)
	}
}

func TestBuildRunRecordGateFail(t *testing.T) {
	gate := &ecosystem.GateResult{Pass: false, Score: 0.3}
	rec := buildRunRecord("bead-2", "worker", "gate_fail", 60, gate, nil)

	if rec.Status != "done" {
		t.Errorf("status = %q, want done", rec.Status)
	}
	if rec.Verification.Tests != "fail" {
		t.Errorf("tests = %q, want fail", rec.Verification.Tests)
	}
	if rec.TemplateName != "custom" {
		t.Errorf("template = %q, want custom (no context)", rec.TemplateName)
	}
}

func TestBuildRunRecordTimeout(t *testing.T) {
	rec := buildRunRecord("bead-3", "worker", "timeout", 1800, nil, nil)

	if rec.Status != "timeout" {
		t.Errorf("status = %q, want timeout", rec.Status)
	}
	if rec.Verification.Tests != "skipped" {
		t.Errorf("tests = %q, want skipped (no gate)", rec.Verification.Tests)
	}
}

func TestBuildRunRecordError(t *testing.T) {
	rec := buildRunRecord("bead-4", "worker", "error", 30, nil, nil)

	if rec.Status != "failed" {
		t.Errorf("status = %q, want failed", rec.Status)
	}
	if rec.ExitCode != 1 {
		t.Errorf("exit_code = %d, want 1", rec.ExitCode)
	}
}
