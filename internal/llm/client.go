// Package llm translates English into Rifatian with a local language
// model, and checks every answer with the Phase 4 glosser.
//
// The model runs in Ollama behind a small Python service (llm/ at the
// repository root). That service does one attempt per request: it builds
// the prompt, asks the model for an answer constrained to a JSON schema,
// and returns it. This package owns everything that decides correctness:
// which dictionary entries go in the prompt (context.go), whether an
// answer is valid Rifatian (check.go), and the retry loop that sends the
// problems back to the model (loop.go).
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultURL is where `uv run uvicorn app:app --port 8765` listens.
const DefaultURL = "http://127.0.0.1:8765"

// DefaultModel is the Ollama model used when none is given.
const DefaultModel = "qwen2.5:7b-instruct-q4_K_M"

// Answer is the model's structured reply (the JSON schema in llm/app.py).
type Answer struct {
	Translation string       `json:"translation"`
	Words       []AnswerWord `json:"words"`
	Notes       string       `json:"notes"`
}

// AnswerWord is the model's claim about one word of its translation.
type AnswerWord struct {
	Text      string   `json:"text"`
	LexiconID string   `json:"lexicon_id"`
	Features  []string `json:"features"`
}

// Turn is a failed attempt and the feedback sent back about it.
type Turn struct {
	Answer   Answer `json:"answer"`
	Feedback string `json:"feedback"`
}

// Request is one attempt: the sentence, the context, and the earlier
// failed attempts of the same sentence.
type Request struct {
	English     string  `json:"english"`
	Model       string  `json:"model"`
	Context     Context `json:"context"`
	History     []Turn  `json:"history,omitempty"`
	NumCtx      int     `json:"num_ctx"`
	Temperature float64 `json:"temperature"`
}

// Response is the service's reply to a Request.
type Response struct {
	Answer       Answer `json:"answer"`
	Model        string `json:"model"`
	PromptTokens int    `json:"prompt_tokens"`
	EvalTokens   int    `json:"eval_tokens"`
	DurationMS   int    `json:"duration_ms"`
}

// Client talks to the Python service.
type Client struct {
	URL  string
	HTTP *http.Client
}

// NewClient returns a client for the service at url ("" = DefaultURL).
// Local models are slow on a laptop GPU, and a long prompt may spill to
// the CPU, so the timeout is generous.
func NewClient(url string) *Client {
	if url == "" {
		url = DefaultURL
	}
	return &Client{URL: strings.TrimRight(url, "/"), HTTP: &http.Client{Timeout: 10 * time.Minute}}
}

// startHint is printed when the service can't be reached.
const startHint = "start it with: ollama serve, then cd llm && uv run uvicorn app:app --port 8765"

// Translate sends one attempt.
func (c *Client) Translate(ctx context.Context, req Request) (*Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL+"/translate", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	hr.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(hr)
	if err != nil {
		return nil, fmt.Errorf("llm service not reachable at %s (%s): %w", c.URL, startHint, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm service: %s: %s", resp.Status, detail(data))
	}
	var out Response
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("llm service: bad reply: %w", err)
	}
	return &out, nil
}

// detail extracts FastAPI's {"detail": "..."} error message.
func detail(data []byte) string {
	var e struct {
		Detail any `json:"detail"`
	}
	if json.Unmarshal(data, &e) == nil && e.Detail != nil {
		if s, ok := e.Detail.(string); ok {
			return s
		}
		b, _ := json.Marshal(e.Detail)
		return string(b)
	}
	return strings.TrimSpace(string(data))
}
