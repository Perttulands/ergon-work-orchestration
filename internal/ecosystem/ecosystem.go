// Package ecosystem provides subprocess integration with gate, bd, and learning-loop.
// Every external dependency degrades gracefully if not on PATH.
package ecosystem

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// BeadCreateResult holds the result of creating a bead.
type BeadCreateResult struct {
	ID string
}

// GateResult holds the verdict from gate check.
type GateResult struct {
	Pass  bool    `json:"pass"`
	Score float64 `json:"score"`
	Raw   string  // raw output
}

// Available checks whether a tool is on PATH.
func Available(tool string) bool {
	_, err := exec.LookPath(tool)
	return err == nil
}

// BdCreate creates a new bead and returns its ID.
// Returns empty result with no error if bd is not available.
func BdCreate(title, repo string) (*BeadCreateResult, error) {
	if !Available("bd") {
		return nil, nil
	}

	args := []string{"q", title}
	cmd := exec.Command("bd", args...)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("bd create: %s: %w", strings.TrimSpace(string(out)), err)
	}

	id := strings.TrimSpace(string(out))
	return &BeadCreateResult{ID: id}, nil
}

// BdClose closes a bead with a reason.
func BdClose(id, reason, repo string) error {
	if !Available("bd") {
		return nil
	}

	args := []string{"close", id, "--reason", reason}
	cmd := exec.Command("bd", args...)
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("bd close %s: %s: %w", id, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// GateCheck runs gate check and returns the verdict.
// Returns nil with no error if gate is not available.
func GateCheck(repo, citizen string) (*GateResult, error) {
	if !Available("gate") {
		return nil, nil
	}

	args := []string{"check", "--json"}
	if citizen != "" {
		args = append(args, "--citizen", citizen)
	}
	cmd := exec.Command("gate", args...)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	raw := strings.TrimSpace(string(out))

	// gate may return non-zero for failures but still produce valid JSON
	var result GateResult
	if jsonErr := json.Unmarshal(out, &result); jsonErr != nil {
		// If JSON parse fails, create result from exit code
		result.Pass = err == nil
		result.Raw = raw
		return &result, nil
	}
	result.Raw = raw
	return &result, nil
}
