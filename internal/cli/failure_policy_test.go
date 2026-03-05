package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestStrictModeFromEnv(t *testing.T) {
	t.Setenv("WORK_STRICT", "1")
	if !strictMode(nil) {
		t.Fatal("strictMode should be true when WORK_STRICT=1")
	}
}

func TestStrictModeFromFlag(t *testing.T) {
	t.Setenv("WORK_STRICT", "0")
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Bool("strict", false, "")
	if err := cmd.Flags().Set("strict", "true"); err != nil {
		t.Fatalf("set strict flag: %v", err)
	}
	if !strictMode(cmd) {
		t.Fatal("strictMode should be true when --strict is set")
	}
}
