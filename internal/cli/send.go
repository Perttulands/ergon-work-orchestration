package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

func newSendCmd() *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "send <session> <prompt>",
		Short: "Send a prompt to a running tmux worker session",
		Long:  `Injects a prompt into a running tmux worker session via tmux send-keys.`,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			session := args[0]
			return runSend(cmd, session, args[1:], filePath)
		},
	}

	cmd.Flags().StringVar(&filePath, "file", "", "read prompt from file instead of args")

	return cmd
}

func runSend(cmd *cobra.Command, session string, promptArgs []string, filePath string) error {
	if filePath != "" && len(promptArgs) > 0 {
		return fmt.Errorf("provide prompt args or --file, not both")
	}

	exists, err := sessionExists(session)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("session not found: %s", session)
	}

	prompt, err := loadSendPrompt(promptArgs, filePath)
	if err != nil {
		return err
	}

	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("prompt is empty")
	}

	if err := sendPrompt(session, prompt); err != nil {
		return err
	}

	cmd.Printf("Sent prompt to session %s\n", session)
	return nil
}

func loadSendPrompt(promptArgs []string, filePath string) (string, error) {
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("read prompt file: %w", err)
		}
		return string(data), nil
	}
	if len(promptArgs) == 0 {
		return "", fmt.Errorf("prompt text required (provide args or --file)")
	}
	return strings.Join(promptArgs, " "), nil
}

func sessionExists(session string) (bool, error) {
	cmd := exec.Command("tmux", "has-session", "-t", session)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return true, nil
	}

	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return false, fmt.Errorf("check session %s: %w", session, err)
	}
	return false, fmt.Errorf("check session %s: %w: %s", session, err, trimmed)
}

func sendPrompt(session, prompt string) error {
	sendPromptCmd := exec.Command("tmux", "send-keys", "-t", session, "-l", prompt)
	if out, err := sendPromptCmd.CombinedOutput(); err != nil {
		trimmed := strings.TrimSpace(string(out))
		if trimmed == "" {
			return fmt.Errorf("send prompt to session %s: %w", session, err)
		}
		return fmt.Errorf("send prompt to session %s: %w: %s", session, err, trimmed)
	}

	sendEnterCmd := exec.Command("tmux", "send-keys", "-t", session, "Enter")
	if out, err := sendEnterCmd.CombinedOutput(); err != nil {
		trimmed := strings.TrimSpace(string(out))
		if trimmed == "" {
			return fmt.Errorf("submit prompt in session %s: %w", session, err)
		}
		return fmt.Errorf("submit prompt in session %s: %w: %s", session, err, trimmed)
	}

	return nil
}
