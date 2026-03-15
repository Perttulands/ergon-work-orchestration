package spine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriterWriteAndReadAll(t *testing.T) {
	dir := t.TempDir()
	writer := NewWriter(dir)

	bead := "pol-123"
	agent := "worker"
	model := "openai/gpt-5.4"
	env := RawEventEnvelope{
		ID:        MintULID(),
		TS:        "2026-03-13T00:00:00Z",
		Kind:      "session.start",
		TraceID:   MintULID(),
		SessionID: MintSessionID(),
		RunID:     MintRunID(),
		BeadID:    &bead,
		AgentID:   &agent,
		Model:     &model,
		Data: map[string]any{
			"cwd":   "/tmp/repo",
			"mode":  "orchestrated",
			"model": model,
		},
	}
	if err := writer.Write(env); err != nil {
		t.Fatalf("Write: %v", err)
	}

	events, err := ReadAll(dir)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].Kind != "session.start" {
		t.Fatalf("kind = %q, want session.start", events[0].Kind)
	}
}

func TestMintIDsMatchContractShapes(t *testing.T) {
	if got := MintULID(); len(got) != 26 {
		t.Fatalf("ulid length = %d, want 26 (%q)", len(got), got)
	}
	if got := MintRunID(); len(got) != 30 || got[:4] != "run-" {
		t.Fatalf("run id = %q, want run-<ulid>", got)
	}
	if got := MintSessionID(); len(got) != 36 {
		t.Fatalf("session id = %q, want uuid length 36", got)
	}
}

func TestDefaultDirUsesEnvOverride(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "spine")
	t.Setenv(DirEnv, dir)
	if got := DefaultDir(); got != dir {
		t.Fatalf("DefaultDir = %q, want %q", got, dir)
	}
}

func TestEnabled(t *testing.T) {
	t.Setenv(EnableEnv, "1")
	if !Enabled() {
		t.Fatal("Enabled should be true for 1")
	}
	t.Setenv(EnableEnv, "")
	if !Enabled() {
		t.Fatal("Enabled should be true when unset (default on since Phase 4)")
	}
	t.Setenv(EnableEnv, "0")
	if Enabled() {
		t.Fatal("Enabled should be false when explicitly set to 0")
	}
	t.Setenv(EnableEnv, "false")
	if Enabled() {
		t.Fatal("Enabled should be false when explicitly set to false")
	}
}

func TestReadAllMissingDir(t *testing.T) {
	events, err := ReadAll(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("ReadAll missing dir: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("events = %d, want 0", len(events))
	}
}

func TestWriterCreatesDirectory(t *testing.T) {
	base := filepath.Join(t.TempDir(), "deep", "path")
	writer := NewWriter(base)
	env := RawEventEnvelope{
		ID:        MintULID(),
		TS:        "2026-03-13T00:00:00Z",
		Kind:      "session.start",
		TraceID:   MintULID(),
		SessionID: MintSessionID(),
		RunID:     MintRunID(),
		Data:      map[string]any{"cwd": "/tmp/repo", "mode": "orchestrated", "model": "openai/gpt"},
	}
	if err := writer.Write(env); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := os.Stat(base); err != nil {
		t.Fatalf("stat base dir: %v", err)
	}
}

func TestMintULIDUniqueAcrossCalls(t *testing.T) {
	a := MintULID()
	b := MintULID()
	if a == b {
		t.Fatalf("expected unique ULIDs, got %q", a)
	}
}

func TestNoPanicUnderParallelWrite(t *testing.T) {
	dir := t.TempDir()
	writer := NewWriter(dir)
	t.Run("parallel", func(t *testing.T) {
		t.Parallel()
		env := RawEventEnvelope{
			ID:        MintULID(),
			TS:        "2026-03-13T00:00:00Z",
			Kind:      "session.start",
			TraceID:   MintULID(),
			SessionID: MintSessionID(),
			RunID:     MintRunID(),
			Data:      map[string]any{"cwd": "/tmp/repo", "mode": "orchestrated", "model": "openai/gpt"},
		}
		if err := writer.Write(env); err != nil {
			t.Fatalf("Write: %v", err)
		}
	})
}
