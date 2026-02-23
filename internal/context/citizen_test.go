package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendCitizenExperience(t *testing.T) {
	workDir := t.TempDir()

	err := AppendCitizenExperience(workDir, "zeus", "add JWT auth", "success", "work-abc")
	if err != nil {
		t.Fatalf("first append: %v", err)
	}

	// Append again
	err = AppendCitizenExperience(workDir, "zeus", "fix flaky test", "timeout", "work-def")
	if err != nil {
		t.Fatalf("second append: %v", err)
	}

	// Read and verify
	data, err := os.ReadFile(filepath.Join(workDir, "citizens", "zeus.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "add JWT auth") {
		t.Error("should contain first task")
	}
	if !strings.Contains(content, "fix flaky test") {
		t.Error("should contain second task")
	}
	if !strings.Contains(content, "work-abc") {
		t.Error("should contain first bead ID")
	}
	if !strings.Contains(content, "timeout") {
		t.Error("should contain timeout outcome")
	}
}

func TestAppendCitizenExperienceCreatesDir(t *testing.T) {
	workDir := filepath.Join(t.TempDir(), "deep", "nested")
	err := AppendCitizenExperience(workDir, "apollo", "task", "success", "work-123")
	if err != nil {
		t.Fatalf("should create nested dirs: %v", err)
	}
}
