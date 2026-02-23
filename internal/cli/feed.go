package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"polis/work/internal/index"

	"github.com/spf13/cobra"
)

// FeedEntry is the stable contract between work and learning-loop.
type FeedEntry struct {
	BeadID   string  `json:"bead_id"`
	Citizen  string  `json:"citizen"`
	Task     string  `json:"task"`
	Outcome  string  `json:"outcome"`
	Duration int64   `json:"duration_s"`
	GatePass *bool   `json:"gate_pass,omitempty"`
	GateScore *float64 `json:"gate_score,omitempty"`
	Error    string  `json:"error,omitempty"`
	Time     string  `json:"time"`
}

func newFeedCmd() *cobra.Command {
	var since string

	cmd := &cobra.Command{
		Use:   "feed",
		Short: "Output structured feed for learning-loop consumption",
		Long:  "Outputs JSONL feed entries for learning-loop. Each line: {bead_id, citizen, task, outcome, duration_s, gate_pass, gate_score, error, time}.",
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
				entry := FeedEntry{
					BeadID:   r.BeadID,
					Citizen:  r.Agent,
					Task:     r.Task,
					Outcome:  r.Outcome,
					Duration: r.DurationS,
					Time:     r.StartTime.Format(time.RFC3339),
				}
				enc.Encode(entry)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&since, "since", "24h", "time window (e.g. 1h, 24h, 7d)")
	return cmd
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
