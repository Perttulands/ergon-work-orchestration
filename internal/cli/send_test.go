package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"polis/work/internal/testutil"
)

func TestSendCommandExists(t *testing.T) {
	root := NewRoot("test")
	found := false
	for _, cmd := range root.Commands() {
		if cmd.Name() == "send" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("send command should be registered")
	}
}

func TestSendCommandSuccess(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "tmux-state")
	t.Setenv("TMUX_STATE_FILE", stateFile)
	t.Setenv("TMUX_TMPDIR", "/tmp/work-send-test")
	t.Setenv("TMUX_TMPDIR_REQUIRED", "/tmp/work-send-test")

	tmuxScript := `
state="${TMUX_STATE_FILE:?}"
seq=0
[ -f "$state" ] && seq="$(cat "$state")"
seq=$((seq+1))
echo "$seq" > "$state"

if [ "${TMUX_TMPDIR_REQUIRED:-}" != "" ] && [ "${TMUX_TMPDIR:-}" != "$TMUX_TMPDIR_REQUIRED" ]; then
  exit 99
fi

case "$seq:$1" in
  1:has-session)
    [ "$2" = "-t" ] && [ "$3" = "agent-hugo" ] && exit 0
    exit 10
    ;;
  2:send-keys)
    [ "$2" = "-t" ] && [ "$3" = "agent-hugo" ] && [ "$4" = "-l" ] && [ "$5" = "do the thing" ] && exit 0
    exit 11
    ;;
  3:send-keys)
    [ "$2" = "-t" ] && [ "$3" = "agent-hugo" ] && [ "$4" = "Enter" ] && exit 0
    exit 12
    ;;
esac
exit 13
`
	testutil.SandboxPATH(t, map[string]string{"tmux": tmuxScript})

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"send", "agent-hugo", "do", "the", "thing"})

	if err := root.Execute(); err != nil {
		t.Fatalf("send failed: %v\noutput: %s", err, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "Sent prompt to session agent-hugo") {
		t.Fatalf("expected confirmation, got: %s", out)
	}
}

func TestSendCommandWithFile(t *testing.T) {
	t.Setenv("EXPECTED_PROMPT", "hello from file")

	tmuxScript := `
case "$1" in
  has-session)  exit 0 ;;
  send-keys)
    if [ "$4" = "-l" ]; then
      [ "$5" = "$EXPECTED_PROMPT" ] && exit 0
      exit 20
    fi
    [ "$4" = "Enter" ] && exit 0
    exit 21
    ;;
  *)            exit 0 ;;
esac
`
	testutil.SandboxPATH(t, map[string]string{"tmux": tmuxScript})

	promptFile := filepath.Join(t.TempDir(), "prompt.md")
	if err := os.WriteFile(promptFile, []byte("hello from file"), 0644); err != nil {
		t.Fatal(err)
	}

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"send", "agent-hugo", "--file", promptFile})

	if err := root.Execute(); err != nil {
		t.Fatalf("send --file failed: %v\noutput: %s", err, buf.String())
	}
	if !strings.Contains(buf.String(), "Sent prompt to session agent-hugo") {
		t.Fatalf("expected confirmation, got: %s", buf.String())
	}
}

func TestSendCommandPromptAndFileConflict(t *testing.T) {
	root := NewRoot("test")
	promptFile := filepath.Join(t.TempDir(), "prompt.md")
	if err := os.WriteFile(promptFile, []byte("hello from file"), 0644); err != nil {
		t.Fatal(err)
	}
	root.SetArgs([]string{"send", "agent-hugo", "hello", "--file", promptFile})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when both prompt args and --file are provided")
	}
	if !strings.Contains(err.Error(), "not both") {
		t.Fatalf("expected conflict error, got: %v", err)
	}
}

func TestSendCommandSessionNotFound(t *testing.T) {
	tmuxScript := `
case "$1" in
  has-session) exit 1 ;;
  *)           exit 0 ;;
esac
`
	testutil.SandboxPATH(t, map[string]string{"tmux": tmuxScript})

	root := NewRoot("test")
	root.SetArgs([]string{"send", "nonexistent", "hello"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for missing session")
	}
	if !strings.Contains(err.Error(), "session not found") {
		t.Fatalf("expected session not found error, got: %v", err)
	}
}

func TestSendCommandNoPrompt(t *testing.T) {
	tmuxScript := `
case "$1" in
  has-session) exit 0 ;;
  *)           exit 0 ;;
esac
`
	testutil.SandboxPATH(t, map[string]string{"tmux": tmuxScript})

	root := NewRoot("test")
	root.SetArgs([]string{"send", "agent-hugo"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when no prompt provided")
	}
	if !strings.Contains(err.Error(), "prompt text required") {
		t.Fatalf("expected prompt required error, got: %v", err)
	}
}

func TestSendCommandMissingFile(t *testing.T) {
	tmuxScript := `
case "$1" in
  has-session) exit 0 ;;
  *)           exit 0 ;;
esac
`
	testutil.SandboxPATH(t, map[string]string{"tmux": tmuxScript})

	root := NewRoot("test")
	root.SetArgs([]string{"send", "agent-hugo", "--file", "/nonexistent/prompt.md"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "read prompt file") {
		t.Fatalf("expected read prompt file error, got: %v", err)
	}
}

func TestSendCommandTmuxSessionCheckFailure(t *testing.T) {
	tmuxScript := `
case "$1" in
  has-session)
    echo "tmux socket error" >&2
    exit 2
    ;;
esac
`
	testutil.SandboxPATH(t, map[string]string{"tmux": tmuxScript})

	root := NewRoot("test")
	root.SetArgs([]string{"send", "agent-hugo", "hello"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected has-session failure")
	}
	if !strings.Contains(err.Error(), "check session agent-hugo") {
		t.Fatalf("expected check session error, got: %v", err)
	}
}

func TestSendCommandNoArgs(t *testing.T) {
	root := NewRoot("test")
	root.SetArgs([]string{"send"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when no session provided")
	}
}
