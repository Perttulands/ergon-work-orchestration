package worker

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"polis/work/internal/testutil"
)

// --- fakeTmux implements tmuxBackend for deterministic, instant tests ---

type fakeSession struct {
	workDir string
	pane    string
	onEnter func(*fakeSession) // called when "Enter" is sent via sendKeysRaw
}

type fakeTmux struct {
	mu        sync.Mutex
	sessions  map[string]*fakeSession
	tmuxAvail bool
}

func newFakeTmux() *fakeTmux {
	return &fakeTmux{
		sessions:  make(map[string]*fakeSession),
		tmuxAvail: true,
	}
}

func (f *fakeTmux) requireTmux() error {
	if !f.tmuxAvail {
		return fmt.Errorf("tmux not found on PATH")
	}
	return nil
}

func (f *fakeTmux) createSession(name, workDir string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.sessions[name]; exists {
		return fmt.Errorf("duplicate session: %s", name)
	}
	f.sessions[name] = &fakeSession{workDir: workDir, pane: "$ "}
	return nil
}

func (f *fakeTmux) sendKeys(session, keys string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[session]
	if !ok {
		return fmt.Errorf("no such session: %s", session)
	}
	s.pane += keys + "\n"
	return nil
}

func (f *fakeTmux) sendKeysRaw(session string, keys ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[session]
	if !ok {
		return fmt.Errorf("no such session: %s", session)
	}
	for _, k := range keys {
		if k == "Enter" {
			if s.onEnter != nil {
				s.onEnter(s)
			}
			s.pane += "\n"
		} else {
			s.pane += k
		}
	}
	return nil
}

func (f *fakeTmux) sendPrompt(session, prompt string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[session]
	if !ok {
		return fmt.Errorf("no such session: %s", session)
	}
	s.pane += prompt + "\n"
	return nil
}

func (f *fakeTmux) capturePane(session string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[session]
	if !ok {
		return "", fmt.Errorf("no such session: %s", session)
	}
	return s.pane, nil
}

func (f *fakeTmux) killSession(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.sessions[name]; !ok {
		return fmt.Errorf("kill session %s: no such session", name)
	}
	delete(f.sessions, name)
	return nil
}

func (f *fakeTmux) sessionExists(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.sessions[name]
	return ok
}

// addSession pre-creates a session with specific pane content.
func (f *fakeTmux) addSession(name, workDir, pane string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions[name] = &fakeSession{workDir: workDir, pane: pane}
}

// useFake installs a fakeTmux as the backend, shortens poll intervals,
// and restores everything on test cleanup.
func useFake(t *testing.T) *fakeTmux {
	t.Helper()
	fake := newFakeTmux()

	origBackend := backend
	origReadyPoll := readyPollInterval
	origCompletionPoll := completionPollInterval
	origTrustSleep := readySleepAfterTrust
	origEnvDelay := spawnEnvSetupDelay

	backend = fake
	readyPollInterval = 1 * time.Millisecond
	completionPollInterval = 1 * time.Millisecond
	readySleepAfterTrust = 0
	spawnEnvSetupDelay = 0

	t.Cleanup(func() {
		backend = origBackend
		readyPollInterval = origReadyPoll
		completionPollInterval = origCompletionPoll
		readySleepAfterTrust = origTrustSleep
		spawnEnvSetupDelay = origEnvDelay
	})

	return fake
}

// --- tests ---

func TestRequireTmux(t *testing.T) {
	useFake(t)
	if err := requireTmux(); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestCreateAndKillSession(t *testing.T) {
	useFake(t)
	name := t.Name()

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
	useFake(t)
	name := t.Name()

	if err := createSession(name, "/tmp"); err != nil {
		t.Fatalf("create session: %v", err)
	}

	if err := sendKeys(name, "echo HELLO_WORK_TEST"); err != nil {
		t.Fatalf("send keys: %v", err)
	}

	output, err := capturePane(name)
	if err != nil {
		t.Fatalf("capture pane: %v", err)
	}

	if !strings.Contains(output, "HELLO_WORK_TEST") {
		t.Errorf("expected output to contain HELLO_WORK_TEST, got: %s", output)
	}
}

func TestSessionExistsNonExistent(t *testing.T) {
	useFake(t)
	if SessionExists("work-test-nonexistent-99999") {
		t.Error("non-existent session should return false")
	}
}

func TestConfigValidation(t *testing.T) {
	useFake(t)

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

func TestDetectCompletionNewUILayout(t *testing.T) {
	// Claude Code v2.1+ shows bypass notice and separator after ❯
	output := "Working on task...\nDone! All tests pass.\n❯ Try \"fix typecheck errors\"\n────────────────────────\n  ⏵⏵ bypass permissions on (shift+tab to cycle)"
	if !detectCompletion(output) {
		t.Error("expected completion detected for new Claude Code UI layout with trailing lines after ❯")
	}
}

func TestDetectCompletionNewUIStillWorking(t *testing.T) {
	// New UI layout but with tool activity before ❯
	output := "some output\nReading src/main.go\n❯ commit this\n────────────────────────\n  ⏵⏵ bypass permissions on"
	if detectCompletion(output) {
		t.Error("should NOT detect completion when tool activity precedes ❯ in new UI layout")
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
	useFake(t)
	name := t.Name()

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
	useFake(t)
	err := KillSession("work-test-nonexistent-99999")
	if err == nil {
		t.Error("KillSession should fail for non-existent session")
	}
}

// --- sendKeysRaw tests ---

func TestSendKeysRaw(t *testing.T) {
	useFake(t)
	name := t.Name()

	if err := createSession(name, "/tmp"); err != nil {
		t.Fatalf("create session: %v", err)
	}

	if err := sendKeysRaw(name, "echo RAW_TEST_OUTPUT", "Enter"); err != nil {
		t.Fatalf("sendKeysRaw: %v", err)
	}

	output, err := capturePane(name)
	if err != nil {
		t.Fatalf("capture pane: %v", err)
	}
	if !strings.Contains(output, "RAW_TEST_OUTPUT") {
		t.Errorf("expected RAW_TEST_OUTPUT in pane, got: %s", output)
	}
}

func TestSendKeysRawNonExistent(t *testing.T) {
	useFake(t)
	err := sendKeysRaw("work-test-noexist-12345", "echo hi")
	if err == nil {
		t.Error("sendKeysRaw should fail for non-existent session")
	}
}

// --- sendPrompt tests ---

func TestSendPrompt(t *testing.T) {
	useFake(t)
	name := t.Name()

	if err := createSession(name, "/tmp"); err != nil {
		t.Fatalf("create session: %v", err)
	}

	prompt := "HELLO_PROMPT_TEST_12345"
	if err := sendPrompt(name, prompt); err != nil {
		t.Fatalf("sendPrompt: %v", err)
	}

	output, err := capturePane(name)
	if err != nil {
		t.Fatalf("capture pane: %v", err)
	}
	if !strings.Contains(output, "HELLO_PROMPT_TEST_12345") {
		t.Errorf("expected prompt text in pane, got: %s", output)
	}
}

func TestSendPromptMultiLine(t *testing.T) {
	useFake(t)
	name := t.Name()

	if err := createSession(name, "/tmp"); err != nil {
		t.Fatalf("create session: %v", err)
	}

	prompt := "LINE_ONE\nLINE_TWO\nLINE_THREE"
	if err := sendPrompt(name, prompt); err != nil {
		t.Fatalf("sendPrompt multiline: %v", err)
	}

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
	fake := useFake(t)
	name := t.Name()

	// Pre-create session with banner already visible
	fake.addSession(name, "/tmp", "Claude Code v1.2.3\n❯ ")

	profile := runtimeSpec{ReadyPatterns: []string{"Claude Code v"}, TrustPatterns: []string{"trust this folder"}}
	if err := waitForReady(name, "claude", profile, 100*time.Millisecond); err != nil {
		t.Fatalf("waitForReady should succeed: %v", err)
	}
}

func TestWaitForReadyTimeout(t *testing.T) {
	fake := useFake(t)
	name := t.Name()

	// Session with no banner — should time out
	fake.addSession(name, "/tmp", "Loading...")

	profile := runtimeSpec{ReadyPatterns: []string{"Claude Code v"}, TrustPatterns: []string{"trust this folder"}}
	err := waitForReady(name, "claude", profile, 50*time.Millisecond)
	if err == nil {
		t.Error("waitForReady should time out")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected timeout error, got: %v", err)
	}
}

func TestWaitForReadyTrustDismissal(t *testing.T) {
	fake := useFake(t)
	name := t.Name()

	// Start with trust dialog; when Enter is pressed, transition to banner
	fake.addSession(name, "/tmp", "Do you trust this folder?\n> 1. Yes")
	fake.mu.Lock()
	fake.sessions[name].onEnter = func(s *fakeSession) {
		s.pane = "Claude Code v1.2.3\n❯ "
		s.onEnter = nil
	}
	fake.mu.Unlock()

	profile := runtimeSpec{ReadyPatterns: []string{"Claude Code v"}, TrustPatterns: []string{"trust this folder"}}
	if err := waitForReady(name, "claude", profile, 2*time.Second); err != nil {
		t.Fatalf("waitForReady should succeed after trust dismissal: %v", err)
	}
}

func TestWaitForReadyCaptureError(t *testing.T) {
	useFake(t)
	// Non-existent session — capturePane will fail
	profile := runtimeSpec{ReadyPatterns: []string{"Claude Code v"}, TrustPatterns: []string{"trust this folder"}}
	err := waitForReady("nonexistent-session", "claude", profile, 50*time.Millisecond)
	if err == nil {
		t.Error("waitForReady should fail on non-existent session")
	}
}

// --- waitForCompletion tests ---

func TestWaitForCompletionDetected(t *testing.T) {
	fake := useFake(t)
	name := t.Name()

	// Pre-create session with completion pattern:
	// - More than 2 lines
	// - Last line starts with ❯
	// - Previous line has no working indicators
	fake.addSession(name, "/tmp", "Working on task...\nDone! All tests pass.\n❯")

	output := waitForCompletion(name, 100*time.Millisecond)
	if !strings.Contains(output, "Done!") {
		t.Errorf("expected 'Done!' in output, got: %s", output)
	}
}

func TestWaitForCompletionSessionKilled(t *testing.T) {
	fake := useFake(t)
	name := t.Name()

	// Session with no completion markers
	fake.addSession(name, "/tmp", "some preliminary output\nstill running...")

	// Kill session after a tiny delay
	go func() {
		time.Sleep(5 * time.Millisecond)
		fake.killSession(name)
	}()

	// waitForCompletion should return when session dies
	output := waitForCompletion(name, 1*time.Second)
	// Just verify it returned (didn't hang)
	_ = output
}

// --- Spawn validation and lifecycle tests ---

func TestSpawnNoTmux(t *testing.T) {
	fake := useFake(t)
	fake.tmuxAvail = false

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
	fake := useFake(t)
	name := t.Name()

	// Pre-create the session to cause a conflict
	fake.addSession(name, "/tmp", "$ ")

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
	fake := useFake(t)
	name := t.Name()

	// Session that never shows completion markers
	fake.addSession(name, "/tmp", "$ sleep 300")

	start := time.Now()
	output := waitForCompletion(name, 50*time.Millisecond)
	elapsed := time.Since(start)

	if elapsed > 1*time.Second {
		t.Errorf("waitForCompletion took %v, expected <1s", elapsed)
	}

	// Session should still be alive (timeout is the caller's responsibility)
	if !SessionExists(name) {
		t.Error("session should still exist — waitForCompletion doesn't kill")
	}
	_ = output
}

// --- integration tests: exercise realTmux methods via SandboxPATH fake tmux ---

// fakeTmuxScript is a shell script that mimics tmux using temp files for state.
// SandboxPATH prepends "#!/bin/sh\nset -e\n" so we just provide the body.
const fakeTmuxScript = `
DIR="$FAKE_TMUX_DIR"
mkdir -p "$DIR"
case "$1" in
  new-session)
    NAME="$4"
    if [ -f "$DIR/sess-$NAME" ]; then
      echo "duplicate session: $NAME" >&2
      exit 1
    fi
    > "$DIR/sess-$NAME"
    > "$DIR/pane-$NAME"
    ;;
  has-session)
    NAME="$3"
    [ -f "$DIR/sess-$NAME" ]
    ;;
  send-keys)
    NAME="$3"
    [ -f "$DIR/sess-$NAME" ] || { echo "can't find session: $NAME" >&2; exit 1; }
    shift 3
    for arg in "$@"; do
      if [ "$arg" != "Enter" ]; then
        printf '%s' "$arg" >> "$DIR/pane-$NAME"
      fi
    done
    printf '\n' >> "$DIR/pane-$NAME"
    ;;
  kill-session)
    NAME="$3"
    [ -f "$DIR/sess-$NAME" ] || { echo "can't find session: $NAME" >&2; exit 1; }
    rm -f "$DIR/sess-$NAME" "$DIR/pane-$NAME"
    ;;
  capture-pane)
    NAME="$3"
    [ -f "$DIR/sess-$NAME" ] || { echo "can't find session: $NAME" >&2; exit 1; }
    cat "$DIR/pane-$NAME" 2>/dev/null
    ;;
  load-buffer)
    cp "$2" "$DIR/buffer" 2>/dev/null
    ;;
  paste-buffer)
    NAME="$3"
    [ -f "$DIR/sess-$NAME" ] || { echo "no session: $NAME" >&2; exit 1; }
    [ -f "$DIR/buffer" ] && cat "$DIR/buffer" >> "$DIR/pane-$NAME"
    ;;
esac
`

// useRealWithSandboxTmux installs the realTmux backend with a fake tmux binary
// on PATH, enabling fast deterministic tests of the realTmux methods.
func useRealWithSandboxTmux(t *testing.T) {
	t.Helper()
	stateDir := t.TempDir()
	t.Setenv("FAKE_TMUX_DIR", stateDir)
	testutil.SandboxPATH(t, map[string]string{"tmux": fakeTmuxScript})

	origBackend := backend
	backend = &realTmux{}
	t.Cleanup(func() { backend = origBackend })
}

func TestRealTmuxSessionLifecycle(t *testing.T) {
	useRealWithSandboxTmux(t)
	name := t.Name()

	// Create
	if err := createSession(name, "/tmp"); err != nil {
		t.Fatalf("createSession: %v", err)
	}
	if !SessionExists(name) {
		t.Error("session should exist after creation")
	}

	// Duplicate should fail
	if err := createSession(name, "/tmp"); err == nil {
		t.Error("duplicate createSession should fail")
	}

	// Kill
	if err := killSession(name); err != nil {
		t.Fatalf("killSession: %v", err)
	}
	if SessionExists(name) {
		t.Error("session should not exist after kill")
	}

	// Kill non-existent should fail
	if err := killSession(name); err == nil {
		t.Error("killSession on non-existent should fail")
	}
}

func TestRealTmuxSendAndCapture(t *testing.T) {
	useRealWithSandboxTmux(t)
	name := t.Name()

	if err := createSession(name, "/tmp"); err != nil {
		t.Fatalf("createSession: %v", err)
	}

	// sendKeys
	if err := sendKeys(name, "echo INTEGRATION_TEST"); err != nil {
		t.Fatalf("sendKeys: %v", err)
	}

	output, err := capturePane(name)
	if err != nil {
		t.Fatalf("capturePane: %v", err)
	}
	if !strings.Contains(output, "INTEGRATION_TEST") {
		t.Errorf("expected INTEGRATION_TEST in pane, got: %q", output)
	}

	// sendKeysRaw
	if err := sendKeysRaw(name, "echo RAW_INTEGRATION", "Enter"); err != nil {
		t.Fatalf("sendKeysRaw: %v", err)
	}

	output, err = capturePane(name)
	if err != nil {
		t.Fatalf("capturePane after raw: %v", err)
	}
	if !strings.Contains(output, "RAW_INTEGRATION") {
		t.Errorf("expected RAW_INTEGRATION in pane, got: %q", output)
	}

	// sendKeys on non-existent session
	if err := sendKeys("no-such-session", "test"); err == nil {
		t.Error("sendKeys on non-existent should fail")
	}

	// capturePane on non-existent session
	if _, err := capturePane("no-such-session"); err == nil {
		t.Error("capturePane on non-existent should fail")
	}

	// sendKeysRaw on non-existent session
	if err := sendKeysRaw("no-such-session", "test"); err == nil {
		t.Error("sendKeysRaw on non-existent should fail")
	}
}

func TestRealTmuxSendPrompt(t *testing.T) {
	useRealWithSandboxTmux(t)
	name := t.Name()

	if err := createSession(name, "/tmp"); err != nil {
		t.Fatalf("createSession: %v", err)
	}

	prompt := "BEAD: test-integration\nTASK: verify sendPrompt"
	if err := sendPrompt(name, prompt); err != nil {
		t.Fatalf("sendPrompt: %v", err)
	}

	output, err := capturePane(name)
	if err != nil {
		t.Fatalf("capturePane: %v", err)
	}
	if !strings.Contains(output, "BEAD: test-integration") {
		t.Errorf("expected prompt text in pane, got: %q", output)
	}

	// sendPrompt on non-existent session
	if err := sendPrompt("no-such-session", "test"); err == nil {
		t.Error("sendPrompt on non-existent should fail")
	}
}

func TestRealTmuxRequire(t *testing.T) {
	useRealWithSandboxTmux(t)
	// Fake tmux is on PATH — should succeed
	if err := requireTmux(); err != nil {
		t.Errorf("requireTmux should succeed with fake tmux on PATH: %v", err)
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
		{
			name:   "idle with new UI layout",
			output: "some output\nDone.\n❯ Try something\n──────\n  ⏵⏵ bypass",
			want:   false,
		},
		{
			name:   "reading with new UI layout",
			output: "some output\nReading src/main.go\n❯ commit\n──────\n  ⏵⏵ bypass",
			want:   true,
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

func TestSendFollowUpOK(t *testing.T) {
	fake := useFake(t)
	fake.sessions["test-session"] = &fakeSession{pane: "❯ "}

	err := SendFollowUp("test-session", "Please add unit tests")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the message was sent to the pane
	pane, _ := fake.capturePane("test-session")
	if !strings.Contains(pane, "Please add unit tests") {
		t.Errorf("pane should contain follow-up message, got: %q", pane)
	}
}

func TestSendFollowUpSessionNotFound(t *testing.T) {
	_ = useFake(t)
	err := SendFollowUp("nonexistent-session", "hello")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestWaitForCompletionExported(t *testing.T) {
	fake := useFake(t)
	fake.sessions["wfc-session"] = &fakeSession{pane: "working..."}

	// Set pane to idle after a brief moment
	go func() {
		time.Sleep(5 * time.Millisecond)
		fake.mu.Lock()
		fake.sessions["wfc-session"].pane = "done\n❯ "
		fake.mu.Unlock()
	}()

	output := WaitForCompletion("wfc-session", 2*time.Second)
	if !strings.Contains(output, "❯") {
		t.Errorf("should detect completion, got: %q", output)
	}
}
