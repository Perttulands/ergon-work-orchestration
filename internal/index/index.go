// Package index provides a SQLite index for fast trace queries.
// Stores trace metadata so CLI commands can query without scanning JSONL files.
package index

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"polis/work/internal/trace"
)

// DB wraps the SQLite index database.
type DB struct {
	db *sql.DB
}

// RunRecord represents a single indexed run.
type RunRecord struct {
	TraceID   string
	Agent     string
	Task      string
	BeadID    string
	StartTime time.Time
	EndTime   time.Time
	Outcome   string
	DurationS int64
	TracePath string
}

// Open opens (or creates) the index database.
func Open(workDir string) (*DB, error) {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, fmt.Errorf("create work dir: %w", err)
	}

	dbPath := filepath.Join(workDir, "index.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open index db: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate index: %w", err)
	}

	d := &DB{db: db}

	// Auto-rebuild from JSONL traces if the index is empty
	if d.isEmpty() {
		d.Rebuild(workDir)
	}

	return d, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS runs (
			trace_id   TEXT PRIMARY KEY,
			agent      TEXT NOT NULL,
			task       TEXT NOT NULL,
			bead_id    TEXT NOT NULL,
			start_time TEXT NOT NULL,
			end_time   TEXT NOT NULL,
			outcome    TEXT NOT NULL,
			duration_s INTEGER NOT NULL,
			trace_path TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_runs_start ON runs(start_time);
		CREATE INDEX IF NOT EXISTS idx_runs_bead ON runs(bead_id);
		CREATE INDEX IF NOT EXISTS idx_runs_agent ON runs(agent);
	`)
	return err
}

// Record inserts a trace metadata record.
func (d *DB) Record(meta trace.Metadata) error {
	_, err := d.db.Exec(`
		INSERT OR REPLACE INTO runs (trace_id, agent, task, bead_id, start_time, end_time, outcome, duration_s, trace_path)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		meta.BeadID,
		meta.Agent,
		meta.Task,
		meta.BeadID,
		meta.StartTime.Format(time.RFC3339),
		meta.EndTime.Format(time.RFC3339),
		meta.Outcome,
		meta.DurationS,
		meta.FilePath,
	)
	if err != nil {
		return fmt.Errorf("record run: %w", err)
	}
	return nil
}

// Recent returns the most recent runs, newest first.
func (d *DB) Recent(limit int) ([]RunRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := d.db.Query(`
		SELECT trace_id, agent, task, bead_id, start_time, end_time, outcome, duration_s, trace_path
		FROM runs ORDER BY start_time DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("query recent: %w", err)
	}
	defer rows.Close()

	return scanRuns(rows)
}

// ByBead returns runs for a specific bead ID.
func (d *DB) ByBead(beadID string) ([]RunRecord, error) {
	rows, err := d.db.Query(`
		SELECT trace_id, agent, task, bead_id, start_time, end_time, outcome, duration_s, trace_path
		FROM runs WHERE bead_id = ? ORDER BY start_time DESC
	`, beadID)
	if err != nil {
		return nil, fmt.Errorf("query by bead: %w", err)
	}
	defer rows.Close()

	return scanRuns(rows)
}

// Rebuild scans JSONL trace files under workDir/traces/ and populates the index.
// Called automatically on Open when the DB is empty.
func (d *DB) Rebuild(workDir string) (int, error) {
	tracesDir := filepath.Join(workDir, "traces")
	if _, err := os.Stat(tracesDir); os.IsNotExist(err) {
		return 0, nil
	}

	var count int
	err := filepath.Walk(tracesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}

		meta, err := metadataFromTrace(path)
		if err != nil {
			return nil // skip malformed traces
		}

		if err := d.Record(meta); err != nil {
			return nil // skip record errors
		}
		count++
		return nil
	})
	if err != nil {
		return count, fmt.Errorf("walk traces: %w", err)
	}
	return count, nil
}

// metadataFromTrace extracts run metadata from a JSONL trace file by reading
// the begin and end events.
func metadataFromTrace(path string) (trace.Metadata, error) {
	events, err := trace.ReadTrace(path)
	if err != nil {
		return trace.Metadata{}, fmt.Errorf("read trace %s: %w", path, err)
	}
	if len(events) == 0 {
		return trace.Metadata{}, fmt.Errorf("empty trace: %s", path)
	}

	// Find begin and end events
	var begin, end *trace.Event
	for i := range events {
		switch events[i].EventType {
		case "begin":
			begin = &events[i]
		case "end":
			end = &events[i]
		}
	}
	if begin == nil {
		return trace.Metadata{}, fmt.Errorf("no begin event in %s", path)
	}

	meta := trace.Metadata{
		BeadID:   begin.Bead,
		Agent:    begin.Agent,
		Task:     begin.Task,
		FilePath: path,
	}

	if t, err := time.Parse(time.RFC3339, begin.Timestamp); err == nil {
		meta.StartTime = t
	}

	if end != nil {
		meta.Outcome = end.Outcome
		if end.DurationS != nil {
			meta.DurationS = *end.DurationS
		}
		if t, err := time.Parse(time.RFC3339, end.Timestamp); err == nil {
			meta.EndTime = t
		}
	} else {
		meta.Outcome = "incomplete"
		meta.EndTime = meta.StartTime
	}

	return meta, nil
}

// isEmpty checks if the runs table has any rows.
func (d *DB) isEmpty() bool {
	var count int
	err := d.db.QueryRow("SELECT COUNT(*) FROM runs").Scan(&count)
	return err != nil || count == 0
}

// Close closes the database.
func (d *DB) Close() error {
	return d.db.Close()
}

func scanRuns(rows *sql.Rows) ([]RunRecord, error) {
	var runs []RunRecord
	for rows.Next() {
		var r RunRecord
		var startStr, endStr string
		if err := rows.Scan(&r.TraceID, &r.Agent, &r.Task, &r.BeadID, &startStr, &endStr, &r.Outcome, &r.DurationS, &r.TracePath); err != nil {
			return nil, fmt.Errorf("scan run: %w", err)
		}
		var parseErr error
		r.StartTime, parseErr = time.Parse(time.RFC3339, startStr)
		if parseErr != nil {
			return nil, fmt.Errorf("parse start time %q: %w", startStr, parseErr)
		}
		r.EndTime, parseErr = time.Parse(time.RFC3339, endStr)
		if parseErr != nil {
			return nil, fmt.Errorf("parse end time %q: %w", endStr, parseErr)
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}
