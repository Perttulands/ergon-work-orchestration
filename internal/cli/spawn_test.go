package cli

import (
	"bytes"
	"strings"
	"testing"

	"polis/work/internal/testutil"
)

func TestSpawnCommandExists(t *testing.T) {
	root := NewRoot("test")
	found := false
	for _, cmd := range root.Commands() {
		if cmd.Name() == "spawn" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("spawn command should be registered")
	}
}

func TestSpawnCommandSuccess(t *testing.T) {
	tmuxScript := `
# skip -L <server> if present
[ "$1" = "-L" ] && shift 2
case "$1" in
  has-session)  exit 1 ;;
  new-session)  exit 0 ;;
  send-keys)    exit 0 ;;
  capture-pane) printf "OpenAI Codex (v0.110.0)\n› ready\n" ; exit 0 ;;
  *)            exit 0 ;;
esac
`
	testutil.SandboxPATH(t, map[string]string{
		"tmux":        tmuxScript,
		"relay":       `exit 0`,
		"systemd-run": `shift; shift; exec "$@"`,
	})
	t.Setenv("HOME", t.TempDir())

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"spawn", "hugo", "--repo", t.TempDir(), "--session", "agent-hugo"})

	if err := root.Execute(); err != nil {
		t.Fatalf("spawn failed: %v\noutput: %s", err, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "Ready: agent-hugo") {
		t.Fatalf("expected ready output, got: %s", out)
	}
	if !strings.Contains(out, "Attach: tmux attach -t agent-hugo") {
		t.Fatalf("expected attach output, got: %s", out)
	}
}

func TestSpawnCommandInvalidRuntime(t *testing.T) {
	root := NewRoot("test")
	root.SetArgs([]string{"spawn", "hugo", "--runtime", "not-real"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for invalid runtime")
	}
}

func TestSpawnCommandRelayFailureWarnsInRelaxedMode(t *testing.T) {
	tmuxScript := `
# skip -L <server> if present
[ "$1" = "-L" ] && shift 2
case "$1" in
  has-session)  exit 1 ;;
  new-session)  exit 0 ;;
  send-keys)    exit 0 ;;
  capture-pane) printf "OpenAI Codex (v0.110.0)\n› ready\n" ; exit 0 ;;
  *)            exit 0 ;;
esac
`
	testutil.SandboxPATH(t, map[string]string{
		"tmux":        tmuxScript,
		"relay":       `exit 1`,
		"systemd-run": `shift; shift; exec "$@"`,
	})
	t.Setenv("WORK_STRICT", "0")
	t.Setenv("HOME", t.TempDir())

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"spawn", "hugo", "--repo", t.TempDir(), "--session", "agent-hugo-warn"})

	if err := root.Execute(); err != nil {
		t.Fatalf("spawn should continue in relaxed mode, got error: %v\noutput: %s", err, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "Warning: relay register:") {
		t.Fatalf("expected relay warning in output, got: %s", out)
	}
}

func TestSpawnCommandRelayFailureFailsInStrictMode(t *testing.T) {
	tmuxScript := `
# skip -L <server> if present
[ "$1" = "-L" ] && shift 2
case "$1" in
  has-session)  exit 1 ;;
  new-session)  exit 0 ;;
  send-keys)    exit 0 ;;
  capture-pane) printf "OpenAI Codex (v0.110.0)\n› ready\n" ; exit 0 ;;
  *)            exit 0 ;;
esac
`
	testutil.SandboxPATH(t, map[string]string{
		"tmux":        tmuxScript,
		"relay":       `exit 1`,
		"systemd-run": `shift; shift; exec "$@"`,
	})
	t.Setenv("HOME", t.TempDir())

	root := NewRoot("test")
	root.SetArgs([]string{"--strict", "spawn", "hugo", "--repo", t.TempDir(), "--session", "agent-hugo-strict"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected strict mode to fail on relay error")
	}
	if !strings.Contains(err.Error(), "relay register") {
		t.Fatalf("expected relay register error, got: %v", err)
	}
}
