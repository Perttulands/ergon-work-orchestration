package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"polis/work/internal/index"
	"polis/work/internal/trace"

	"github.com/spf13/cobra"
)

// FeedEntry is the stable contract between work and learning-loop.
// Top-level fields match learning-loop's db.Run schema for direct ingestion.
// Work-specific fields live in metadata.
type FeedEntry struct {
	ID        string         `json:"id"`
	Task      string         `json:"task"`
	Outcome   string         `json:"outcome"`
	DurationS *int           `json:"duration_seconds,omitempty"`
	Timestamp string         `json:"timestamp"`
	Agent     string         `json:"agent,omitempty"`
	ErrorMsg  string         `json:"error_message,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

func newFeedCmd() *cobra.Command {
	var since string

	cmd := &cobra.Command{
		Use:   "feed",
		Short: "Output structured feed for learning-loop consumption",
		Long: `Outputs JSONL feed entries for learning-loop ingestion.
Each line is a JSON object matching the learning-loop db.Run schema.
Work-specific fields (gate_score, citizen, repo, bead_id) are in metadata.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("get home dir: %w", err)
			}
			workDir := filepath.Join(home, ".work")

			sinceTime, err := parseSince(since)
			if err != nil {
				return fmt.Errorf("parse --since: %w", err)
			}

			idx, err := index.Open(workDir)
			if err != nil {
				return fmt.Errorf("open index: %w", err)
			}
			defer idx.Close()

			runs, err := idx.Recent(1000) // generous limit
			if err != nil {
				return fmt.Errorf("query runs: %w", err)
			}

			enc := json.NewEncoder(cmd.OutOrStdout())
			for _, r := range runs {
				if r.StartTime.Before(sinceTime) {
					continue
				}
				entry := buildFeedEntry(r)
				enc.Encode(entry)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&since, "since", "24h", "time window (e.g. 1h, 24h, 7d)")
	return cmd
}

// buildFeedEntry maps an index RunRecord to a learning-loop compatible FeedEntry.
// Enriches with gate data from the trace file when available.
func buildFeedEntry(r index.RunRecord) FeedEntry {
	dur := int(r.DurationS)
	entry := FeedEntry{
		ID:        r.BeadID,
		Task:      r.Task,
		Outcome:   mapOutcome(r.Outcome),
		DurationS: &dur,
		Timestamp: r.StartTime.Format(time.RFC3339),
		Agent:     r.Agent,
		Metadata: map[string]any{
			"bead_id": r.BeadID,
			"citizen": r.Agent,
		},
	}

	// Enrich with gate data from trace file
	if r.TracePath != "" {
		enrichFromTrace(&entry, r.TracePath)
	}

	return entry
}

// enrichFromTrace reads gate_result events from the trace JSONL to populate
// gate_score and gate_pass in metadata.
func enrichFromTrace(entry *FeedEntry, tracePath string) {
	events, err := trace.ReadTrace(tracePath)
	if err != nil {
		return
	}

	for _, e := range events {
		switch e.EventType {
		case "gate_result":
			if e.Score != nil {
				entry.Metadata["gate_score"] = *e.Score
			}
			if e.Pass != nil {
				entry.Metadata["gate_pass"] = *e.Pass
			}
		case "error":
			if e.Error != "" && entry.ErrorMsg == "" {
				entry.ErrorMsg = e.Error
			}
		}
	}
}

// mapOutcome maps work outcomes to learning-loop valid outcomes.
// Learning-loop accepts: success, partial, failure, error.
func mapOutcome(outcome string) string {
	if outcome == "success" {
		return "success"
	}
	if outcome == "gate_fail" {
		return "failure"
	}
	return "error"
}

func parseSince(s string) (time.Time, error) {
	if len(s) < 2 {
		return time.Time{}, fmt.Errorf("invalid duration: %s", s)
	}

	unit := s[len(s)-1]
	numStr := s[:len(s)-1]

	var num int
	if _, err := fmt.Sscanf(numStr, "%d", &num); err != nil {
		return time.Time{}, fmt.Errorf("invalid number in %q: %w", s, err)
	}

	var dur time.Duration
	switch unit {
	case 'h':
		dur = time.Duration(num) * time.Hour
	case 'd':
		dur = time.Duration(num) * 24 * time.Hour
	case 'm':
		dur = time.Duration(num) * time.Minute
	default:
		return time.Time{}, fmt.Errorf("unknown unit %c in %q (use h, d, or m)", unit, s)
	}

	return time.Now().Add(-dur), nil
}
