// Package trace captures structured JSONL traces of worker runs.
//
// Schema (stable — other tools depend on this):
//
//	Event types: begin, end, tool_call, file_write, gate_result, worker_output, error
//	All events have: ts (RFC3339), event (type string)
//	begin: agent, task, bead
//	end: outcome (success|error|timeout|gate_fail), duration_s, agent, bead, error?
//	tool_call: tool, cmd, duration_ms?
//	file_write: path, lines?
//	gate_result: pass, score
//	worker_output: output (captured pane snapshot)
//	error: error, agent?, bead?
package trace

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"polis/work/internal/spine"
)

// Event represents a single trace event.
type Event struct {
	Timestamp  string   `json:"ts"`
	EventType  string   `json:"event"`
	TraceID    string   `json:"trace_id,omitempty"`
	SessionID  string   `json:"session_id,omitempty"`
	RunID      string   `json:"run_id,omitempty"`
	Model      string   `json:"model,omitempty"`
	Agent      string   `json:"agent,omitempty"`
	Task       string   `json:"task,omitempty"`
	Bead       string   `json:"bead,omitempty"`
	Tool       string   `json:"tool,omitempty"`
	Cmd        string   `json:"cmd,omitempty"`
	Path       string   `json:"path,omitempty"`
	Lines      *int     `json:"lines,omitempty"`
	Output     string   `json:"output,omitempty"`
	Outcome    string   `json:"outcome,omitempty"`
	Pass       *bool    `json:"pass,omitempty"`
	Score      *float64 `json:"score,omitempty"`
	DurationMs *int64   `json:"duration_ms,omitempty"`
	DurationS  *int64   `json:"duration_s,omitempty"`
	Error      string   `json:"error,omitempty"`
}

// Trace manages writing events to a JSONL file.
type Trace struct {
	mu        sync.Mutex
	file      *os.File
	filePath  string
	beadID    string
	agent     string
	task      string
	repo      string
	traceID   string
	sessionID string
	runID     string
	model     string
	started   time.Time
	spine     *spine.Writer
	shadowErr error
}

// Metadata holds trace summary info for indexing.
type Metadata struct {
	BeadID    string
	TraceID   string
	SessionID string
	RunID     string
	Agent     string
	Task      string
	StartTime time.Time
	EndTime   time.Time
	Outcome   string
	DurationS int64
	FilePath  string
}

type OpenOptions struct {
	Repo        string
	Model       string
	EnableSpine bool
	SpineDir    string
}

// Open creates a new trace file organized by date.
func Open(workDir, beadID, agent, task string) (*Trace, error) {
	return OpenWithOptions(workDir, beadID, agent, task, OpenOptions{})
}

// OpenWithOptions creates a new trace file and optionally dual-writes to the spine.
func OpenWithOptions(workDir, beadID, agent, task string, opts OpenOptions) (*Trace, error) {
	now := time.Now()
	dir := filepath.Join(workDir, "traces", now.Format("2006"), now.Format("01"), now.Format("02"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create trace dir: %w", err)
	}

	filename := fmt.Sprintf("trace-%s.jsonl", beadID)
	filePath := filepath.Join(dir, filename)
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open trace file: %w", err)
	}

	t := &Trace{
		file:      f,
		filePath:  filePath,
		beadID:    beadID,
		agent:     agent,
		task:      task,
		repo:      opts.Repo,
		traceID:   spine.MintULID(),
		sessionID: spine.MintSessionID(),
		runID:     spine.MintRunID(),
		model:     opts.Model,
		started:   now,
	}
	if opts.EnableSpine || spine.Enabled() {
		t.spine = spine.NewWriter(opts.SpineDir)
	}

	t.Emit(Event{
		EventType: "begin",
		Agent:     agent,
		Task:      task,
		Bead:      beadID,
	})

	return t, nil
}

// Emit writes an event to the trace.
func (t *Trace) Emit(e Event) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if e.Timestamp == "" {
		e.Timestamp = time.Now().Format(time.RFC3339)
	}
	if e.TraceID == "" {
		e.TraceID = t.traceID
	}
	if e.SessionID == "" {
		e.SessionID = t.sessionID
	}
	if e.RunID == "" {
		e.RunID = t.runID
	}
	if e.Model == "" {
		e.Model = t.model
	}

	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	if _, err := t.file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write event: %w", err)
	}
	if t.spine != nil {
		for _, env := range t.toSpineEnvelopes(e) {
			if err := t.spine.Write(env); err != nil && t.shadowErr == nil {
				t.shadowErr = err
			}
		}
	}
	return nil
}

// EmitToolCall is a convenience for recording tool/command usage.
func (t *Trace) EmitToolCall(tool, cmd string, durationMs int64) error {
	return t.Emit(Event{
		EventType:  "tool_call",
		Tool:       tool,
		Cmd:        cmd,
		DurationMs: &durationMs,
	})
}

// EmitFileWrite is a convenience for recording file writes.
func (t *Trace) EmitFileWrite(path string, lines int) error {
	return t.Emit(Event{
		EventType: "file_write",
		Path:      path,
		Lines:     &lines,
	})
}

// EmitWorkerOutput captures a pane snapshot.
func (t *Trace) EmitWorkerOutput(output string) error {
	return t.Emit(Event{
		EventType: "worker_output",
		Output:    output,
	})
}

// EmitError records an error event.
func (t *Trace) EmitError(errMsg string) error {
	return t.Emit(Event{
		EventType: "error",
		Error:     errMsg,
		Agent:     t.agent,
		Bead:      t.beadID,
	})
}

// Close writes the end event and closes the file.
func (t *Trace) Close(outcome string, runErr error) error {
	duration := int64(time.Since(t.started).Seconds())
	e := Event{
		EventType: "end",
		Outcome:   outcome,
		DurationS: &duration,
		Agent:     t.agent,
		Bead:      t.beadID,
	}
	if runErr != nil {
		e.Error = runErr.Error()
	}
	t.Emit(e)
	return t.file.Close()
}

// GetMetadata returns trace metadata for indexing.
func (t *Trace) GetMetadata(outcome string) Metadata {
	return Metadata{
		BeadID:    t.beadID,
		TraceID:   t.traceID,
		SessionID: t.sessionID,
		RunID:     t.runID,
		Agent:     t.agent,
		Task:      t.task,
		StartTime: t.started,
		EndTime:   time.Now(),
		Outcome:   outcome,
		DurationS: int64(time.Since(t.started).Seconds()),
		FilePath:  t.filePath,
	}
}

// FilePath returns the path to the trace file.
func (t *Trace) FilePath() string {
	return t.filePath
}

// BeadID returns the bead ID for this trace.
func (t *Trace) BeadID() string {
	return t.beadID
}

// ShadowError returns the first spine dual-write error observed during the run.
func (t *Trace) ShadowError() error {
	return t.shadowErr
}

func (t *Trace) toSpineEnvelopes(e Event) []spine.RawEventEnvelope {
	base := func(kind string, data map[string]any) spine.RawEventEnvelope {
		var beadID *string
		if strings.TrimSpace(t.beadID) != "" {
			beadID = &t.beadID
		}
		var agentID *string
		if isAgentID(t.agent) {
			agent := t.agent
			agentID = &agent
		}
		var model *string
		if strings.TrimSpace(t.model) != "" {
			model = &t.model
		}
		return spine.RawEventEnvelope{
			ID:        spine.MintULID(),
			TS:        e.Timestamp,
			Kind:      kind,
			TraceID:   t.traceID,
			SessionID: t.sessionID,
			RunID:     t.runID,
			BeadID:    beadID,
			AgentID:   agentID,
			Model:     model,
			Data:      data,
		}
	}

	switch e.EventType {
	case "begin":
		return []spine.RawEventEnvelope{
			base("session.start", map[string]any{
				"cwd":   t.repo,
				"model": t.model,
				"mode":  "orchestrated",
			}),
			base("agent.start", map[string]any{
				"agent_name": t.agent,
				"role":       nil,
			}),
		}
	case "end":
		return []spine.RawEventEnvelope{
			base("session.end", map[string]any{
				"exit_reason": mapOutcome(e.Outcome),
				"turn_count":  0,
			}),
			base("agent.end", map[string]any{
				"exit_reason": mapAgentExit(e.Outcome),
			}),
		}
	case "tool_call":
		if e.Tool == "bash" {
			var duration int64
			if e.DurationMs != nil {
				duration = *e.DurationMs
			}
			return []spine.RawEventEnvelope{
				base("bash.run", map[string]any{
					"command":     e.Cmd,
					"exit_code":   0,
					"duration_ms": duration,
					"stdout":      "",
					"stderr":      "",
					"truncated":   false,
				}),
			}
		}
	case "file_write":
		lines := 0
		if e.Lines != nil {
			lines = *e.Lines
		}
		return []spine.RawEventEnvelope{
			base("file.edit", map[string]any{
				"path":          e.Path,
				"lines_changed": lines,
			}),
		}
	case "gate_result":
		verdict := "error"
		passed := false
		if e.Pass != nil {
			passed = *e.Pass
			if passed {
				verdict = "pass"
			} else {
				verdict = "fail"
			}
		}
		msg := "score unavailable"
		if e.Score != nil {
			msg = fmt.Sprintf("score %.2f", *e.Score)
		}
		return []spine.RawEventEnvelope{
			base("gate.result", map[string]any{
				"verdict": verdict,
				"checks": []map[string]any{{
					"name":    "gate",
					"passed":  passed,
					"message": msg,
				}},
				"duration_ms": 0,
			}),
		}
	case "error":
		return []spine.RawEventEnvelope{
			base("error.tool_failure", map[string]any{
				"message":         e.Error,
				"source_event_id": nil,
				"context": map[string]any{
					"bead_id": t.beadID,
				},
			}),
		}
	}
	return nil
}

func mapOutcome(outcome string) string {
	switch outcome {
	case "success", "gate_fail":
		return "completed"
	case "timeout":
		return "timeout"
	case "error":
		return "error"
	default:
		return "aborted"
	}
}

func mapAgentExit(outcome string) string {
	switch outcome {
	case "success", "gate_fail", "timeout":
		return "completed"
	default:
		return "error"
	}
}

func isAgentID(agent string) bool {
	if agent == "" {
		return false
	}
	for _, r := range agent {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

// ReadTrace reads all events from a trace JSONL file.
func ReadTrace(path string) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open trace: %w", err)
	}
	defer f.Close()

	var events []Event
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB lines
	for scanner.Scan() {
		var e Event
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue // skip malformed lines
		}
		events = append(events, e)
	}
	if err := scanner.Err(); err != nil {
		return events, fmt.Errorf("scan trace: %w", err)
	}
	return events, nil
}
