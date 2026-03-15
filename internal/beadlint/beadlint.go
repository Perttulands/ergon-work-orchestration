// Package beadlint validates bead quality before dispatch.
// Beads with vague titles, missing descriptions, or invalid types
// produce vague work. This package enforces minimum quality.
package beadlint

import (
	"regexp"
	"strings"
)

// Severity indicates how serious a lint issue is.
type Severity int

const (
	Warning Severity = iota
	Error
)

// Issue is a single lint finding.
type Issue struct {
	Field    string   `json:"field"`
	Message  string   `json:"message"`
	Severity Severity `json:"severity"`
}

// Bead is the subset of bead fields needed for linting.
type Bead struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Type        string `json:"issue_type"`
	Priority    int    `json:"priority"`
}

var genericWords = map[string]bool{
	"test": true, "temp": true, "foo": true, "bar": true,
	"baz": true, "asdf": true, "xxx": true, "todo": true,
	"stuff": true, "thing": true, "wip": true,
}

var validTypes = map[string]bool{
	"bug": true, "task": true, "feature": true, "chore": true, "epic": true,
}

// beadIDPattern matches configurable br IDs like pol-2dl, relay-ab12, or pol-10j3.6.
var beadIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]+-[a-z0-9]{2,8}(?:\.[a-z0-9]{1,8})*$`)

// IsBeadID returns true if s looks like a bead ID.
func IsBeadID(s string) bool {
	return beadIDPattern.MatchString(strings.TrimSpace(s))
}

// LintBead validates a full bead struct.
func LintBead(b Bead) []Issue {
	var issues []Issue

	issues = append(issues, lintTitle(b.Title)...)

	if len(strings.TrimSpace(b.Description)) < 20 {
		issues = append(issues, Issue{
			Field:    "description",
			Message:  "description must be >= 20 characters",
			Severity: Error,
		})
	}

	if !validTypes[strings.TrimSpace(b.Type)] {
		issues = append(issues, Issue{
			Field:    "type",
			Message:  "type must be one of: bug, task, feature, chore, epic",
			Severity: Error,
		})
	}

	if b.Priority == 0 {
		issues = append(issues, Issue{
			Field:    "priority",
			Message:  "priority must be set (1-5)",
			Severity: Error,
		})
	}

	return issues
}

// LintTitle validates just a task title string (for work run free-text tasks).
func LintTitle(title string) []Issue {
	return lintTitle(title)
}

func lintTitle(title string) []Issue {
	var issues []Issue
	title = strings.TrimSpace(title)

	words := strings.Fields(title)
	if len(words) < 5 {
		issues = append(issues, Issue{
			Field:    "title",
			Message:  "title must be >= 5 words",
			Severity: Error,
		})
	}

	for _, w := range words {
		lower := strings.ToLower(w)
		if genericWords[lower] {
			issues = append(issues, Issue{
				Field:    "title",
				Message:  "title contains generic word: " + lower,
				Severity: Warning,
			})
			break // one warning is enough
		}
	}

	return issues
}

// HasErrors returns true if any issue has Error severity.
func HasErrors(issues []Issue) bool {
	for _, issue := range issues {
		if issue.Severity == Error {
			return true
		}
	}
	return false
}

// FormatIssues returns a human-readable summary of lint issues.
func FormatIssues(issues []Issue) string {
	if len(issues) == 0 {
		return ""
	}
	var b strings.Builder
	for _, issue := range issues {
		prefix := "warning"
		if issue.Severity == Error {
			prefix = "error"
		}
		b.WriteString("  " + prefix + ": [" + issue.Field + "] " + issue.Message + "\n")
	}
	return b.String()
}
