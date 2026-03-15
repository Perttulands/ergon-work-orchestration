// Package contracts validates cross-tool CLI boundaries.
// Each test calls a real downstream tool and asserts exit codes,
// output format, and required fields.
package contracts

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// ContractTestResult captures the full diagnostic context of a contract test.
type ContractTestResult struct {
	Tool             string   `json:"tool"`
	Args             []string `json:"args"`
	ExitCode         int      `json:"exit_code"`
	Stdout           string   `json:"stdout"`
	Stderr           string   `json:"stderr"`
	ExpectedExitCode int      `json:"expected_exit_code"`
	ExpectedFields   []string `json:"expected_fields,omitempty"`
	Passed           bool     `json:"passed"`
	DiagMessage      string   `json:"diag_message"`
}

// ReportJSON writes a JSON representation of the result to w.
func (r ContractTestResult) ReportJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// ReportHuman writes a human-readable failure report to w.
// Only writes output for failures; returns immediately for passing tests.
func (r ContractTestResult) ReportHuman(w io.Writer) {
	if r.Passed {
		return
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("CONTRACT FAILURE: %s\n", r.Tool))
	b.WriteString(fmt.Sprintf("  Command:  %s %s\n", r.Tool, strings.Join(r.Args, " ")))
	b.WriteString(fmt.Sprintf("  Exit:     %d (expected %d)\n", r.ExitCode, r.ExpectedExitCode))
	if len(r.ExpectedFields) > 0 {
		b.WriteString(fmt.Sprintf("  Expected: %s\n", strings.Join(r.ExpectedFields, ", ")))
	}
	b.WriteString(fmt.Sprintf("  Diag:     %s\n", r.DiagMessage))
	if r.Stdout != "" {
		b.WriteString(fmt.Sprintf("  Stdout:   %s\n", truncate(r.Stdout, 500)))
	}
	if r.Stderr != "" {
		b.WriteString(fmt.Sprintf("  Stderr:   %s\n", truncate(r.Stderr, 500)))
	}
	fmt.Fprint(w, b.String())
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
