package llm

import (
	"context"
	"time"
)

// Options configure a translation.
type Options struct {
	Model       string // Ollama model; "" = DefaultModel
	MaxAttempts int    // 0 = 3
	NumCtx      int    // model context window in tokens; 0 = sized to the prompt
}

// Attempt is one round trip to the model and its check.
type Attempt struct {
	Answer       Answer
	Check        *Check
	PromptTokens int
	EvalTokens   int
	Duration     time.Duration
}

// Result is every attempt at one sentence; the last one passed if OK.
type Result struct {
	Context  Context
	Attempts []Attempt
	OK       bool
}

// Last is the final attempt, or nil if there was none.
func (r *Result) Last() *Attempt {
	if len(r.Attempts) == 0 {
		return nil
	}
	return &r.Attempts[len(r.Attempts)-1]
}

// Translator runs the generate → check → retry loop.
type Translator struct {
	Client  *Client
	Checker *Checker
}

// Translate asks the model for a translation, checks it, and on failure
// asks again with the problems, up to MaxAttempts times. The whole
// conversation is resent each time: the service is stateless, and the
// model sees its own earlier answers and what was wrong with them.
//
// This is the generate-and-test pattern of the analyzer and the
// rule-based translator, with a model as the generator: it proposes,
// the rule-based tools decide.
//
// An error (service down, malformed reply) ends the loop; the attempts so
// far are still in the result.
func (t *Translator) Translate(ctx context.Context, english string, c Context, opt Options) (*Result, error) {
	if opt.Model == "" {
		opt.Model = DefaultModel
	}
	if opt.MaxAttempts == 0 {
		opt.MaxAttempts = 3
	}
	if opt.NumCtx == 0 {
		opt.NumCtx = numCtx(c)
	}
	res := &Result{Context: c}
	var history []Turn
	for range opt.MaxAttempts {
		resp, err := t.Client.Translate(ctx, Request{
			English: english, Model: opt.Model, Context: c, History: history, NumCtx: opt.NumCtx,
		})
		if err != nil {
			return res, err
		}
		chk := t.Checker.Verify(resp.Answer)
		res.Attempts = append(res.Attempts, Attempt{
			Answer: resp.Answer, Check: chk,
			PromptTokens: resp.PromptTokens, EvalTokens: resp.EvalTokens,
			Duration: time.Duration(resp.DurationMS) * time.Millisecond,
		})
		if chk.OK() {
			res.OK = true
			break
		}
		history = append(history, Turn{Answer: resp.Answer, Feedback: chk.Feedback()})
	}
	return res, nil
}

// numCtx sizes the model's context window for a prompt: about 3 bytes per
// token for this mix of English, Rifatian and markup, plus room for the
// instructions, three rounds of answers and feedback, and the reply, rounded
// up to 1024. A window that's too small makes Ollama silently cut the
// start of the prompt (the grammar); one that's too big wastes GPU memory
// on the KV cache.
func numCtx(c Context) int {
	n := c.Size()/3 + 4096
	return (n + 1023) / 1024 * 1024
}
