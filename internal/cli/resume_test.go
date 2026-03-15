package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"polis/work/internal/index"
	"polis/work/internal/runstate"
	"polis/work/internal/testutil"
	"polis/work/internal/trace"
)

func TestResumeCommandExists(t *testing.T) {
	root := NewRoot("test")
	for _, cmd := range root.Commands() {
		if cmd.Name() == "resume" {
			return
		}
	}
	t.Fatal("resume command should be registered")
}

func TestResumeCommandHandlesPreWorkerCrash(t *testing.T) {
	_, workDir := configureWorkHome(t)
	store, err := runstate.Create(workDir, runstate.Config{
		BeadID:      "pol-resume-precrash",
		BeadManaged: true,
		Task:        "pre-worker crash recovery test for resume functionality",
		Citizen:     "zeus",
		Repo:        t.TempDir(),
		Runtime:     "codex",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Checkpoint("context_gather", runstate.PhaseContextReady, "cp-1", "context ready"); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}

	// Pre-worker resume now calls runTask, which will fail (no tmux, no agent)
	// but it should NOT fail with "only post-worker recovery is enabled"
	root := NewRoot("test")
	root.SetArgs([]string{"resume", "pol-resume-precrash"})
	err = root.Execute()
	if err == nil {
		t.Fatal("resume should fail (no agent available), but should attempt the run")
	}
	if strings.Contains(err.Error(), "only post-worker recovery is enabled") {
		t.Fatalf("resume should not reject pre-worker phases anymore, got: %v", err)
	}
}

func TestResumeCommandCompletesManagedRunAndRepairsMissingTraceEnd(t *testing.T) {
	_, workDir := configureWorkHome(t)
	repo := t.TempDir()
	learningLoopDir := configureLearningLoop(t)
	t.Setenv("LEARNING_LOOP_DIR", learningLoopDir)

	gateLog := filepath.Join(t.TempDir(), "gate.log")
	relayLog := filepath.Join(t.TempDir(), "relay.log")
	brLog := filepath.Join(t.TempDir(), "br.log")
	loopLog := filepath.Join(t.TempDir(), "loop.log")
	loopPayload := filepath.Join(t.TempDir(), "loop.json")
	t.Setenv("MOCK_GATE_LOG", gateLog)
	t.Setenv("MOCK_RELAY_LOG", relayLog)
	t.Setenv("MOCK_BR_LOG", brLog)
	t.Setenv("MOCK_LOOP_LOG", loopLog)
	t.Setenv("MOCK_LOOP_PAYLOAD", loopPayload)

	testutil.SandboxPATH(t, map[string]string{
		"gate": `printf '%s\n' "$*" >> "$MOCK_GATE_LOG"
printf '{"pass":true,"score":0.93}\n'`,
		"relay": `printf '%s\n' "$*" >> "$MOCK_RELAY_LOG"`,
		"br":    `printf '%s\n' "$*" >> "$MOCK_BR_LOG"`,
		"loop": `printf '%s\n' "$*" >> "$MOCK_LOOP_LOG"
if [ "$1" = "ingest" ]; then
  cat > "$MOCK_LOOP_PAYLOAD"
fi`,
		"git": `if [ "$1" = "diff" ] && [ "$2" = "--stat" ]; then
  printf ' 1 file changed, 1 insertion(+)\n'
fi`,
	})

	store, state := seedResumeRun(t, workDir, repo, resumeSeedOptions{
		beadID:         "pol-resume-managed",
		managed:        true,
		outcome:        "success",
		includeEnd:     false,
		workerComplete: true,
	})

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"resume", state.BeadID})
	if err := root.Execute(); err != nil {
		t.Fatalf("resume failed: %v\noutput: %s", err, buf.String())
	}

	updated, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if updated.Phase != runstate.PhaseCompleted {
		t.Fatalf("phase = %q, want %q", updated.Phase, runstate.PhaseCompleted)
	}
	if updated.Attempt != 2 {
		t.Fatalf("attempt = %d, want 2", updated.Attempt)
	}
	if updated.CloseReason == "" {
		t.Fatal("close reason should be set")
	}
	if _, ok := updated.CompletedSteps["bead_close"]; !ok {
		t.Fatal("bead_close checkpoint should be recorded")
	}
	if _, ok := updated.CompletedEffects["br_close"]; !ok {
		t.Fatal("br_close effect should be recorded")
	}

	events, err := trace.ReadTrace(updated.TracePath)
	if err != nil {
		t.Fatalf("ReadTrace: %v", err)
	}
	if got := events[len(events)-1].EventType; got != "end" {
		t.Fatalf("last trace event = %q, want end", got)
	}

	idx, err := index.Open(workDir)
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	defer idx.Close()
	runs, err := idx.ByBead(updated.BeadID)
	if err != nil {
		t.Fatalf("index.ByBead: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("index runs = %d, want 1", len(runs))
	}

	assertNonEmptyLog(t, gateLog, "gate should run during resume")
	assertNonEmptyLog(t, brLog, "br close should run for managed beads")
	assertNonEmptyLog(t, relayLog, "relay send should run during resume")
	assertNonEmptyLog(t, loopLog, "loop ingest should run during resume")

	if _, err := os.Stat(filepath.Join(workDir, "run-records", updated.BeadID+".json")); err != nil {
		t.Fatalf("run record missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "citizens", updated.Citizen+".md")); err != nil {
		t.Fatalf("citizen experience missing: %v", err)
	}

	entries, err := store.ReadJournal()
	if err != nil {
		t.Fatalf("ReadJournal: %v", err)
	}
	if !journalHasKind(entries, "resume_started") {
		t.Fatal("journal should record resume_started")
	}
}

func TestResumeCommandForceStealsLeaseAndSkipsCommittedEffects(t *testing.T) {
	_, workDir := configureWorkHome(t)
	repo := t.TempDir()
	learningLoopDir := configureLearningLoop(t)
	t.Setenv("LEARNING_LOOP_DIR", learningLoopDir)

	gateLog := filepath.Join(t.TempDir(), "gate.log")
	relayLog := filepath.Join(t.TempDir(), "relay.log")
	brLog := filepath.Join(t.TempDir(), "br.log")
	loopLog := filepath.Join(t.TempDir(), "loop.log")
	t.Setenv("MOCK_GATE_LOG", gateLog)
	t.Setenv("MOCK_RELAY_LOG", relayLog)
	t.Setenv("MOCK_BR_LOG", brLog)
	t.Setenv("MOCK_LOOP_LOG", loopLog)

	testutil.SandboxPATH(t, map[string]string{
		"gate": `printf '%s\n' "$*" >> "$MOCK_GATE_LOG"
printf '{"pass":true,"score":0.88}\n'`,
		"relay": `printf '%s\n' "$*" >> "$MOCK_RELAY_LOG"`,
		"br":    `printf '%s\n' "$*" >> "$MOCK_BR_LOG"`,
		"loop": `printf '%s\n' "$*" >> "$MOCK_LOOP_LOG"
cat > /dev/null`,
		"git": `if [ "$1" = "diff" ] && [ "$2" = "--stat" ]; then
  printf ' 1 file changed, 1 insertion(+)\n'
fi`,
	})

	store, state := seedResumeRun(t, workDir, repo, resumeSeedOptions{
		beadID:         "work-local-resume",
		managed:        false,
		outcome:        "success",
		includeEnd:     true,
		workerComplete: true,
	})
	if err := store.Checkpoint("gate_check", runstate.PhaseGateChecked, checkpointKey(state.BeadID, "gate_check", 1), "success"); err != nil {
		t.Fatalf("Checkpoint gate_check: %v", err)
	}
	if err := store.MarkEffect("relay_send_athena", checkpointKey(state.BeadID, "relay_send_athena", 1), "already sent"); err != nil {
		t.Fatalf("MarkEffect relay_send_athena: %v", err)
	}
	if err := store.AcquireLease("other-worker", time.Hour); err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}

	root := NewRoot("test")
	root.SetArgs([]string{"resume", state.BeadID})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), runstate.ErrFreshLease.Error()) {
		t.Fatalf("resume without --force error = %v, want fresh lease", err)
	}

	root = NewRoot("test")
	root.SetArgs([]string{"resume", state.BeadID, "--force"})
	if err := root.Execute(); err != nil {
		t.Fatalf("resume --force failed: %v", err)
	}

	updated, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if updated.Phase != runstate.PhaseCompleted {
		t.Fatalf("phase = %q, want completed", updated.Phase)
	}
	if got := readLogLines(t, gateLog); len(got) != 0 {
		t.Fatalf("gate should be skipped when gate_check is already recorded, got %v", got)
	}
	if got := readLogLines(t, relayLog); len(got) != 0 {
		t.Fatalf("relay should not resend committed athena notification, got %v", got)
	}
	if got := readLogLines(t, brLog); len(got) != 0 {
		t.Fatalf("br close should be skipped for synthetic bead ids, got %v", got)
	}
	if got := readLogLines(t, loopLog); len(got) == 0 {
		t.Fatal("loop ingest should still run after force resume")
	}

	entries, err := store.ReadJournal()
	if err != nil {
		t.Fatalf("ReadJournal: %v", err)
	}
	if !journalHasKind(entries, "lease_stolen") {
		t.Fatal("journal should record lease_stolen")
	}
}

func TestResumeCommandSkipsRecloseForLegacyManagedBead(t *testing.T) {
	_, workDir := configureWorkHome(t)
	repo := t.TempDir()
	learningLoopDir := configureLearningLoop(t)
	t.Setenv("LEARNING_LOOP_DIR", learningLoopDir)

	brLog := filepath.Join(t.TempDir(), "br.log")
	t.Setenv("MOCK_BR_LOG", brLog)

	testutil.SandboxPATH(t, map[string]string{
		"br":   `printf '%s\n' "$*" >> "$MOCK_BR_LOG"`,
		"loop": `cat > /dev/null`,
		"git": `if [ "$1" = "diff" ] && [ "$2" = "--stat" ]; then
  printf ' 1 file changed, 1 insertion(+)\n'
fi`,
	})

	store, state := seedResumeRun(t, workDir, repo, resumeSeedOptions{
		beadID:         "pol-legacy-resume",
		managed:        false,
		outcome:        "success",
		includeEnd:     true,
		workerComplete: true,
	})
	if err := store.MarkEffect("br_create", checkpointKey(state.BeadID, "br_create", 1), "legacy managed bead"); err != nil {
		t.Fatalf("MarkEffect br_create: %v", err)
	}
	if err := store.MarkEffect("br_close", checkpointKey(state.BeadID, "br_close", 1), "already closed before crash"); err != nil {
		t.Fatalf("MarkEffect br_close: %v", err)
	}

	root := NewRoot("test")
	root.SetArgs([]string{"resume", state.BeadID})
	if err := root.Execute(); err != nil {
		t.Fatalf("resume failed: %v", err)
	}

	updated, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := updated.CompletedSteps["bead_close"]; !ok {
		t.Fatal("bead_close checkpoint should be recorded for legacy managed bead")
	}
	if got := readLogLines(t, brLog); len(got) != 0 {
		t.Fatalf("br close should not rerun when br_close effect is already recorded, got %v", got)
	}
}

func TestResumeCommandKeepsEndEventLastWhenGateRunsAfterClosedTrace(t *testing.T) {
	_, workDir := configureWorkHome(t)
	repo := t.TempDir()

	gateLog := filepath.Join(t.TempDir(), "gate.log")
	t.Setenv("MOCK_GATE_LOG", gateLog)

	testutil.SandboxPATH(t, map[string]string{
		"gate": `printf '%s\n' "$*" >> "$MOCK_GATE_LOG"
printf '{"pass":true,"score":0.91}\n'`,
		"git": `if [ "$1" = "diff" ] && [ "$2" = "--stat" ]; then
  printf ' 1 file changed, 1 insertion(+)\n'
fi`,
	})

	store, state := seedResumeRun(t, workDir, repo, resumeSeedOptions{
		beadID:         "work-end-order",
		managed:        false,
		outcome:        "success",
		includeEnd:     true,
		workerComplete: true,
	})

	root := NewRoot("test")
	root.SetArgs([]string{"resume", state.BeadID})
	if err := root.Execute(); err != nil {
		t.Fatalf("resume failed: %v", err)
	}

	updated, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	events, err := trace.ReadTrace(updated.TracePath)
	if err != nil {
		t.Fatalf("ReadTrace: %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}
	if got := events[len(events)-1].EventType; got != "end" {
		t.Fatalf("last event = %q, want end", got)
	}
	if got := events[len(events)-2].EventType; got != "gate_result" {
		t.Fatalf("penultimate event = %q, want gate_result", got)
	}
	if got := readLogLines(t, gateLog); len(got) == 0 {
		t.Fatal("gate should run when gate_check checkpoint is missing")
	}
}

type resumeSeedOptions struct {
	beadID         string
	managed        bool
	outcome        string
	includeEnd     bool
	workerComplete bool
}

func configureWorkHome(t *testing.T) (string, string) {
	t.Helper()
	homeDir := t.TempDir()
	workDir := filepath.Join(homeDir, ".work")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("mkdir work dir: %v", err)
	}
	t.Setenv("HOME", homeDir)
	return homeDir, workDir
}

func configureLearningLoop(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	scriptsDir := filepath.Join(dir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("mkdir scripts dir: %v", err)
	}
	script := filepath.Join(scriptsDir, "feedback-collector.sh")
	body := "#!/bin/sh\nset -e\nmkdir -p \"$FEEDBACK_DIR\"\ncp \"$1\" \"$FEEDBACK_DIR/$(basename \"$1\")\"\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write feedback collector: %v", err)
	}
	return dir
}

func seedResumeRun(t *testing.T, workDir, repo string, opts resumeSeedOptions) (*runstate.Store, runstate.State) {
	t.Helper()
	store, err := runstate.Create(workDir, runstate.Config{
		BeadID:      opts.beadID,
		BeadManaged: opts.managed,
		Task:        "resume seeded task",
		Citizen:     "zeus",
		Repo:        repo,
		Runtime:     "codex",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	tr, err := trace.OpenWithOptions(workDir, state.BeadID, state.Citizen, state.Task, trace.OpenOptions{
		Repo:      repo,
		RunID:     state.RunID,
		TraceID:   state.TraceID,
		SessionID: state.SessionID,
	})
	if err != nil {
		t.Fatalf("OpenWithOptions: %v", err)
	}
	if err := tr.EmitToolCall("bash", "go test ./...", 25); err != nil {
		t.Fatalf("EmitToolCall: %v", err)
	}
	if err := tr.EmitFileWrite("main.go", 12); err != nil {
		t.Fatalf("EmitFileWrite: %v", err)
	}
	if err := tr.Close(opts.outcome, nil); err != nil {
		t.Fatalf("Close trace: %v", err)
	}
	if !opts.includeEnd {
		removeTraceEnd(t, tr.FilePath())
	}

	if err := store.SetTracePath(tr.FilePath()); err != nil {
		t.Fatalf("SetTracePath: %v", err)
	}
	if err := store.SetOutcome(opts.outcome); err != nil {
		t.Fatalf("SetOutcome: %v", err)
	}
	if opts.workerComplete {
		if err := store.Checkpoint("worker_completed", runstate.PhaseWorkerCompleted, checkpointKey(state.BeadID, "worker_completed", 1), opts.outcome); err != nil {
			t.Fatalf("Checkpoint worker_completed: %v", err)
		}
	}

	state, err = store.Load()
	if err != nil {
		t.Fatalf("Load seeded state: %v", err)
	}
	return store, state
}

func removeTraceEnd(t *testing.T, tracePath string) {
	t.Helper()
	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 {
		t.Fatalf("trace should contain at least begin/end events, got %d lines", len(lines))
	}
	trimmed := strings.Join(lines[:len(lines)-1], "\n") + "\n"
	if err := os.WriteFile(tracePath, []byte(trimmed), 0o644); err != nil {
		t.Fatalf("rewrite trace without end: %v", err)
	}
}

func assertNonEmptyLog(t *testing.T, path, message string) {
	t.Helper()
	if got := readLogLines(t, path); len(got) == 0 {
		t.Fatal(message)
	}
}

func readLogLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read log %s: %v", path, err)
	}
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func journalHasKind(entries []runstate.JournalEntry, kind string) bool {
	for _, entry := range entries {
		if entry.Kind == kind {
			return true
		}
	}
	return false
}

// --- pol-04e3: crash recovery tests ---

// TestResumeCommandMidWorkerCrash simulates a crash during the worker phase
// (phase=worker_running, no worker_completed checkpoint). Resume should detect
// this as a pre-worker crash and re-run the task from scratch.
func TestResumeCommandMidWorkerCrash(t *testing.T) {
	_, workDir := configureWorkHome(t)
	store, err := runstate.Create(workDir, runstate.Config{
		BeadID:      "pol-midcrash-01",
		BeadManaged: true,
		Task:        "mid-worker crash recovery test for resume",
		Citizen:     "zeus",
		Repo:        t.TempDir(),
		Runtime:     "codex",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Simulate: bead created → context ready → worker running → crash
	if err := store.Checkpoint("bead_create", runstate.PhaseBeadCreated, "ck-1", "bead created"); err != nil {
		t.Fatalf("Checkpoint bead_create: %v", err)
	}
	if err := store.Checkpoint("context_gather", runstate.PhaseContextReady, "ck-2", "context ready"); err != nil {
		t.Fatalf("Checkpoint context_gather: %v", err)
	}
	if err := store.Checkpoint("worker_spawn", runstate.PhaseWorkerRunning, "ck-3", "worker spawned"); err != nil {
		t.Fatalf("Checkpoint worker_spawn: %v", err)
	}

	// Resume should detect pre-worker crash (no worker_completed checkpoint)
	// and attempt to re-run the task via runTask, which will fail (no tmux/agent)
	// but the key assertion is that it doesn't reject the phase.
	root := NewRoot("test")
	root.SetArgs([]string{"resume", "pol-midcrash-01"})
	err = root.Execute()
	if err == nil {
		t.Fatal("resume should fail (no agent), but should attempt the run")
	}
	// Should NOT say "only post-worker recovery" or "not supported"
	if strings.Contains(err.Error(), "not supported") {
		t.Fatalf("resume should accept worker_running phase, got: %v", err)
	}

	// Verify state: attempt should have been incremented by BeginResume
	state, loadErr := store.Load()
	if loadErr != nil {
		t.Fatalf("Load: %v", loadErr)
	}
	if state.Attempt < 2 {
		t.Fatalf("attempt = %d, want >= 2 (BeginResume should increment)", state.Attempt)
	}
}

// TestResumeCommandNoRunState verifies that resuming a bead with no prior
// run state produces a clear error.
func TestResumeCommandNoRunState(t *testing.T) {
	configureWorkHome(t)

	root := NewRoot("test")
	root.SetArgs([]string{"resume", "pol-nonexistent-99"})
	err := root.Execute()
	if err == nil {
		t.Fatal("resume should fail when no run state exists")
	}
	if !strings.Contains(err.Error(), "unfinished") && !strings.Contains(err.Error(), "no run") && !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error should indicate no run state found, got: %v", err)
	}
}

// TestResumeCommandAlreadyCompleted verifies that resuming a bead whose
// latest run is already completed produces a clear error (not silently
// re-running or corrupting state).
func TestResumeCommandAlreadyCompleted(t *testing.T) {
	_, workDir := configureWorkHome(t)
	store, err := runstate.Create(workDir, runstate.Config{
		BeadID:      "pol-alreadydone-01",
		BeadManaged: true,
		Task:        "already completed run should not be resumable",
		Citizen:     "zeus",
		Repo:        t.TempDir(),
		Runtime:     "codex",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Mark as completed
	if err := store.Complete("success"); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	root := NewRoot("test")
	root.SetArgs([]string{"resume", "pol-alreadydone-01"})
	err = root.Execute()
	if err == nil {
		t.Fatal("resume should fail for already completed run")
	}
	if !strings.Contains(err.Error(), "unfinished") && !strings.Contains(err.Error(), "completed") && !strings.Contains(err.Error(), "no run") && !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error should indicate run is completed, got: %v", err)
	}
}
