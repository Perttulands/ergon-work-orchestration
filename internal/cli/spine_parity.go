package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"polis/work/internal/spine"

	"github.com/spf13/cobra"
)

func newSpineParityCmd() *cobra.Command {
	var (
		workDir  string
		spineDir string
		jsonOut  bool
	)

	cmd := &cobra.Command{
		Use:   "spine-parity",
		Short: "Compare legacy work traces with the Polis spine shadow stream",
		RunE: func(cmd *cobra.Command, args []string) error {
			if workDir == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("get home dir: %w", err)
				}
				workDir = filepath.Join(home, ".work")
			}
			if spineDir == "" {
				spineDir = spine.DefaultDir()
			}

			report, err := spine.Compare(workDir, spineDir)
			if err != nil {
				return fmt.Errorf("compare parity: %w", err)
			}

			if jsonOut {
				data, err := json.MarshalIndent(report, "", "  ")
				if err != nil {
					return fmt.Errorf("marshal parity report: %w", err)
				}
				cmd.Println(string(data))
			} else {
				printParityReport(cmd, workDir, spineDir, report)
			}

			if report.HasMismatches() {
				return fmt.Errorf("spine parity mismatches detected")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&workDir, "work-dir", "", "work state directory (default: ~/.work)")
	cmd.Flags().StringVar(&spineDir, "spine-dir", "", "Polis spine directory (default: ~/.polis/spine/events or POLIS_SPINE_DIR)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output report as JSON")

	return cmd
}

func printParityReport(cmd *cobra.Command, workDir, spineDir string, report spine.ParityReport) {
	cmd.Printf("Work dir: %s\n", workDir)
	cmd.Printf("Spine dir: %s\n", spineDir)
	cmd.Printf("Legacy runs: %d\n", report.LegacyRuns)
	cmd.Printf("Spine runs: %d\n", report.SpineRuns)
	cmd.Printf("Missing in spine: %d\n", len(report.MissingInSpine))
	cmd.Printf("Missing in legacy: %d\n", len(report.MissingInLegacy))
	cmd.Printf("Outcome mismatches: %d\n", len(report.OutcomeMismatches))
	cmd.Printf("Gate mismatches: %d\n", len(report.GateMismatches))
	cmd.Printf("Ordering mismatches: %d\n", len(report.OrderingMismatches))

	if !report.HasMismatches() {
		cmd.Println("Parity OK")
		return
	}

	for _, runKey := range report.MissingInSpine {
		cmd.Printf("  missing_in_spine: %s\n", runKey)
	}
	for _, runKey := range report.MissingInLegacy {
		cmd.Printf("  missing_in_legacy: %s\n", runKey)
	}
	for _, mismatch := range report.OutcomeMismatches {
		cmd.Printf("  outcome_mismatch: %s (%s)\n", mismatch.RunKey, mismatch.Detail)
	}
	for _, mismatch := range report.GateMismatches {
		cmd.Printf("  gate_mismatch: %s (%s)\n", mismatch.RunKey, mismatch.Detail)
	}
	for _, mismatch := range report.OrderingMismatches {
		cmd.Printf("  ordering_mismatch: %s (%s)\n", mismatch.RunKey, mismatch.Detail)
	}
}
