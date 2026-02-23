package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Graceful degradation: Gather works even when nothing is available.
func TestGatherDegradesFully(t *testing.T) {
	cfg := Config{
		WorkDir:   t.TempDir(),
		BeadsRoot: t.TempDir(), // isolate from real bv data
	}
	result, err := Gather(cfg)
	if err != nil {
		t.Fatalf("gather should not error even with nothing available: %v", err)
	}
	if result.Markdown != "No prior context available. This is a fresh start." {
		t.Errorf("unexpected markdown for fully degraded: %q", result.Markdown)
	}
	if result.CitizenExperience != "" {
		t.Error("should have no citizen experience")
	}
	if result.LearningPatterns != "" {
		t.Error("should have no learning patterns")
	}
	if len(result.PastBeads) != 0 {
		t.Error("should have no past beads")
	}
}

// Graceful degradation: Gather with citizen file but no bd/loop.
func TestGatherWithCitizenButNoBdOrLoop(t *testing.T) {
	workDir := t.TempDir()
	citizenDir := filepath.Join(workDir, "citizens")
	os.MkdirAll(citizenDir, 0o755)
	os.WriteFile(filepath.Join(citizenDir, "degraded-citizen.md"), []byte("Prefer short PRs."), 0o644)

	// Isolate from real learning-loop scripts
	t.Setenv("LEARNING_LOOP_DIR", t.TempDir())

	cfg := Config{
		Citizen:   "degraded-citizen",
		Task:      "some task",
		Repo:      "/tmp",
		WorkDir:   workDir,
		BeadsRoot: t.TempDir(), // isolate from real bv data
	}
	result, err := Gather(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.CitizenExperience != "Prefer short PRs." {
		t.Errorf("expected citizen experience, got: %q", result.CitizenExperience)
	}
	if !strings.Contains(result.Markdown, "degraded-citizen's Experience Notes") {
		t.Error("markdown should contain citizen section")
	}
	if result.LearningPatterns != "" {
		t.Error("should have no learning patterns when learning-loop unavailable")
	}
}

// Graceful degradation: Gather with missing citizen file continues.
func TestGatherMissingCitizenFileOK(t *testing.T) {
	cfg := Config{
		Citizen:   "nonexistent-citizen",
		WorkDir:   t.TempDir(),
		BeadsRoot: t.TempDir(), // isolate from real bv data
	}
	result, err := Gather(cfg)
	if err != nil {
		t.Fatalf("should not error for missing citizen file: %v", err)
	}
	if result.CitizenExperience != "" {
		t.Error("should have empty citizen experience for missing file")
	}
}
