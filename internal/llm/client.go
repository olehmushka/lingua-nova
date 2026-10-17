// Package llm translates English into Rifatian with a local language
// model, and checks every answer with the Phase 4 glosser.
//
// The model runs in Ollama behind a small Python service (llm/ at the
// repository root). That service does one model call per request: it
// builds the prompt, asks the model for tool calls or for an answer
// constrained to a JSON schema, and returns them. This package owns
// everything that decides correctness: which dictionary entries go in the
// prompt (context.go), whether an answer is valid Rifatian (check.go,
// meaning.go), the tools the model can call (tools.go), and the loops
// that run them and send the problems back to the model (loop.go).
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

// Turn is a failed attempt (its tool steps, then its answer) and the
// feedback sent back about it.
type Turn struct {
	Steps    []Step `json:"steps,omitempty"`
	Answer   Answer `json:"answer"`
	Feedback string `json:"feedback"`
}

// Tool is a function the model may call, described for the model.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON schema of the arguments
}

// ToolCall is the model's request to run a tool.
type ToolCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// ToolResult is what a tool returned, as the model will read it.
type ToolResult struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// Step is one round of tool use: the calls in one model reply and their
// results, in the same order.
type Step struct {
	Calls   []ToolCall   `json:"calls"`
	Results []ToolResult `json:"results"`
}

// Request is one model call: the sentence, the context, the earlier
// failed attempts of the same sentence and, with tools, the tool steps of
// the current attempt so far.
type Request struct {
	English     string  `json:"english"`
	Model       string  `json:"model"`
	Context     Context `json:"context"`
	History     []Turn  `json:"history,omitempty"`
	Tools       []Tool  `json:"tools,omitempty"`
	Steps       []Step  `json:"steps,omitempty"`
	Final       bool    `json:"final,omitempty"` // ask for the answer, not for more calls
	NumCtx      int     `json:"num_ctx"`
	Temperature float64 `json:"temperature"`
}

// Response is the service's reply to a Request: the answer when there
// are no tools or Final is set, otherwise the tool calls to run (none
// means the model is done, and Content holds whatever it said).
type Response struct {
	Answer       *Answer    `json:"answer"`
	Calls        []ToolCall `json:"calls"`
	Content      string     `json:"content"`
	Model        string     `json:"model"`
	PromptTokens int        `json:"prompt_tokens"`
	EvalTokens   int        `json:"eval_tokens"`
	DurationMS   int        `json:"duration_ms"`
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

// Translate sends one model call.
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
	if ue, ok := err.(*url.Error); ok && ue.Timeout() {
		return nil, fmt.Errorf("llm service at %s timed out after %s; the model may be stuck (check: ollama ps): %w", c.URL, c.HTTP.Timeout, err)
	}
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
