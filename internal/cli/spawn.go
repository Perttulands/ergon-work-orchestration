package cli

import (
	"fmt"
	"os"

	"polis/work/internal/ecosystem"
	"polis/work/internal/worker"

	"github.com/spf13/cobra"
)

func newSpawnCmd() *cobra.Command {
	var (
		repo    string
		session string
		runtime string
	)

	cmd := &cobra.Command{
		Use:   "spawn <citizen>",
		Short: "Spawn a ready worker session in tmux",
		Long: `Spawns an agent session with explicit runtime policy:
1. Creates tmux session in target repo
2. Unsets nested-agent env vars
3. Exports RELAY_AGENT for identity
4. Launches runtime with explicit safety flags
5. Waits until worker is ready for input`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			citizen := args[0]
			return runSpawn(cmd, citizen, repo, session, runtime)
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "repository path (default: cwd)")
	cmd.Flags().StringVar(&session, "session", "", "tmux session name (default: agent-<citizen>)")
	cmd.Flags().StringVar(&runtime, "runtime", "", "worker runtime profile (default from runtime config)")

	return cmd
}

func runSpawn(cmd *cobra.Command, citizen, repo, session, runtime string) error {
	if repo == "" {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %w", err)
		}
		repo = wd
	}
	if session == "" {
		session = fmt.Sprintf("agent-%s", citizen)
	}
	resolvedRuntime, err := worker.ResolveRuntime(runtime, citizen)
	if err != nil {
		return fmt.Errorf("resolve runtime: %w", err)
	}
	if worker.SessionExists(session) {
		return fmt.Errorf("session already exists: %s", session)
	}

	cmd.Printf("Spawning citizen: %s\n", citizen)
	cmd.Printf("  Repo: %s | Runtime: %s | Session: %s\n", repo, resolvedRuntime, session)

	if _, err := worker.Start(worker.Config{
		SessionName: session,
		WorkDir:     repo,
		AgentName:   citizen,
		Runtime:     resolvedRuntime,
	}); err != nil {
		return fmt.Errorf("start worker session: %w", err)
	}

	// Best-effort visibility on relay bus.
	if policyErr := applyFailurePolicy(cmd, stepRelayRegister, ecosystem.RelayRegister(citizen)); policyErr != nil {
		return policyErr
	}
	if policyErr := applyFailurePolicy(cmd, stepRelayHeartbeat, ecosystem.RelayHeartbeat(citizen)); policyErr != nil {
		return policyErr
	}

	cmd.Printf("Ready: %s\n", session)
	cmd.Printf("Attach: tmux attach -t %s\n", session)
	return nil
}
