package context

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// AppendCitizenExperience appends a run outcome to a citizen's experience file.
func AppendCitizenExperience(workDir, citizen, task, outcome, beadID string) error {
	dir := filepath.Join(workDir, "citizens")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create citizen dir: %w", err)
	}

	path := filepath.Join(dir, citizen+".md")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open citizen file: %w", err)
	}
	defer f.Close()

	entry := fmt.Sprintf("\n## %s — %s\n- **Task:** %s\n- **Outcome:** %s\n",
		time.Now().Format("2006-01-02 15:04"), beadID, task, outcome)

	if _, err := f.WriteString(entry); err != nil {
		return fmt.Errorf("write citizen experience: %w", err)
	}
	return nil
}
