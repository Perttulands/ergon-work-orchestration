package loopfeed

// Entry is the stable contract between work and learning-loop.
// Top-level fields match learning-loop's db.Run schema for direct ingestion.
// Work-specific fields live in metadata.
type Entry struct {
	ID        string         `json:"id"`
	Task      string         `json:"task"`
	Outcome   string         `json:"outcome"`
	DurationS *int           `json:"duration_seconds,omitempty"`
	Timestamp string         `json:"timestamp"`
	Agent     string         `json:"agent,omitempty"`
	ErrorMsg  string         `json:"error_message,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// Verification holds per-check signals for the run record and feed metadata.
type Verification struct {
	Tests      string `json:"tests"`
	Lint       string `json:"lint"`
	UBS        string `json:"ubs"`
	Truthsayer string `json:"truthsayer"`
}

// MapOutcome normalizes work outcomes to learning-loop's accepted values.
func MapOutcome(outcome string) string {
	if outcome == "success" {
		return "success"
	}
	if outcome == "gate_fail" {
		return "failure"
	}
	return "error"
}
