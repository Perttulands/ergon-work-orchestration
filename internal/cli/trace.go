package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"polis/work/internal/index"
	"polis/work/internal/trace"

	"github.com/spf13/cobra"
)

func newTraceCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "trace <bead-id>",
		Short: "Pretty-print a trace timeline",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			beadID := args[0]

			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("get home dir: %w", err)
			}
			workDir := filepath.Join(home, ".work")

			// Find trace path: try index first, then scan
			tracePath, err := findTracePath(workDir, beadID)
			if err != nil {
				return fmt.Errorf("find trace: %w", err)
			}

			events, err := trace.ReadTrace(tracePath)
			if err != nil {
				return fmt.Errorf("read trace: %w", err)
			}

			if jsonOut {
				data, _ := json.MarshalIndent(events, "", "  ")
				cmd.Println(string(data))
				return nil
			}

			return prettyPrintTrace(cmd, beadID, events)
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON")
	return cmd
}

func findTracePath(workDir, beadID string) (string, error) {
	// Try index first
	idx, err := index.Open(workDir)
	if err == nil {
		defer idx.Close()
		runs, err := idx.ByBead(beadID)
		if err == nil && len(runs) > 0 {
			return runs[0].TracePath, nil
		}
	}

	// Fall back to scanning trace directories
	pattern := filepath.Join(workDir, "traces", "*", "*", "*", fmt.Sprintf("trace-%s.jsonl", beadID))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("glob trace files: %w", err)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no trace found for bead %s", beadID)
	}
	return matches[len(matches)-1], nil // most recent
}

func prettyPrintTrace(cmd *cobra.Command, beadID string, events []trace.Event) error {
	cmd.Printf("Trace: %s\n", beadID)
	cmd.Println(strings.Repeat("─", 60))

	for _, e := range events {
		ts := e.Timestamp
		if len(ts) > 19 {
			ts = ts[:19] // trim to YYYY-MM-DDTHH:MM:SS
		}

		switch e.EventType {
		case "begin":
			cmd.Printf("%s  BEGIN  agent=%s task=%q\n", ts, e.Agent, e.Task)
		case "end":
			dur := ""
			if e.DurationS != nil {
				dur = fmt.Sprintf(" (%s)", formatDuration(*e.DurationS))
			}
			errStr := ""
			if e.Error != "" {
				errStr = fmt.Sprintf(" error=%q", e.Error)
			}
			cmd.Printf("%s  END    outcome=%s%s%s\n", ts, e.Outcome, dur, errStr)
		case "tool_call":
			dur := ""
			if e.DurationMs != nil {
				dur = fmt.Sprintf(" %dms", *e.DurationMs)
			}
			cmd.Printf("%s  TOOL   %s: %s%s\n", ts, e.Tool, e.Cmd, dur)
		case "file_write":
			lines := ""
			if e.Lines != nil {
				lines = fmt.Sprintf(" (%d lines)", *e.Lines)
			}
			cmd.Printf("%s  WRITE  %s%s\n", ts, e.Path, lines)
		case "gate_result":
			pass := "FAIL"
			if e.Pass != nil && *e.Pass {
				pass = "PASS"
			}
			score := ""
			if e.Score != nil {
				score = fmt.Sprintf(" score=%.2f", *e.Score)
			}
			cmd.Printf("%s  GATE   %s%s\n", ts, pass, score)
		case "worker_output":
			// Truncate long output
			out := e.Output
			if len(out) > 200 {
				out = out[:200] + "..."
			}
			cmd.Printf("%s  OUTPUT %s\n", ts, out)
		case "error":
			cmd.Printf("%s  ERROR  %s\n", ts, e.Error)
		default:
			cmd.Printf("%s  %s\n", ts, e.EventType)
		}
	}
	return nil
}
