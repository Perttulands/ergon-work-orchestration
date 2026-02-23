package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestHistoryCommandExists(t *testing.T) {
	root := NewRoot("test")
	found := false
	for _, cmd := range root.Commands() {
		if cmd.Name() == "history" {
			found = true
			break
		}
	}
	if !found {
		t.Error("history command should be registered")
	}
}

func TestHistoryCommandFlags(t *testing.T) {
	root := NewRoot("test")
	var histCmd *cobra.Command
	for _, cmd := range root.Commands() {
		if cmd.Name() == "history" {
			histCmd = cmd
			break
		}
	}
	if histCmd == nil {
		t.Fatal("history not found")
	}
	if histCmd.Flags().Lookup("limit") == nil {
		t.Error("expected --limit flag")
	}
	if histCmd.Flags().Lookup("json") == nil {
		t.Error("expected --json flag")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		sec  int64
		want string
	}{
		{30, "30s"},
		{90, "1m30s"},
		{3661, "1h1m"},
	}
	for _, tt := range tests {
		got := formatDuration(tt.sec)
		if got != tt.want {
			t.Errorf("formatDuration(%d) = %q, want %q", tt.sec, got, tt.want)
		}
	}
}
