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
				data, err := json.MarshalIndent(sessions, "", "  ")
				if err != nil {
					return fmt.Errorf("marshal json: %w", err)
				}
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

	// nosec: no command injection — args are static string literals, not user input
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name} #{session_created_string}").Output()
	if err != nil {
		return nil, fmt.Errorf("list tmux sessions: %w", err)
	}

	return parseTmuxSessions(string(out)), nil
}

// parseTmuxSessions parses tmux list-sessions output into work sessions.
// Only sessions with names starting with "work-" are returned.
func parseTmuxSessions(output string) []activeSession {
	var sessions []activeSession
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
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
	return sessions
}
