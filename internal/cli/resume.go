package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	workctx "polis/work/internal/context"
	"polis/work/internal/ecosystem"
	"polis/work/internal/index"
	"polis/work/internal/runstate"
	"polis/work/internal/trace"

	"github.com/spf13/cobra"
)

func newResumeCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "resume <bead-id>",
		Short: "Resume the latest unfinished work run for a bead",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("get home dir: %w", err)
			}
			workDir := filepath.Join(home, ".work")

			store, state, err := runstate.LatestUnfinishedByBead(workDir, args[0])
			if err != nil {
				return fmt.Errorf("load unfinished run: %w", err)
			}
			if err := validateResumeState(state); err != nil {
				return fmt.Errorf("validate resume state: %w", err)
			}

			cmd.Printf("Resuming: %s\n", state.BeadID)
			cmd.Printf("  Run: %s | Phase: %s | Attempt: %d\n", state.RunID, state.Phase, state.Attempt)

			leaseTTL := 35 * time.Minute
			if state.DeadlineSeconds > 0 {
				leaseTTL = time.Duration(state.DeadlineSeconds)*time.Second + 5*time.Minute
			}
			leaseErr := store.AcquireLease(state.Citizen, leaseTTL)
			if leaseErr != nil {
				if force && leaseErr == runstate.ErrFreshLease {
					if err := store.StealLease(state.Citizen, leaseTTL); err != nil {
						return fmt.Errorf("steal lease: %w", err)
					}
				} else {
					return fmt.Errorf("acquire lease: %w", leaseErr)
				}
			}
			defer store.ReleaseLease()

			state, err = store.BeginResume("work resume")
			if err != nil {
				return fmt.Errorf("begin resume: %w", err)
			}
			return resumePostWorker(cmd, workDir, store, state)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "steal an active lease for the latest unfinished run")
	return cmd
}

func validateResumeState(state runstate.State) error {
	if hasCheckpoint(state, "worker_completed") {
		return nil
	}
	return fmt.Errorf("resume from phase %s is not supported yet; only post-worker recovery is enabled", state.Phase)
}

func resumePostWorker(cmd *cobra.Command, workDir string, store *runstate.Store, state runstate.State) error {
	meta, gatePass, gateScore, err := loadResumeTraceData(workDir, state)
	if err != nil {
		recordResumeFailure(cmd, store, "trace_metadata", err)
		return fmt.Errorf("load trace metadata: %w", err)
	}
	resumeEnd, err := detachTrailingEnd(meta.FilePath, !hasCheckpoint(state, "gate_check"))
	if err != nil {
		recordResumeFailure(cmd, store, "trace_rewrite", err)
		return fmt.Errorf("prepare trace tail: %w", err)
	}

	outcome := state.Outcome
	if strings.TrimSpace(outcome) == "" {
		outcome = meta.Outcome
	}
	if outcome == "" {
		outcome = "error"
	}
	recordStateCheckpoint(cmd, store, state.BeadID, "worker_completed", runstate.PhaseWorkerCompleted, outcome)

	if !hasCheckpoint(state, "gate_check") {
		gate, gateErr := ecosystem.GateCheck(state.Repo, state.Citizen)
		if gateErr != nil {
			recordResumeFailure(cmd, store, stepGateCheck, gateErr)
			return fmt.Errorf("gate check: %w", gateErr)
		}
		if gate != nil {
			pass := gate.Pass
			score := gate.Score
			gatePass = &pass
			gateScore = &score
			if appendErr := appendTraceEvent(meta.FilePath, trace.Event{
				EventType: "gate_result",
				TraceID:   meta.TraceID,
				SessionID: meta.SessionID,
				RunID:     meta.RunID,
				Pass:      &pass,
				Score:     &score,
			}); appendErr != nil {
				recordResumeFailure(cmd, store, "trace_append_gate", appendErr)
				return appendErr
			}
			if !gate.Pass {
				outcome = "gate_fail"
			}
			recordStateCheckpoint(cmd, store, state.BeadID, "gate_check", runstate.PhaseGateChecked, outcome)
		}
	}

	if !hasCheckpoint(state, "index_record") {
		idx, idxErr := index.Open(workDir)
		if idxErr != nil {
			recordResumeFailure(cmd, store, stepIndexOpen, idxErr)
			return fmt.Errorf("open index: %w", idxErr)
		}
		meta.Outcome = outcome
		if recErr := idx.Record(meta); recErr != nil {
			idx.Close()
			recordResumeFailure(cmd, store, stepIndexRecord, recErr)
			return fmt.Errorf("index record: %w", recErr)
		}
		idx.Close()
		recordStateCheckpoint(cmd, store, state.BeadID, "index_record", runstate.PhaseIndexed, outcome)
	}

	if !hasCheckpoint(state, "feedback_collect") {
		if hasEffect(state, "feedback_collect") || feedbackExists(workDir, state.BeadID) {
			recordStateEffect(cmd, store, state.BeadID, "feedback_collect", "feedback already collected")
			recordStateCheckpoint(cmd, store, state.BeadID, "feedback_collect", runstate.PhaseFeedbackDone, "feedback already collected")
		} else {
			record := buildRunRecord(state.BeadID, state.Citizen, state.Runtime, outcome, meta.DurationS, gateResultFromResume(gatePass, gateScore), nil)
			record.Attempt = state.Attempt
			if fbErr := ecosystem.CollectFeedback(record, workDir); fbErr != nil {
				recordResumeFailure(cmd, store, stepFeedbackCollect, fbErr)
				return fmt.Errorf("feedback collect: %w", fbErr)
			}
			recordStateEffect(cmd, store, state.BeadID, "feedback_collect", "feedback collected")
			recordStateCheckpoint(cmd, store, state.BeadID, "feedback_collect", runstate.PhaseFeedbackDone, "feedback collected")
		}
	}

	if !hasCheckpoint(state, "loop_ingest") {
		if hasEffect(state, "loop_ingest") {
			recordStateCheckpoint(cmd, store, state.BeadID, "loop_ingest", runstate.PhaseLoopIngested, "loop ingest already committed")
		} else {
			if ingestErr := ecosystem.IngestRun(state.BeadID, state.Task, outcome, state.Citizen, meta.DurationS, gatePass != nil && *gatePass, gatePass != nil && *gatePass, nil, resumeErrorMessage(state)); ingestErr != nil {
				recordResumeFailure(cmd, store, stepLoopIngest, ingestErr)
				return fmt.Errorf("loop ingest: %w", ingestErr)
			}
			recordStateEffect(cmd, store, state.BeadID, "loop_ingest", outcome)
			recordStateCheckpoint(cmd, store, state.BeadID, "loop_ingest", runstate.PhaseLoopIngested, outcome)
		}
	}

	if !hasCheckpoint(state, "experience_append") {
		if hasEffect(state, "experience_append") || citizenExperienceExists(workDir, state.Citizen, state.BeadID) {
			recordStateEffect(cmd, store, state.BeadID, "experience_append", "experience already appended")
			recordStateCheckpoint(cmd, store, state.BeadID, "experience_append", runstate.PhaseExperienceDone, "experience already appended")
		} else {
			if expErr := workctx.AppendCitizenExperience(workDir, state.Citizen, state.Task, outcome, state.BeadID); expErr != nil {
				recordResumeFailure(cmd, store, stepExperienceAppend, expErr)
				return fmt.Errorf("experience append: %w", expErr)
			}
			recordStateEffect(cmd, store, state.BeadID, "experience_append", outcome)
			recordStateCheckpoint(cmd, store, state.BeadID, "experience_append", runstate.PhaseExperienceDone, outcome)
		}
	}

	closeReason := state.CloseReason
	if !hasCheckpoint(state, "close_reason") {
		if strings.TrimSpace(closeReason) == "" {
			cr, crErr := ecosystem.DeriveCloseReason(meta.FilePath, state.Repo)
			if crErr != nil {
				recordResumeFailure(cmd, store, stepCloseReasonDerive, crErr)
				return fmt.Errorf("derive close reason: %w", crErr)
			}
			closeReason = ecosystem.FormatCloseReason(cr)
		}
		if err := store.SetCloseReason(closeReason); err != nil {
			recordResumeFailure(cmd, store, stepRunState, err)
			return fmt.Errorf("persist close reason: %w", err)
		}
		recordStateCheckpoint(cmd, store, state.BeadID, "close_reason", runstate.PhaseCloseReasonReady, closeReason)
	}

	if shouldCloseBead(state) && !hasCheckpoint(state, "bead_close") {
		if !hasEffect(state, "br_close") {
			if closeErr := ecosystem.BrClose(state.BeadID, closeReason, state.Repo); closeErr != nil {
				recordResumeFailure(cmd, store, stepBrClose, closeErr)
				return fmt.Errorf("close bead: %w", closeErr)
			}
			recordStateEffect(cmd, store, state.BeadID, "br_close", closeReason)
		}
		recordStateCheckpoint(cmd, store, state.BeadID, "bead_close", runstate.PhaseBeadClosed, closeReason)
	}

	meta, gatePass, gateScore, err = ensureResumeTraceEnd(workDir, state, meta, gatePass, gateScore, outcome, resumeEnd)
	if err != nil {
		recordResumeFailure(cmd, store, "trace_close", err)
		return err
	}

	if !hasCheckpoint(state, "notifications_sent") {
		summary := fmt.Sprintf("[%s] %s — %s (%s)", state.BeadID, state.Task, outcome, closeReason)
		gateScoreStr := "null"
		if gateScore != nil {
			gateScoreStr = fmt.Sprintf("%.2f", *gateScore)
		}
		relayPayload := fmt.Sprintf(`{"bead_id":"%s","outcome":"%s","gate_score":%s,"duration":"%ds"}`, state.BeadID, outcome, gateScoreStr, meta.DurationS)
		if !hasEffect(state, "relay_send_athena") {
			if relayErr := ecosystem.RelaySend(state.Citizen, "athena", summary, state.BeadID, "task_result", relayPayload); relayErr != nil {
				recordResumeFailure(cmd, store, stepRelaySendAthena, relayErr)
				return fmt.Errorf("relay send athena: %w", relayErr)
			}
			recordStateEffect(cmd, store, state.BeadID, "relay_send_athena", summary)
		}
		if state.Notify != "" && state.Notify != "athena" && !hasEffect(state, "relay_send_notify") {
			if relayErr := ecosystem.RelaySend(state.Citizen, state.Notify, summary, state.BeadID, "task_result", relayPayload); relayErr != nil {
				recordResumeFailure(cmd, store, stepRelaySendNotify, relayErr)
				return fmt.Errorf("relay send notify: %w", relayErr)
			}
			recordStateEffect(cmd, store, state.BeadID, "relay_send_notify", state.Notify)
		}
		recordStateCheckpoint(cmd, store, state.BeadID, "notifications_sent", runstate.PhaseNotificationsSent, "relay notifications sent")
	}

	if !hasCheckpoint(state, "agent_idle") {
		if idleErr := ecosystem.BrAgentState(state.Citizen, "idle"); idleErr != nil {
			recordResumeFailure(cmd, store, stepBrAgentIdle, idleErr)
			return fmt.Errorf("agent idle: %w", idleErr)
		}
		recordStateEffect(cmd, store, state.BeadID, "br_agent_state_idle", "agent returned to idle")
		recordStateCheckpoint(cmd, store, state.BeadID, "agent_idle", runstate.PhaseAgentIdle, outcome)
	}

	if err := store.Complete(outcome); err != nil {
		recordResumeFailure(cmd, store, stepRunState, err)
		return fmt.Errorf("complete resumed run: %w", err)
	}

	cmd.Printf("Done: %s [%s]\n", state.BeadID, outcome)
	return nil
}

func hasCheckpoint(state runstate.State, step string) bool {
	_, ok := state.CompletedSteps[step]
	return ok
}

func hasEffect(state runstate.State, effect string) bool {
	_, ok := state.CompletedEffects[effect]
	return ok
}

func loadResumeTraceData(workDir string, state runstate.State) (trace.Metadata, *bool, *float64, error) {
	tracePath := state.TracePath
	if strings.TrimSpace(tracePath) == "" {
		var err error
		tracePath, err = findTracePath(workDir, state.BeadID)
		if err != nil {
			return trace.Metadata{}, nil, nil, fmt.Errorf("find trace path: %w", err)
		}
	}
	events, err := trace.ReadTrace(tracePath)
	if err != nil {
		return trace.Metadata{}, nil, nil, fmt.Errorf("read trace: %w", err)
	}
	if len(events) == 0 {
		return trace.Metadata{}, nil, nil, fmt.Errorf("empty trace: %s", tracePath)
	}

	var begin, end *trace.Event
	var gatePass *bool
	var gateScore *float64
	for i := range events {
		switch events[i].EventType {
		case "begin":
			begin = &events[i]
		case "end":
			end = &events[i]
		case "gate_result":
			if events[i].Pass != nil {
				pass := *events[i].Pass
				gatePass = &pass
			}
			if events[i].Score != nil {
				score := *events[i].Score
				gateScore = &score
			}
		}
	}
	if begin == nil {
		return trace.Metadata{}, nil, nil, fmt.Errorf("trace missing begin event")
	}

	meta := trace.Metadata{
		BeadID:    begin.Bead,
		TraceID:   begin.TraceID,
		SessionID: begin.SessionID,
		RunID:     begin.RunID,
		Agent:     begin.Agent,
		Task:      begin.Task,
		FilePath:  tracePath,
	}
	if ts, err := time.Parse(time.RFC3339, begin.Timestamp); err == nil {
		meta.StartTime = ts
	}
	if end != nil {
		meta.Outcome = end.Outcome
		if end.DurationS != nil {
			meta.DurationS = *end.DurationS
		}
		if ts, err := time.Parse(time.RFC3339, end.Timestamp); err == nil {
			meta.EndTime = ts
		}
	}
	return meta, gatePass, gateScore, nil
}

func appendTraceEvent(tracePath string, event trace.Event) error {
	if event.Timestamp == "" {
		event.Timestamp = time.Now().Format(time.RFC3339)
	}
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal trace event: %w", err)
	}
	f, err := os.OpenFile(tracePath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open trace append: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("append trace event: %w", err)
	}
	return nil
}

func hasTraceEnd(tracePath string) bool {
	events, err := trace.ReadTrace(tracePath)
	if err != nil {
		return false
	}
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].EventType == "end" {
			return true
		}
	}
	return false
}

func detachTrailingEnd(tracePath string, enabled bool) (*trace.Event, error) {
	if !enabled {
		return nil, nil
	}
	events, err := trace.ReadTrace(tracePath)
	if err != nil {
		return nil, fmt.Errorf("read trace: %w", err)
	}
	if len(events) == 0 || events[len(events)-1].EventType != "end" {
		return nil, nil
	}
	end := events[len(events)-1]
	if err := rewriteTraceEvents(tracePath, events[:len(events)-1]); err != nil {
		return nil, err
	}
	return &end, nil
}

func rewriteTraceEvents(tracePath string, events []trace.Event) error {
	f, err := os.OpenFile(tracePath, os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open trace rewrite: %w", err)
	}
	defer f.Close()
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("marshal trace event: %w", err)
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("rewrite trace event: %w", err)
		}
	}
	return nil
}

func durationSince(start time.Time) *int64 {
	if start.IsZero() {
		return nil
	}
	seconds := int64(time.Since(start).Seconds())
	if seconds < 0 {
		seconds = 0
	}
	return &seconds
}

func ensureResumeTraceEnd(workDir string, state runstate.State, meta trace.Metadata, gatePass *bool, gateScore *float64, outcome string, end *trace.Event) (trace.Metadata, *bool, *float64, error) {
	if end == nil && hasTraceEnd(meta.FilePath) {
		return meta, gatePass, gateScore, nil
	}
	if end == nil {
		end = &trace.Event{
			EventType: "end",
			TraceID:   meta.TraceID,
			SessionID: meta.SessionID,
			RunID:     meta.RunID,
			Agent:     meta.Agent,
			Bead:      state.BeadID,
		}
	}
	end.Outcome = outcome
	end.Error = resumeErrorMessage(state)
	if end.DurationS == nil {
		end.DurationS = durationSince(meta.StartTime)
	}
	if end.Timestamp == "" {
		end.Timestamp = time.Now().Format(time.RFC3339)
	}
	if err := appendTraceEvent(meta.FilePath, *end); err != nil {
		return trace.Metadata{}, nil, nil, fmt.Errorf("append end event: %w", err)
	}
	updatedMeta, updatedPass, updatedScore, err := loadResumeTraceData(workDir, state)
	if err != nil {
		return trace.Metadata{}, nil, nil, fmt.Errorf("reload trace metadata: %w", err)
	}
	return updatedMeta, updatedPass, updatedScore, nil
}

func gateResultFromResume(pass *bool, score *float64) *ecosystem.GateResult {
	if pass == nil {
		return nil
	}
	result := &ecosystem.GateResult{Pass: *pass}
	if score != nil {
		result.Score = *score
	}
	return result
}

func resumeErrorMessage(state runstate.State) string {
	if state.LastError == nil {
		return ""
	}
	return state.LastError.Message
}

func shouldCloseBead(state runstate.State) bool {
	if state.BeadManaged {
		return true
	}
	if hasEffect(state, "local_bead_alloc") {
		return false
	}
	if hasEffect(state, "br_create") {
		return true
	}
	return !strings.HasPrefix(state.BeadID, "work-")
}

func feedbackExists(workDir, beadID string) bool {
	_, err := os.Stat(filepath.Join(workDir, "feedback", beadID+".json"))
	return err == nil
}

func citizenExperienceExists(workDir, citizen, beadID string) bool {
	data, err := os.ReadFile(filepath.Join(workDir, "citizens", citizen+".md"))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), beadID)
}

func recordResumeFailure(cmd *cobra.Command, store *runstate.Store, step string, err error) {
	if store == nil || err == nil {
		return
	}
	if recErr := store.RecordFailure(step, err); recErr != nil {
		_ = applyFailurePolicy(cmd, stepRunState, recErr)
	}
}
