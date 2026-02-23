package cli

import (
	"fmt"
	"os"

	workctx "polis/work/internal/context"

	"github.com/spf13/cobra"
)

func newContextCmd() *cobra.Command {
	var (
		citizen string
		repo    string
		task    string
	)

	cmd := &cobra.Command{
		Use:   "context [bead-id]",
		Short: "Gather context for a bead — what should I know before starting?",
		Long:  "Queries past beads, citizen experience, and learning-loop patterns to produce injectable context for a worker prompt.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := workctx.Config{
				Citizen: citizen,
				Repo:    repo,
				Task:    task,
			}
			if len(args) > 0 {
				cfg.BeadID = args[0]
			}
			if cfg.Repo == "" {
				wd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("get working directory: %w", err)
				}
				cfg.Repo = wd
			}

			result, err := workctx.Gather(cfg)
			if err != nil {
				return fmt.Errorf("gather context: %w", err)
			}

			cmd.Println(result.Markdown)
			return nil
		},
	}

	cmd.Flags().StringVar(&citizen, "citizen", "", "citizen name")
	cmd.Flags().StringVar(&repo, "repo", "", "repository path (default: cwd)")
	cmd.Flags().StringVar(&task, "task", "", "task description for search")

	return cmd
}
