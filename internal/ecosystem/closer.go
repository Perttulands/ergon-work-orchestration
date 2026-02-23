package ecosystem

import (
	"fmt"
	"os/exec"
	"strings"

	"polis/work/internal/trace"
)

// CloseReason holds the auto-derived close reason components.
type CloseReason struct {
	FilesWritten []string
	ToolCalls    int
	GatePass     *bool
	GateScore    *float64
	Outcome      string
	DurationS    int64
	DiffStat     string
	Error        string
}

// DeriveCloseReason reads a trace file and git diff to build a structured close reason.
func DeriveCloseReason(tracePath, repo string) (*CloseReason, error) {
	cr := &CloseReason{}

	events, err := trace.ReadTrace(tracePath)
	if err != nil {
		return nil, fmt.Errorf("read trace: %w", err)
	}

	for _, e := range events {
		switch e.EventType {
		case "file_write":
			if e.Path != "" {
				cr.FilesWritten = append(cr.FilesWritten, e.Path)
			}
		case "tool_call":
			cr.ToolCalls++
		case "gate_result":
			cr.GatePass = e.Pass
			cr.GateScore = e.Score
		case "end":
			cr.Outcome = e.Outcome
			if e.DurationS != nil {
				cr.DurationS = *e.DurationS
			}
			if e.Error != "" {
				cr.Error = e.Error
			}
		}
	}

	cr.DiffStat = gitDiffStat(repo)
	return cr, nil
}

// FormatCloseReason produces a human-readable close reason string.
func FormatCloseReason(cr *CloseReason) string {
	var parts []string

	parts = append(parts, cr.Outcome)

	if cr.DurationS > 0 {
		m := cr.DurationS / 60
		s := cr.DurationS % 60
		if m > 0 {
			parts = append(parts, fmt.Sprintf("%dm%ds", m, s))
		} else {
			parts = append(parts, fmt.Sprintf("%ds", s))
		}
	}

	if n := len(cr.FilesWritten); n > 0 {
		if n <= 3 {
			parts = append(parts, fmt.Sprintf("wrote %s", strings.Join(cr.FilesWritten, ", ")))
		} else {
			parts = append(parts, fmt.Sprintf("wrote %d files (%s, ...)", n, strings.Join(cr.FilesWritten[:3], ", ")))
		}
	}

	if cr.GatePass != nil {
		if *cr.GatePass {
			parts = append(parts, fmt.Sprintf("gate:pass(%.2f)", *cr.GateScore))
		} else {
			parts = append(parts, fmt.Sprintf("gate:fail(%.2f)", *cr.GateScore))
		}
	}

	if cr.DiffStat != "" {
		parts = append(parts, cr.DiffStat)
	}

	if cr.Error != "" {
		parts = append(parts, "error: "+cr.Error)
	}

	return strings.Join(parts, ". ")
}

func gitDiffStat(repo string) string {
	if repo == "" {
		return ""
	}
	// nosec: no command injection — args are static string literals, not user input
	cmd := exec.Command("git", "diff", "--stat")
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		return ""
	}
	summary := strings.TrimSpace(lines[len(lines)-1])
	if strings.Contains(summary, "changed") {
		return summary
	}
	return ""
}
