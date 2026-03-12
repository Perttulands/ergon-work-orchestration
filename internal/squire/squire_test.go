package squire

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request structure
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/v1/messages") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("missing api key header")
		}

		resp := `{"content":[{"type":"text","text":"{\"complete\":true,\"reasoning\":\"All changes look good\",\"follow_up\":\"\"}"}]}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(resp))
	}))
	defer srv.Close()

	v, err := check("test-key", srv.URL, "implement auth", "diff --git a/auth.go", "Tests passed\n❯")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !v.Complete {
		t.Error("expected complete=true")
	}
	if v.Reasoning != "All changes look good" {
		t.Errorf("reasoning = %q", v.Reasoning)
	}
	if v.FollowUp != "" {
		t.Errorf("follow_up should be empty, got %q", v.FollowUp)
	}
}

func TestCheckIncomplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := `{"content":[{"type":"text","text":"{\"complete\":false,\"reasoning\":\"Tests were requested but not written\",\"follow_up\":\"Please add unit tests for the auth module\"}"}]}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(resp))
	}))
	defer srv.Close()

	v, err := check("test-key", srv.URL, "implement auth with tests", "", "Done\n❯")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Complete {
		t.Error("expected complete=false")
	}
	if v.FollowUp == "" {
		t.Error("expected follow_up to be set")
	}
}

func TestCheckAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal"}`))
	}))
	defer srv.Close()

	_, err := check("test-key", srv.URL, "task", "diff", "output")
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
}

func TestCheckMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := `{"content":[{"type":"text","text":"not valid json at all"}]}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(resp))
	}))
	defer srv.Close()

	_, err := check("test-key", srv.URL, "task", "diff", "output")
	if err == nil {
		t.Fatal("expected error on malformed JSON verdict")
	}
}

func TestCheckMissingFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Valid JSON but missing reasoning — should still parse
		resp := `{"content":[{"type":"text","text":"{\"complete\":true}"}]}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(resp))
	}))
	defer srv.Close()

	v, err := check("test-key", srv.URL, "task", "diff", "output")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !v.Complete {
		t.Error("expected complete=true")
	}
}

func TestCheckEmptyAPIKey(t *testing.T) {
	_, err := check("", "http://localhost", "task", "diff", "output")
	if err == nil {
		t.Fatal("expected error with empty API key")
	}
}

func TestCheckRequestBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model     string `json:"model"`
			MaxTokens int    `json:"max_tokens"`
			System    string `json:"system"`
			Messages  []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}

		if body.Model != "claude-haiku-4-5-20251001" {
			t.Errorf("model = %q, want claude-haiku-4-5-20251001", body.Model)
		}
		if body.MaxTokens != 512 {
			t.Errorf("max_tokens = %d, want 512", body.MaxTokens)
		}
		if body.System == "" {
			t.Error("system prompt should not be empty")
		}
		if len(body.Messages) != 1 {
			t.Errorf("expected 1 message, got %d", len(body.Messages))
		}
		if body.Messages[0].Role != "user" {
			t.Errorf("message role = %q, want user", body.Messages[0].Role)
		}
		if !strings.Contains(body.Messages[0].Content, "my task") {
			t.Error("message should contain the task")
		}
		if !strings.Contains(body.Messages[0].Content, "my diff") {
			t.Error("message should contain the diff")
		}
		if !strings.Contains(body.Messages[0].Content, "my output") {
			t.Error("message should contain the output")
		}

		resp := `{"content":[{"type":"text","text":"{\"complete\":true,\"reasoning\":\"ok\"}"}]}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(resp))
	}))
	defer srv.Close()

	_, err := check("test-key", srv.URL, "my task", "my diff", "my output")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckConnectionRefused(t *testing.T) {
	_, err := check("test-key", "http://127.0.0.1:1", "task", "diff", "output")
	if err == nil {
		t.Fatal("expected error on connection refused")
	}
}

func TestCheckPublicAPIDefaultsToAnthropic(t *testing.T) {
	// Test that Check() (public) uses ANTHROPIC_API_KEY and default base URL
	t.Setenv("ANTHROPIC_API_KEY", "")
	_, err := Check("task", "diff", "output")
	if err == nil {
		t.Fatal("expected error with empty ANTHROPIC_API_KEY")
	}
}

func TestCheckJSONWithMarkdownFencing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Some models wrap JSON in markdown code fences
		text := "```json\n{\"complete\":true,\"reasoning\":\"looks good\"}\n```"
		resp := fmt.Sprintf(`{"content":[{"type":"text","text":%s}]}`, jsonEscape(text))
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(resp))
	}))
	defer srv.Close()

	v, err := check("test-key", srv.URL, "task", "diff", "output")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !v.Complete {
		t.Error("expected complete=true even with markdown fencing")
	}
}

func TestTruncateDiff(t *testing.T) {
	short := "small diff"
	if got := truncateDiff(short); got != short {
		t.Errorf("short diff should not be truncated")
	}

	big := strings.Repeat("x", maxDiffBytes+100)
	got := truncateDiff(big)
	if len(got) > maxDiffBytes+100 { // allow for suffix
		t.Errorf("big diff should be truncated, got len=%d", len(got))
	}
	if !strings.Contains(got, "[truncated") {
		t.Error("truncated diff should contain truncation notice")
	}
}

// jsonEscape returns a JSON-encoded string value (with quotes).
func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
