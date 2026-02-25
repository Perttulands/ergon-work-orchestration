// Package context implements the work context engine.
// It gathers relevant history, citizen experience, and patterns
// to inject into worker prompts.
package context

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"polis/work/internal/ecosystem"
)

// BeadResult represents a bead from br search.
type BeadResult struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	Priority  int    `json:"priority"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Config holds parameters for a context query.
type Config struct {
	BeadID    string // bead being worked on
	Citizen   string // citizen name
	Repo      string // repo path
	Task      string // task description (for search)
	WorkDir   string // ~/.work directory, defaults to ~/.work
	BeadsRoot string // root of beads tracking (for bv), defaults to /home/polis/projects
}

// Result holds the assembled context.
type Result struct {
	PastBeads          []BeadResult
	CitizenExperience  string
	LearningPatterns   string
	LearningInsights   string // formatted output from loop query
	TemplateSelection  *ecosystem.TemplateSelection
	BvSearch           *ecosystem.BvSearchResponse
	BvRelated          *ecosystem.BvRelatedResponse
	BvPlan             *ecosystem.BvPlanResponse
	PRD                string // contents of PRD.md from repo
	Markdown           string // final assembled markdown
}

// Gather collects all available context and assembles it into injectable markdown.
func Gather(cfg Config) (*Result, error) {
	workDir := cfg.WorkDir
	if workDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		workDir = filepath.Join(home, ".work")
	}

	beadsRoot := cfg.BeadsRoot
	if beadsRoot == "" {
		beadsRoot = "/home/polis/projects"
	}

	r := &Result{}
	var sections []string

	// 1. bv --robot-search: find existing beads matching task
	if cfg.Task != "" {
		search, err := ecosystem.BvSearch(cfg.Task, beadsRoot)
		if err == nil && search != nil && len(search.Results) > 0 {
			r.BvSearch = search
			sections = append(sections, formatBvSearch(search))
		}
	}

	// 2. bv --robot-related: context on picked-up bead
	if cfg.BeadID != "" {
		related, err := ecosystem.BvRelated(cfg.BeadID, beadsRoot)
		if err == nil && related != nil && related.TotalRelated > 0 {
			r.BvRelated = related
			sections = append(sections, formatBvRelated(related))
		}
	}

	// 3. bv --robot-plan: what depends on this / execution plan
	plan, err := ecosystem.BvPlan(beadsRoot)
	if err == nil && plan != nil && plan.Plan.TotalActionable > 0 {
		r.BvPlan = plan
		sections = append(sections, formatBvPlan(plan))
	}

	// 4. Read PRD.md from repo for project context
	if cfg.Repo != "" {
		prd, err := readPRD(cfg.Repo)
		if err == nil && prd != "" {
			r.PRD = prd
			sections = append(sections, formatPRD(prd))
		}
	}

	// 5. Query past beads via br search
	beads, err := queryBeads(cfg)
	if err == nil && len(beads) > 0 {
		r.PastBeads = beads
		sections = append(sections, formatBeads(beads))
	}

	// 6. Read citizen experience file
	if cfg.Citizen != "" {
		exp, err := readCitizenExperience(workDir, cfg.Citizen)
		if err == nil && exp != "" {
			r.CitizenExperience = exp
			sections = append(sections, formatCitizenExperience(cfg.Citizen, exp))
		}
	}

	// 7. Learning-loop: query past run insights
	if cfg.Task != "" {
		if raw, err := ecosystem.QueryLearningLoop(cfg.Task); err == nil && len(raw) > 0 {
			formatted := formatLearningInsights(raw)
			if formatted != "" {
				r.LearningInsights = formatted
				sections = append(sections, formatted)
			}
		}
	}

	// 8. Learning-loop: select-template recommendation
	if cfg.Task != "" {
		sel, err := ecosystem.SelectTemplate(cfg.Task)
		if err == nil && sel != nil {
			r.TemplateSelection = sel
			r.LearningPatterns = sel.Reasoning
			sections = append(sections, formatTemplateSelection(sel))
		}
	}

	if len(sections) == 0 {
		r.Markdown = "No prior context available. This is a fresh start."
	} else {
		r.Markdown = strings.Join(sections, "\n\n---\n\n")
	}

	return r, nil
}

// queryBeads searches br for relevant past beads.
func queryBeads(cfg Config) ([]BeadResult, error) {
	if _, err := exec.LookPath("br"); err != nil {
		return nil, fmt.Errorf("br not on PATH")
	}

	// Build search query from task description keywords
	query := cfg.Task
	if query == "" && cfg.BeadID != "" {
		query = cfg.BeadID
	}
	if query == "" {
		return nil, fmt.Errorf("no search query available")
	}

	args := []string{"search", query, "--json", "--limit", "10", "--status", "closed"}
	cmd := exec.Command("br", args...)
	cmd.Dir = cfg.Repo
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("br search: %w", err)
	}

	var beads []BeadResult
	if err := json.Unmarshal(out, &beads); err != nil {
		return nil, fmt.Errorf("parse br output: %w", err)
	}

	return beads, nil
}

// readCitizenExperience reads the citizen's experience file.
func readCitizenExperience(workDir, citizen string) (string, error) {
	path := filepath.Join(workDir, "citizens", citizen+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read citizen %s: %w", citizen, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// formatTemplateSelection formats a template recommendation as context markdown.
func formatTemplateSelection(sel *ecosystem.TemplateSelection) string {
	var b strings.Builder
	b.WriteString("## Recommended Approach (learning-loop)\n\n")
	b.WriteString(fmt.Sprintf("- **Template:** %s", sel.Template))
	if sel.Variant != nil {
		b.WriteString(fmt.Sprintf(" (variant: %s)", *sel.Variant))
	}
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("- **Task type:** %s\n", sel.TaskType))
	if sel.Score > 0 {
		b.WriteString(fmt.Sprintf("- **Score:** %.2f (%s confidence)\n", sel.Score, sel.Confidence))
	}
	if sel.Agent != "" && sel.Agent != "unknown" {
		b.WriteString(fmt.Sprintf("- **Recommended agent:** %s\n", sel.Agent))
	}
	if sel.Reasoning != "" {
		b.WriteString(fmt.Sprintf("- %s\n", sel.Reasoning))
	}
	for _, w := range sel.Warnings {
		b.WriteString(fmt.Sprintf("- Warning: %s\n", w))
	}
	return b.String()
}

func formatBeads(beads []BeadResult) string {
	var b strings.Builder
	b.WriteString("## Past Work\n\n")
	for _, bead := range beads {
		status := bead.Status
		b.WriteString(fmt.Sprintf("- **%s** [%s] %s\n", bead.ID, status, bead.Title))
	}
	return b.String()
}

func formatCitizenExperience(citizen, exp string) string {
	return fmt.Sprintf("## %s's Experience Notes\n\n%s", citizen, exp)
}

func formatPatterns(patterns string) string {
	return fmt.Sprintf("## Learned Patterns\n\n%s", patterns)
}

// formatLearningInsights formats the JSON output from `loop query --json` as
// injectable markdown context. The JSON matches the learning-loop query.Result
// structure with fields: matched_runs, success_rate, insights, top_patterns,
// success_signals, relevant_runs.
func formatLearningInsights(raw []byte) string {
	var result struct {
		MatchedRuns    int     `json:"matched_runs"`
		SuccessRate    float64 `json:"success_rate"`
		Insights       []struct {
			Text       string  `json:"text"`
			Confidence float64 `json:"confidence"`
		} `json:"insights"`
		TopPatterns []struct {
			Name   string `json:"name"`
			Count  int    `json:"count"`
			Impact string `json:"impact"`
		} `json:"top_patterns"`
		SuccessSignals []string `json:"success_signals"`
	}

	if err := json.Unmarshal(raw, &result); err != nil {
		return ""
	}

	// Nothing useful to show
	if result.MatchedRuns == 0 && len(result.Insights) == 0 {
		return ""
	}

	var buf strings.Builder
	buf.WriteString("## Learning (from past runs)\n\n")
	buf.WriteString(fmt.Sprintf("From %d similar runs (%.0f%% success rate)\n\n", result.MatchedRuns, result.SuccessRate*100))

	for i, ins := range result.Insights {
		if i >= 5 {
			break
		}
		buf.WriteString(fmt.Sprintf("- %s\n", ins.Text))
	}

	if len(result.TopPatterns) > 0 {
		buf.WriteString("\n**Watch out:**\n")
		for i, p := range result.TopPatterns {
			if i >= 5 {
				break
			}
			buf.WriteString(fmt.Sprintf("- %s (%dx, %s impact)\n", p.Name, p.Count, p.Impact))
		}
	}

	if len(result.SuccessSignals) > 0 {
		buf.WriteString("\n**What works:**\n")
		for _, sig := range result.SuccessSignals {
			buf.WriteString(fmt.Sprintf("- %s\n", sig))
		}
	}

	return buf.String()
}

// readPRD reads PRD.md from the repo root. Returns empty string if not found.
func readPRD(repo string) (string, error) {
	path := filepath.Join(repo, "PRD.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func formatPRD(prd string) string {
	// Truncate long PRDs to keep context manageable
	const maxLen = 2000
	text := prd
	if len(text) > maxLen {
		text = text[:maxLen] + "\n\n[...truncated]"
	}
	return fmt.Sprintf("## Project Context (PRD.md)\n\n%s", text)
}

func formatBvSearch(resp *ecosystem.BvSearchResponse) string {
	var b strings.Builder
	b.WriteString("## Similar Beads (bv search)\n\n")
	for _, r := range resp.Results {
		b.WriteString(fmt.Sprintf("- **%s** (%.0f%%) %s\n", r.IssueID, r.Score*100, r.Title))
	}
	return b.String()
}

func formatBvRelated(resp *ecosystem.BvRelatedResponse) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Related Work (bv related: %s)\n\n", resp.TargetBeadID))
	for _, item := range resp.Concurrent {
		b.WriteString(fmt.Sprintf("- **%s** [%s] %s — %s\n", item.BeadID, item.Status, item.Title, item.Reason))
	}
	return b.String()
}

func formatBvPlan(resp *ecosystem.BvPlanResponse) string {
	var b strings.Builder
	b.WriteString("## Execution Plan (bv plan)\n\n")
	b.WriteString(fmt.Sprintf("Actionable: %d | Blocked: %d\n\n", resp.Plan.TotalActionable, resp.Plan.TotalBlocked))
	for _, track := range resp.Plan.Tracks {
		for _, item := range track.Items {
			marker := " "
			if item.Status == "closed" {
				marker = "x"
			}
			b.WriteString(fmt.Sprintf("- [%s] **%s** %s (P%d)\n", marker, item.ID, item.Title, item.Priority))
		}
	}
	if resp.Plan.Summary.HighestImpact != "" {
		b.WriteString(fmt.Sprintf("\nHighest impact: **%s** — %s\n", resp.Plan.Summary.HighestImpact, resp.Plan.Summary.ImpactReason))
	}
	return b.String()
}
