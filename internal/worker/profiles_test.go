package worker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRuntime_DefaultProfile(t *testing.T) {
	t.Setenv(RuntimeConfigEnv, "")
	t.Setenv("HOME", t.TempDir()) // ensure ~/.work/worker_profiles.json does not exist

	got, err := ResolveRuntime("", "any-agent")
	if err != nil {
		t.Fatalf("ResolveRuntime default failed: %v", err)
	}
	if got != "codex" {
		t.Fatalf("default runtime = %q, want codex", got)
	}
}

func TestResolveRuntime_FromCustomConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "profiles.json")
	cfg := `{
  "default_runtime": "claude",
  "runtimes": {
    "codex": {"command":"codex","args":["--ask-for-approval","never"],"model":"gpt-5.3-codex","ready_patterns":["OpenAI Codex"],"trust_patterns":["trust this folder"]},
    "claude": {"command":"claude","args":["--dangerously-skip-permissions"],"model":"claude-sonnet","ready_patterns":["Claude Code v"],"trust_patterns":["trust this folder"]}
  },
  "agents": {
    "hugo": {"runtime":"codex"}
  }
}`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv(RuntimeConfigEnv, cfgPath)

	got, err := ResolveRuntime("", "hugo")
	if err != nil {
		t.Fatalf("ResolveRuntime agent mapping failed: %v", err)
	}
	if got != "codex" {
		t.Fatalf("agent runtime = %q, want codex", got)
	}

	got, err = ResolveRuntime("", "unknown")
	if err != nil {
		t.Fatalf("ResolveRuntime default failed: %v", err)
	}
	if got != "claude" {
		t.Fatalf("default runtime = %q, want claude", got)
	}

	model := ModelForRuntime("codex", "hugo")
	if model != "gpt-5.3-codex" {
		t.Fatalf("model = %q, want gpt-5.3-codex", model)
	}
}

func TestResolveRuntime_InvalidRuntime(t *testing.T) {
	t.Setenv(RuntimeConfigEnv, "")
	t.Setenv("HOME", t.TempDir())

	if _, err := ResolveRuntime("invalid-runtime", "agent"); err == nil {
		t.Fatal("expected error for invalid runtime")
	}
}
