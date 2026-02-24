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
		notify   string
	)

	cmd := &cobra.Command{
		Use:   "run <task>",
		Short: "Run a task — full lifecycle with context, worker, and tracing",
		Long: `Orchestrates the full work lifecycle:
1. Creates a bead — identity from the start
2. Sets agent state to working, sends relay heartbeat
3. Gathers context — bv search/related/plan, past beads, citizen experience
4. Assembles a rich prompt — task + context + quality expectations
5. Spawns a Claude Code worker in tmux
6. Opens a trace — timestamped JSONL events
7. On completion: gate check, close trace, derive close reason
8. Records experience, sends relay notifications
9. Sets agent state to idle, closes the bead`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			task := args[0]
			return runTask(cmd, task, repo, citizen, deadline, notify)
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "repository path (default: cwd)")
	cmd.Flags().StringVar(&citizen, "citizen", "", "citizen name (default: worker)")
	cmd.Flags().DurationVar(&deadline, "deadline", 30*time.Minute, "max time for the worker")
	cmd.Flags().StringVar(&notify, "notify", "", "agent to notify on completion (in addition to athena)")

	return cmd
}

func runTask(cmd *cobra.Command, task, repo, citizen string, deadline time.Duration, notify string) error {
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
	bead, err := ecosystem.BrCreate(task, repo)
	if err != nil {
		cmd.Printf("  Warning: br create failed: %v (continuing without bead tracking)\n", err)
	} else if bead != nil {
		beadID = bead.ID
		cmd.Printf("  Bead: %s\n", beadID)
	} else {
		cmd.Printf("  Note: br not available, using generated ID: %s\n", beadID)
	}

	// Step 1b: Agent state → working + relay heartbeat
	if err := ecosystem.BrAgentState(citizen, "working"); err != nil {
		cmd.Printf("  Warning: br agent state: %v\n", err)
	}
	if err := ecosystem.RelayHeartbeat(citizen); err != nil {
		cmd.Printf("  Warning: relay heartbeat: %v\n", err)
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
	var gateResult *ecosystem.GateResult
	if spawnErr == nil {
		gate, gateErr := ecosystem.GateCheck(repo, citizen)
		if gateErr != nil {
			cmd.Printf("  Warning: gate check failed: %v\n", gateErr)
		} else if gate != nil {
			gateResult = gate
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
	var durationS int64
	if tr != nil {
		meta := tr.GetMetadata(outcome)
		durationS = meta.DurationS
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

	// Step 8: Learning-loop feedback collection
	feedbackRecord := buildRunRecord(beadID, citizen, outcome, durationS, gateResult, ctx)
	if fbErr := ecosystem.CollectFeedback(feedbackRecord, workDir); fbErr != nil {
		cmd.Printf("  Warning: feedback collection failed: %v\n", fbErr)
	}

	// Step 9: Record citizen experience
	if expErr := workctx.AppendCitizenExperience(workDir, citizen, task, outcome, beadID); expErr != nil {
		cmd.Printf("  Warning: failed to record experience: %v\n", expErr)
	}

	// Step 10: Auto close reason from trace + diff
	closeReason := fmt.Sprintf("%s: %s", outcome, task)
	if tr != nil {
		cr, crErr := ecosystem.DeriveCloseReason(tr.FilePath(), repo)
		if crErr != nil {
			cmd.Printf("  Warning: derive close reason: %v\n", crErr)
		} else {
			closeReason = ecosystem.FormatCloseReason(cr)
			cmd.Printf("  Close reason: %s\n", closeReason)
		}
	}

	// Step 11: Close bead with derived reason
	if bead != nil {
		if closeErr := ecosystem.BrClose(beadID, closeReason, repo); closeErr != nil {
			cmd.Printf("  Warning: br close failed: %v\n", closeErr)
		}
	}

	// Step 12: Relay notification
	summary := fmt.Sprintf("[%s] %s — %s (%s)", beadID, task, outcome, closeReason)
	if err := ecosystem.RelaySend(citizen, "athena", summary, beadID); err != nil {
		cmd.Printf("  Warning: relay send athena: %v\n", err)
	}
	if notify != "" && notify != "athena" {
		if err := ecosystem.RelaySend(citizen, notify, summary, beadID); err != nil {
			cmd.Printf("  Warning: relay send %s: %v\n", notify, err)
		}
	}

	// Step 13: Agent state → idle
	if err := ecosystem.BrAgentState(citizen, "idle"); err != nil {
		cmd.Printf("  Warning: br agent state idle: %v\n", err)
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

// buildRunRecord maps work run outcome data to the format expected by feedback-collector.sh.
func buildRunRecord(beadID, citizen, outcome string, durationS int64, gate *ecosystem.GateResult, ctx *workctx.Result) ecosystem.RunRecord {
	rec := ecosystem.RunRecord{
		Bead:            beadID,
		Agent:           citizen,
		Model:           "claude-sonnet",
		TemplateName:    "custom",
		Attempt:         1,
		DurationSeconds: durationS,
		Verification: ecosystem.Verification{
			Tests:      "skipped",
			Lint:       "skipped",
			UBS:        "skipped",
			Truthsayer: "skipped",
		},
	}

	// Map template name from context engine's learning-loop selection
	if ctx != nil && ctx.TemplateSelection != nil {
		rec.TemplateName = ctx.TemplateSelection.TaskType
	}

	// Map outcome to status/exit_code
	switch outcome {
	case "success":
		rec.Status = "done"
		rec.ExitCode = 0
	case "error":
		rec.Status = "failed"
		rec.ExitCode = 1
	case "timeout":
		rec.Status = "timeout"
		rec.ExitCode = 0
	case "gate_fail":
		rec.Status = "done"
		rec.ExitCode = 0
	default:
		rec.Status = "done"
		rec.ExitCode = 0
	}

	// Map gate result to verification signals
	if gate != nil {
		if gate.Pass {
			rec.Verification.Tests = "pass"
			rec.Verification.Lint = "pass"
		} else {
			rec.Verification.Tests = "fail"
		}
	}

	return rec
}
