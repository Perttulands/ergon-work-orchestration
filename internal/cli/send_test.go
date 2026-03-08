package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"polis/work/internal/testutil"
)

// These tests use SandboxPATH to mock tmux, which the worker package
// calls via exec.Command. The worker.SendPrompt path uses:
// 1. tmux has-session -t <session>
// 2. tmux load-buffer <tmpfile>
// 3. tmux paste-buffer -t <session>
// 4. tmux send-keys -t <session> Enter (twice with delay)

func tmuxMockAcceptAll() string {
	return `#!/bin/bash
exit 0
`
}

func tmuxMockSessionNotFound() string {
	return `#!/bin/bash
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
case "$1" in
  has-session) exit 0 ;;
  send-keys)   exit 0 ;;
  *)           exit 0 ;;
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
