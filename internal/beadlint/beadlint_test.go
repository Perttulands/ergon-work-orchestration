package beadlint

import (
	"testing"
)

func TestLintBead_AllValid(t *testing.T) {
	b := Bead{
		ID:          "pol-abc",
		Title:       "senate cmdAsk has no test double for CLI",
		Description: "The core deliberation path has zero E2E coverage because cmdAsk shells out directly.",
		Type:        "bug",
		Priority:    2,
	}
	issues := LintBead(b)
	if HasErrors(issues) {
		t.Errorf("expected no errors for valid bead, got: %s", FormatIssues(issues))
	}
}

func TestLintBead_TitleTooShort(t *testing.T) {
	b := Bead{
		ID:          "pol-abc",
		Title:       "fix the thing",
		Description: "This is a long enough description for the lint check.",
		Type:        "bug",
		Priority:    1,
	}
	issues := LintBead(b)
	if !HasErrors(issues) {
		t.Fatal("expected error for short title")
	}
	found := false
	for _, i := range issues {
		if i.Field == "title" && i.Severity == Error {
			found = true
		}
	}
	if !found {
		t.Error("expected title error")
	}
}

func TestLintBead_TitleGenericWord(t *testing.T) {
	b := Bead{
		ID:          "pol-abc",
		Title:       "test thing for the foo bar module",
		Description: "This is a sufficiently detailed description of the work.",
		Type:        "task",
		Priority:    3,
	}
	issues := LintBead(b)
	hasWarning := false
	for _, i := range issues {
		if i.Field == "title" && i.Severity == Warning {
			hasWarning = true
		}
	}
	if !hasWarning {
		t.Error("expected warning for generic word in title")
	}
}

func TestLintBead_DescriptionTooShort(t *testing.T) {
	b := Bead{
		ID:          "pol-abc",
		Title:       "enforce minimum bead quality before dispatch",
		Description: "too short",
		Type:        "feature",
		Priority:    2,
	}
	issues := LintBead(b)
	if !HasErrors(issues) {
		t.Fatal("expected error for short description")
	}
	found := false
	for _, i := range issues {
		if i.Field == "description" && i.Severity == Error {
			found = true
		}
	}
	if !found {
		t.Error("expected description error")
	}
}

func TestLintBead_EmptyDescription(t *testing.T) {
	b := Bead{
		ID:          "pol-abc",
		Title:       "enforce minimum bead quality before dispatch",
		Description: "",
		Type:        "task",
		Priority:    1,
	}
	issues := LintBead(b)
	if !HasErrors(issues) {
		t.Fatal("expected error for empty description")
	}
}

func TestLintBead_InvalidType(t *testing.T) {
	b := Bead{
		ID:          "pol-abc",
		Title:       "enforce minimum bead quality before dispatch",
		Description: "A detailed description of the feature.",
		Type:        "story",
		Priority:    2,
	}
	issues := LintBead(b)
	if !HasErrors(issues) {
		t.Fatal("expected error for invalid type")
	}
	found := false
	for _, i := range issues {
		if i.Field == "type" && i.Severity == Error {
			found = true
		}
	}
	if !found {
		t.Error("expected type error")
	}
}

func TestLintBead_EmptyType(t *testing.T) {
	b := Bead{
		ID:          "pol-abc",
		Title:       "enforce minimum bead quality before dispatch",
		Description: "A detailed description of the task at hand.",
		Type:        "",
		Priority:    1,
	}
	issues := LintBead(b)
	if !HasErrors(issues) {
		t.Fatal("expected error for empty type")
	}
}

func TestLintBead_AllValidTypes(t *testing.T) {
	for _, typ := range []string{"bug", "task", "feature", "chore", "epic"} {
		b := Bead{
			ID:          "pol-abc",
			Title:       "enforce minimum bead quality before dispatch",
			Description: "A detailed description of the task at hand.",
			Type:        typ,
			Priority:    1,
		}
		issues := LintBead(b)
		for _, i := range issues {
			if i.Field == "type" {
				t.Errorf("type %q should be valid, got issue: %s", typ, i.Message)
			}
		}
	}
}

func TestLintBead_NoPriority(t *testing.T) {
	b := Bead{
		ID:          "pol-abc",
		Title:       "enforce minimum bead quality before dispatch",
		Description: "A detailed description of the task at hand.",
		Type:        "task",
		Priority:    0,
	}
	issues := LintBead(b)
	if !HasErrors(issues) {
		t.Fatal("expected error for missing priority")
	}
	found := false
	for _, i := range issues {
		if i.Field == "priority" && i.Severity == Error {
			found = true
		}
	}
	if !found {
		t.Error("expected priority error")
	}
}

func TestLintBead_MultipleErrors(t *testing.T) {
	b := Bead{
		ID:          "pol-abc",
		Title:       "fix it",
		Description: "short",
		Type:        "invalid",
		Priority:    0,
	}
	issues := LintBead(b)
	errorCount := 0
	for _, i := range issues {
		if i.Severity == Error {
			errorCount++
		}
	}
	if errorCount < 4 {
		t.Errorf("expected >= 4 errors for all-bad bead, got %d: %s", errorCount, FormatIssues(issues))
	}
}

func TestLintTitle_Valid(t *testing.T) {
	issues := LintTitle("enforce minimum bead quality before dispatch")
	if HasErrors(issues) {
		t.Errorf("expected no errors: %s", FormatIssues(issues))
	}
}

func TestLintTitle_TooShort(t *testing.T) {
	issues := LintTitle("fix bug")
	if !HasErrors(issues) {
		t.Fatal("expected error for 2-word title")
	}
}

func TestLintTitle_ExactlyFiveWords(t *testing.T) {
	issues := LintTitle("add user auth via JWT")
	if HasErrors(issues) {
		t.Errorf("5-word title should pass: %s", FormatIssues(issues))
	}
}

func TestLintTitle_Empty(t *testing.T) {
	issues := LintTitle("")
	if !HasErrors(issues) {
		t.Fatal("expected error for empty title")
	}
}

func TestLintTitle_GenericOnly(t *testing.T) {
	issues := LintTitle("temp foo bar baz asdf thing")
	hasWarning := false
	for _, i := range issues {
		if i.Severity == Warning {
			hasWarning = true
		}
	}
	if !hasWarning {
		t.Error("expected warning for generic words")
	}
}

func TestIsBeadID(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// existing pol- prefixed IDs
		{"pol-2dl", true},
		{"pol-x4r5", true},
		{"pol-abc", true},
		{"pol-abcdef", true},
		{"pol-10j3.6", true},

		// widened: relay-, gate-, sen- prefixes (pol-zyhu)
		{"relay-abc123", true},
		{"gate-xy1z", true},
		{"sen-0001", true},
		{"projects-i01", true},
		{"relay-abc", true},

		// invalid formats
		{"pol-a", false},         // too short ID segment
		{"pol-abcdefghi", false}, // too long single segment
		{"relay-", false},        // missing ID
		{"fix the bug", false},   // not a bead ID
		{"", false},              // empty
		{"pol-ABC", false},       // uppercase
		{"Pol-abc", false},       // uppercase prefix
		{"a-xx", false},          // single-char prefix too short
		{"-abc", false},          // no prefix
		{"123-abc", false},       // numeric prefix start
	}
	for _, tt := range tests {
		got := IsBeadID(tt.input)
		if got != tt.want {
			t.Errorf("IsBeadID(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestHasErrors_NoIssues(t *testing.T) {
	if HasErrors(nil) {
		t.Error("nil issues should not have errors")
	}
	if HasErrors([]Issue{}) {
		t.Error("empty issues should not have errors")
	}
}

func TestHasErrors_WarningsOnly(t *testing.T) {
	issues := []Issue{{Field: "title", Message: "generic word", Severity: Warning}}
	if HasErrors(issues) {
		t.Error("warnings-only should not count as errors")
	}
}

func TestHasErrors_WithError(t *testing.T) {
	issues := []Issue{
		{Field: "title", Message: "generic word", Severity: Warning},
		{Field: "description", Message: "too short", Severity: Error},
	}
	if !HasErrors(issues) {
		t.Error("should detect error among mixed issues")
	}
}

func TestFormatIssues_Empty(t *testing.T) {
	if FormatIssues(nil) != "" {
		t.Error("nil issues should format to empty string")
	}
}

func TestFormatIssues_Mixed(t *testing.T) {
	issues := []Issue{
		{Field: "title", Message: "too short", Severity: Error},
		{Field: "title", Message: "generic word", Severity: Warning},
	}
	out := FormatIssues(issues)
	if out == "" {
		t.Fatal("expected non-empty output")
	}
	if !contains(out, "error") {
		t.Error("should contain 'error'")
	}
	if !contains(out, "warning") {
		t.Error("should contain 'warning'")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && stringContains(s, substr)
}

func stringContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
