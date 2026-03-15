// Package squire provides completion verification for worker runs.
// It asks a fast model (Haiku) whether the agent completed the assigned task.
package squire

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.anthropic.com"
	model          = "claude-haiku-4-5-20251001"
	maxTokens      = 512
	httpTimeout    = 15 * time.Second
	maxDiffBytes   = 8192
)

const systemPrompt = `You are a completion verifier for a coding agent. You will be given:

1. TASK: What the agent was asked to do
2. DIFF: The git diff of changes the agent made
3. OUTPUT: The agent's final terminal output

Your job: determine whether the agent completed the task.

Rules:
- A task is COMPLETE if the core ask is satisfied, even if minor polish remains
- A task is INCOMPLETE if the agent clearly stopped short: missing files, half-written code, no tests when tests were requested, only part of a multi-step task done
- When in doubt, say COMPLETE — false positives (unnecessary retries) cost more than false negatives
- If INCOMPLETE, write a specific, actionable follow-up prompt (1-2 sentences) that tells the agent exactly what to finish

Respond with JSON only:
{"complete": true/false, "reasoning": "...", "follow_up": "..."}`

// Verdict from the completion check.
type Verdict struct {
	Complete  bool   `json:"complete"`
	Reasoning string `json:"reasoning"`
	FollowUp  string `json:"follow_up,omitempty"`
}

// Check asks Haiku whether the agent completed the task.
// Reads ANTHROPIC_API_KEY from environment. Errors are non-fatal — caller should proceed on error.
func Check(task, diff, paneOutput string) (*Verdict, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	return check(apiKey, defaultBaseURL, task, diff, paneOutput)
}

// check is the internal implementation, accepting explicit key and base URL for testing.
func check(apiKey, baseURL, task, diff, paneOutput string) (*Verdict, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("squire: ANTHROPIC_API_KEY not set")
	}

	diff = truncateDiff(diff)

	userMsg := fmt.Sprintf("TASK:\n%s\n\nDIFF:\n%s\n\nOUTPUT:\n%s", task, diff, paneOutput)

	reqBody := apiRequest{
		Model:     model,
		MaxTokens: maxTokens,
		System:    systemPrompt,
		Messages: []message{
			{Role: "user", Content: userMsg},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("squire: marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", baseURL+"/v1/messages", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("squire: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("squire: API call: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("squire: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("squire: API returned %d: %s", resp.StatusCode, string(respBytes))
	}

	var apiResp apiResponse
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("squire: parse response: %w", err)
	}

	if len(apiResp.Content) == 0 {
		return nil, fmt.Errorf("squire: empty response content")
	}

	text := apiResp.Content[0].Text
	text = stripMarkdownFences(text)

	var verdict Verdict
	if err := json.Unmarshal([]byte(text), &verdict); err != nil {
		return nil, fmt.Errorf("squire: parse verdict JSON: %w (raw: %s)", err, text)
	}

	return &verdict, nil
}

func truncateDiff(diff string) string {
	if len(diff) <= maxDiffBytes {
		return diff
	}
	return diff[:maxDiffBytes] + "\n[truncated at 8KB]"
}

func stripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		// Remove opening fence line
		if idx := strings.Index(s, "\n"); idx != -1 {
			s = s[idx+1:]
		}
		// Remove closing fence
		if idx := strings.LastIndex(s, "```"); idx != -1 {
			s = s[:idx]
		}
		s = strings.TrimSpace(s)
	}
	return s
}

// --- Anthropic API types (minimal, no SDK) ---

type apiRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system"`
	Messages  []message `json:"messages"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type apiResponse struct {
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
