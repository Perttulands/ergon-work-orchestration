// Package trace captures structured JSONL traces of worker runs.
package trace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Event represents a single trace event.
type Event struct {
	Timestamp string `json:"ts"`
	EventType string `json:"event"`
	Agent     string `json:"agent,omitempty"`
	Task      string `json:"task,omitempty"`
	Bead      string `json:"bead,omitempty"`
	Tool      string `json:"tool,omitempty"`
	Cmd       string `json:"cmd,omitempty"`
	Path      string `json:"path,omitempty"`
	Outcome   string `json:"outcome,omitempty"`
	Pass      *bool  `json:"pass,omitempty"`
	Score     *float64 `json:"score,omitempty"`
	DurationMs *int64 `json:"duration_ms,omitempty"`
	DurationS  *int64 `json:"duration_s,omitempty"`
	Error     string `json:"error,omitempty"`
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

	// Write begin event
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

// FilePath returns the path to the trace file.
func (t *Trace) FilePath() string {
	return t.filePath
}
