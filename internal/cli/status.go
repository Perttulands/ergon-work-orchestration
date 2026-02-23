package cli

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

type activeSession struct {
	Name    string `json:"name"`
	Created string `json:"created"`
}

func newStatusCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show currently active work runs",
		RunE: func(cmd *cobra.Command, args []string) error {
			sessions, err := getActiveSessions()
			if err != nil {
				cmd.Println("No active work sessions.")
				return nil
			}

			if jsonOut {
				data, _ := json.MarshalIndent(sessions, "", "  ")
				cmd.Println(string(data))
				return nil
			}

			if len(sessions) == 0 {
				cmd.Println("No active work sessions.")
				return nil
			}

			cmd.Printf("Active work sessions: %d\n\n", len(sessions))
			for _, s := range sessions {
				cmd.Printf("  %s  (created %s)\n", s.Name, s.Created)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON")
	return cmd
}

func getActiveSessions() ([]activeSession, error) {
	if _, err := exec.LookPath("tmux"); err != nil {
		return nil, fmt.Errorf("tmux not on PATH")
	}

	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name} #{session_created_string}").Output()
	if err != nil {
		return nil, err
	}

	var sessions []activeSession
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		name := parts[0]
		if !strings.HasPrefix(name, "work-") {
			continue
		}
		created := ""
		if len(parts) > 1 {
			created = parts[1]
		}
		sessions = append(sessions, activeSession{Name: name, Created: created})
	}
	return sessions, nil
}
