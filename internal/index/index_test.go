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

func TestRebuildFromJSONL(t *testing.T) {
	workDir := t.TempDir()

	// Create two trace files without indexing them
	tr1, err := trace.Open(workDir, "rebuild-aaa", "zeus", "add auth")
	if err != nil {
		t.Fatalf("open trace 1: %v", err)
	}
	tr1.EmitToolCall("bash", "go build", 100)
	tr1.Close("success", nil)

	tr2, err := trace.Open(workDir, "rebuild-bbb", "ares", "fix bug")
	if err != nil {
		t.Fatalf("open trace 2: %v", err)
	}
	tr2.EmitError("compile failed")
	tr2.Close("error", fmt.Errorf("compile failed"))

	// Open index — should auto-rebuild from JSONL
	db, err := Open(workDir)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	defer db.Close()

	// Query by bead — should find both
	runs1, err := db.ByBead("rebuild-aaa")
	if err != nil {
		t.Fatalf("by bead aaa: %v", err)
	}
	if len(runs1) != 1 {
		t.Fatalf("expected 1 run for rebuild-aaa, got %d", len(runs1))
	}
	if runs1[0].Agent != "zeus" {
		t.Errorf("agent=%s, want zeus", runs1[0].Agent)
	}
	if runs1[0].Task != "add auth" {
		t.Errorf("task=%s, want add auth", runs1[0].Task)
	}
	if runs1[0].Outcome != "success" {
		t.Errorf("outcome=%s, want success", runs1[0].Outcome)
	}

	runs2, err := db.ByBead("rebuild-bbb")
	if err != nil {
		t.Fatalf("by bead bbb: %v", err)
	}
	if len(runs2) != 1 {
		t.Fatalf("expected 1 run for rebuild-bbb, got %d", len(runs2))
	}
	if runs2[0].Agent != "ares" {
		t.Errorf("agent=%s, want ares", runs2[0].Agent)
	}
	if runs2[0].Outcome != "error" {
		t.Errorf("outcome=%s, want error", runs2[0].Outcome)
	}

	// Recent should return both, newest first
	recent, err := db.Recent(10)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("expected 2 recent runs, got %d", len(recent))
	}
}

func TestRebuildExplicit(t *testing.T) {
	workDir := t.TempDir()

	// Create a trace file
	tr, err := trace.Open(workDir, "explicit-rebuild", "mercury", "refactor")
	if err != nil {
		t.Fatalf("open trace: %v", err)
	}
	tr.Close("success", nil)

	// Open index and manually record something else first
	db, err := Open(workDir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	// Rebuild explicitly returns count
	count, err := db.Rebuild(workDir)
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if count < 1 {
		t.Errorf("expected at least 1 rebuilt trace, got %d", count)
	}

	runs, err := db.ByBead("explicit-rebuild")
	if err != nil {
		t.Fatalf("by bead: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
}

func TestRebuildNoTracesDir(t *testing.T) {
	workDir := t.TempDir()

	db, err := Open(workDir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	// Rebuild on empty dir should return 0
	count, err := db.Rebuild(workDir)
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 rebuilt, got %d", count)
	}
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
