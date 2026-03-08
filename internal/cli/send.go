package cli

import (
	"fmt"
	"os"
	"strings"

	"polis/work/internal/worker"

	"github.com/spf13/cobra"
)

func newSendCmd() *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "send <session> [prompt...]",
		Short: "Send a prompt to a running tmux worker session",
		Long:  `Injects a prompt into a running tmux worker session via load-buffer + paste-buffer for reliable delivery.`,
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
	if !worker.SessionExists(session) {
		return fmt.Errorf("session not found: %s", session)
	}

	var prompt string
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("read prompt file: %w", err)
		}
		prompt = string(data)
	} else {
		if len(promptArgs) == 0 {
			return fmt.Errorf("prompt text required (provide args or --file)")
		}
		prompt = strings.Join(promptArgs, " ")
	}

	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("prompt is empty")
	}

	if err := worker.SendPrompt(session, prompt); err != nil {
		return fmt.Errorf("send prompt: %w", err)
	}

	cmd.Printf("Sent prompt to session %s\n", session)
	return nil
}
