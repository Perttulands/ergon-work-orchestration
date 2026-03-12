package runstate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateAndLoad(t *testing.T) {
	workDir := t.TempDir()
	store, err := Create(workDir, Config{
		BeadID:      "pol-abc",
		Task:        "resume test",
		Citizen:     "worker",
		Repo:        "/tmp/repo",
		Runtime:     "codex",
		Model:       "openai/gpt-5",
		TmuxSession: "work-pol-abc",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state.BeadID != "pol-abc" {
		t.Fatalf("bead = %q, want pol-abc", state.BeadID)
	}
	if state.Phase != PhaseInitialized {
		t.Fatalf("phase = %q, want %q", state.Phase, PhaseInitialized)
	}
	if state.RunID == "" || state.TraceID == "" || state.SessionID == "" {
		t.Fatal("expected generated ids")
	}
}

func TestCheckpointPromptAndEffects(t *testing.T) {
	workDir := t.TempDir()
	store, err := Create(workDir, Config{BeadID: "pol-cp"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.WritePrompt("hello"); err != nil {
		t.Fatalf("WritePrompt: %v", err)
	}
	if err := store.Checkpoint("context_gather", PhaseContextReady, "cp-1", "gathered"); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	if err := store.MarkEffect("relay_register", "effect-1", "registered"); err != nil {
		t.Fatalf("MarkEffect: %v", err)
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state.PromptPath == "" {
		t.Fatal("prompt path should be set")
	}
	if state.CompletedSteps["context_gather"].IdempotencyKey != "cp-1" {
		t.Fatalf("checkpoint key = %q, want cp-1", state.CompletedSteps["context_gather"].IdempotencyKey)
	}
	if state.CompletedEffects["relay_register"].IdempotencyKey != "effect-1" {
		t.Fatalf("effect key = %q, want effect-1", state.CompletedEffects["relay_register"].IdempotencyKey)
	}
	prompt, err := store.ReadPrompt()
	if err != nil || prompt != "hello" {
		t.Fatalf("ReadPrompt = %q, %v", prompt, err)
	}
}

func TestLatestUnfinishedByBead(t *testing.T) {
	workDir := t.TempDir()
	oldStore, err := Create(workDir, Config{BeadID: "pol-same"})
	if err != nil {
		t.Fatalf("Create old: %v", err)
	}
	if err := oldStore.Complete("success"); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	newStore, err := Create(workDir, Config{BeadID: "pol-same"})
	if err != nil {
		t.Fatalf("Create new: %v", err)
	}
	if err := newStore.Checkpoint("trace_open", PhaseTraceOpen, "", ""); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}

	store, state, err := LatestUnfinishedByBead(workDir, "pol-same")
	if err != nil {
		t.Fatalf("LatestUnfinishedByBead: %v", err)
	}
	if store.RunID() != newStore.RunID() {
		t.Fatalf("run id = %q, want %q", store.RunID(), newStore.RunID())
	}
	if state.Phase != PhaseTraceOpen {
		t.Fatalf("phase = %q, want %q", state.Phase, PhaseTraceOpen)
	}
}

func TestAcquireLeaseRejectsFreshLease(t *testing.T) {
	workDir := t.TempDir()
	store, err := Create(workDir, Config{BeadID: "pol-lease"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.AcquireLease("worker", time.Minute); err != nil {
		t.Fatalf("AcquireLease first: %v", err)
	}
	err = store.AcquireLease("worker-2", time.Minute)
	if !errors.Is(err, ErrFreshLease) {
		t.Fatalf("AcquireLease second error = %v, want ErrFreshLease", err)
	}
}

func TestAcquireLeaseAllowsExpiredLease(t *testing.T) {
	workDir := t.TempDir()
	store, err := Create(workDir, Config{BeadID: "pol-expired"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	expiredLease := []byte("{\"holder\":\"worker\",\"hostname\":\"host\",\"pid\":1,\"acquired_at\":\"2000-01-01T00:00:00Z\",\"expires_at\":\"2000-01-01T00:00:01Z\"}\n")
	if err := os.WriteFile(filepath.Join(store.RunDir(), "lease.json"), expiredLease, 0o644); err != nil {
		t.Fatalf("write expired lease: %v", err)
	}
	if err := store.AcquireLease("worker-2", time.Minute); err != nil {
		t.Fatalf("AcquireLease after expiry: %v", err)
	}
}
