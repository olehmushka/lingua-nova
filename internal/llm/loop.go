package llm

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"time"
)

// DefaultMaxNumCtx is the biggest context window Translate asks for.
// qwen2.5's limit is 32K tokens; past it Ollama silently cuts the start
// of the prompt (the grammar), and long before it the KV cache fills the
// GPU. A full context of a big lexicon needs millions.
const DefaultMaxNumCtx = 32768

// Options configure a translation.
type Options struct {
	Model       string // Ollama model; "" = DefaultModel
	MaxAttempts int    // 0 = 3
	NumCtx      int    // model context window in tokens; 0 = sized to the prompt
	MaxNumCtx   int    // refuse a prompt that needs a bigger window; 0 = DefaultMaxNumCtx

	Tools         *Toolbox // nil: no tools, the model answers straight away
	MaxToolRounds int      // tool rounds per attempt before the answer is forced; 0 = 8

	// Temperature 0 is greedy decoding: the most likely token every
	// time, so the same prompt gives (nearly) the same answer. Above 0
	// the model samples, and Seed fixes the sampler's random numbers:
	// same seed, same prompt, same answer. Every call of the sentence
	// uses the same seed.
	Temperature float64
	Seed        *int
}

// Attempt is one answer and its check, with the tool steps that led to
// it. Tokens and duration add up every model call of the attempt.
type Attempt struct {
	Steps        []Step
	Answer       Answer
	Check        *Check
	PromptTokens int
	EvalTokens   int
	Duration     time.Duration
}

// Result is every attempt at one sentence; the last one passed if OK.
type Result struct {
	Context  Context
	Expect   *Expectation // what the answer must mean; nil: not checked
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
// With tools, each attempt is itself a loop (the agent loop: act,
// observe, repeat): the model replies with tool calls, Go runs them and
// sends the results back, until the model stops calling or MaxToolRounds
// is reached. Then a final call asks for the answer in the JSON schema.
// The two can't be one call: constrained decoding to the schema would
// mask the tokens of a tool call.
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
	if opt.MaxToolRounds == 0 {
		opt.MaxToolRounds = 8
	}
	if opt.NumCtx == 0 {
		opt.NumCtx = numCtx(c, opt.Tools != nil)
	}
	if limit := cmp.Or(opt.MaxNumCtx, DefaultMaxNumCtx); opt.NumCtx > limit {
		return nil, fmt.Errorf("the prompt needs a %d-token context window (%d entries), over the limit of %d; use a retrieved context (-ctx retrieved)",
			opt.NumCtx, len(c.Entries), limit)
	}
	res := &Result{Context: c, Expect: t.Checker.Expect(english)}
	var history []Turn
	seen := map[string]string{} // tool calls made for this sentence, over all attempts
	for range opt.MaxAttempts {
		req := Request{English: english, Model: opt.Model, Context: c, History: history, NumCtx: opt.NumCtx,
			Temperature: opt.Temperature, Seed: opt.Seed}
		a, err := t.attempt(ctx, req, opt, res.Expect, seen)
		if a != nil {
			res.Attempts = append(res.Attempts, *a)
		}
		if err != nil {
			return res, err
		}
		if a.Check.OK() {
			res.OK = true
			break
		}
		history = append(history, Turn{Steps: a.Steps, Answer: a.Answer, Feedback: a.Check.Feedback()})
	}
	return res, nil
}

// attempt runs the tool rounds (if any) and the final call of one
// attempt, and checks the answer. On an error during the tool rounds it
// returns nil: there's no answer to show.
func (t *Translator) attempt(ctx context.Context, req Request, opt Options, x *Expectation, seen map[string]string) (*Attempt, error) {
	a := &Attempt{}
	call := func(req Request) (*Response, error) {
		resp, err := t.Client.Translate(ctx, req)
		if err != nil {
			return nil, err
		}
		a.PromptTokens += resp.PromptTokens
		a.EvalTokens += resp.EvalTokens
		a.Duration += time.Duration(resp.DurationMS) * time.Millisecond
		return resp, nil
	}
	if opt.Tools != nil {
		req.Tools = opt.Tools.Specs()
		for range opt.MaxToolRounds {
			req.Steps = a.Steps
			resp, err := call(req)
			if err != nil {
				return nil, err
			}
			if len(resp.Calls) == 0 {
				break
			}
			st := Step{Calls: resp.Calls}
			for _, c := range resp.Calls {
				st.Results = append(st.Results, opt.Tools.runOnce(ctx, c, x, seen))
			}
			a.Steps = append(a.Steps, st)
		}
		req.Steps, req.Final = a.Steps, true
	}
	resp, err := call(req)
	if err != nil {
		return nil, err
	}
	if resp.Answer == nil {
		return nil, errors.New("llm service: reply has no answer")
	}
	a.Answer = *resp.Answer
	a.Check = t.Checker.Verify(a.Answer, x)
	return a, nil
}

// numCtx sizes the model's context window for a prompt: its Tokens, plus
// room for the
// instructions, three rounds of answers and feedback, and the reply, rounded
// up to 1024. A window that's too small makes Ollama silently cut the
// start of the prompt (the grammar); one that's too big wastes GPU memory
// on the KV cache. Tool rounds (calls and their results) need about
// 2K tokens more.
func numCtx(c Context, tools bool) int {
	n := c.Tokens() + 4096
	if tools {
		n += 2048
	}
	return (n + 1023) / 1024 * 1024
}
