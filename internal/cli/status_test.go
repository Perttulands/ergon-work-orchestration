package cli

import (
	"bytes"
	"testing"
)

func TestStatusCommandExists(t *testing.T) {
	root := NewRoot("test")
	found := false
	for _, cmd := range root.Commands() {
		if cmd.Name() == "status" {
			found = true
			break
		}
	}
	if !found {
		t.Error("status command should be registered")
	}
}

func TestStatusCommandRuns(t *testing.T) {
	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"status"})

	if err := root.Execute(); err != nil {
		t.Fatalf("status failed: %v", err)
	}
	// Should produce some output (either sessions or "no active")
	if buf.Len() == 0 {
		t.Error("expected output from status command")
	}
}

func TestStatusJSON(t *testing.T) {
	root := NewRoot("test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"status", "--json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("status --json failed: %v", err)
	}
}

func TestParseTmuxSessionsNormal(t *testing.T) {
	output := `work-pol-abc Thu Feb 27 10:30:00 2026
other-session Thu Feb 27 09:00:00 2026
work-pol-xyz Fri Feb 28 14:15:00 2026
`
	sessions := parseTmuxSessions(output)

	if len(sessions) != 2 {
		t.Fatalf("expected 2 work sessions, got %d", len(sessions))
	}
	if sessions[0].Name != "work-pol-abc" {
		t.Errorf("first session name = %q, want work-pol-abc", sessions[0].Name)
	}
	if sessions[0].Created != "Thu Feb 27 10:30:00 2026" {
		t.Errorf("first session created = %q, want timestamp", sessions[0].Created)
	}
	if sessions[1].Name != "work-pol-xyz" {
		t.Errorf("second session name = %q, want work-pol-xyz", sessions[1].Name)
	}
}

func TestParseTmuxSessionsEmpty(t *testing.T) {
	sessions := parseTmuxSessions("")
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions from empty output, got %d", len(sessions))
	}
}

func TestParseTmuxSessionsMalformedLine(t *testing.T) {
	// A line with no spaces (name only, no created timestamp)
	output := "work-orphan\n"
	sessions := parseTmuxSessions(output)

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Name != "work-orphan" {
		t.Errorf("name = %q, want work-orphan", sessions[0].Name)
	}
	if sessions[0].Created != "" {
		t.Errorf("created should be empty for malformed line, got %q", sessions[0].Created)
	}
}

func TestParseTmuxSessionsSpacesInName(t *testing.T) {
	// tmux session names can't actually contain spaces, but the created
	// timestamp does — verify the SplitN(,2) correctly keeps the full timestamp.
	output := "work-my-task Mon Feb 27 10:30:00 2026\n"
	sessions := parseTmuxSessions(output)

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Name != "work-my-task" {
		t.Errorf("name = %q, want work-my-task", sessions[0].Name)
	}
	if sessions[0].Created != "Mon Feb 27 10:30:00 2026" {
		t.Errorf("created = %q, want full timestamp", sessions[0].Created)
	}
}

func TestParseTmuxSessionsFiltersNonWork(t *testing.T) {
	output := `main Thu Feb 27 09:00:00 2026
dev Thu Feb 27 09:30:00 2026
work-task1 Thu Feb 27 10:00:00 2026
random Thu Feb 27 10:30:00 2026
`
	sessions := parseTmuxSessions(output)

	if len(sessions) != 1 {
		t.Fatalf("expected 1 work session, got %d", len(sessions))
	}
	if sessions[0].Name != "work-task1" {
		t.Errorf("name = %q, want work-task1", sessions[0].Name)
	}
}

func TestParseTmuxSessionsBlankLines(t *testing.T) {
	output := "\n\nwork-a Thu Feb 27 10:00:00 2026\n\n\nwork-b Thu Feb 27 11:00:00 2026\n\n"
	sessions := parseTmuxSessions(output)

	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions (blank lines ignored), got %d", len(sessions))
	}
}
