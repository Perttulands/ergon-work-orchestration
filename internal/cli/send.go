package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"polis/work/internal/worker"
)

func newSendCmd() *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "send <session> <prompt>",
		Short: "Send a prompt to a running tmux worker session",
		Long:  `Injects a prompt into a running tmux worker session via load-buffer/paste-buffer.`,
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

	if !worker.SessionExists(session) {
		return fmt.Errorf("session not found: %s", session)
	}

	prompt, err := loadSendPrompt(promptArgs, filePath)
	if err != nil {
		return err
	}

	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("prompt is empty")
	}

	if err := worker.SendPrompt(session, prompt); err != nil {
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

