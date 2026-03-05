package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"polis/work/internal/ecosystem"

	"github.com/spf13/cobra"
)

func newDecideCmd() *cobra.Command {
	var (
		evidence []string
		decider  string
		priority string
	)

	cmd := &cobra.Command{
		Use:   "decide <question>",
		Short: "Quick ruling — gate bead with relay notification",
		Long: `For decisions that don't need full senate deliberation:
1. Creates a gate bead (blocks until decided)
2. Assembles evidence from linked beads
3. Sends question + evidence to decider via relay
4. Prints gate bead ID — close with: br close <id> --reason "decision + reasoning"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			question := args[0]
			return runDecide(cmd, question, evidence, decider, priority)
		},
	}

	cmd.Flags().StringSliceVar(&evidence, "evidence", nil, "evidence bead IDs (comma-separated)")
	cmd.Flags().StringVar(&decider, "decider", "athena", "who to notify: agent name for relay")
	cmd.Flags().StringVar(&priority, "priority", "normal", "priority: low, normal, high, urgent")

	return cmd
}

func runDecide(cmd *cobra.Command, question string, evidence []string, decider, priority string) error {
	repo, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	workDir := filepath.Join(home, ".work")
	_ = os.MkdirAll(workDir, 0o755)

	cmd.Printf("Decision needed: %s\n", question)

	// Step 1: Create gate bead
	beadID := "work-" + randomID()
	title := fmt.Sprintf("decide: %s", truncate(question, 60))
	bead, err := ecosystem.BrCreate(title, repo)
	if err != nil {
		if policyErr := applyFailurePolicy(cmd, stepBrCreate, err); policyErr != nil {
			return policyErr
		}
	} else if bead != nil {
		beadID = bead.ID
		cmd.Printf("  Gate bead: %s\n", beadID)
	} else {
		cmd.Printf("  Note: br not available, using generated ID: %s\n", beadID)
	}

	// Step 2: Assemble evidence from linked beads
	var evidenceText strings.Builder
	if len(evidence) > 0 {
		evidenceText.WriteString("\n\nEVIDENCE:\n")
		for _, beadRef := range evidence {
			desc := gatherBeadEvidence(beadRef, repo)
			if desc != "" {
				evidenceText.WriteString(fmt.Sprintf("\n--- %s ---\n%s\n", beadRef, desc))
			} else {
				evidenceText.WriteString(fmt.Sprintf("\n--- %s --- (could not retrieve)\n", beadRef))
			}
		}
	}

	// Step 3: Send notification via relay
	message := fmt.Sprintf("DECISION REQUESTED [%s]\n\nGate: %s\nQuestion: %s\nPriority: %s\nTime: %s%s\n\nTo rule: br close %s --reason \"your decision + reasoning\"",
		priority, beadID, question, priority, time.Now().Format(time.RFC3339), evidenceText.String(), beadID)

	if ecosystem.Available("relay") {
		relayArgs := []string{"send", decider, message,
			"--subject", fmt.Sprintf("Decision: %s", truncate(question, 60)),
			"--thread", beadID,
			"--priority", priority,
			"--agent", "work",
		}
		relayCmd := exec.Command("relay", relayArgs...)
		if out, relayErr := relayCmd.CombinedOutput(); relayErr != nil {
			cmd.Printf("  Warning: relay send failed: %v (%s)\n", relayErr, strings.TrimSpace(string(out)))
		} else {
			cmd.Printf("  Notified: %s via relay\n", decider)
		}
	} else {
		cmd.Printf("  Note: relay not available, notification not sent\n")
		cmd.Printf("  Message:\n%s\n", message)
	}

	cmd.Printf("\nGate bead: %s\n", beadID)
	cmd.Printf("To rule:   br close %s --reason \"decision + reasoning\"\n", beadID)

	return nil
}

// gatherBeadEvidence retrieves a bead's description for evidence assembly.
func gatherBeadEvidence(beadID, repo string) string {
	if !ecosystem.Available("br") {
		return ""
	}

	cmd := exec.Command("br", "show", beadID)
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
