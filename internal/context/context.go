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
)

// BeadResult represents a bead from bd search.
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
	BeadID  string // bead being worked on
	Citizen string // citizen name
	Repo    string // repo path
	Task    string // task description (for search)
	WorkDir string // ~/.work directory, defaults to ~/.work
}

// Result holds the assembled context.
type Result struct {
	PastBeads         []BeadResult
	CitizenExperience string
	LearningPatterns  string
	Markdown          string // final assembled markdown
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

	r := &Result{}
	var sections []string

	// 1. Query past beads via bd search
	beads, err := queryBeads(cfg)
	if err == nil && len(beads) > 0 {
		r.PastBeads = beads
		sections = append(sections, formatBeads(beads))
	}

	// 2. Read citizen experience file
	if cfg.Citizen != "" {
		exp, err := readCitizenExperience(workDir, cfg.Citizen)
		if err == nil && exp != "" {
			r.CitizenExperience = exp
			sections = append(sections, formatCitizenExperience(cfg.Citizen, exp))
		}
	}

	// 3. Query learning-loop if available
	patterns, err := queryLearningLoop(cfg)
	if err == nil && patterns != "" {
		r.LearningPatterns = patterns
		sections = append(sections, formatPatterns(patterns))
	}

	if len(sections) == 0 {
		r.Markdown = "No prior context available. This is a fresh start."
	} else {
		r.Markdown = strings.Join(sections, "\n\n---\n\n")
	}

	return r, nil
}

// queryBeads searches bd for relevant past beads.
func queryBeads(cfg Config) ([]BeadResult, error) {
	if _, err := exec.LookPath("bd"); err != nil {
		return nil, fmt.Errorf("bd not on PATH")
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
	cmd := exec.Command("bd", args...)
	cmd.Dir = cfg.Repo
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bd search: %w", err)
	}

	var beads []BeadResult
	if err := json.Unmarshal(out, &beads); err != nil {
		return nil, fmt.Errorf("parse bd output: %w", err)
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

// queryLearningLoop calls learning-loop for patterns if available.
func queryLearningLoop(cfg Config) (string, error) {
	if _, err := exec.LookPath("loop"); err != nil {
		return "", fmt.Errorf("loop not on PATH")
	}

	args := []string{"context"}
	if cfg.Repo != "" {
		args = append(args, "--repo", cfg.Repo)
	}
	if cfg.Citizen != "" {
		args = append(args, "--citizen", cfg.Citizen)
	}

	cmd := exec.Command("loop", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("loop context: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
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
