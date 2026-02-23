package cli

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"time"

	workctx "polis/work/internal/context"
	"polis/work/internal/ecosystem"
	"polis/work/internal/index"
	"polis/work/internal/trace"
	"polis/work/internal/worker"

	"github.com/spf13/cobra"
)

func newRunCmd() *cobra.Command {
	var (
		repo     string
		citizen  string
		deadline time.Duration
	)

	cmd := &cobra.Command{
		Use:   "run <task>",
		Short: "Run a task — full lifecycle with context, worker, and tracing",
		Long: `Orchestrates the full work lifecycle:
1. Creates a bead (bd create) — identity from the start
2. Gathers context — past beads, citizen experience, patterns
3. Assembles a rich prompt — task + context + quality expectations
4. Spawns a Claude Code worker in tmux
5. Opens a trace — timestamped JSONL events
6. On completion: calls gate check for quality (if available)
7. Closes trace with outcome
8. Records the experience — appends to citizen's history
9. Closes the bead`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			task := args[0]
			return runTask(cmd, task, repo, citizen, deadline)
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "repository path (default: cwd)")
	cmd.Flags().StringVar(&citizen, "citizen", "", "citizen name (default: worker)")
	cmd.Flags().DurationVar(&deadline, "deadline", 30*time.Minute, "max time for the worker")

	return cmd
}

func runTask(cmd *cobra.Command, task, repo, citizen string, deadline time.Duration) error {
	// Defaults
	if repo == "" {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %w", err)
		}
		repo = wd
	}
	if citizen == "" {
		citizen = "worker"
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	workDir := filepath.Join(home, ".work")

	cmd.Printf("Starting: %s\n", task)
	cmd.Printf("  Citizen: %s | Repo: %s | Deadline: %s\n", citizen, repo, deadline)

	// Step 1: Create bead
	beadID := "work-" + randomID()
	bead, err := ecosystem.BdCreate(task, repo)
	if err != nil {
		cmd.Printf("  Warning: bd create failed: %v (continuing without bead tracking)\n", err)
	} else if bead != nil {
		beadID = bead.ID
		cmd.Printf("  Bead: %s\n", beadID)
	} else {
		cmd.Printf("  Note: bd not available, using generated ID: %s\n", beadID)
	}

	// Step 2: Gather context
	cmd.Printf("  Gathering context...\n")
	ctx, err := workctx.Gather(workctx.Config{
		BeadID:  beadID,
		Citizen: citizen,
		Repo:    repo,
		Task:    task,
		WorkDir: workDir,
	})
	if err != nil {
		cmd.Printf("  Warning: context gathering failed: %v\n", err)
	}

	// Step 3: Assemble prompt
	prompt := assemblePrompt(task, citizen, beadID, repo, ctx)

	// Step 4: Open trace
	tr, traceErr := trace.Open(workDir, beadID, citizen, task)
	if traceErr != nil {
		cmd.Printf("  Warning: trace open failed: %v\n", traceErr)
	} else {
		cmd.Printf("  Trace: %s\n", tr.FilePath())
	}

	// Step 5: Spawn worker
	sessionName := fmt.Sprintf("work-%s", beadID)
	cmd.Printf("  Spawning worker in tmux session: %s\n", sessionName)

	result, spawnErr := worker.Spawn(worker.Config{
		SessionName: sessionName,
		WorkDir:     repo,
		Prompt:      prompt,
		Deadline:    deadline,
	})

	outcome := "success"
	if spawnErr != nil {
		outcome = "error"
		cmd.Printf("  Worker error: %v\n", spawnErr)
	} else if result.TimedOut {
		outcome = "timeout"
		cmd.Printf("  Worker timed out after %s\n", deadline)
	} else {
		cmd.Printf("  Worker finished in %s\n", result.Finished.Sub(result.Started).Round(time.Second))
	}

	// Step 6: Gate check
	if spawnErr == nil {
		gate, gateErr := ecosystem.GateCheck(repo, citizen)
		if gateErr != nil {
			cmd.Printf("  Warning: gate check failed: %v\n", gateErr)
		} else if gate != nil {
			if tr != nil {
				pass := gate.Pass
				score := gate.Score
				tr.Emit(trace.Event{
					EventType: "gate_result",
					Pass:      &pass,
					Score:     &score,
				})
			}
			if gate.Pass {
				cmd.Printf("  Gate: PASS (score: %.2f)\n", gate.Score)
			} else {
				cmd.Printf("  Gate: FAIL (score: %.2f)\n", gate.Score)
				outcome = "gate_fail"
			}
		} else {
			cmd.Printf("  Note: gate not available, skipping quality check\n")
		}
	}

	// Step 7: Close trace and record to index
	if tr != nil {
		meta := tr.GetMetadata(outcome)
		tr.Close(outcome, spawnErr)

		// Index the run for fast queries
		idx, idxErr := index.Open(workDir)
		if idxErr != nil {
			cmd.Printf("  Warning: index open failed: %v\n", idxErr)
		} else {
			if recErr := idx.Record(meta); recErr != nil {
				cmd.Printf("  Warning: index record failed: %v\n", recErr)
			}
			idx.Close()
		}
	}

	// Step 8: Record citizen experience
	if expErr := workctx.AppendCitizenExperience(workDir, citizen, task, outcome, beadID); expErr != nil {
		cmd.Printf("  Warning: failed to record experience: %v\n", expErr)
	}

	// Step 9: Close bead
	if bead != nil {
		reason := fmt.Sprintf("%s: %s", outcome, task)
		if closeErr := ecosystem.BdClose(beadID, reason, repo); closeErr != nil {
			cmd.Printf("  Warning: bd close failed: %v\n", closeErr)
		}
	}

	cmd.Printf("Done: %s [%s]\n", beadID, outcome)
	return spawnErr
}

func assemblePrompt(task, citizen, beadID, repo string, ctx *workctx.Result) string {
	prompt := fmt.Sprintf(`BEAD: %s
CITIZEN: %s
REPO: %s

TASK: %s

QUALITY EXPECTATIONS:
- Write tests for new functionality
- Run existing tests and ensure they pass
- Handle errors gracefully
- Keep changes focused on the task

CONSTRAINTS:
- Stay in the project directory
- Read AGENTS.md if it exists for project conventions
`, beadID, citizen, repo, task)

	if ctx != nil && ctx.Markdown != "No prior context available. This is a fresh start." {
		prompt += fmt.Sprintf("\nCONTEXT FROM PAST WORK:\n%s\n", ctx.Markdown)
	}

	prompt += "\nDONE WHEN: The task is complete, tests pass, and code is clean."
	return prompt
}

func randomID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}
