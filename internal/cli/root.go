package cli

import (
	"github.com/spf13/cobra"
)

// NewRoot creates the root cobra command for the work CLI.
func NewRoot(version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "work",
		Short: "Working memory for Polis — orchestrate agent work",
		Long:  "work is how Polis orchestrates. It holds what's happening now, what happened before, and what any citizen should know before starting a task.",
	}
	root.PersistentFlags().Bool("strict", true, "fail on optional integration errors by default (set --strict=false or WORK_STRICT=0 to relax)")

	root.AddCommand(newVersionCmd(version))
	root.AddCommand(newContextCmd())
	root.AddCommand(newRunCmd())
	root.AddCommand(newResumeCmd())
	root.AddCommand(newSpawnCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newHistoryCmd())
	root.AddCommand(newTraceCmd())
	root.AddCommand(newSpineParityCmd())
	root.AddCommand(newFeedCmd())
	root.AddCommand(newDeliberateCmd())
	root.AddCommand(newDecideCmd())
	root.AddCommand(newSendCmd())

	return root
}

func newVersionCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the work version",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Println("work " + version)
		},
	}
}
