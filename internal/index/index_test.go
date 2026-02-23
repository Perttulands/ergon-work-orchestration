package index

import (
	"fmt"
	"testing"
	"time"

	"polis/work/internal/trace"
)

func TestOpenAndClose(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestRecordAndRecent(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	now := time.Now()
	for i, task := range []string{"add auth", "fix tests", "refactor db"} {
		meta := trace.Metadata{
			BeadID:    fmt.Sprintf("bead-%d", i),
			Agent:     "zeus",
			Task:      task,
			StartTime: now.Add(time.Duration(i) * time.Minute),
			EndTime:   now.Add(time.Duration(i)*time.Minute + 5*time.Minute),
			Outcome:   "success",
			DurationS: 300,
			FilePath:  fmt.Sprintf("/tmp/trace-%d.jsonl", i),
		}
		if err := db.Record(meta); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}

	runs, err := db.Recent(10)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("expected 3 runs, got %d", len(runs))
	}

	// Should be newest first
	if runs[0].Task != "refactor db" {
		t.Errorf("first should be newest, got: %s", runs[0].Task)
	}
	if runs[2].Task != "add auth" {
		t.Errorf("last should be oldest, got: %s", runs[2].Task)
	}
}

func TestRecentLimit(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	now := time.Now()
	for i := 0; i < 5; i++ {
		db.Record(trace.Metadata{
			BeadID:    fmt.Sprintf("bead-%d", i),
			Agent:     "zeus",
			Task:      fmt.Sprintf("task %d", i),
			StartTime: now.Add(time.Duration(i) * time.Minute),
			EndTime:   now.Add(time.Duration(i)*time.Minute + time.Minute),
			Outcome:   "success",
			DurationS: 60,
			FilePath:  "/tmp/t.jsonl",
		})
	}

	runs, err := db.Recent(2)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(runs) != 2 {
		t.Errorf("expected 2 runs, got %d", len(runs))
	}
}

func TestByBead(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	now := time.Now()
	db.Record(trace.Metadata{
		BeadID: "target", Agent: "zeus", Task: "task A",
		StartTime: now, EndTime: now.Add(time.Minute),
		Outcome: "success", DurationS: 60, FilePath: "/tmp/a.jsonl",
	})
	db.Record(trace.Metadata{
		BeadID: "other", Agent: "ares", Task: "task B",
		StartTime: now, EndTime: now.Add(time.Minute),
		Outcome: "error", DurationS: 60, FilePath: "/tmp/b.jsonl",
	})

	runs, err := db.ByBead("target")
	if err != nil {
		t.Fatalf("by bead: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].Agent != "zeus" {
		t.Errorf("agent=%s, want zeus", runs[0].Agent)
	}
}

func TestRecentDefault(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	// With 0 limit, should default to 20
	runs, err := db.Recent(0)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if runs == nil {
		// nil is fine for empty result
	}
	_ = runs
}

func TestRecordUpsert(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	now := time.Now()
	meta := trace.Metadata{
		BeadID: "dup", Agent: "zeus", Task: "original",
		StartTime: now, EndTime: now.Add(time.Minute),
		Outcome: "error", DurationS: 60, FilePath: "/tmp/dup.jsonl",
	}
	db.Record(meta)

	// Update same trace_id
	meta.Outcome = "success"
	meta.Task = "updated"
	db.Record(meta)

	runs, err := db.Recent(10)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run after upsert, got %d", len(runs))
	}
	if runs[0].Outcome != "success" {
		t.Errorf("expected updated outcome, got: %s", runs[0].Outcome)
	}
}
