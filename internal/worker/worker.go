// Package worker handles tmux-based worker spawning.
package worker

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// TmuxClient abstracts tmux operations for testing.
type TmuxClient interface {
	requireTmux() error
	createSession(name, workDir string) error
	sendKeys(session, keys string) error
	sendKeysRaw(session string, keys ...string) error
	sendPrompt(session, prompt string) error
	capturePane(session string) (string, error)
	killSession(name string) error
	sessionExists(name string) bool
}

// backend is the active tmux backend. Tests may replace it.
var backend TmuxClient = &RealTmuxClient{}

// Poll/delay intervals. Tests may shorten these.
var (
	readyPollInterval      = 2 * time.Second
	completionPollInterval = 5 * time.Second
	readySleepAfterTrust   = 1 * time.Second
	spawnEnvSetupDelay     = 500 * time.Millisecond
)

// Config holds worker spawn configuration.
type Config struct {
	SessionName string        // tmux session name (must be unique)
	WorkDir     string        // working directory for the session
	Prompt      string        // the assembled task prompt to send
	Deadline    time.Duration // max time before killing the session
	AgentName   string        // relay identity (sets RELAY_AGENT env var)
	Runtime     string        // worker runtime profile: codex, claude (default: codex)
}

// Result holds the outcome of a worker run.
type Result struct {
	SessionName string
	Started     time.Time
	Finished    time.Time
	TimedOut    bool
	Output      string // last captured pane output
}

// Spawn creates a tmux session, starts claude, sends the prompt, and waits for completion.
func Spawn(cfg Config) (*Result, error) {
	result, runtime, err := startSession(cfg)
	if err != nil {
		return nil, fmt.Errorf("spawn worker session: %w", err)
	}

	// For spawn-only workflows we allow an empty prompt and return once the
	// session is ready.
	if strings.TrimSpace(cfg.Prompt) == "" {
		if output, capErr := capturePane(cfg.SessionName); capErr == nil {
			result.Output = output
		}
		return result, nil
	}

	// 5. Send the task prompt
	if err := sendPrompt(cfg.SessionName, cfg.Prompt); err != nil {
		killSession(cfg.SessionName)
		return nil, fmt.Errorf("send prompt: %w", err)
	}

	// 6. Start deadline watchdog and wait for completion
	done := make(chan struct{})
	var mu sync.Mutex
	timedOut := false

	if cfg.Deadline > 0 {
		go func() {
			select {
			case <-time.After(cfg.Deadline):
				mu.Lock()
				timedOut = true
				mu.Unlock()
				killSession(cfg.SessionName)
			case <-done:
			}
		}()
	}

	// 7. Monitor for completion
	maxWait := 30 * time.Minute
	if cfg.Deadline > 0 {
		maxWait = cfg.Deadline + time.Minute
	}
	output := waitForCompletion(cfg.SessionName, maxWait)
	close(done)

	mu.Lock()
	result.TimedOut = timedOut
	mu.Unlock()
	result.Finished = time.Now()
	result.Output = output

	_ = runtime // resolved in startSession; kept here for clarity in call flow.
	return result, nil
}

// Start creates a tmux session, launches the runtime, and waits until ready.
// It does not send a prompt or wait for task completion.
func Start(cfg Config) (*Result, error) {
	result, _, err := startSession(cfg)
	if err != nil {
		return nil, fmt.Errorf("start worker session: %w", err)
	}
	if output, capErr := capturePane(cfg.SessionName); capErr == nil {
		result.Output = output
	}
	return result, nil
}

func startSession(cfg Config) (*Result, string, error) {
	runtime, runtimeProfile, err := resolveRuntimeProfile(cfg.Runtime, cfg.AgentName)
	if err != nil {
		return nil, "", fmt.Errorf("resolve runtime profile: %w", err)
	}

	if err := requireTmux(); err != nil {
		return nil, "", fmt.Errorf("spawn: %w", err)
	}
	if cfg.SessionName == "" {
		return nil, "", fmt.Errorf("session name required")
	}
	if cfg.WorkDir == "" {
		return nil, "", fmt.Errorf("work directory required")
	}

	result := &Result{
		SessionName: cfg.SessionName,
		Started:     time.Now(),
	}

	// 1. Create tmux session
	if err := createSession(cfg.SessionName, cfg.WorkDir); err != nil {
		return nil, "", fmt.Errorf("create session: %w", err)
	}

	// 2. Unset CLAUDECODE (critical — nested sessions crash otherwise)
	if err := sendKeys(cfg.SessionName, "unset CLAUDECODE CLAUDE_CODE_ENTRYPOINT ANTHROPIC_API_KEY_PARENT"); err != nil {
		killSession(cfg.SessionName)
		return nil, "", fmt.Errorf("unset env: %w", err)
	}
	time.Sleep(spawnEnvSetupDelay)

	// 2b. Set RELAY_AGENT so the worker identifies correctly on the relay bus
	if cfg.AgentName != "" {
		if err := sendKeys(cfg.SessionName, fmt.Sprintf("export RELAY_AGENT=%s", cfg.AgentName)); err != nil {
			killSession(cfg.SessionName)
			return nil, "", fmt.Errorf("set relay agent: %w", err)
		}
		time.Sleep(spawnEnvSetupDelay)
	}

	launchCmd, cmdErr := buildLaunchCommand(runtimeProfile)
	if cmdErr != nil {
		killSession(cfg.SessionName)
		return nil, "", cmdErr
	}
	if err := sendKeys(cfg.SessionName, launchCmd); err != nil {
		killSession(cfg.SessionName)
		return nil, "", fmt.Errorf("launch %s: %w", runtime, err)
	}

	// 4. Wait for the runtime to be ready (60s allows for trust dialog + slow startup)
	if err := waitForReady(cfg.SessionName, runtime, runtimeProfile, 60*time.Second); err != nil {
		killSession(cfg.SessionName)
		return nil, "", fmt.Errorf("wait for ready: %w", err)
	}
	return result, runtime, nil
}

// KillSession terminates a tmux session.
func KillSession(name string) error {
	return killSession(name)
}

// SessionExists checks if a tmux session exists.
func SessionExists(name string) bool {
	return backend.sessionExists(name)
}

// SendPrompt injects a prompt into a running tmux session using load-buffer + paste-buffer.
func SendPrompt(session, prompt string) error {
	return backend.sendPrompt(session, prompt)
}

// --- package-level delegators to backend ---

func requireTmux() error                               { return backend.requireTmux() }
func createSession(name, workDir string) error         { return backend.createSession(name, workDir) }
func sendKeys(session, keys string) error              { return backend.sendKeys(session, keys) }
func sendKeysRaw(session string, keys ...string) error { return backend.sendKeysRaw(session, keys...) }
func sendPrompt(session, prompt string) error          { return backend.sendPrompt(session, prompt) }
func capturePane(session string) (string, error)       { return backend.capturePane(session) }
func killSession(name string) error                    { return backend.killSession(name) }

// --- RealTmuxClient implements TmuxClient using actual tmux commands ---

type RealTmuxClient struct{}

func (r *RealTmuxClient) requireTmux() error {
	if _, err := exec.LookPath("tmux"); err != nil {
		return fmt.Errorf("tmux not found on PATH")
	}
	return nil
}

func (r *RealTmuxClient) createSession(name, workDir string) error {
	cmd := exec.Command("tmux", "new-session", "-d", "-s", name, "-c", workDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %s", err, out)
	}
	return nil
}

func (r *RealTmuxClient) sendKeys(session, keys string) error {
	cmd := exec.Command("tmux", "send-keys", "-t", session, keys)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %s", err, out)
	}
	// Delay before Enter to let the TUI process pasted content
	time.Sleep(200 * time.Millisecond)
	// Double Enter — second is a no-op safety net for TUIs that sometimes miss the first
	if err := r.sendKeysRaw(session, "ENTER"); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	return r.sendKeysRaw(session, "ENTER")
}

func (r *RealTmuxClient) sendKeysRaw(session string, keys ...string) error {
	args := append([]string{"send-keys", "-t", session}, keys...)
	cmd := exec.Command("tmux", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %s", err, out)
	}
	return nil
}

func (r *RealTmuxClient) sendPrompt(session, prompt string) error {
	// Write prompt to a temp file and use tmux load-buffer + paste-buffer.
	// This is safer than send-keys for multi-line / long prompts because
	// tmux send-keys interprets special characters and can corrupt text.
	f, err := os.CreateTemp("", "work-prompt-*.txt")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := f.Name()
	defer os.Remove(tmpPath)

	if _, err := f.WriteString(prompt); err != nil {
		f.Close()
		return fmt.Errorf("write prompt: %w", err)
	}
	f.Close()

	// Load into tmux buffer
	loadCmd := exec.Command("tmux", "load-buffer", tmpPath)
	if out, err := loadCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("load-buffer: %s: %s", err, out)
	}

	// Paste into the target pane
	pasteCmd := exec.Command("tmux", "paste-buffer", "-t", session)
	if out, err := pasteCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("paste-buffer: %s: %s", err, out)
	}

	// Small delay to let the TUI process the pasted content before Enter
	time.Sleep(200 * time.Millisecond)

	// Send Enter twice — first submits the prompt, second is a no-op safety net
	// (hitting Enter on an empty prompt field does nothing in Codex/Claude)
	if err := r.sendKeysRaw(session, "ENTER"); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	return r.sendKeysRaw(session, "ENTER")
}

func (r *RealTmuxClient) capturePane(session string) (string, error) {
	cmd := exec.Command("tmux", "capture-pane", "-t", session, "-p")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("capture pane %s: %w", session, err)
	}
	return string(out), nil
}

func (r *RealTmuxClient) killSession(name string) error {
	cmd := exec.Command("tmux", "kill-session", "-t", name)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kill session %s: %w", name, err)
	}
	return nil
}

func (r *RealTmuxClient) sessionExists(name string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", name)
	return cmd.Run() == nil
}

// --- detection and wait logic ---

// readyState describes what waitForReady detected in a pane capture.
type readyState int

const (
	readyNotYet    readyState = iota // still loading
	readyOK                          // Claude Code banner visible
	readyNeedTrust                   // trust dialog visible — needs Enter
)

// detectReady inspects captured pane output and returns the ready state.
func detectReady(output string) readyState {
	return detectReadyWithPatterns(output, []string{"Claude Code v", "OpenAI Codex (v", ">_ OpenAI Codex"}, []string{"trust this folder"})
}

func detectReadyWithPatterns(output string, readyPatterns, trustPatterns []string) readyState {
	for _, p := range readyPatterns {
		if p != "" && strings.Contains(output, p) {
			return readyOK
		}
	}
	lower := strings.ToLower(output)
	for _, p := range trustPatterns {
		p = strings.ToLower(strings.TrimSpace(p))
		if p != "" && strings.Contains(lower, p) {
			return readyNeedTrust
		}
	}
	// Backward-compatible fallback when config patterns are missing.
	if strings.Contains(output, "Claude Code v") || strings.Contains(output, "OpenAI Codex (v") {
		return readyOK
	}
	if strings.Contains(lower, "trust this folder") {
		return readyNeedTrust
	}
	return readyNotYet
}

func waitForReady(session, runtime string, profile runtimeSpec, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	trustDismissed := false
	for time.Now().Before(deadline) {
		output, err := capturePane(session)
		if err != nil {
			return fmt.Errorf("wait for ready: %w", err)
		}

		state := detectReadyWithPatterns(output, profile.ReadyPatterns, profile.TrustPatterns)
		if state == readyOK {
			return nil
		}
		if state == readyNeedTrust && !trustDismissed {
			_ = sendKeysRaw(session, "ENTER")
			time.Sleep(100 * time.Millisecond)
			_ = sendKeysRaw(session, "ENTER")
			trustDismissed = true
			time.Sleep(readySleepAfterTrust)
			continue
		}

		time.Sleep(readyPollInterval)
	}
	return fmt.Errorf("timed out waiting for %s to start (session: %s)", runtime, session)
}

// detectCompletion checks if pane output indicates the worker is done.
// Returns true when the idle prompt (❯) appears near the bottom of the
// pane and no tool activity is detected nearby. Claude Code's UI may show
// additional lines after ❯ (separator, bypass permissions notice, suggestions),
// so we scan the last several lines rather than just the last one.
func detectCompletion(output string) bool {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) <= 2 {
		return false
	}
	// Scan the last 5 lines for the ❯ prompt
	start := len(lines) - 5
	if start < 0 {
		start = 0
	}
	for _, line := range lines[start:] {
		if isPromptLine(strings.TrimSpace(line)) {
			return !isStillWorking(output)
		}
	}
	return false
}

func waitForCompletion(session string, maxWait time.Duration) string {
	deadline := time.Now().Add(maxWait)
	var lastOutput string

	for time.Now().Before(deadline) {
		output, err := capturePane(session)
		if err != nil {
			// Session likely killed
			return lastOutput
		}
		lastOutput = output

		if detectCompletion(output) {
			return output
		}

		// Also check if session is gone
		if !SessionExists(session) {
			return lastOutput
		}

		time.Sleep(completionPollInterval)
	}
	return lastOutput
}

// isStillWorking checks if the pane output suggests active work.
// Scans the last several lines for tool activity markers.
func isStillWorking(output string) bool {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 3 {
		return false
	}
	// Check lines near the bottom for tool activity markers
	start := len(lines) - 6
	if start < 0 {
		start = 0
	}
	workingIndicators := []string{"Reading", "Writing", "Editing", "Running", "Searching"}
	for _, line := range lines[start:] {
		trimmed := strings.TrimSpace(line)
		// Skip the ❯ line itself and lines below it
		if isPromptLine(trimmed) {
			break
		}
		for _, ind := range workingIndicators {
			if strings.Contains(line, ind) {
				return true
			}
		}
	}
	return false
}

func isPromptLine(line string) bool {
	return strings.HasPrefix(line, "❯") || strings.HasPrefix(line, "›")
}

// SendFollowUp sends a follow-up message to an existing tmux session.
// Uses tmux send-keys -l (literal mode) to handle special characters.
func SendFollowUp(session, message string) error {
	if !backend.sessionExists(session) {
		return fmt.Errorf("session %s not found", session)
	}
	// Use send-keys -l for literal text (no special character interpretation),
	// then send Enter to submit.
	if err := backend.sendKeysRaw(session, "-l", message); err != nil {
		return fmt.Errorf("send follow-up text: %w", err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := backend.sendKeysRaw(session, "ENTER"); err != nil {
		return fmt.Errorf("send follow-up enter: %w", err)
	}
	return nil
}

// WaitForCompletion re-exports waitForCompletion for use by the Squire retry path.
func WaitForCompletion(session string, maxWait time.Duration) string {
	return waitForCompletion(session, maxWait)
}
