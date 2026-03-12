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
	"polis/work/internal/runstate"
	"polis/work/internal/squire"
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
	model := worker.ModelForRuntime(resolvedRuntime, citizen)

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
		if policyErr := applyFailurePolicy(cmd, stepBrCreate, err); policyErr != nil {
			return policyErr
		}
	} else if bead != nil {
		beadID = bead.ID
		cmd.Printf("  Bead: %s\n", beadID)
	} else {
		cmd.Printf("  Warning: br not on PATH — operating in bead-free mode (ID: %s)\n", beadID)
	}

	sessionName := fmt.Sprintf("work-%s", beadID)
	var stateStore *runstate.Store
	if store, stateErr := runstate.Create(workDir, runstate.Config{
		BeadID:          beadID,
		BeadManaged:     bead != nil,
		Task:            task,
		Citizen:         citizen,
		Repo:            repo,
		Runtime:         resolvedRuntime,
		Model:           model,
		Notify:          notify,
		DeadlineSeconds: int64(deadline.Seconds()),
		TmuxSession:     sessionName,
	}); stateErr != nil {
		if policyErr := applyFailurePolicy(cmd, stepRunState, stateErr); policyErr != nil {
			return policyErr
		}
	} else {
		stateStore = store
		cmd.Printf("  Run: %s\n", stateStore.RunID())
		leaseTTL := deadline + 5*time.Minute
		if leaseTTL <= 0 {
			leaseTTL = 35 * time.Minute
		}
		if leaseErr := stateStore.AcquireLease(citizen, leaseTTL); leaseErr != nil {
			if policyErr := applyFailurePolicy(cmd, stepRunState, leaseErr); policyErr != nil {
				return policyErr
			}
		}
		defer stateStore.ReleaseLease()
		if bead != nil {
			if cpErr := stateStore.MarkEffect("br_create", checkpointKey(beadID, "br_create", 1), "bead created via br"); cpErr != nil {
				if policyErr := applyFailurePolicy(cmd, stepRunState, cpErr); policyErr != nil {
					return policyErr
				}
			}
		} else if cpErr := stateStore.MarkEffect("local_bead_alloc", checkpointKey(beadID, "local_bead_alloc", 1), "synthetic bead id allocated locally"); cpErr != nil {
			if policyErr := applyFailurePolicy(cmd, stepRunState, cpErr); policyErr != nil {
				return policyErr
			}
		}
		if cpErr := stateStore.Checkpoint("bead_created", runstate.PhaseBeadCreated, checkpointKey(beadID, "bead_created", 1), "initial run state created"); cpErr != nil {
			if policyErr := applyFailurePolicy(cmd, stepRunState, cpErr); policyErr != nil {
				return policyErr
			}
		}
	}

	// Step 1b: Agent state → working + relay heartbeat
	agentWorkingErr := ecosystem.BrAgentState(citizen, "working")
	if policyErr := applyFailurePolicy(cmd, stepBrAgentWorking, agentWorkingErr); policyErr != nil {
		return policyErr
	}
	if agentWorkingErr == nil {
		recordStateEffect(cmd, stateStore, beadID, "br_agent_state_working", "agent marked working")
	}
	relayRegisterErr := ecosystem.RelayRegister(citizen)
	if policyErr := applyFailurePolicy(cmd, stepRelayRegister, relayRegisterErr); policyErr != nil {
		return policyErr
	}
	if relayRegisterErr == nil {
		recordStateEffect(cmd, stateStore, beadID, "relay_register", "agent registered on relay")
	}
	relayHeartbeatErr := ecosystem.RelayHeartbeat(citizen)
	if policyErr := applyFailurePolicy(cmd, stepRelayHeartbeat, relayHeartbeatErr); policyErr != nil {
		return policyErr
	}
	if relayHeartbeatErr == nil {
		recordStateEffect(cmd, stateStore, beadID, "relay_heartbeat", "initial heartbeat sent")
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
		if policyErr := applyFailurePolicy(cmd, stepContextGather, err); policyErr != nil {
			return policyErr
		}
	} else {
		recordStateCheckpoint(cmd, stateStore, beadID, "context_gather", runstate.PhaseContextReady, "context gathered")
	}

	// Step 3: Assemble prompt
	prompt := assemblePrompt(task, citizen, beadID, repo, ctx)
	if stateStore != nil {
		if promptErr := stateStore.WritePrompt(prompt); promptErr != nil {
			if policyErr := applyFailurePolicy(cmd, stepRunState, promptErr); policyErr != nil {
				return policyErr
			}
		} else {
			recordStateCheckpoint(cmd, stateStore, beadID, "prompt_ready", runstate.PhasePromptReady, "prompt assembled")
		}
	}

	// Step 4: Open trace
	traceOpts := trace.OpenOptions{
		Repo:  repo,
		Model: model,
	}
	if stateStore != nil {
		if state, loadErr := stateStore.Load(); loadErr == nil {
			traceOpts.RunID = state.RunID
			traceOpts.TraceID = state.TraceID
			traceOpts.SessionID = state.SessionID
		}
	}
	tr, traceErr := trace.OpenWithOptions(workDir, beadID, citizen, task, traceOpts)
	if traceErr != nil {
		if policyErr := applyFailurePolicy(cmd, stepTraceOpen, traceErr); policyErr != nil {
			return policyErr
		}
	} else {
		cmd.Printf("  Trace: %s\n", tr.FilePath())
		if stateStore != nil {
			if pathErr := stateStore.SetTracePath(tr.FilePath()); pathErr != nil {
				if policyErr := applyFailurePolicy(cmd, stepRunState, pathErr); policyErr != nil {
					return policyErr
				}
			}
			recordStateCheckpoint(cmd, stateStore, beadID, "trace_open", runstate.PhaseTraceOpen, tr.FilePath())
		}
	}

	// Step 5: Spawn worker
	cmd.Printf("  Spawning worker in tmux session: %s\n", sessionName)
	recordStateCheckpoint(cmd, stateStore, beadID, "worker_spawn", runstate.PhaseWorkerRunning, sessionName)

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
	if stateStore != nil {
		if outcomeErr := stateStore.SetOutcome(outcome); outcomeErr != nil {
			if policyErr := applyFailurePolicy(cmd, stepRunState, outcomeErr); policyErr != nil {
				return policyErr
			}
		}
		recordStateCheckpoint(cmd, stateStore, beadID, "worker_completed", runstate.PhaseWorkerCompleted, outcome)
	}

	// Step 5b: Squire completion check
	if spawnErr == nil && !result.TimedOut && squireEnabled() {
		diff := ecosystem.CaptureGitDiff(repo)
		verdict, sqErr := squire.Check(task, diff, result.Output)
		if sqErr != nil {
			cmd.Printf("  Squire: error (proceeding): %v\n", sqErr)
		} else if verdict != nil {
			if tr != nil {
				tr.Emit(trace.Event{
					EventType: "squire_verdict",
					Output:    verdict.Reasoning,
					Pass:      &verdict.Complete,
				})
			}
			if !verdict.Complete && verdict.FollowUp != "" {
				cmd.Printf("  Squire: INCOMPLETE — %s\n", verdict.Reasoning)
				cmd.Printf("  Squire: Re-engaging agent...\n")
				if sendErr := worker.SendFollowUp(sessionName, verdict.FollowUp); sendErr != nil {
					cmd.Printf("  Squire: follow-up failed: %v\n", sendErr)
				} else {
					retryOutput := worker.WaitForCompletion(sessionName, 5*time.Minute)
					result.Output = retryOutput
				}
			} else {
				cmd.Printf("  Squire: COMPLETE — %s\n", verdict.Reasoning)
			}
		}
	}

	// Step 6: Gate check
	var gateResult *ecosystem.GateResult
	gateSkipped := false
	if spawnErr == nil {
		gate, gateErr := ecosystem.GateCheck(repo, citizen)
		if gateErr != nil {
			if policyErr := applyFailurePolicy(cmd, stepGateCheck, gateErr); policyErr != nil {
				return policyErr
			}
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
			recordStateCheckpoint(cmd, stateStore, beadID, "gate_check", runstate.PhaseGateChecked, outcome)
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
		if shadowErr := tr.ShadowError(); shadowErr != nil {
			cmd.Printf("  Warning: spine dual-write failed — legacy trace retained: %v\n", shadowErr)
		}

		// Index the run for fast queries
		idx, idxErr := index.Open(workDir)
		if idxErr != nil {
			if policyErr := applyFailurePolicy(cmd, stepIndexOpen, idxErr); policyErr != nil {
				return policyErr
			}
		} else {
			if recErr := idx.Record(meta); recErr != nil {
				if policyErr := applyFailurePolicy(cmd, stepIndexRecord, recErr); policyErr != nil {
					return policyErr
				}
			} else {
				recordStateCheckpoint(cmd, stateStore, beadID, "index_record", runstate.PhaseIndexed, outcome)
			}
			idx.Close()
		}
	}

	// Step 8: Learning-loop feedback collection
	feedbackRecord := buildRunRecord(beadID, citizen, resolvedRuntime, outcome, durationS, gateResult, ctx)
	if fbErr := ecosystem.CollectFeedback(feedbackRecord, workDir); fbErr != nil {
		if policyErr := applyFailurePolicy(cmd, stepFeedbackCollect, fbErr); policyErr != nil {
			return policyErr
		}
	} else {
		recordStateEffect(cmd, stateStore, beadID, "feedback_collect", "feedback collected")
		recordStateCheckpoint(cmd, stateStore, beadID, "feedback_collect", runstate.PhaseFeedbackDone, "feedback collected")
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
			if policyErr := applyFailurePolicy(cmd, stepLoopIngest, ingestErr); policyErr != nil {
				return policyErr
			}
		} else {
			recordStateEffect(cmd, stateStore, beadID, "loop_ingest", outcome)
			recordStateCheckpoint(cmd, stateStore, beadID, "loop_ingest", runstate.PhaseLoopIngested, outcome)
		}
	}

	// Step 9: Record citizen experience
	if expErr := workctx.AppendCitizenExperience(workDir, citizen, task, outcome, beadID); expErr != nil {
		if policyErr := applyFailurePolicy(cmd, stepExperienceAppend, expErr); policyErr != nil {
			return policyErr
		}
	} else {
		recordStateEffect(cmd, stateStore, beadID, "experience_append", outcome)
		recordStateCheckpoint(cmd, stateStore, beadID, "experience_append", runstate.PhaseExperienceDone, outcome)
	}

	// Step 10: Auto close reason from trace + diff
	closeReason := fmt.Sprintf("%s: %s", outcome, task)
	if tr != nil {
		cr, crErr := ecosystem.DeriveCloseReason(tr.FilePath(), repo)
		if crErr != nil {
			if policyErr := applyFailurePolicy(cmd, stepCloseReasonDerive, crErr); policyErr != nil {
				return policyErr
			}
		} else {
			closeReason = ecosystem.FormatCloseReason(cr)
			cmd.Printf("  Close reason: %s\n", closeReason)
		}
	}
	if stateStore != nil {
		if reasonErr := stateStore.SetCloseReason(closeReason); reasonErr != nil {
			if policyErr := applyFailurePolicy(cmd, stepRunState, reasonErr); policyErr != nil {
				return policyErr
			}
		}
		recordStateCheckpoint(cmd, stateStore, beadID, "close_reason", runstate.PhaseCloseReasonReady, closeReason)
	}

	// Step 11: Close bead with derived reason
	if bead != nil {
		if closeErr := ecosystem.BrClose(beadID, closeReason, repo); closeErr != nil {
			if policyErr := applyFailurePolicy(cmd, stepBrClose, closeErr); policyErr != nil {
				return policyErr
			}
		} else {
			recordStateEffect(cmd, stateStore, beadID, "br_close", closeReason)
			recordStateCheckpoint(cmd, stateStore, beadID, "bead_close", runstate.PhaseBeadClosed, closeReason)
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
	athenaRelayErr := ecosystem.RelaySend(citizen, "athena", summary, beadID, "task_result", relayPayload)
	if athenaRelayErr != nil {
		if policyErr := applyFailurePolicy(cmd, stepRelaySendAthena, athenaRelayErr); policyErr != nil {
			return policyErr
		}
	}
	if athenaRelayErr == nil {
		recordStateEffect(cmd, stateStore, beadID, "relay_send_athena", summary)
	}
	notifyRelayOK := notify == "" || notify == "athena"
	if notify != "" && notify != "athena" {
		notifyRelayErr := ecosystem.RelaySend(citizen, notify, summary, beadID, "task_result", relayPayload)
		if notifyRelayErr != nil {
			if policyErr := applyFailurePolicy(cmd, stepRelaySendNotify, notifyRelayErr); policyErr != nil {
				return policyErr
			}
		} else {
			notifyRelayOK = true
			recordStateEffect(cmd, stateStore, beadID, "relay_send_notify", notify)
		}
	}
	if athenaRelayErr == nil && notifyRelayOK {
		recordStateCheckpoint(cmd, stateStore, beadID, "notifications_sent", runstate.PhaseNotificationsSent, "relay notifications sent")
	}

	// Step 13: Agent state → idle
	agentIdleErr := ecosystem.BrAgentState(citizen, "idle")
	if policyErr := applyFailurePolicy(cmd, stepBrAgentIdle, agentIdleErr); policyErr != nil {
		return policyErr
	}
	if agentIdleErr == nil {
		recordStateEffect(cmd, stateStore, beadID, "br_agent_state_idle", "agent returned to idle")
		recordStateCheckpoint(cmd, stateStore, beadID, "agent_idle", runstate.PhaseAgentIdle, outcome)
	}
	if stateStore != nil && agentIdleErr == nil {
		if completeErr := stateStore.Complete(outcome); completeErr != nil {
			if policyErr := applyFailurePolicy(cmd, stepRunState, completeErr); policyErr != nil {
				return policyErr
			}
		}
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

func squireEnabled() bool {
	return os.Getenv("WORK_SQUIRE") == "1"
}

func randomID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func checkpointKey(beadID, step string, attempt int) string {
	return fmt.Sprintf("%s:%s:%d", beadID, step, attempt)
}

func recordStateCheckpoint(cmd *cobra.Command, store *runstate.Store, beadID, step, phase, note string) {
	if store == nil {
		return
	}
	attempt := stateAttempt(store)
	if err := store.Checkpoint(step, phase, checkpointKey(beadID, step, attempt), note); err != nil {
		_ = applyFailurePolicy(cmd, stepRunState, err)
	}
}

func recordStateEffect(cmd *cobra.Command, store *runstate.Store, beadID, effect, note string) {
	if store == nil {
		return
	}
	attempt := stateAttempt(store)
	if err := store.MarkEffect(effect, checkpointKey(beadID, effect, attempt), note); err != nil {
		_ = applyFailurePolicy(cmd, stepRunState, err)
	}
}

func stateAttempt(store *runstate.Store) int {
	if store == nil {
		return 1
	}
	state, err := store.Load()
	if err != nil || state.Attempt <= 0 {
		return 1
	}
	return state.Attempt
}

// lintBeforeDispatch validates the task (or bead) quality before starting work.
// If task looks like a bead ID (prefix-suffix), fetches the bead and lints it.
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
