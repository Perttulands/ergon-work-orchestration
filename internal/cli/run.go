package cli

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"polis/work/internal/beadlint"
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
		runtime  string
	)

	cmd := &cobra.Command{
		Use:   "run <task>",
		Short: "Run a task — full lifecycle with context, worker, and tracing",
		Long: `Orchestrates the full work lifecycle:
1. Creates a bead — identity from the start
2. Sets agent state to working, sends relay heartbeat
3. Gathers context — bv search/related/plan, past beads, citizen experience
4. Assembles a rich prompt — task + context + quality expectations
5. Spawns a worker runtime (codex/claude) in tmux
6. Opens a trace — timestamped JSONL events
7. On completion: gate check, close trace, derive close reason
8. Records experience, sends relay notifications
9. Sets agent state to idle, closes the bead`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			task := args[0]
			return runTask(cmd, task, repo, citizen, deadline, notify, runtime)
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "repository path (default: cwd)")
	cmd.Flags().StringVar(&citizen, "citizen", "", "citizen name (default: worker)")
	cmd.Flags().StringVar(&runtime, "runtime", "", "worker runtime profile (default from runtime config)")
	cmd.Flags().DurationVar(&deadline, "deadline", 30*time.Minute, "max time for the worker")
	cmd.Flags().StringVar(&notify, "notify", "", "agent to notify on completion (in addition to athena)")

	return cmd
}

func runTask(cmd *cobra.Command, task, repo, citizen string, deadline time.Duration, notify, runtime string) error {
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

	resolvedRuntime, rtErr := worker.ResolveRuntime(runtime, citizen)
	if rtErr != nil {
		return rtErr
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	workDir := filepath.Join(home, ".work")

	cmd.Printf("Starting: %s\n", task)
	cmd.Printf("  Citizen: %s | Repo: %s | Runtime: %s | Deadline: %s\n", citizen, repo, resolvedRuntime, deadline)

	// Pre-flight: check tool availability and warn about degradation
	degradation := checkTools()
	for _, d := range degradation {
		if d.Warning != "" {
			cmd.Printf("  Warning: %s\n", d.Warning)
		}
	}

	// Pre-dispatch lint: refuse to start if bead quality is too low
	if lintErr := lintBeforeDispatch(cmd, task); lintErr != nil {
		return lintErr
	}

	// Step 1: Create bead
	beadID := "work-" + randomID()
	bead, err := ecosystem.BrCreate(task, repo)
	if err != nil {
		cmd.Printf("  Warning: br create failed: %v (continuing in bead-free mode)\n", err)
	} else if bead != nil {
		beadID = bead.ID
		cmd.Printf("  Bead: %s\n", beadID)
	} else {
		cmd.Printf("  Warning: br not on PATH — operating in bead-free mode (ID: %s)\n", beadID)
	}

	// Step 1b: Agent state → working + relay heartbeat
	if err := ecosystem.BrAgentState(citizen, "working"); err != nil {
		cmd.Printf("  Warning: br agent state: %v\n", err)
	}
	if err := ecosystem.RelayRegister(citizen); err != nil {
		cmd.Printf("  Warning: relay register: %v\n", err)
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
		AgentName:   citizen,
		Runtime:     resolvedRuntime,
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
	gateSkipped := false
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
			gateSkipped = true
			cmd.Printf("  Warning: gate not on PATH — result is unverified\n")
			if tr != nil {
				tr.EmitError("gate_skipped: result is unverified")
			}
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
	feedbackRecord := buildRunRecord(beadID, citizen, resolvedRuntime, outcome, durationS, gateResult, ctx)
	if fbErr := ecosystem.CollectFeedback(feedbackRecord, workDir); fbErr != nil {
		cmd.Printf("  Warning: feedback collection failed: %v\n", fbErr)
	}

	// Step 8b: Feed run to learning-loop binary for pattern extraction
	{
		testsPassed := gateResult != nil && gateResult.Pass
		lintPassed := gateResult != nil && gateResult.Pass
		var errMsg string
		if spawnErr != nil {
			errMsg = spawnErr.Error()
		}
		if ingestErr := ecosystem.IngestRun(beadID, task, outcome, citizen, durationS, testsPassed, lintPassed, nil, errMsg); ingestErr != nil {
			cmd.Printf("  Warning: loop ingest failed: %v\n", ingestErr)
		}
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

	// Step 12: Relay notification (typed task_result messages)
	summary := fmt.Sprintf("[%s] %s — %s (%s)", beadID, task, outcome, closeReason)
	gateScoreStr := "null"
	if gateResult != nil {
		gateScoreStr = fmt.Sprintf("%.2f", gateResult.Score)
	}
	relayPayload := fmt.Sprintf(`{"bead_id":"%s","outcome":"%s","gate_score":%s,"duration":"%ds"}`,
		beadID, outcome, gateScoreStr, durationS)
	if err := ecosystem.RelaySend(citizen, "athena", summary, beadID, "task_result", relayPayload); err != nil {
		cmd.Printf("  Warning: relay send athena: %v\n", err)
	}
	if notify != "" && notify != "athena" {
		if err := ecosystem.RelaySend(citizen, notify, summary, beadID, "task_result", relayPayload); err != nil {
			cmd.Printf("  Warning: relay send %s: %v\n", notify, err)
		}
	}

	// Step 13: Agent state → idle
	if err := ecosystem.BrAgentState(citizen, "idle"); err != nil {
		cmd.Printf("  Warning: br agent state idle: %v\n", err)
	}

	if gateSkipped {
		cmd.Printf("Done: %s [%s] (unverified)\n", beadID, outcome)
	} else {
		cmd.Printf("Done: %s [%s]\n", beadID, outcome)
	}
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

// lintBeforeDispatch validates the task (or bead) quality before starting work.
// If task looks like a bead ID (pol-xxx), fetches the bead and lints it.
// Otherwise lints the task title for minimum quality.
func lintBeforeDispatch(cmd *cobra.Command, task string) error {
	if beadlint.IsBeadID(task) {
		bead, err := ecosystem.BrShow(task)
		if err != nil {
			cmd.Printf("  Warning: could not fetch bead for lint: %v\n", err)
			// Degrade gracefully — lint the raw task string instead
			return lintTaskTitle(cmd, task)
		}
		if bead == nil {
			// br not available — skip lint
			return nil
		}
		issues := beadlint.LintBead(beadlint.Bead{
			ID:          bead.ID,
			Title:       bead.Title,
			Description: bead.Description,
			Type:        bead.Type,
			Priority:    bead.Priority,
		})
		if beadlint.HasErrors(issues) {
			cmd.Printf("Bead %s failed lint:\n%s", task, beadlint.FormatIssues(issues))
			return fmt.Errorf("bead %s failed quality lint — fix issues before dispatch", task)
		}
		if len(issues) > 0 {
			cmd.Printf("  Lint warnings:\n%s", beadlint.FormatIssues(issues))
		}
		return nil
	}
	return lintTaskTitle(cmd, task)
}

func lintTaskTitle(cmd *cobra.Command, task string) error {
	issues := beadlint.LintTitle(task)
	if beadlint.HasErrors(issues) {
		cmd.Printf("Task failed lint:\n%s", beadlint.FormatIssues(issues))
		return fmt.Errorf("task title failed quality lint — be more specific (>= 5 words)")
	}
	if len(issues) > 0 {
		cmd.Printf("  Lint warnings:\n%s", beadlint.FormatIssues(issues))
	}
	return nil
}

// toolDegradation describes what happens when an optional tool is missing.
type toolDegradation struct {
	Name    string
	Present bool
	Warning string // empty if tool is present or degrades silently
	Mode    string // "normal", "bead-free", "unverified", "silent-skip"
}

// checkTools checks which optional tools are available and returns degradation info.
func checkTools() []toolDegradation {
	var report []toolDegradation

	if !ecosystem.Available("gate") {
		report = append(report, toolDegradation{
			Name:    "gate",
			Present: false,
			Warning: "gate not on PATH — results will be unverified",
			Mode:    "unverified",
		})
	}
	if !ecosystem.Available("br") {
		report = append(report, toolDegradation{
			Name:    "br",
			Present: false,
			Warning: "br not on PATH — operating in bead-free mode",
			Mode:    "bead-free",
		})
	}
	if !ecosystem.Available("relay") {
		report = append(report, toolDegradation{
			Name:    "relay",
			Present: false,
			Mode:    "silent-skip",
		})
	}
	if !ecosystem.Available("loop") {
		report = append(report, toolDegradation{
			Name:    "loop",
			Present: false,
			Mode:    "silent-skip",
		})
	}

	return report
}

// buildRunRecord maps work run outcome data to the format expected by feedback-collector.sh.
func buildRunRecord(beadID, citizen, runtime, outcome string, durationS int64, gate *ecosystem.GateResult, ctx *workctx.Result) ecosystem.RunRecord {
	rec := ecosystem.RunRecord{
		Bead:            beadID,
		Agent:           citizen,
		Model:           worker.ModelForRuntime(runtime, citizen),
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
