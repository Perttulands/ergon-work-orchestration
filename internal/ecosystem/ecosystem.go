// Package ecosystem provides subprocess integration with gate, bd, bv, and learning-loop.
// Every external dependency degrades gracefully if not on PATH.
package ecosystem

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// BvSearchResult is one hit from bv --robot-search.
type BvSearchResult struct {
	IssueID string  `json:"issue_id"`
	Score   float64 `json:"score"`
	Title   string  `json:"title"`
}

// BvSearchResponse is the top-level JSON from bv --robot-search.
type BvSearchResponse struct {
	Results []BvSearchResult `json:"results"`
}

// BvRelatedItem is one related bead from bv --robot-related.
type BvRelatedItem struct {
	BeadID       string `json:"bead_id"`
	Title        string `json:"title"`
	Status       string `json:"status"`
	RelationType string `json:"relation_type"`
	Relevance    int    `json:"relevance"`
	Reason       string `json:"reason"`
}

// BvRelatedResponse is the top-level JSON from bv --robot-related.
type BvRelatedResponse struct {
	TargetBeadID string          `json:"target_bead_id"`
	TargetTitle  string          `json:"target_title"`
	Concurrent   []BvRelatedItem `json:"concurrent"`
	TotalRelated int             `json:"total_related"`
}

// BvPlanItem is one work item inside a plan track.
type BvPlanItem struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Priority int      `json:"priority"`
	Status   string   `json:"status"`
	Unblocks []string `json:"unblocks"`
}

// BvPlanTrack is one parallel execution track.
type BvPlanTrack struct {
	TrackID string       `json:"track_id"`
	Items   []BvPlanItem `json:"items"`
	Reason  string       `json:"reason"`
}

// BvPlanSummary is the high-level plan summary.
type BvPlanSummary struct {
	HighestImpact string `json:"highest_impact"`
	ImpactReason  string `json:"impact_reason"`
	UnblocksCount int    `json:"unblocks_count"`
}

// BvPlanResponse is the top-level JSON from bv --robot-plan.
type BvPlanResponse struct {
	Plan struct {
		Tracks          []BvPlanTrack `json:"tracks"`
		TotalActionable int           `json:"total_actionable"`
		TotalBlocked    int           `json:"total_blocked"`
		Summary         BvPlanSummary `json:"summary"`
	} `json:"plan"`
}

// BvSearch calls bv --robot-search --search "query" and returns matching beads.
// Returns nil, nil if bv is not available.
func BvSearch(query, beadsRoot string) (*BvSearchResponse, error) {
	if !Available("bv") {
		return nil, nil
	}
	if query == "" {
		return nil, nil
	}

	cmd := exec.Command("bv", "--robot-search", "--search", query)
	cmd.Dir = beadsRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bv search: %w", err)
	}

	var resp BvSearchResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse bv search: %w", err)
	}
	return &resp, nil
}

// BvRelated calls bv --robot-related <bead-id> and returns context on related beads.
// Returns nil, nil if bv is not available or beadID is empty.
func BvRelated(beadID, beadsRoot string) (*BvRelatedResponse, error) {
	if !Available("bv") {
		return nil, nil
	}
	if beadID == "" {
		return nil, nil
	}

	cmd := exec.Command("bv", "--robot-related", beadID)
	cmd.Dir = beadsRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bv related: %w", err)
	}

	var resp BvRelatedResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse bv related: %w", err)
	}
	return &resp, nil
}

// BvPlan calls bv --robot-plan and returns the execution plan.
// Returns nil, nil if bv is not available.
func BvPlan(beadsRoot string) (*BvPlanResponse, error) {
	if !Available("bv") {
		return nil, nil
	}

	cmd := exec.Command("bv", "--robot-plan")
	cmd.Dir = beadsRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bv plan: %w", err)
	}

	var resp BvPlanResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse bv plan: %w", err)
	}
	return &resp, nil
}

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

// --- Learning-loop integration ---

// TemplateSelection holds the recommendation from select-template.sh.
type TemplateSelection struct {
	Template   string   `json:"template"`
	Variant    *string  `json:"variant"`
	Agent      string   `json:"agent"`
	Model      string   `json:"model"`
	TaskType   string   `json:"task_type"`
	Score      float64  `json:"score"`
	Confidence string   `json:"confidence"`
	Reasoning  string   `json:"reasoning"`
	Warnings   []string `json:"warnings"`
}

// RunRecord is the input format expected by feedback-collector.sh.
type RunRecord struct {
	Bead            string       `json:"bead"`
	Agent           string       `json:"agent"`
	Model           string       `json:"model"`
	TemplateName    string       `json:"template_name"`
	Status          string       `json:"status"`
	ExitCode        int          `json:"exit_code"`
	FailureReason   string       `json:"failure_reason,omitempty"`
	DurationSeconds int64        `json:"duration_seconds"`
	Attempt         int          `json:"attempt"`
	PromptHash      string       `json:"prompt_hash,omitempty"`
	Verification    Verification `json:"verification"`
}

// Verification holds per-check signals for the run record.
type Verification struct {
	Tests      string `json:"tests"`
	Lint       string `json:"lint"`
	UBS        string `json:"ubs"`
	Truthsayer string `json:"truthsayer"`
}

// LearningLoopDir returns the learning-loop scripts directory.
// Checks LEARNING_LOOP_DIR env var, then falls back to well-known path.
// Returns empty string if not found.
func LearningLoopDir() string {
	if dir := os.Getenv("LEARNING_LOOP_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	candidate := filepath.Join(home, "tools", "learning-loop")
	if _, err := os.Stat(filepath.Join(candidate, "scripts", "select-template.sh")); err == nil {
		return candidate
	}
	return ""
}

// SelectTemplate calls select-template.sh to get a template recommendation.
// Returns nil with no error if learning-loop is not available.
func SelectTemplate(task string) (*TemplateSelection, error) {
	dir := LearningLoopDir()
	if dir == "" {
		return nil, nil
	}

	script := filepath.Join(dir, "scripts", "select-template.sh")
	if _, err := os.Stat(script); err != nil {
		return nil, nil // script doesn't exist, degrade gracefully
	}

	cmd := exec.Command("bash", script, task)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("select-template: %w", err)
	}

	var sel TemplateSelection
	if err := json.Unmarshal(out, &sel); err != nil {
		return nil, fmt.Errorf("parse select-template output: %w", err)
	}
	return &sel, nil
}

// CollectFeedback writes a run record and calls feedback-collector.sh.
// Returns nil if learning-loop is not available.
func CollectFeedback(record RunRecord, workDir string) error {
	dir := LearningLoopDir()
	if dir == "" {
		return nil
	}

	script := filepath.Join(dir, "scripts", "feedback-collector.sh")
	if _, err := os.Stat(script); err != nil {
		return nil
	}

	// Write run record to temp file
	recordDir := filepath.Join(workDir, "run-records")
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		return fmt.Errorf("create run-records dir: %w", err)
	}
	recordPath := filepath.Join(recordDir, record.Bead+".json")

	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal run record: %w", err)
	}
	if err := os.WriteFile(recordPath, data, 0o644); err != nil {
		return fmt.Errorf("write run record: %w", err)
	}

	cmd := exec.Command("bash", script, recordPath)
	cmd.Env = append(os.Environ(),
		"FEEDBACK_DIR="+filepath.Join(workDir, "feedback"),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("feedback-collector: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}
