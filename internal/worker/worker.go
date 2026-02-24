// Package worker handles tmux-based Claude Code worker spawning.
package worker

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Config holds worker spawn configuration.
type Config struct {
	SessionName string        // tmux session name (must be unique)
	WorkDir     string        // working directory for the session
	Prompt      string        // the assembled task prompt to send
	Deadline    time.Duration // max time before killing the session
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
	if err := requireTmux(); err != nil {
		return nil, fmt.Errorf("spawn: %w", err)
	}
	if cfg.SessionName == "" {
		return nil, fmt.Errorf("session name required")
	}
	if cfg.WorkDir == "" {
		return nil, fmt.Errorf("work directory required")
	}

	result := &Result{
		SessionName: cfg.SessionName,
		Started:     time.Now(),
	}

	// 1. Create tmux session
	if err := createSession(cfg.SessionName, cfg.WorkDir); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	// 2. Unset CLAUDECODE (critical — nested sessions crash otherwise)
	if err := sendKeys(cfg.SessionName, "unset CLAUDECODE CLAUDE_CODE_ENTRYPOINT ANTHROPIC_API_KEY_PARENT"); err != nil {
		killSession(cfg.SessionName)
		return nil, fmt.Errorf("unset env: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	// 3. Launch claude --dangerously-skip-permissions
	if err := sendKeys(cfg.SessionName, "claude --dangerously-skip-permissions"); err != nil {
		killSession(cfg.SessionName)
		return nil, fmt.Errorf("launch claude: %w", err)
	}

	// 4. Wait for claude to be ready
	if err := waitForReady(cfg.SessionName, 30*time.Second); err != nil {
		killSession(cfg.SessionName)
		return nil, fmt.Errorf("wait for ready: %w", err)
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
	output := waitForCompletion(cfg.SessionName, cfg.Deadline+time.Minute)
	close(done)

	mu.Lock()
	result.TimedOut = timedOut
	mu.Unlock()
	result.Finished = time.Now()
	result.Output = output

	return result, nil
}

// KillSession terminates a tmux session.
func KillSession(name string) error {
	return killSession(name)
}

// SessionExists checks if a tmux session exists.
func SessionExists(name string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", name)
	return cmd.Run() == nil
}

func requireTmux() error {
	if _, err := exec.LookPath("tmux"); err != nil {
		return fmt.Errorf("tmux not found on PATH")
	}
	return nil
}

func createSession(name, workDir string) error {
	cmd := exec.Command("tmux", "new-session", "-d", "-s", name, "-c", workDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %s", err, out)
	}
	return nil
}

func sendKeys(session, keys string) error {
	cmd := exec.Command("tmux", "send-keys", "-t", session, keys, "Enter")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %s", err, out)
	}
	return nil
}

func sendPrompt(session, prompt string) error {
	// For multi-line prompts, write to a temp approach: send as single block
	// tmux send-keys handles this, but we need to escape special chars
	cmd := exec.Command("tmux", "send-keys", "-t", session, prompt, "Enter")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %s", err, out)
	}
	return nil
}

func capturePane(session string) (string, error) {
	cmd := exec.Command("tmux", "capture-pane", "-t", session, "-p")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("capture pane %s: %w", session, err)
	}
	return string(out), nil
}

func waitForReady(session string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		output, err := capturePane(session)
		if err != nil {
			return fmt.Errorf("wait for ready: %w", err)
		}
		// Claude Code shows a prompt character when ready
		if strings.Contains(output, "❯") || strings.Contains(output, ">") {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timed out waiting for claude to start (session: %s)", session)
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

		// Detect completion: claude shows prompt again after finishing work
		lines := strings.Split(strings.TrimSpace(output), "\n")
		if len(lines) > 2 {
			lastLine := strings.TrimSpace(lines[len(lines)-1])
			// Claude shows ❯ when idle/waiting for input
			if strings.HasPrefix(lastLine, "❯") && !isStillWorking(output) {
				return output
			}
		}

		// Also check if session is gone
		if !SessionExists(session) {
			return lastOutput
		}

		time.Sleep(5 * time.Second)
	}
	return lastOutput
}

// isStillWorking checks if the pane output suggests active work.
func isStillWorking(output string) bool {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 3 {
		return false
	}
	// If the second-to-last line contains tool activity markers, still working
	prev := lines[len(lines)-2]
	workingIndicators := []string{"Reading", "Writing", "Editing", "Running", "Searching"}
	for _, ind := range workingIndicators {
		if strings.Contains(prev, ind) {
			return true
		}
	}
	return false
}

func killSession(name string) error {
	cmd := exec.Command("tmux", "kill-session", "-t", name)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kill session %s: %w", name, err)
	}
	return nil
}
