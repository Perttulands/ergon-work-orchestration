// Package ecosystem provides subprocess integration with gate, br, bv, and learning-loop.
// Every external dependency degrades gracefully if not on PATH.
package ecosystem

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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

// BrCreate creates a new bead and returns its ID.
// Returns empty result with no error if br is not available.
func BrCreate(title, repo string) (*BeadCreateResult, error) {
	if !Available("br") {
		return nil, nil
	}

	args := []string{"create", title, "--silent"}
	cmd := exec.Command("br", args...)
	cmd.Dir = repo
	// Use Output() to avoid stderr contamination from br's INFO logs.
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("br create: %s: %w", strings.TrimSpace(string(out)), err)
	}

	id := strings.TrimSpace(string(out))
	return &BeadCreateResult{ID: id}, nil
}

// BrClose closes a bead with a reason.
func BrClose(id, reason, repo string) error {
	if !Available("br") {
		return nil
	}

	args := []string{"close", id, "--reason", reason}
	cmd := exec.Command("br", args...)
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("br close %s: %s: %w", id, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// --- Relay + agent state integration ---

// BrAgentState sets the state of a br agent.
// br (beads_rust) does not currently support agent state tracking,
// so this is a no-op that degrades gracefully.
func BrAgentState(agent, state string) error {
	return nil
}

// RelayHeartbeat updates the heartbeat for an agent.
// Returns nil if relay is not available.
func RelayHeartbeat(agent string) error {
	if !Available("relay") {
		return nil
	}
	cmd := exec.Command("relay", "heartbeat", "--agent", agent)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("relay heartbeat %s: %s: %w", agent, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// RelaySend sends a message from one agent to another, optionally threaded.
// msgType and payload are optional — pass empty strings to omit.
// Returns nil if relay is not available.
func RelaySend(from, to, message, thread, msgType, payload string) error {
	if !Available("relay") {
		return nil
	}
	args := []string{"send", to, message, "--agent", from}
	if thread != "" {
		args = append(args, "--thread", thread)
	}
	if msgType != "" {
		args = append(args, "--type", msgType)
	}
	if payload != "" {
		args = append(args, "--payload", payload)
	}
	cmd := exec.Command("relay", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("relay send %s->%s: %s: %w", from, to, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// GateCheck runs gate check and returns the verdict.
// Returns nil with no error if gate is not available.
func GateCheck(repo, citizen string) (*GateResult, error) {
	if !Available("gate") {
		return nil, nil
	}

	args := []string{"check", ".", "--json"}
	if citizen != "" {
		args = append(args, "--citizen", citizen)
	}
	cmd := exec.Command("gate", args...)
	cmd.Dir = repo
	// Use Output() not CombinedOutput() — gate writes JSON to stdout only.
	// CombinedOutput() captures stderr too, where br's INFO logs leak through
	// gate's bead.Record calls, contaminating the JSON and breaking parsing.
	out, err := cmd.Output()
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
		// Graceful degradation: learning-loop dir exists but script is missing
		log.Printf("warning: select-template.sh not found: %v", err)
		return nil, nil
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

// QueryLearningLoop calls `loop query <task> --json` and returns structured results.
// Returns nil, nil on graceful degradation (loop not on PATH or no results).
func QueryLearningLoop(task string) ([]byte, error) {
	if !Available("loop") {
		return nil, nil
	}
	if task == "" {
		return nil, nil
	}
	cmd := exec.Command("loop", "query", task, "--json")
	out, err := cmd.Output()
	if err != nil {
		log.Printf("warning: loop query failed: %v", err)
		return nil, nil // graceful degradation — loop may not have data yet
	}
	return out, nil
}

// IngestRun feeds a completed work run to learning-loop for pattern extraction.
// Returns nil if loop is not on PATH.
func IngestRun(beadID, task, outcome, agent string, durationSec int64, testsPassed, lintPassed bool, filesChanged []string, errMsg string) error {
	if !Available("loop") {
		return nil
	}

	run := map[string]interface{}{
		"id":               beadID,
		"task":             task,
		"outcome":          outcome,
		"agent":            agent,
		"duration_seconds": durationSec,
		"tests_passed":     testsPassed,
		"lint_passed":      lintPassed,
		"files_touched":    filesChanged,
		"timestamp":        time.Now().UTC().Format(time.RFC3339),
	}
	if errMsg != "" {
		run["error_message"] = errMsg
	}

	data, err := json.Marshal(run)
	if err != nil {
		return fmt.Errorf("marshal run data for ingest: %w", err)
	}

	cmd := exec.Command("loop", "ingest", "-")
	cmd.Stdin = bytes.NewReader(data)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("loop ingest: %w", err)
	}
	return nil
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
		// Graceful degradation: learning-loop dir exists but script is missing
		log.Printf("warning: feedback-collector.sh not found: %v", err)
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
