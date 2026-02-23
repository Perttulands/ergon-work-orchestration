package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"polis/work/internal/index"

	"github.com/spf13/cobra"
)

func newHistoryCmd() *cobra.Command {
	var (
		limit   int
		jsonOut bool
	)

	cmd := &cobra.Command{
		Use:   "history",
		Short: "List recent completed runs",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("get home dir: %w", err)
			}
			workDir := filepath.Join(home, ".work")

			idx, err := index.Open(workDir)
			if err != nil {
				return fmt.Errorf("open index: %w", err)
			}
			defer idx.Close()

			runs, err := idx.Recent(limit)
			if err != nil {
				return fmt.Errorf("query recent: %w", err)
			}

			if jsonOut {
				data, _ := json.MarshalIndent(runs, "", "  ")
				cmd.Println(string(data))
				return nil
			}

			if len(runs) == 0 {
				cmd.Println("No completed runs yet.")
				return nil
			}

			cmd.Printf("Recent runs (last %d):\n\n", len(runs))
			for _, r := range runs {
				dur := formatDuration(r.DurationS)
				cmd.Printf("  %s  %-10s  %-8s  %s  (%s)\n",
					r.StartTime.Format("2006-01-02 15:04"),
					r.BeadID,
					r.Outcome,
					r.Task,
					dur,
				)
			}
			return nil
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 20, "max runs to show")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON")
	return cmd
}

func formatDuration(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm%ds", seconds/60, seconds%60)
	}
	return fmt.Sprintf("%dh%dm", seconds/3600, (seconds%3600)/60)
}
