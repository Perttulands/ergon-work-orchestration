// Package ecosystem provides subprocess integration with gate, br, bv, and learning-loop.
// Every external dependency degrades gracefully if not on PATH.
package ecosystem

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	beadsadapter "polis/work/internal/adapters/beads"
	gateadapter "polis/work/internal/adapters/gate"
	relayadapter "polis/work/internal/adapters/relay"
	"polis/work/internal/loopfeed"
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

// --- br triage/related/plan (v2) with bv fallback ---

// brTriageResult matches the JSON shape from `br triage --search`.
type brTriageResult struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Status   string  `json:"status"`
	Priority int     `json:"priority"`
	Score    float64 `json:"score"`
}

// brTriageResponse matches the JSON shape from `br triage --search`.
type brTriageResponse struct {
	Query   string           `json:"query"`
	Results []brTriageResult `json:"results"`
	Total   int              `json:"total"`
}

// brRelatedItem matches the JSON shape from `br related`.
type brRelatedItem struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Relationship string  `json:"relationship"`
	Strength     float64 `json:"strength"`
}

// brRelatedResponse matches the JSON shape from `br related`.
type brRelatedResponse struct {
	TargetBeadID string          `json:"target_bead_id"`
	Related      []brRelatedItem `json:"related"`
	TotalRelated int             `json:"total_related"`
}

// brPlanResponse matches the JSON shape from `br plan`.
type brPlanResponse = BvPlanResponse // identical structure

// BrTriage calls `br triage --search <query> --json` and returns results
// in the BvSearchResponse format. Returns nil, nil if br is not available.
func BrTriage(query, beadsRoot string) (*BvSearchResponse, error) {
	if !beadsadapter.Available() {
		return nil, nil
	}
	if query == "" {
		return nil, nil
	}

	cmd := exec.Command("br", "triage", "--search", query, "--json")
	cmd.Env = brEnv(beadsRoot)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("br triage: %w", err)
	}

	var resp brTriageResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse br triage: %w", err)
	}

	// Convert to BvSearchResponse
	results := make([]BvSearchResult, len(resp.Results))
	for i, r := range resp.Results {
		results[i] = BvSearchResult{
			IssueID: r.ID,
			Score:   r.Score,
			Title:   r.Title,
		}
	}
	return &BvSearchResponse{Results: results}, nil
}

// BrRelated calls `br related <bead-id> --json` and returns results
// in the BvRelatedResponse format. Returns nil, nil if br is not available.
func BrRelated(beadID, beadsRoot string) (*BvRelatedResponse, error) {
	if !beadsadapter.Available() {
		return nil, nil
	}
	if beadID == "" {
		return nil, nil
	}

	cmd := exec.Command("br", "related", beadID, "--json")
	cmd.Env = brEnv(beadsRoot)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("br related: %w", err)
	}

	var resp brRelatedResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse br related: %w", err)
	}

	// Convert to BvRelatedResponse
	items := make([]BvRelatedItem, len(resp.Related))
	for i, r := range resp.Related {
		items[i] = BvRelatedItem{
			BeadID:       r.ID,
			Title:        r.Title,
			Status:       "",
			RelationType: r.Relationship,
			Relevance:    int(r.Strength * 10),
			Reason:       r.Relationship,
		}
	}
	return &BvRelatedResponse{
		TargetBeadID: resp.TargetBeadID,
		Concurrent:   items,
		TotalRelated: resp.TotalRelated,
	}, nil
}

// BrPlan calls `br plan --json` and returns results
// in the BvPlanResponse format. Returns nil, nil if br is not available.
func BrPlan(beadsRoot string) (*BvPlanResponse, error) {
	if !beadsadapter.Available() {
		return nil, nil
	}

	cmd := exec.Command("br", "plan", "--json")
	cmd.Env = brEnv(beadsRoot)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("br plan: %w", err)
	}

	var resp BvPlanResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse br plan: %w", err)
	}
	return &resp, nil
}

// brEnv returns the environment for br subcommands, setting BEADS_DIR if beadsRoot is provided.
func brEnv(beadsRoot string) []string {
	env := os.Environ()
	if beadsRoot != "" {
		env = append(env, "BEADS_DIR="+beadsRoot)
	}
	return env
}

// BvSearch calls br triage first, falling back to bv --robot-search.
// Returns nil, nil if neither is available.
func BvSearch(query, beadsRoot string) (*BvSearchResponse, error) {
	// Try br triage first (v2)
	if resp, err := BrTriage(query, beadsRoot); resp != nil || err != nil {
		return resp, err
	}

	// Fallback to bv
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

// BvRelated calls br related first, falling back to bv --robot-related.
// Returns nil, nil if neither is available or beadID is empty.
func BvRelated(beadID, beadsRoot string) (*BvRelatedResponse, error) {
	// Try br related first (v2)
	if resp, err := BrRelated(beadID, beadsRoot); resp != nil || err != nil {
		return resp, err
	}

	// Fallback to bv
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

// BvPlan calls br plan first, falling back to bv --robot-plan.
// Returns nil, nil if neither is available.
func BvPlan(beadsRoot string) (*BvPlanResponse, error) {
	// Try br plan first (v2)
	if resp, err := BrPlan(beadsRoot); resp != nil || err != nil {
		return resp, err
	}

	// Fallback to bv
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
	if !beadsadapter.Available() {
		return nil, nil
	}
	id, err := beadsadapter.Create(title, repo)
	if err != nil {
		return nil, err
	}
	return &BeadCreateResult{ID: id}, nil
}

// BrClose closes a bead with a reason.
func BrClose(id, reason, repo string) error {
	if !beadsadapter.Available() {
		return nil
	}
	return beadsadapter.Close(id, reason, repo)
}

// BrShow fetches a bead by ID and returns its metadata.
// Returns nil with no error if br is not available.
func BrShow(id string) (*BrShowResult, error) {
	if !beadsadapter.Available() {
		return nil, nil
	}
	if strings.TrimSpace(id) == "" {
		return nil, nil
	}

	out, err := beadsadapter.ShowJSON(id, "")
	if err != nil {
		return nil, err
	}

	// br show --json returns a single object in v2 (was array in v1).
	// Try single object first, fall back to array for compatibility.
	var single BrShowResult
	if err := json.Unmarshal(out, &single); err != nil {
		var results []BrShowResult
		if err2 := json.Unmarshal(out, &results); err2 != nil {
			return nil, fmt.Errorf("parse br show: %w", err)
		}
		if len(results) == 0 {
			return nil, fmt.Errorf("br show %s: no results", id)
		}
		return &results[0], nil
	}
	if single.ID == "" {
		return nil, fmt.Errorf("br show %s: empty result", id)
	}
	return &single, nil
}

// BrShowResult is the bead metadata from br show --json.
type BrShowResult struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Type        string `json:"issue_type"`
	Priority    int    `json:"priority"`
	Status      string `json:"status"`
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
	return relayadapter.Heartbeat(agent)
}

// RelayRegister registers an agent identity on the relay bus.
// Returns nil if relay is not available.
func RelayRegister(agent string) error {
	return relayadapter.Register(agent)
}

// RelaySend sends a message from one agent to another, optionally threaded.
// msgType and payload are optional — pass empty strings to omit.
// Returns nil if relay is not available.
func RelaySend(from, to, message, thread, msgType, payload string) error {
	return relayadapter.Send(from, to, message, thread, msgType, payload)
}

// GateCheck runs gate check and returns the verdict.
// Returns nil with no error if gate is not available.
func GateCheck(repo, citizen string) (*GateResult, error) {
	result, err := gateadapter.Check(repo, citizen)
	if result == nil || err != nil {
		return nil, err
	}
	return &GateResult{Pass: result.Pass, Score: result.Score, Raw: result.Raw}, nil
}

// --- Learning-loop integration ---

// loopDB returns the canonical learning-loop database path.
// Reads POLIS_LOOP_DB env var; falls back to ~/.polis/learning/loop.db.
func loopDB() string {
	if p := os.Getenv("POLIS_LOOP_DB"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".learning-loop/loop.db"
	}
	return filepath.Join(home, ".polis", "learning", "loop.db")
}

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
type Verification = loopfeed.Verification

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
		return nil, fmt.Errorf("select-template script missing at %s: %w", script, err)
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
	cmd := exec.Command("loop", "query", "--db", loopDB(), task, "--json")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("loop query %q: %w", task, err)
	}
	return out, nil
}

// IngestRun feeds a completed work run to learning-loop for pattern extraction.
// Returns nil if loop is not on PATH.
func IngestRun(entry loopfeed.Entry) error {
	if !Available("loop") {
		return nil
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal run data for ingest: %w", err)
	}

	cmd := exec.Command("loop", "ingest", "--db", loopDB(), "-")
	cmd.Stdin = bytes.NewReader(data)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("loop ingest: %w", err)
	}
	return nil
}

// WriteRunRecord persists a run record for operator inspection and recovery.
func WriteRunRecord(record RunRecord, workDir string) error {
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
	return nil
}
