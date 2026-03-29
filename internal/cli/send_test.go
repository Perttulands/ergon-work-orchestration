package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"polis/work/internal/testutil"
)

// These tests mock the tmux binary via SandboxPATH and exercise the send
// command path that delegates to worker.SessionExists and worker.SendPrompt
// (which uses tmux load-buffer + paste-buffer internally).

func tmuxMockAcceptAll() string {
	return `#!/bin/bash
exit 0
`
}

func tmuxMockSessionNotFound() string {
	return `#!/bin/bash
# skip -L <server> if present
[ "$1" = "-L" ] && shift 2
case "$1" in
  has-session) exit 1 ;;
  *)           exit 0 ;;
esac
`
}

func tmuxMockRequireTMPDIR() string {
	return `#!/bin/bash
if [ "${TMUX_TMPDIR}" != "${EXPECT_TMUX_TMPDIR}" ]; then
  exit 7
fi
# skip -L <server> if present
[ "$1" = "-L" ] && shift 2
case "$1" in
  has-session) exit 0 ;;
  send-keys)   exit 0 ;;
  *)           exit 0 ;;
esac
`
}

func tmuxMockRecordPromptInjection() string {
	return `#!/bin/bash
set -eu
log="${TMUX_LOG}"
printf '%s\n' "$*" >> "$log"
# skip -L <server> if present
[ "$1" = "-L" ] && shift 2
case "$1" in
  has-session)
    exit 0
    ;;
  load-buffer)
    cp "$2" "${TMUX_BUFFER}"
    exit 0
    ;;
  paste-buffer)
    cat "${TMUX_BUFFER}" > "${TMUX_PASTED}"
    exit 0
    ;;
  send-keys)
    shift
    while [ "$#" -gt 0 ]; do
      if [ "$1" = "ENTER" ]; then
        exit 0
      fi
      shift
    done
    echo "unexpected send-keys payload" >&2
    exit 9
    ;;
  *)
    exit 0
    ;;
esac
`
}

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
	testutil.SandboxPATH(t, map[string]string{"tmux": tmuxMockAcceptAll()})

	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"send", "agent-hugo", "do", "the", "thing"})

	if err := root.Execute(); err != nil {
		t.Fatalf("send failed: %v\noutput: %s", err, buf.String())
	}
	if !strings.Contains(buf.String(), "Sent prompt to session agent-hugo") {
		t.Fatalf("expected confirmation, got: %s", buf.String())
	}
}

func TestSendCommandUsesWorkerSendPrompt(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "tmux.log")
	bufferPath := filepath.Join(t.TempDir(), "tmux-buffer.txt")
	pastedPath := filepath.Join(t.TempDir(), "tmux-pasted.txt")
	t.Setenv("TMUX_LOG", logPath)
	t.Setenv("TMUX_BUFFER", bufferPath)
	t.Setenv("TMUX_PASTED", pastedPath)
	testutil.SandboxPATH(t, map[string]string{"tmux": tmuxMockRecordPromptInjection()})

	root := NewRoot("test")
	root.SetArgs([]string{"send", "agent-hugo", "line one", "line two"})

	if err := root.Execute(); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read tmux log: %v", err)
	}
	logText := string(logData)
	if !strings.Contains(logText, "load-buffer ") {
		t.Fatalf("expected load-buffer call, got log: %s", logText)
	}
	if !strings.Contains(logText, "paste-buffer -t agent-hugo") {
		t.Fatalf("expected paste-buffer call, got log: %s", logText)
	}
	if strings.Contains(logText, "send-keys -t agent-hugo line one line two") {
		t.Fatalf("prompt should not be delivered via send-keys, got log: %s", logText)
	}

	pastedData, err := os.ReadFile(pastedPath)
	if err != nil {
		t.Fatalf("read pasted prompt: %v", err)
	}
	if string(pastedData) != "line one line two" {
		t.Fatalf("unexpected pasted prompt: %q", string(pastedData))
	}
}

func TestSendCommandWithFile(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{"tmux": tmuxMockAcceptAll()})

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
	promptFile := filepath.Join(t.TempDir(), "prompt.md")
	if err := os.WriteFile(promptFile, []byte("hello from file"), 0644); err != nil {
		t.Fatal(err)
	}

	root := NewRoot("test")
	root.SetArgs([]string{"send", "agent-hugo", "hello", "--file", promptFile})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when both prompt args and --file are provided")
	}
	if !strings.Contains(err.Error(), "not both") {
		t.Fatalf("expected conflict error, got: %v", err)
	}
}

func TestSendCommandRespectsTMUXTMPDIR(t *testing.T) {
	t.Setenv("TMUX_TMPDIR", "/tmp/work-tmux-dir")
	t.Setenv("EXPECT_TMUX_TMPDIR", "/tmp/work-tmux-dir")
	testutil.SandboxPATH(t, map[string]string{"tmux": tmuxMockRequireTMPDIR()})

	root := NewRoot("test")
	root.SetArgs([]string{"send", "agent-hugo", "hello"})
	if err := root.Execute(); err != nil {
		t.Fatalf("send should respect TMUX_TMPDIR, got: %v", err)
	}
}

func TestSendCommandSessionNotFound(t *testing.T) {
	testutil.SandboxPATH(t, map[string]string{"tmux": tmuxMockSessionNotFound()})

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
	testutil.SandboxPATH(t, map[string]string{"tmux": tmuxMockAcceptAll()})

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
	testutil.SandboxPATH(t, map[string]string{"tmux": tmuxMockAcceptAll()})

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

func TestSendCommandNoArgs(t *testing.T) {
	root := NewRoot("test")
	root.SetArgs([]string{"send"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when no session provided")
	}
}
