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
	"sync"
	"time"
)

// Event represents a single trace event.
type Event struct {
	Timestamp  string   `json:"ts"`
	EventType  string   `json:"event"`
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
	mu       sync.Mutex
	file     *os.File
	filePath string
	beadID   string
	agent    string
	task     string
	started  time.Time
}

// Metadata holds trace summary info for indexing.
type Metadata struct {
	BeadID    string
	Agent     string
	Task      string
	StartTime time.Time
	EndTime   time.Time
	Outcome   string
	DurationS int64
	FilePath  string
}

// Open creates a new trace file organized by date.
func Open(workDir, beadID, agent, task string) (*Trace, error) {
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
		file:     f,
		filePath: filePath,
		beadID:   beadID,
		agent:    agent,
		task:     task,
		started:  now,
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

	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	if _, err := t.file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write event: %w", err)
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
func (t *Trace) Close(outcome string, err error) error {
	duration := int64(time.Since(t.started).Seconds())
	e := Event{
		EventType: "end",
		Outcome:   outcome,
		DurationS: &duration,
		Agent:     t.agent,
		Bead:      t.beadID,
	}
	if err != nil {
		e.Error = err.Error()
	}
	t.Emit(e)
	return t.file.Close()
}

// GetMetadata returns trace metadata for indexing.
func (t *Trace) GetMetadata(outcome string) Metadata {
	return Metadata{
		BeadID:    t.beadID,
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
