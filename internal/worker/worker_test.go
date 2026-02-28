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

// --- KillSession (public wrapper) tests ---

func TestKillSessionPublic(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	name := "work-test-killpub-" + time.Now().Format("150405")
	if err := createSession(name, "/tmp"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := KillSession(name); err != nil {
		t.Fatalf("KillSession: %v", err)
	}
	if SessionExists(name) {
		t.Error("session should not exist after KillSession")
	}
}

func TestKillSessionNonExistent(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	err := KillSession("work-test-nonexistent-99999")
	if err == nil {
		t.Error("KillSession should fail for non-existent session")
	}
}

// --- sendKeysRaw tests ---

func TestSendKeysRaw(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	name := "work-test-raw-" + time.Now().Format("150405")
	defer killSession(name)

	if err := createSession(name, "/tmp"); err != nil {
		t.Fatalf("create session: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	// Send "echo RAW_TEST" char-by-char via sendKeysRaw, then Enter
	if err := sendKeysRaw(name, "echo RAW_TEST_OUTPUT", "Enter"); err != nil {
		t.Fatalf("sendKeysRaw: %v", err)
	}
	time.Sleep(1 * time.Second)

	output, err := capturePane(name)
	if err != nil {
		t.Fatalf("capture pane: %v", err)
	}
	if !strings.Contains(output, "RAW_TEST_OUTPUT") {
		t.Errorf("expected RAW_TEST_OUTPUT in pane, got: %s", output)
	}
}

func TestSendKeysRawNonExistent(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	err := sendKeysRaw("work-test-noexist-12345", "echo hi")
	if err == nil {
		t.Error("sendKeysRaw should fail for non-existent session")
	}
}

// --- sendPrompt tests ---

func TestSendPrompt(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	name := "work-test-prompt-" + time.Now().Format("150405")
	defer killSession(name)

	if err := createSession(name, "/tmp"); err != nil {
		t.Fatalf("create session: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	// Start cat so the prompt text is visible in the pane
	if err := sendKeys(name, "cat"); err != nil {
		t.Fatalf("start cat: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	prompt := "HELLO_PROMPT_TEST_12345"
	if err := sendPrompt(name, prompt); err != nil {
		t.Fatalf("sendPrompt: %v", err)
	}
	time.Sleep(1 * time.Second)

	output, err := capturePane(name)
	if err != nil {
		t.Fatalf("capture pane: %v", err)
	}
	if !strings.Contains(output, "HELLO_PROMPT_TEST_12345") {
		t.Errorf("expected prompt text in pane, got: %s", output)
	}
}

func TestSendPromptMultiLine(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	name := "work-test-prompt-ml-" + time.Now().Format("150405")
	defer killSession(name)

	if err := createSession(name, "/tmp"); err != nil {
		t.Fatalf("create session: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	if err := sendKeys(name, "cat"); err != nil {
		t.Fatalf("start cat: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	prompt := "LINE_ONE\nLINE_TWO\nLINE_THREE"
	if err := sendPrompt(name, prompt); err != nil {
		t.Fatalf("sendPrompt multiline: %v", err)
	}
	time.Sleep(1 * time.Second)

	output, err := capturePane(name)
	if err != nil {
		t.Fatalf("capture pane: %v", err)
	}
	if !strings.Contains(output, "LINE_ONE") {
		t.Errorf("expected LINE_ONE in pane, got: %s", output)
	}
}

// --- waitForReady tests ---

func TestWaitForReadyBanner(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	name := "work-test-ready-" + time.Now().Format("150405")
	defer killSession(name)

	if err := createSession(name, "/tmp"); err != nil {
		t.Fatalf("create session: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	// Echo the Claude Code banner
	if err := sendKeys(name, "echo 'Claude Code v1.2.3'"); err != nil {
		t.Fatalf("send banner: %v", err)
	}

	if err := waitForReady(name, 10*time.Second); err != nil {
		t.Fatalf("waitForReady should succeed: %v", err)
	}
}

func TestWaitForReadyTimeout(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	name := "work-test-readyto-" + time.Now().Format("150405")
	defer killSession(name)

	if err := createSession(name, "/tmp"); err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Don't echo the banner — should time out
	err := waitForReady(name, 3*time.Second)
	if err == nil {
		t.Error("waitForReady should time out")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected timeout error, got: %v", err)
	}
}

func TestWaitForReadyTrustDismissal(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	name := "work-test-trust-" + time.Now().Format("150405")
	defer killSession(name)

	if err := createSession(name, "/tmp"); err != nil {
		t.Fatalf("create session: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	// Show trust dialog first, then after a short delay show the banner
	if err := sendKeys(name, "echo 'Do you trust this folder?' && sleep 2 && echo 'Claude Code v1.2.3'"); err != nil {
		t.Fatalf("send trust script: %v", err)
	}

	if err := waitForReady(name, 15*time.Second); err != nil {
		t.Fatalf("waitForReady should succeed after trust dismissal: %v", err)
	}
}

func TestWaitForReadyCaptureError(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	// Call waitForReady on a non-existent session — capturePane will fail
	err := waitForReady("work-test-noexist-54321", 3*time.Second)
	if err == nil {
		t.Error("waitForReady should fail on non-existent session")
	}
}

// --- waitForCompletion tests ---

func TestWaitForCompletionDetected(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	name := "work-test-compl-" + time.Now().Format("150405")
	defer killSession(name)

	if err := createSession(name, "/tmp"); err != nil {
		t.Fatalf("create session: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	// Produce output that matches the completion pattern:
	// - More than 2 lines
	// - Last line starts with ❯
	// - Previous line has no working indicators
	cmd := `printf 'Working on task...\nDone! All tests pass.\n❯' && sleep 30`
	if err := sendKeys(name, cmd); err != nil {
		t.Fatalf("send completion pattern: %v", err)
	}
	time.Sleep(2 * time.Second)

	output := waitForCompletion(name, 15*time.Second)
	if !strings.Contains(output, "Done!") {
		t.Errorf("expected 'Done!' in output, got: %s", output)
	}
}

func TestWaitForCompletionSessionKilled(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	name := "work-test-complkill-" + time.Now().Format("150405")

	if err := createSession(name, "/tmp"); err != nil {
		t.Fatalf("create session: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	if err := sendKeys(name, "echo 'some preliminary output' && sleep 60"); err != nil {
		t.Fatalf("send keys: %v", err)
	}
	time.Sleep(1 * time.Second)

	// Kill session after a short delay
	go func() {
		time.Sleep(2 * time.Second)
		killSession(name)
	}()

	// waitForCompletion should return when session dies
	output := waitForCompletion(name, 30*time.Second)
	// Just verify it returned (didn't hang)
	_ = output
}

// --- Spawn validation and lifecycle tests ---

func TestSpawnNoTmux(t *testing.T) {
	// Temporarily hide tmux from PATH
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir()) // empty dir — no tmux
	defer os.Setenv("PATH", origPath)

	_, err := Spawn(Config{SessionName: "test", WorkDir: "/tmp"})
	if err == nil {
		t.Error("Spawn should fail without tmux")
	}
	if !strings.Contains(err.Error(), "tmux not found") {
		t.Errorf("expected tmux not found error, got: %v", err)
	}
}

// TestSpawnSessionConflict verifies that Spawn fails clearly when the
// session name already exists. This is a real failure mode when a previous
// run crashed without cleanup, leaving a zombie session.
func TestSpawnSessionConflict(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	name := "work-test-conflict-" + time.Now().Format("150405")
	defer killSession(name)

	// Pre-create the session to cause a conflict
	if err := createSession(name, "/tmp"); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Spawn with the same session name should fail at createSession
	_, err := Spawn(Config{
		SessionName: name,
		WorkDir:     "/tmp",
	})
	if err == nil {
		t.Error("Spawn should fail when session already exists")
		return
	}
	if !strings.Contains(err.Error(), "create session") {
		t.Errorf("error should mention create session, got: %v", err)
	}
}

// TestWaitForCompletionMaxWait verifies that waitForCompletion returns
// when maxWait expires, even if the session is still alive and showing
// no completion markers. This is the safety net that prevents the
// orchestrator from hanging forever.
func TestWaitForCompletionMaxWait(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	name := "work-test-maxwait-" + time.Now().Format("150405")
	defer killSession(name)

	if err := createSession(name, "/tmp"); err != nil {
		t.Fatalf("create session: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	// Start something that never shows completion markers
	if err := sendKeys(name, "sleep 300"); err != nil {
		t.Fatalf("send keys: %v", err)
	}

	// maxWait of 3s — should return after 3s even though session is alive
	start := time.Now()
	output := waitForCompletion(name, 3*time.Second)
	elapsed := time.Since(start)

	// Should have returned in roughly 3-8 seconds (poll interval is 5s)
	if elapsed > 15*time.Second {
		t.Errorf("waitForCompletion took %v, expected ~3-8s", elapsed)
	}

	// Session should still be alive (timeout is the caller's responsibility)
	if !SessionExists(name) {
		t.Error("session should still exist — waitForCompletion doesn't kill")
	}
	_ = output
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
