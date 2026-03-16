package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"polis/work/internal/beadlint"
	workctx "polis/work/internal/context"
	"polis/work/internal/ecosystem"
	"polis/work/internal/testutil"

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
	flags := []string{"repo", "citizen", "runtime", "deadline", "notify"}
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
	rec := buildRunRecord("bead-1", "mercury", "codex", "success", 120, gate, ctx)

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
	rec := buildRunRecord("bead-2", "worker", "codex", "gate_fail", 60, gate, nil)

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
	rec := buildRunRecord("bead-3", "worker", "codex", "timeout", 1800, nil, nil)

	if rec.Status != "timeout" {
		t.Errorf("status = %q, want timeout", rec.Status)
	}
	if rec.Verification.Tests != "skipped" {
		t.Errorf("tests = %q, want skipped (no gate)", rec.Verification.Tests)
	}
}

func TestBuildRunRecordError(t *testing.T) {
	rec := buildRunRecord("bead-4", "worker", "codex", "error", 30, nil, nil)

	if rec.Status != "failed" {
		t.Errorf("status = %q, want failed", rec.Status)
	}
	if rec.ExitCode != 1 {
		t.Errorf("exit_code = %d, want 1", rec.ExitCode)
	}
}

func TestRunTaskOrchestration(t *testing.T) {
	// Mock tmux that simulates the full worker lifecycle:
	// - new-session: succeed
	// - send-keys: succeed
	// - capture-pane: return "Claude Code v1.0" banner + idle prompt (ready + done)
	// - has-session: succeed on first call, fail on subsequent (session cleaned up)
	// - kill-session: succeed
	// - load-buffer / paste-buffer: succeed
	tmuxScript := `
case "$1" in
  new-session)  exit 0 ;;
  send-keys)    exit 0 ;;
  capture-pane) printf "Welcome to Claude Code v1.0\nTask complete. All tests pass.\n❯\n" ; exit 0 ;;
  has-session)  exit 1 ;;
  kill-session) exit 0 ;;
  load-buffer)  exit 0 ;;
  paste-buffer) exit 0 ;;
  *)            exit 0 ;;
esac
`
	testutil.SandboxPATH(t, map[string]string{
		"tmux": tmuxScript,
		"br":   `echo "test-bead-001"`,
		"gate": `echo '{"pass":true,"score":0.95}'`,
		"git":  `echo "abc1234 initial commit"`,
	})

	// Set up work directory
	workDir := t.TempDir()
	homeDir := filepath.Dir(workDir)
	dotWork := filepath.Join(homeDir, ".work")
	os.Symlink(workDir, dotWork)
	t.Cleanup(func() { os.Remove(dotWork) })
	t.Setenv("HOME", homeDir)

	repoDir := t.TempDir()

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{
		"run", "add unit tests for auth module",
		"--repo", repoDir,
		"--citizen", "test-worker",
		"--deadline", "10s",
	})

	err := root.Execute()
	out := buf.String()

	// runTask returns spawnErr; with our mock tmux it should succeed
	if err != nil {
		t.Fatalf("run failed: %v\noutput: %s", err, out)
	}

	// Verify key orchestration outputs
	if !strings.Contains(out, "Starting: add unit tests for auth module") {
		t.Error("should print task name")
	}
	if !strings.Contains(out, "Citizen: test-worker") {
		t.Error("should print citizen name")
	}
	if !strings.Contains(out, "Spawning worker") {
		t.Error("should mention spawning worker")
	}
	if !strings.Contains(out, "Done:") {
		t.Error("should print Done: on completion")
	}
}

// TestRunTaskTraceRecordedOnSpawnError verifies that when worker.Spawn fails,
// the trace file still gets a proper "end" event with the error. Without this,
// debugging agent failures is impossible — the trace just ends with "begin"
// and no indication of what went wrong.
func TestRunTaskTraceRecordedOnSpawnError(t *testing.T) {
	// Mock tmux that fails at create-session (simulates tmux down)
	tmuxScript := `
case "$1" in
  new-session)  echo "server not found" >&2; exit 1 ;;
  send-keys)    exit 1 ;;
  capture-pane) exit 1 ;;
  has-session)  exit 1 ;;
  kill-session) exit 0 ;;
  load-buffer)  exit 0 ;;
  paste-buffer) exit 0 ;;
  *)            exit 0 ;;
esac
`
	testutil.SandboxPATH(t, map[string]string{
		"tmux": tmuxScript,
		"git":  `echo "abc1234 commit"`,
	})

	// Set up isolated HOME with .work directory
	homeDir := t.TempDir()
	dotWork := filepath.Join(homeDir, ".work")
	os.MkdirAll(dotWork, 0o755)
	t.Setenv("HOME", homeDir)

	repoDir := t.TempDir()

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{
		"run", "handle failing task when tmux unavailable",
		"--repo", repoDir,
		"--citizen", "test-worker",
		"--deadline", "5s",
	})

	err := root.Execute()
	out := buf.String()

	// runTask returns spawnErr — should be non-nil
	if err == nil {
		t.Fatal("expected error when tmux fails")
	}

	// The output should still show the error
	if !strings.Contains(out, "Worker error:") {
		t.Error("should report worker error")
	}

	// Trace should exist with an end event that has the error.
	// Traces are stored under traces/YYYY/MM/DD/, so walk to find the file.
	tracesDir := filepath.Join(dotWork, "traces")
	var traceFile string
	filepath.Walk(tracesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".jsonl" {
			traceFile = path
		}
		return nil
	})
	if traceFile == "" {
		t.Fatal("expected a trace .jsonl file to be written")
	}

	// Read the trace and verify it has begin + end events
	traceData, _ := os.ReadFile(traceFile)
	traceStr := string(traceData)
	if !strings.Contains(traceStr, `"event":"begin"`) {
		t.Error("trace should have begin event")
	}
	if !strings.Contains(traceStr, `"event":"end"`) {
		t.Error("trace should have end event even on spawn failure")
	}
	if !strings.Contains(traceStr, `"outcome":"error"`) {
		t.Error("trace end event should record the error outcome")
	}
}

// TestRunTaskGateFailOutcome verifies that when gate check returns fail,
// the outcome is set to "gate_fail" and the trace contains the gate result.
// This is critical for the learning-loop: misclassified outcomes corrupt
// the pattern database.
func TestRunTaskGateFailOutcome(t *testing.T) {
	tmuxScript := `
case "$1" in
  new-session)  exit 0 ;;
  send-keys)    exit 0 ;;
  capture-pane) printf "Claude Code v1.0\nDone.\n❯\n" ; exit 0 ;;
  has-session)  exit 1 ;;
  kill-session) exit 0 ;;
  load-buffer)  exit 0 ;;
  paste-buffer) exit 0 ;;
  *)            exit 0 ;;
esac
`
	testutil.SandboxPATH(t, map[string]string{
		"tmux": tmuxScript,
		"br":   `echo "gate-fail-bead"`,
		"gate": `printf '{"pass":false,"score":0.25}'; exit 1`,
		"git":  `echo "abc1234 commit"`,
	})

	homeDir := t.TempDir()
	dotWork := filepath.Join(homeDir, ".work")
	os.MkdirAll(dotWork, 0o755)
	t.Setenv("HOME", homeDir)

	repoDir := t.TempDir()

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{
		"run", "sloppy task with bad gate outcome",
		"--repo", repoDir,
		"--citizen", "test-worker",
		"--deadline", "5s",
	})

	err := root.Execute()
	out := buf.String()

	if err != nil {
		t.Fatalf("run should succeed even with gate fail: %v\noutput: %s", err, out)
	}

	// Verify gate failure is reported
	if !strings.Contains(out, "Gate: FAIL") {
		t.Error("should report gate failure")
	}
	// Outcome should be gate_fail, not success
	if !strings.Contains(out, "gate_fail") {
		t.Error("outcome should be gate_fail")
	}
}

func TestRunTaskBeadFreeMode(t *testing.T) {
	// Test orchestration when br is not on PATH (bead-free mode).
	// tmux mock still needed for worker spawn.
	tmuxScript := `
case "$1" in
  new-session)  exit 0 ;;
  send-keys)    exit 0 ;;
  capture-pane) printf "Claude Code v1.0\nDone.\n❯\n" ; exit 0 ;;
  has-session)  exit 1 ;;
  kill-session) exit 0 ;;
  load-buffer)  exit 0 ;;
  paste-buffer) exit 0 ;;
  *)            exit 0 ;;
esac
`
	testutil.SandboxPATH(t, map[string]string{
		"tmux": tmuxScript,
		"git":  `echo "deadbeef commit msg"`,
	})
	t.Setenv("WORK_STRICT", "0") // bead-free mode requires relaxed: br commands are absent by design

	workDir := t.TempDir()
	homeDir := filepath.Dir(workDir)
	dotWork := filepath.Join(homeDir, ".work")
	os.Symlink(workDir, dotWork)
	t.Cleanup(func() { os.Remove(dotWork) })
	t.Setenv("HOME", homeDir)

	repoDir := t.TempDir()

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{
		"run", "fix authentication bug in login flow",
		"--repo", repoDir,
		"--citizen", "solo-worker",
		"--deadline", "5s",
	})

	err := root.Execute()
	out := buf.String()

	if err != nil {
		t.Fatalf("run in bead-free mode failed: %v\noutput: %s", err, out)
	}

	// Should warn about missing tools
	if !strings.Contains(out, "bead-free mode") {
		t.Error("should warn about bead-free mode when br is missing")
	}
	if !strings.Contains(out, "Done:") {
		t.Error("should complete even in bead-free mode")
	}
}

// --- Bead lint integration tests ---

func TestRunTaskRejectsShortTitle(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{
		"git": `echo "abc1234 commit"`,
	})

	homeDir := t.TempDir()
	os.MkdirAll(filepath.Join(homeDir, ".work"), 0o755)
	t.Setenv("HOME", homeDir)

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{
		"run", "fix bug",
		"--repo", t.TempDir(),
		"--citizen", "test-worker",
		"--deadline", "5s",
	})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for short task title")
	}
	if !strings.Contains(err.Error(), "quality lint") {
		t.Errorf("error should mention lint: %v", err)
	}
}

func TestRunTaskAcceptsLongTitle(t *testing.T) {
	// Verify that a well-formed title passes lint
	// (test exits at worker spawn, not at lint)
	tmuxScript := `
case "$1" in
  new-session)  exit 0 ;;
  send-keys)    exit 0 ;;
  capture-pane) printf "Claude Code v1.0\nDone.\n❯\n" ; exit 0 ;;
  has-session)  exit 1 ;;
  kill-session) exit 0 ;;
  load-buffer)  exit 0 ;;
  paste-buffer) exit 0 ;;
  *)            exit 0 ;;
esac
`
	testutil.SandboxPATH(t, map[string]string{
		"tmux": tmuxScript,
		"br":   `echo "lint-pass-bead"`,
		"gate": `echo '{"pass":true,"score":0.9}'`,
		"git":  `echo "abc1234 commit"`,
	})

	homeDir := t.TempDir()
	os.MkdirAll(filepath.Join(homeDir, ".work"), 0o755)
	t.Setenv("HOME", homeDir)

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{
		"run", "add comprehensive unit tests for authentication module",
		"--repo", t.TempDir(),
		"--citizen", "test-worker",
		"--deadline", "5s",
	})

	err := root.Execute()
	if err != nil {
		t.Fatalf("well-formed task should pass lint: %v", err)
	}
}

func TestRunTaskRejectsBeadWithMissingFields(t *testing.T) {
	// Mock br show returning a bead with missing fields
	brScript := `
case "$1" in
  show) echo '[{"id":"pol-bad1","title":"fix it","description":"short","issue_type":"story","priority":0}]' ;;
  create) echo "pol-bad1" ;;
  *) exit 0 ;;
esac
`
	testutil.SandboxPATH(t, map[string]string{
		"br":  brScript,
		"git": `echo "abc1234 commit"`,
	})

	homeDir := t.TempDir()
	os.MkdirAll(filepath.Join(homeDir, ".work"), 0o755)
	t.Setenv("HOME", homeDir)

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{
		"run", "pol-bad1",
		"--repo", t.TempDir(),
		"--citizen", "test-worker",
		"--deadline", "5s",
	})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for bad bead")
	}
	out := buf.String()
	if !strings.Contains(out, "failed lint") {
		t.Errorf("output should mention lint failure: %s", out)
	}
}

func TestRunTaskPassesBeadWithGoodFields(t *testing.T) {
	brScript := `
case "$1" in
  show) echo '[{"id":"pol-good","title":"enforce minimum bead quality before dispatch","description":"A detailed description of what needs to happen with enough context.","issue_type":"feature","priority":2}]' ;;
  create) echo "pol-good" ;;
  close) exit 0 ;;
  *) exit 0 ;;
esac
`
	tmuxScript := `
case "$1" in
  new-session)  exit 0 ;;
  send-keys)    exit 0 ;;
  capture-pane) printf "Claude Code v1.0\nDone.\n❯\n" ; exit 0 ;;
  has-session)  exit 1 ;;
  kill-session) exit 0 ;;
  load-buffer)  exit 0 ;;
  paste-buffer) exit 0 ;;
  *)            exit 0 ;;
esac
`
	testutil.SandboxPATH(t, map[string]string{
		"br":   brScript,
		"tmux": tmuxScript,
		"gate": `echo '{"pass":true,"score":0.9}'`,
		"git":  `echo "abc1234 commit"`,
	})

	homeDir := t.TempDir()
	os.MkdirAll(filepath.Join(homeDir, ".work"), 0o755)
	t.Setenv("HOME", homeDir)

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{
		"run", "pol-good",
		"--repo", t.TempDir(),
		"--citizen", "test-worker",
		"--deadline", "5s",
	})

	err := root.Execute()
	if err != nil {
		t.Fatalf("well-formed bead should pass lint: %v\noutput: %s", err, buf.String())
	}
}

func TestRunTaskPassesNonPolBeadWithGoodFields(t *testing.T) {
	brScript := `
case "$1" in
  show) echo '[{"id":"relay-good","title":"enforce minimum bead quality before dispatch","description":"A detailed description of what needs to happen with enough context.","issue_type":"feature","priority":2}]' ;;
  create) echo "relay-good" ;;
  close) exit 0 ;;
  *) exit 0 ;;
esac
`
	tmuxScript := `
case "$1" in
  new-session)  exit 0 ;;
  send-keys)    exit 0 ;;
  capture-pane) printf "Claude Code v1.0\nDone.\n❯\n" ; exit 0 ;;
  has-session)  exit 1 ;;
  kill-session) exit 0 ;;
  load-buffer)  exit 0 ;;
  paste-buffer) exit 0 ;;
  *)            exit 0 ;;
esac
`
	testutil.SandboxPATH(t, map[string]string{
		"br":   brScript,
		"tmux": tmuxScript,
		"gate": `echo '{"pass":true,"score":0.9}'`,
		"git":  `echo "abc1234 commit"`,
	})

	homeDir := t.TempDir()
	os.MkdirAll(filepath.Join(homeDir, ".work"), 0o755)
	t.Setenv("HOME", homeDir)

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{
		"run", "relay-good",
		"--repo", t.TempDir(),
		"--citizen", "test-worker",
		"--deadline", "5s",
	})

	err := root.Execute()
	if err != nil {
		t.Fatalf("well-formed non-pol bead should pass lint: %v\noutput: %s", err, buf.String())
	}
}

// --- beadlint package integration ---

func TestLintBeforeDispatch_ShortTitle(t *testing.T) {
	issues := beadlint.LintTitle("fix bug")
	if !beadlint.HasErrors(issues) {
		t.Fatal("2-word title should fail lint")
	}
}

func TestLintBeforeDispatch_ValidTitle(t *testing.T) {
	issues := beadlint.LintTitle("add comprehensive unit tests for authentication")
	if beadlint.HasErrors(issues) {
		t.Errorf("good title should pass lint: %s", beadlint.FormatIssues(issues))
	}
}

func TestLintBeforeDispatch_BeadAllBad(t *testing.T) {
	issues := beadlint.LintBead(beadlint.Bead{
		ID:          "pol-bad",
		Title:       "fix",
		Description: "x",
		Type:        "unknown",
		Priority:    0,
	})
	errors := 0
	for _, i := range issues {
		if i.Severity == beadlint.Error {
			errors++
		}
	}
	if errors < 4 {
		t.Errorf("expected >= 4 errors, got %d: %s", errors, beadlint.FormatIssues(issues))
	}
}
