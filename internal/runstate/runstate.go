package runstate

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"polis/work/internal/spine"
)

const (
	SchemaVersion = 1

	PhaseInitialized       = "initialized"
	PhaseBeadCreated       = "bead_created"
	PhaseContextReady      = "context_ready"
	PhasePromptReady       = "prompt_ready"
	PhaseTraceOpen         = "trace_open"
	PhaseWorkerRunning     = "worker_running"
	PhaseWorkerCompleted   = "worker_completed"
	PhaseGateChecked       = "gate_checked"
	PhaseIndexed           = "indexed"
	PhaseFeedbackDone      = "feedback_done"
	PhaseLoopIngested      = "loop_ingested"
	PhaseExperienceDone    = "experience_done"
	PhaseCloseReasonReady  = "close_reason_ready"
	PhaseBeadClosed        = "bead_closed"
	PhaseNotificationsSent = "notifications_sent"
	PhaseAgentIdle         = "agent_idle"
	PhaseCompleted         = "completed"
	PhaseFailed            = "failed"
)

var ErrFreshLease = errors.New("run has an active lease")

type Config struct {
	RunID           string
	TraceID         string
	SessionID       string
	BeadID          string
	BeadManaged     bool
	Task            string
	Citizen         string
	Repo            string
	Runtime         string
	Model           string
	Notify          string
	DeadlineSeconds int64
	TmuxSession     string
}

type Checkpoint struct {
	At             string `json:"at"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	Note           string `json:"note,omitempty"`
}

type RecordedError struct {
	Step    string `json:"step"`
	Message string `json:"message"`
}

type State struct {
	Version          int                   `json:"version"`
	RunID            string                `json:"run_id"`
	TraceID          string                `json:"trace_id"`
	SessionID        string                `json:"session_id"`
	BeadID           string                `json:"bead_id"`
	BeadManaged      bool                  `json:"bead_managed,omitempty"`
	Task             string                `json:"task"`
	Citizen          string                `json:"citizen"`
	Repo             string                `json:"repo"`
	Runtime          string                `json:"runtime"`
	Model            string                `json:"model"`
	Notify           string                `json:"notify,omitempty"`
	DeadlineSeconds  int64                 `json:"deadline_seconds,omitempty"`
	Phase            string                `json:"phase"`
	Attempt          int                   `json:"attempt"`
	TmuxSession      string                `json:"tmux_session,omitempty"`
	TracePath        string                `json:"trace_path,omitempty"`
	PromptPath       string                `json:"prompt_path,omitempty"`
	Outcome          string                `json:"outcome,omitempty"`
	CloseReason      string                `json:"close_reason,omitempty"`
	StartedAt        string                `json:"started_at"`
	UpdatedAt        string                `json:"updated_at"`
	CompletedSteps   map[string]Checkpoint `json:"completed_steps,omitempty"`
	CompletedEffects map[string]Checkpoint `json:"completed_effects,omitempty"`
	LastError        *RecordedError        `json:"last_error,omitempty"`
}

type Lease struct {
	Holder     string `json:"holder"`
	Hostname   string `json:"hostname"`
	PID        int    `json:"pid"`
	AcquiredAt string `json:"acquired_at"`
	ExpiresAt  string `json:"expires_at"`
}

type JournalEntry struct {
	TS      string         `json:"ts"`
	Kind    string         `json:"kind"`
	Step    string         `json:"step,omitempty"`
	Phase   string         `json:"phase,omitempty"`
	Attempt int            `json:"attempt,omitempty"`
	Note    string         `json:"note,omitempty"`
	Error   *RecordedError `json:"error,omitempty"`
}

type Store struct {
	workDir string
	runDir  string
	runID   string
}

func Create(workDir string, cfg Config) (*Store, error) {
	if cfg.BeadID == "" {
		return nil, fmt.Errorf("bead id required")
	}
	if cfg.RunID == "" {
		cfg.RunID = spine.MintRunID()
	}
	if cfg.TraceID == "" {
		cfg.TraceID = spine.MintULID()
	}
	if cfg.SessionID == "" {
		cfg.SessionID = spine.MintSessionID()
	}

	s := &Store{
		workDir: workDir,
		runDir:  filepath.Join(workDir, "runs", cfg.RunID),
		runID:   cfg.RunID,
	}
	if err := os.MkdirAll(s.runDir, 0o755); err != nil {
		return nil, fmt.Errorf("create run dir: %w", err)
	}

	now := nowRFC3339()
	state := State{
		Version:          SchemaVersion,
		RunID:            cfg.RunID,
		TraceID:          cfg.TraceID,
		SessionID:        cfg.SessionID,
		BeadID:           cfg.BeadID,
		BeadManaged:      cfg.BeadManaged,
		Task:             cfg.Task,
		Citizen:          cfg.Citizen,
		Repo:             cfg.Repo,
		Runtime:          cfg.Runtime,
		Model:            cfg.Model,
		Notify:           cfg.Notify,
		DeadlineSeconds:  cfg.DeadlineSeconds,
		Phase:            PhaseInitialized,
		Attempt:          1,
		TmuxSession:      cfg.TmuxSession,
		StartedAt:        now,
		UpdatedAt:        now,
		CompletedSteps:   map[string]Checkpoint{},
		CompletedEffects: map[string]Checkpoint{},
	}
	if err := atomicWriteJSON(s.statePath(), state); err != nil {
		return nil, err
	}
	if err := s.appendJournal(JournalEntry{
		TS:      now,
		Kind:    "run_created",
		Phase:   state.Phase,
		Attempt: state.Attempt,
	}); err != nil {
		return nil, err
	}
	return s, nil
}

func Open(workDir, runID string) (*Store, error) {
	s := &Store{
		workDir: workDir,
		runDir:  filepath.Join(workDir, "runs", runID),
		runID:   runID,
	}
	if _, err := os.Stat(s.statePath()); err != nil {
		return nil, fmt.Errorf("open run state: %w", err)
	}
	return s, nil
}

func LatestUnfinishedByBead(workDir, beadID string) (*Store, State, error) {
	runsDir := filepath.Join(workDir, "runs")
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, State{}, fmt.Errorf("no run state for bead %s", beadID)
		}
		return nil, State{}, fmt.Errorf("read runs dir: %w", err)
	}

	var newest State
	var newestStore *Store
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		store, err := Open(workDir, entry.Name())
		if err != nil {
			continue
		}
		state, err := store.Load()
		if err != nil || state.BeadID != beadID || state.Phase == PhaseCompleted {
			continue
		}
		if newestStore == nil || state.UpdatedAt > newest.UpdatedAt {
			newest = state
			newestStore = store
		}
	}
	if newestStore == nil {
		return nil, State{}, fmt.Errorf("no unfinished run for bead %s", beadID)
	}
	return newestStore, newest, nil
}

func (s *Store) RunID() string {
	return s.runID
}

func (s *Store) RunDir() string {
	return s.runDir
}

func (s *Store) Load() (State, error) {
	var state State
	data, err := os.ReadFile(s.statePath())
	if err != nil {
		return state, fmt.Errorf("read state: %w", err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("parse state: %w", err)
	}
	return state, nil
}

func (s *Store) Update(mutator func(*State) error) error {
	state, err := s.Load()
	if err != nil {
		return err
	}
	if err := mutator(&state); err != nil {
		return err
	}
	state.UpdatedAt = nowRFC3339()
	return atomicWriteJSON(s.statePath(), state)
}

func (s *Store) WritePrompt(prompt string) error {
	if err := atomicWriteFile(s.promptPath(), []byte(prompt)); err != nil {
		return err
	}
	return s.Update(func(state *State) error {
		state.PromptPath = s.promptPath()
		return nil
	})
}

func (s *Store) ReadPrompt() (string, error) {
	data, err := os.ReadFile(s.promptPath())
	if err != nil {
		return "", fmt.Errorf("read prompt: %w", err)
	}
	return string(data), nil
}

func (s *Store) Checkpoint(step, phase, idempotencyKey, note string) error {
	now := nowRFC3339()
	var attempt int
	if err := s.Update(func(state *State) error {
		if state.CompletedSteps == nil {
			state.CompletedSteps = map[string]Checkpoint{}
		}
		attempt = state.Attempt
		state.Phase = phase
		state.CompletedSteps[step] = Checkpoint{
			At:             now,
			IdempotencyKey: idempotencyKey,
			Note:           note,
		}
		return nil
	}); err != nil {
		return err
	}
	return s.appendJournal(JournalEntry{
		TS:      now,
		Kind:    "checkpoint",
		Step:    step,
		Phase:   phase,
		Attempt: attempt,
		Note:    note,
	})
}

func (s *Store) MarkEffect(effect, idempotencyKey, note string) error {
	now := nowRFC3339()
	if err := s.Update(func(state *State) error {
		if state.CompletedEffects == nil {
			state.CompletedEffects = map[string]Checkpoint{}
		}
		state.CompletedEffects[effect] = Checkpoint{
			At:             now,
			IdempotencyKey: idempotencyKey,
			Note:           note,
		}
		return nil
	}); err != nil {
		return err
	}
	return s.appendJournal(JournalEntry{
		TS:   now,
		Kind: "effect_committed",
		Step: effect,
		Note: note,
	})
}

func (s *Store) SetTracePath(path string) error {
	return s.Update(func(state *State) error {
		state.TracePath = path
		return nil
	})
}

func (s *Store) SetOutcome(outcome string) error {
	return s.Update(func(state *State) error {
		state.Outcome = outcome
		return nil
	})
}

func (s *Store) SetCloseReason(reason string) error {
	return s.Update(func(state *State) error {
		state.CloseReason = reason
		return nil
	})
}

func (s *Store) RecordFailure(step string, runErr error) error {
	now := nowRFC3339()
	if err := s.Update(func(state *State) error {
		state.Phase = PhaseFailed
		state.LastError = &RecordedError{
			Step:    step,
			Message: runErr.Error(),
		}
		return nil
	}); err != nil {
		return err
	}
	return s.appendJournal(JournalEntry{
		TS:    now,
		Kind:  "failed",
		Step:  step,
		Phase: PhaseFailed,
		Error: &RecordedError{
			Step:    step,
			Message: runErr.Error(),
		},
	})
}

func (s *Store) Complete(outcome string) error {
	now := nowRFC3339()
	if err := s.Update(func(state *State) error {
		state.Phase = PhaseCompleted
		state.Outcome = outcome
		return nil
	}); err != nil {
		return err
	}
	return s.appendJournal(JournalEntry{
		TS:    now,
		Kind:  "completed",
		Phase: PhaseCompleted,
		Note:  outcome,
	})
}

func (s *Store) AcquireLease(holder string, ttl time.Duration) error {
	lease, hasLease, err := s.readLeaseIfPresent()
	if err != nil {
		return fmt.Errorf("read current lease: %w", err)
	}
	if hasLease {
		expiry, parseErr := time.Parse(time.RFC3339, lease.ExpiresAt)
		if parseErr == nil && time.Now().Before(expiry) {
			return ErrFreshLease
		}
	}

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("resolve hostname: %w", err)
	}
	now := time.Now().UTC()
	lease = Lease{
		Holder:     holder,
		Hostname:   hostname,
		PID:        os.Getpid(),
		AcquiredAt: now.Format(time.RFC3339),
		ExpiresAt:  now.Add(ttl).Format(time.RFC3339),
	}
	if err := atomicWriteJSON(s.leasePath(), lease); err != nil {
		return err
	}
	return s.appendJournal(JournalEntry{
		TS:   now.Format(time.RFC3339),
		Kind: "lease_acquired",
		Note: holder,
	})
}

func (s *Store) StealLease(holder string, ttl time.Duration) error {
	prior, hasPrior, err := s.readLeaseIfPresent()
	if err != nil {
		return fmt.Errorf("read current lease: %w", err)
	}
	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("resolve hostname: %w", err)
	}
	now := time.Now().UTC()
	lease := Lease{
		Holder:     holder,
		Hostname:   hostname,
		PID:        os.Getpid(),
		AcquiredAt: now.Format(time.RFC3339),
		ExpiresAt:  now.Add(ttl).Format(time.RFC3339),
	}
	if err := atomicWriteJSON(s.leasePath(), lease); err != nil {
		return err
	}
	note := holder
	if hasPrior && prior.Holder != "" {
		note = fmt.Sprintf("%s (stole from %s pid=%d host=%s)", holder, prior.Holder, prior.PID, prior.Hostname)
	}
	return s.appendJournal(JournalEntry{
		TS:   now.Format(time.RFC3339),
		Kind: "lease_stolen",
		Note: note,
	})
}

func (s *Store) ReleaseLease() error {
	if err := os.Remove(s.leasePath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("release lease: %w", err)
	}
	return s.appendJournal(JournalEntry{
		TS:   nowRFC3339(),
		Kind: "lease_released",
	})
}

func (s *Store) BeginResume(note string) (State, error) {
	var updated State
	now := nowRFC3339()
	if err := s.Update(func(state *State) error {
		state.Attempt++
		state.UpdatedAt = now
		state.LastError = nil
		updated = *state
		return nil
	}); err != nil {
		return State{}, err
	}
	if err := s.appendJournal(JournalEntry{
		TS:      now,
		Kind:    "resume_started",
		Phase:   updated.Phase,
		Attempt: updated.Attempt,
		Note:    note,
	}); err != nil {
		return State{}, err
	}
	return updated, nil
}

func (s *Store) ReadJournal() ([]JournalEntry, error) {
	f, err := os.Open(s.journalPath())
	if err != nil {
		return nil, fmt.Errorf("open journal: %w", err)
	}
	defer f.Close()

	var entries []JournalEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		var entry JournalEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, fmt.Errorf("parse journal entry: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan journal: %w", err)
	}
	return entries, nil
}

func (s *Store) appendJournal(entry JournalEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal journal entry: %w", err)
	}
	f, err := os.OpenFile(s.journalPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open journal: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("append journal: %w", err)
	}
	return nil
}

func (s *Store) readLease() (Lease, error) {
	var lease Lease
	data, err := os.ReadFile(s.leasePath())
	if err != nil {
		return lease, fmt.Errorf("read lease: %w", err)
	}
	if err := json.Unmarshal(data, &lease); err != nil {
		return lease, fmt.Errorf("parse lease: %w", err)
	}
	return lease, nil
}

func (s *Store) readLeaseIfPresent() (Lease, bool, error) {
	lease, err := s.readLease()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Lease{}, false, nil
		}
		return Lease{}, false, err
	}
	return lease, true, nil
}

func (s *Store) statePath() string {
	return filepath.Join(s.runDir, "state.json")
}

func (s *Store) leasePath() string {
	return filepath.Join(s.runDir, "lease.json")
}

func (s *Store) journalPath() string {
	return filepath.Join(s.runDir, "journal.jsonl")
}

func (s *Store) promptPath() string {
	return filepath.Join(s.runDir, "prompt.txt")
}

func atomicWriteJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", filepath.Base(path), err)
	}
	data = append(data, '\n')
	return atomicWriteFile(path, data)
}

func atomicWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create dir for %s: %w", filepath.Base(path), err)
	}
	tmp, err := os.CreateTemp(dir, ".work-runstate-*")
	if err != nil {
		return fmt.Errorf("create temp for %s: %w", filepath.Base(path), err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp for %s: %w", filepath.Base(path), err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp for %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename temp for %s: %w", filepath.Base(path), err)
	}
	return nil
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
