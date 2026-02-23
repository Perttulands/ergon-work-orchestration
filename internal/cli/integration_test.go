package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"polis/work/internal/index"
	"polis/work/internal/trace"

	workctx "polis/work/internal/context"
)

// Integration test: full lifecycle without actually spawning claude.
// Tests that all the pieces wire together correctly.
func TestFullLifecycleIntegration(t *testing.T) {
	workDir := t.TempDir()

	// Set up citizen experience
	citizenDir := filepath.Join(workDir, "citizens")
	os.MkdirAll(citizenDir, 0o755)
	os.WriteFile(filepath.Join(citizenDir, "test-agent.md"), []byte("Experienced with Go."), 0o644)

	// 1. Context gathering works
	ctx, err := workctx.Gather(workctx.Config{
		Citizen: "test-agent",
		Task:    "add feature",
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("gather context: %v", err)
	}
	if ctx.CitizenExperience != "Experienced with Go." {
		t.Errorf("citizen experience: %q", ctx.CitizenExperience)
	}

	// 2. Prompt assembly works
	prompt := assemblePrompt("add feature", "test-agent", "test-bead", "/tmp", ctx)
	if prompt == "" {
		t.Fatal("prompt should not be empty")
	}

	// 3. Trace capture works
	tr, err := trace.Open(workDir, "integ-test", "test-agent", "add feature")
	if err != nil {
		t.Fatalf("open trace: %v", err)
	}
	tr.EmitToolCall("bash", "go build", 100)
	tr.EmitFileWrite("main.go", 10)
	meta := tr.GetMetadata("success")
	tr.Close("success", nil)

	// Verify trace file exists and is readable
	events, err := trace.ReadTrace(tr.FilePath())
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	if len(events) != 4 { // begin, tool_call, file_write, end
		t.Errorf("expected 4 events, got %d", len(events))
	}

	// 4. Index recording works
	idx, err := index.Open(workDir)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	if err := idx.Record(meta); err != nil {
		t.Fatalf("record: %v", err)
	}

	runs, err := idx.Recent(10)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].Agent != "test-agent" {
		t.Errorf("agent=%s, want test-agent", runs[0].Agent)
	}
	if runs[0].Outcome != "success" {
		t.Errorf("outcome=%s, want success", runs[0].Outcome)
	}
	idx.Close()

	// 5. Citizen experience recording works
	if err := workctx.AppendCitizenExperience(workDir, "test-agent", "add feature", "success", "integ-test"); err != nil {
		t.Fatalf("append experience: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(citizenDir, "test-agent.md"))
	if err != nil {
		t.Fatalf("read citizen file: %v", err)
	}
	content := string(data)
	if len(content) <= len("Experienced with Go.") {
		t.Error("citizen file should have grown")
	}

	// 6. History and trace lookup work
	idx2, err := index.Open(workDir)
	if err != nil {
		t.Fatalf("reopen index: %v", err)
	}
	defer idx2.Close()

	byBead, err := idx2.ByBead("integ-test")
	if err != nil {
		t.Fatalf("by bead: %v", err)
	}
	if len(byBead) != 1 {
		t.Errorf("expected 1 run by bead, got %d", len(byBead))
	}

	// 7. Trace lookup by glob works
	path, err := findTracePath(workDir, "integ-test")
	if err != nil {
		t.Fatalf("find trace: %v", err)
	}
	if path == "" {
		t.Error("trace path should not be empty")
	}
}

// Integration test: verify the full lifecycle degrades gracefully
// when no external tools are available.
func TestLifecycleDegradationIntegration(t *testing.T) {
	workDir := t.TempDir()

	// Gather context with nothing available
	ctx, err := workctx.Gather(workctx.Config{
		WorkDir: workDir,
		Citizen: "ghost",
		Task:    "phantom task",
	})
	if err != nil {
		t.Fatalf("gather should not fail: %v", err)
	}

	// Prompt still assembles
	prompt := assemblePrompt("phantom task", "ghost", "no-bead", "/tmp", ctx)
	if prompt == "" {
		t.Fatal("prompt should not be empty even without context")
	}

	// Trace still works
	tr, err := trace.Open(workDir, "degrade-test", "ghost", "phantom task")
	if err != nil {
		t.Fatalf("trace should work: %v", err)
	}
	meta := tr.GetMetadata("success")
	tr.Close("success", nil)

	// Index still works
	idx, err := index.Open(workDir)
	if err != nil {
		t.Fatalf("index should work: %v", err)
	}
	idx.Record(meta)
	idx.Close()

	// Citizen experience creation works for new citizen
	if err := workctx.AppendCitizenExperience(workDir, "ghost", "phantom task", "success", "degrade-test"); err != nil {
		t.Fatalf("citizen experience should work: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(filepath.Join(workDir, "citizens", "ghost.md")); os.IsNotExist(err) {
		t.Error("citizen file should have been created")
	}
}

// Test that multiple concurrent traces don't interfere.
func TestConcurrentTraces(t *testing.T) {
	workDir := t.TempDir()
	done := make(chan bool, 3)

	for i := 0; i < 3; i++ {
		go func(n int) {
			beadID := randomID()
			tr, err := trace.Open(workDir, beadID, "agent", "task")
			if err != nil {
				t.Errorf("trace %d: %v", n, err)
				done <- false
				return
			}
			tr.EmitToolCall("bash", "echo", 10)
			time.Sleep(10 * time.Millisecond)
			tr.Close("success", nil)

			events, err := trace.ReadTrace(tr.FilePath())
			if err != nil {
				t.Errorf("read %d: %v", n, err)
				done <- false
				return
			}
			if len(events) != 3 {
				t.Errorf("trace %d: expected 3 events, got %d", n, len(events))
			}
			done <- true
		}(i)
	}

	for i := 0; i < 3; i++ {
		<-done
	}
}
