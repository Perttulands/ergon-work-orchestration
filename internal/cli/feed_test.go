package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestFeedCommandExists(t *testing.T) {
	root := NewRoot("test")
	found := false
	for _, cmd := range root.Commands() {
		if cmd.Name() == "feed" {
			found = true
			break
		}
	}
	if !found {
		t.Error("feed command should be registered")
	}
}

func TestFeedCommandFlags(t *testing.T) {
	root := NewRoot("test")
	var feedCmd *cobra.Command
	for _, cmd := range root.Commands() {
		if cmd.Name() == "feed" {
			feedCmd = cmd
			break
		}
	}
	if feedCmd == nil {
		t.Fatal("feed command not found")
	}
	if feedCmd.Flags().Lookup("since") == nil {
		t.Error("expected --since flag")
	}
}

func TestParseSince(t *testing.T) {
	tests := []struct {
		input string
		ok    bool
	}{
		{"1h", true},
		{"24h", true},
		{"7d", true},
		{"30m", true},
		{"x", false},
		{"", false},
		{"24x", false},
	}

	for _, tt := range tests {
		result, err := parseSince(tt.input)
		if tt.ok && err != nil {
			t.Errorf("parseSince(%q) unexpected error: %v", tt.input, err)
		}
		if !tt.ok && err == nil {
			t.Errorf("parseSince(%q) expected error", tt.input)
		}
		if tt.ok {
			// Result should be in the past
			if result.After(time.Now()) {
				t.Errorf("parseSince(%q) returned future time", tt.input)
			}
		}
	}
}

func TestFeedEntryJSON(t *testing.T) {
	entry := FeedEntry{
		BeadID:   "work-abc",
		Citizen:  "zeus",
		Task:     "add auth",
		Outcome:  "success",
		Duration: 300,
		Time:     "2026-02-23T10:00:00Z",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	s := string(data)
	checks := []string{"bead_id", "citizen", "task", "outcome", "duration_s", "time"}
	for _, c := range checks {
		if !strings.Contains(s, c) {
			t.Errorf("JSON should contain %q", c)
		}
	}

	// gate_pass and gate_score should be omitted when nil
	if strings.Contains(s, "gate_pass") {
		t.Error("gate_pass should be omitted when nil")
	}
}

func TestFeedOutputIsJSONL(t *testing.T) {
	// Feed command runs against real index, so just test the output format
	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"feed", "--since", "1h"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("feed failed: %v", err)
	}
	// Output should be valid JSONL (each line is valid JSON) or empty
	out := strings.TrimSpace(buf.String())
	if out == "" {
		return // empty is valid
	}
	for _, line := range strings.Split(out, "\n") {
		var entry FeedEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("line is not valid JSON: %s", line)
		}
	}
}
