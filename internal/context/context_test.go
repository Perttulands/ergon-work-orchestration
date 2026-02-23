package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadCitizenExperience(t *testing.T) {
	// Create temp work dir with citizen file
	workDir := t.TempDir()
	citizenDir := filepath.Join(workDir, "citizens")
	if err := os.MkdirAll(citizenDir, 0o755); err != nil {
		t.Fatal(err)
	}

	content := "I work best with clear specifications.\nPrefer small, focused commits."
	if err := os.WriteFile(filepath.Join(citizenDir, "mercury.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := readCitizenExperience(workDir, "mercury")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != content {
		t.Errorf("got %q, want %q", got, content)
	}
}

func TestReadCitizenExperienceMissing(t *testing.T) {
	workDir := t.TempDir()
	_, err := readCitizenExperience(workDir, "nobody")
	if err == nil {
		t.Error("expected error for missing citizen file")
	}
}

func TestFormatBeads(t *testing.T) {
	beads := []BeadResult{
		{ID: "work-abc", Status: "closed", Title: "Add JWT auth"},
		{ID: "work-def", Status: "closed", Title: "Fix flaky test"},
	}
	result := formatBeads(beads)
	if !strings.Contains(result, "## Past Work") {
		t.Error("missing header")
	}
	if !strings.Contains(result, "work-abc") {
		t.Error("missing first bead")
	}
	if !strings.Contains(result, "Fix flaky test") {
		t.Error("missing second bead title")
	}
}

func TestFormatCitizenExperience(t *testing.T) {
	result := formatCitizenExperience("zeus", "Always runs tests first.")
	if !strings.Contains(result, "## zeus's Experience Notes") {
		t.Error("missing citizen header")
	}
	if !strings.Contains(result, "Always runs tests first.") {
		t.Error("missing experience content")
	}
}

func TestFormatPatterns(t *testing.T) {
	result := formatPatterns("Use structured logging.")
	if !strings.Contains(result, "## Learned Patterns") {
		t.Error("missing patterns header")
	}
}

func TestGatherWithCitizenOnly(t *testing.T) {
	workDir := t.TempDir()
	citizenDir := filepath.Join(workDir, "citizens")
	if err := os.MkdirAll(citizenDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(citizenDir, "apollo.md"), []byte("Good with UI work."), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Citizen: "apollo",
		WorkDir: workDir,
	}
	result, err := Gather(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.CitizenExperience != "Good with UI work." {
		t.Errorf("citizen experience = %q, want %q", result.CitizenExperience, "Good with UI work.")
	}
	if !strings.Contains(result.Markdown, "apollo's Experience Notes") {
		t.Error("markdown should contain citizen section")
	}
}

func TestGatherNoContext(t *testing.T) {
	cfg := Config{
		WorkDir: t.TempDir(),
	}
	result, err := Gather(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Markdown != "No prior context available. This is a fresh start." {
		t.Errorf("unexpected markdown: %q", result.Markdown)
	}
}
