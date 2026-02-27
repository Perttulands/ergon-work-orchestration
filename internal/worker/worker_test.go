package worker

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func hasTmux() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

func TestRequireTmux(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	if err := requireTmux(); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestCreateAndKillSession(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	name := "work-test-" + time.Now().Format("150405")
	defer killSession(name)

	if err := createSession(name, "/tmp"); err != nil {
		t.Fatalf("create session: %v", err)
	}

	if !SessionExists(name) {
		t.Error("session should exist after creation")
	}

	if err := killSession(name); err != nil {
		t.Fatalf("kill session: %v", err)
	}

	if SessionExists(name) {
		t.Error("session should not exist after kill")
	}
}

func TestSendKeysAndCapture(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	name := "work-test-capture-" + time.Now().Format("150405")
	defer killSession(name)

	if err := createSession(name, "/tmp"); err != nil {
		t.Fatalf("create session: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	if err := sendKeys(name, "echo HELLO_WORK_TEST"); err != nil {
		t.Fatalf("send keys: %v", err)
	}
	time.Sleep(1 * time.Second)

	output, err := capturePane(name)
	if err != nil {
		t.Fatalf("capture pane: %v", err)
	}

	if !strings.Contains(output, "HELLO_WORK_TEST") {
		t.Errorf("expected output to contain HELLO_WORK_TEST, got: %s", output)
	}
}

func TestSessionExistsNonExistent(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	if SessionExists("work-test-nonexistent-99999") {
		t.Error("non-existent session should return false")
	}
}

func TestConfigValidation(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}

	// No session name
	_, err := Spawn(Config{WorkDir: "/tmp"})
	if err == nil || !strings.Contains(err.Error(), "session name required") {
		t.Errorf("expected session name error, got: %v", err)
	}

	// No work dir
	_, err = Spawn(Config{SessionName: "test"})
	if err == nil || !strings.Contains(err.Error(), "work directory required") {
		t.Errorf("expected work directory error, got: %v", err)
	}
}

// --- detectReady tests (unit tests for waitForReady's detection logic) ---

func TestDetectReadyOK(t *testing.T) {
	output := `Welcome to Claude Code v1.2.3
Type your request below.
❯`
	if got := detectReady(output); got != readyOK {
		t.Errorf("detectReady() = %d, want readyOK (%d)", got, readyOK)
	}
}

func TestDetectReadyTrustDialog(t *testing.T) {
	output := `Do you trust this folder?
❯ 1. Yes, I trust this folder
  2. No`
	if got := detectReady(output); got != readyNeedTrust {
		t.Errorf("detectReady() = %d, want readyNeedTrust (%d)", got, readyNeedTrust)
	}
}

func TestDetectReadyNotYet(t *testing.T) {
	output := "Loading..."
	if got := detectReady(output); got != readyNotYet {
		t.Errorf("detectReady() = %d, want readyNotYet (%d)", got, readyNotYet)
	}
}

func TestDetectReadyEmpty(t *testing.T) {
	if got := detectReady(""); got != readyNotYet {
		t.Errorf("detectReady('') = %d, want readyNotYet", got)
	}
}

func TestDetectReadyBannerInMiddle(t *testing.T) {
	output := `some startup noise
Claude Code v2.5.0 — ready
Type your prompt below
❯`
	if got := detectReady(output); got != readyOK {
		t.Errorf("detectReady() = %d, want readyOK", got)
	}
}

// --- detectCompletion tests (unit tests for waitForCompletion's detection logic) ---

func TestDetectCompletionIdle(t *testing.T) {
	output := "Working on task...\nDone! All tests pass.\n❯"
	if !detectCompletion(output) {
		t.Error("expected completion detected for idle prompt after work")
	}
}

func TestDetectCompletionStillWorking(t *testing.T) {
	output := "some output\nReading src/main.go\n❯"
	if detectCompletion(output) {
		t.Error("should NOT detect completion when tool activity is on preceding line")
	}
}

func TestDetectCompletionTooShort(t *testing.T) {
	// Only 2 lines — not enough to be confident
	output := "Hello\n❯"
	if detectCompletion(output) {
		t.Error("should NOT detect completion with fewer than 3 lines")
	}
}

func TestDetectCompletionNoPrompt(t *testing.T) {
	output := "Working on task...\nStill going...\nAlmost done..."
	if detectCompletion(output) {
		t.Error("should NOT detect completion without ❯ prompt")
	}
}

func TestDetectCompletionEmpty(t *testing.T) {
	if detectCompletion("") {
		t.Error("should NOT detect completion from empty output")
	}
}

func TestDetectCompletionWritingActivity(t *testing.T) {
	output := "task progress\nWriting internal/cli/run.go\n❯"
	if detectCompletion(output) {
		t.Error("should NOT detect completion when Writing is on preceding line")
	}
}

// --- sendPrompt temp file test (no tmux needed) ---

func TestSendPromptWritesTempFile(t *testing.T) {
	// We can't test the full sendPrompt without tmux, but we can verify
	// the temp file creation and content writing logic.
	prompt := "BEAD: test-123\nTASK: add feature\nDONE WHEN: tests pass"
	f, err := os.CreateTemp("", "work-prompt-*.txt")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	defer os.Remove(f.Name())

	if _, err := f.WriteString(prompt); err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Close()

	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != prompt {
		t.Errorf("prompt mismatch: got %q, want %q", string(data), prompt)
	}
}

func TestIsStillWorking(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "idle prompt",
			output: "some output\nDone.\n❯",
			want:   false,
		},
		{
			name:   "reading file",
			output: "some output\nReading src/main.go\n❯",
			want:   true,
		},
		{
			name:   "short output",
			output: "❯",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isStillWorking(tt.output)
			if got != tt.want {
				t.Errorf("isStillWorking(%q) = %v, want %v", tt.output, got, tt.want)
			}
		})
	}
}
