package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"polis/work/internal/ecosystem"

	"github.com/spf13/cobra"
)

// SenateCase is the case file format senate deliberate --case expects.
type SenateCase struct {
	ID                string   `json:"id"`
	Type              string   `json:"type"`
	Summary           string   `json:"summary"`
	Question          string   `json:"question"`
	Evidence          []string `json:"evidence,omitempty"`
	RequestedDecision string   `json:"requested_decision,omitempty"`
	FiledAt           string   `json:"filed_at"`
	FiledBy           string   `json:"filed_by,omitempty"`
}

// SenateVerdict is the verdict returned by senate deliberate.
type SenateVerdict struct {
	CaseID         string `json:"case_id"`
	Verdict        string `json:"verdict"`
	Reasoning      string `json:"reasoning"`
	Implementation string `json:"implementation"`
	Dissent        string `json:"dissent"`
	Binding        bool   `json:"binding"`
}

func newDeliberateCmd() *cobra.Command {
	var (
		caseType     string
		participants int
		evidence     []string
		filedBy      string
		stateDir     string
		noHandoff    bool
	)

	cmd := &cobra.Command{
		Use:   "deliberate <question>",
		Short: "Structured deliberation via Senate with bead tracking",
		Long: `Wraps senate deliberate with molecule bead lifecycle:
1. Creates a molecule bead for the deliberation
2. Writes a case file from the question
3. Calls senate deliberate --case <file>
4. Captures the verdict
5. Closes the molecule bead with verdict text
6. If approved, senate handoff creates implementation beads`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			question := args[0]
			return runDeliberate(cmd, question, caseType, participants, evidence, filedBy, stateDir, noHandoff)
		},
	}

	cmd.Flags().StringVar(&caseType, "type", "general", "case type: rule_evolution, gate_criteria, dispute, priority, architecture, general")
	cmd.Flags().IntVar(&participants, "participants", 3, "number of panel agents")
	cmd.Flags().StringSliceVar(&evidence, "evidence", nil, "evidence paths or bead:id references")
	cmd.Flags().StringVar(&filedBy, "filed-by", "", "who is filing (default: citizen name)")
	cmd.Flags().StringVar(&stateDir, "state-dir", "", "senate state directory (default: senate default)")
	cmd.Flags().BoolVar(&noHandoff, "no-handoff", false, "skip automatic bead creation from verdict")

	return cmd
}

func runDeliberate(cmd *cobra.Command, question, caseType string, participants int, evidence []string, filedBy, stateDir string, noHandoff bool) error {
	if !ecosystem.Available("senate") {
		return fmt.Errorf("senate not on PATH — install: cd /home/polis/tools/senate && go build -o ~/.local/bin/senate ./cmd/senate/")
	}

	repo, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	workDir := filepath.Join(home, ".work")

	cmd.Printf("Deliberating: %s\n", question)

	// Step 1: Create molecule bead
	beadID := "work-" + randomID()
	title := fmt.Sprintf("deliberate: %s", truncate(question, 60))
	bead, err := ecosystem.BdCreate(title, repo)
	if err != nil {
		cmd.Printf("  Warning: bd create failed: %v\n", err)
	} else if bead != nil {
		beadID = bead.ID
		cmd.Printf("  Bead: %s (molecule)\n", beadID)
	} else {
		cmd.Printf("  Note: bd not available, using generated ID: %s\n", beadID)
	}

	// Step 2: Write case file
	if filedBy == "" {
		filedBy = "work"
	}
	caseID := fmt.Sprintf("senate-%s", beadID)
	senateCase := SenateCase{
		ID:       caseID,
		Type:     caseType,
		Summary:  truncate(question, 80),
		Question: question,
		Evidence: evidence,
		FiledAt:  time.Now().UTC().Format(time.RFC3339),
		FiledBy:  filedBy,
	}

	caseDir := filepath.Join(workDir, "senate-cases")
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		return fmt.Errorf("create case dir: %w", err)
	}
	casePath := filepath.Join(caseDir, caseID+".json")

	caseData, err := json.MarshalIndent(senateCase, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal case: %w", err)
	}
	if err := os.WriteFile(casePath, caseData, 0o644); err != nil {
		return fmt.Errorf("write case file: %w", err)
	}
	cmd.Printf("  Case: %s\n", casePath)

	// Step 3: Call senate deliberate
	args := []string{"deliberate", "--case", casePath, "--json",
		"--agents", fmt.Sprintf("%d", participants)}
	if stateDir != "" {
		args = append(args, "--state-dir", stateDir)
	}
	if noHandoff {
		args = append(args, "--no-handoff")
	}

	cmd.Printf("  Running senate deliberate (this may take several minutes)...\n")
	senateCmd := exec.Command("senate", args...)
	senateCmd.Dir = repo
	out, senateErr := senateCmd.CombinedOutput()
	raw := strings.TrimSpace(string(out))

	// Step 4: Parse verdict
	var verdict SenateVerdict
	verdictParsed := false
	if senateErr == nil {
		if jsonErr := json.Unmarshal(out, &verdict); jsonErr == nil {
			verdictParsed = true
		}
	}

	if verdictParsed {
		cmd.Printf("  Verdict: %s\n", verdict.Verdict)
		if verdict.Reasoning != "" {
			// Print first 200 chars of reasoning
			reasoning := verdict.Reasoning
			if len(reasoning) > 200 {
				reasoning = reasoning[:200] + "..."
			}
			cmd.Printf("  Reasoning: %s\n", reasoning)
		}
		if verdict.Implementation != "" {
			cmd.Printf("  Implementation: %s\n", truncate(verdict.Implementation, 200))
		}
	} else {
		if senateErr != nil {
			cmd.Printf("  Senate error: %v\n", senateErr)
		}
		if raw != "" {
			cmd.Printf("  Senate output: %s\n", truncate(raw, 500))
		}
	}

	// Step 5: Close molecule bead with verdict
	closeReason := "deliberation incomplete"
	if verdictParsed {
		closeReason = fmt.Sprintf("%s: %s", verdict.Verdict, truncate(verdict.Reasoning, 200))
	} else if senateErr != nil {
		closeReason = fmt.Sprintf("error: %v", senateErr)
	}

	if bead != nil {
		if closeErr := ecosystem.BdClose(beadID, closeReason, repo); closeErr != nil {
			cmd.Printf("  Warning: bd close failed: %v\n", closeErr)
		}
	}

	// Step 6: Handoff if approved and not skipped
	if verdictParsed && verdict.Verdict == "approved" && !noHandoff {
		cmd.Printf("  Verdict approved — running senate handoff...\n")
		handoffArgs := []string{"handoff", "--case-id", caseID, "--json"}
		if stateDir != "" {
			handoffArgs = append(handoffArgs, "--state-dir", stateDir)
		}
		handoffCmd := exec.Command("senate", handoffArgs...)
		handoffCmd.Dir = repo
		handoffOut, handoffErr := handoffCmd.CombinedOutput()
		if handoffErr != nil {
			cmd.Printf("  Warning: handoff failed: %v\n", handoffErr)
		} else {
			cmd.Printf("  Handoff: %s\n", strings.TrimSpace(string(handoffOut)))
		}
	}

	cmd.Printf("Done: %s [%s]\n", beadID, func() string {
		if verdictParsed {
			return verdict.Verdict
		}
		return "error"
	}())

	if senateErr != nil {
		return fmt.Errorf("senate deliberate failed: %w", senateErr)
	}
	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
