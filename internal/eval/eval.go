// Package eval measures LLM translation: a fixed set of English sentences
// (language/evals.yaml), each translated several times per setup, gives a
// pass rate per setup with a confidence interval.
//
// One run per sentence says little: at temperature > 0 the model samples,
// and the same sentence can pass once and fail the next time. So each
// sentence runs N times, run i with seed i. The spread is part of the
// measurement, and any single run can be replayed with
// `lnova llm -temp T -seed i`.
package eval

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/olehmushka/lingua-nova/internal/llm"
)

// Case is one eval sentence.
type Case struct {
	English string   `yaml:"en"`
	Tags    []string `yaml:"tags"`
	Line    int      `yaml:"-"`
}

// Load reads the eval set. Errors name the file and line.
func Load(path string) ([]Case, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s: want a list of sentences", path)
	}
	var cases []Case
	seen := map[string]int{}
	for _, item := range doc.Content[0].Content {
		var c Case
		if err := item.Decode(&c); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, item.Line, err)
		}
		c.Line = item.Line
		c.English = strings.TrimSpace(c.English)
		if c.English == "" {
			return nil, fmt.Errorf("%s:%d: no en", path, item.Line)
		}
		if l, ok := seen[c.English]; ok {
			return nil, fmt.Errorf("%s:%d: %q is already on line %d", path, item.Line, c.English, l)
		}
		seen[c.English] = c.Line
		cases = append(cases, c)
	}
	return cases, nil
}

// WithTag keeps the cases that have the tag; "" keeps all.
func WithTag(cases []Case, tag string) []Case {
	if tag == "" {
		return cases
	}
	var out []Case
	for _, c := range cases {
		if slices.Contains(c.Tags, tag) {
			out = append(out, c)
		}
	}
	return out
}

// Setup is one way of asking the model: which context, with tools or not.
type Setup struct {
	Ctx   string // llm.Retrieved, llm.Full or llm.None
	Tools bool
}

func (s Setup) String() string {
	if s.Tools {
		return s.Ctx + "+tools"
	}
	return s.Ctx
}

// ParseSetups reads "retrieved,retrieved+tools,none+tools".
func ParseSetups(s string) ([]Setup, error) {
	var out []Setup
	for name := range strings.SplitSeq(s, ",") {
		name = strings.TrimSpace(name)
		ctx, tools := strings.CutSuffix(name, "+tools")
		if !slices.Contains([]string{llm.Retrieved, llm.Full, llm.None}, ctx) {
			return nil, fmt.Errorf("setup %q: want retrieved, full or none, optionally +tools", name)
		}
		su := Setup{Ctx: ctx, Tools: tools}
		if slices.Contains(out, su) {
			return nil, fmt.Errorf("setup %q twice", name)
		}
		out = append(out, su)
	}
	return out, nil
}

// Record is one translation of one sentence in one setup.
type Record struct {
	English      string   `json:"english"`
	Setup        string   `json:"setup"`
	Run          int      `json:"run"`
	Seed         *int     `json:"seed,omitempty"`
	OK           bool     `json:"ok"`
	FirstOK      bool     `json:"first_ok"` // passed without feedback
	Attempts     int      `json:"attempts"`
	Translation  string   `json:"translation"` // the last attempt's
	Problems     []string `json:"problems,omitempty"`
	DurationMS   int64    `json:"duration_ms"`
	PromptTokens int      `json:"prompt_tokens"`
	EvalTokens   int      `json:"eval_tokens"`
	ToolCalls    int      `json:"tool_calls"`
	Repeats      int      `json:"repeats"` // tool calls answered "repeated call"
	Error        string   `json:"error,omitempty"`
}

// Runner translates the cases. Options holds what every setup shares
// (model, attempts, tool rounds, temperature); Run sets the tools and the
// seed.
type Runner struct {
	Translator *llm.Translator
	Builder    *llm.Builder
	Tools      *llm.Toolbox
	Options    llm.Options
	Timeout    time.Duration // per translation; 0 = none
	Progress   io.Writer     // a line per translation; nil = quiet

	// Done are the records of an earlier, unfinished run: Run keeps
	// them and doesn't translate those again (same sentence, setup and
	// run number).
	Done []Record
	// Saved, if set, is called with every record so far after each new
	// one, so a run that dies (a crash, a closed terminal) loses at most
	// the translation in progress.
	Saved func([]Record)
}

// Run translates every case in every setup, runs times, except what's
// in Done. Run 1 of
// everything comes first, then run 2, …, so a run stopped early (ctx
// cancelled) still has every cell, just with fewer runs. A translation
// that times out or fails (service down) is recorded with its Error and
// counts as not passed; a cancelled ctx stops Run, and the translation in
// progress isn't recorded.
func (r *Runner) Run(ctx context.Context, cases []Case, setups []Setup, runs int) ([]Record, error) {
	type key struct {
		english, setup string
		run            int
	}
	done := map[key]bool{}
	for _, rec := range r.Done {
		done[key{rec.English, rec.Setup, rec.Run}] = true
	}
	out := slices.Clone(r.Done)
	total, n := runs*len(setups)*len(cases), 0
	for run := 1; run <= runs; run++ {
		for _, su := range setups {
			for _, c := range cases {
				n++
				if done[key{c.English, su.String(), run}] {
					continue
				}
				rec, err := r.one(ctx, c, su, run)
				if err != nil {
					return out, err
				}
				out = append(out, rec)
				if r.Saved != nil {
					r.Saved(out)
				}
				if r.Progress != nil {
					mark := "✗"
					switch {
					case rec.Error != "":
						mark = "! " + rec.Error
					case rec.OK:
						mark = "✓"
					}
					fmt.Fprintf(r.Progress, "[%d/%d] run %d %s  %s → %s  %s  %.1fs\n", n, total, run, su, c.English,
						rec.Translation, mark, float64(rec.DurationMS)/1000)
				}
			}
		}
	}
	return out, nil
}

// one translates a case once. Its error is only ctx's: everything else
// goes into the record.
func (r *Runner) one(ctx context.Context, c Case, su Setup, run int) (Record, error) {
	opt := r.Options
	if su.Tools {
		opt.Tools = r.Tools
	}
	rec := Record{English: c.English, Setup: su.String(), Run: run}
	if opt.Temperature > 0 {
		seed := run
		opt.Seed, rec.Seed = &seed, &seed
	}
	tctx, cancel := ctx, context.CancelFunc(func() {})
	if r.Timeout > 0 {
		tctx, cancel = context.WithTimeout(ctx, r.Timeout)
	}
	defer cancel()
	start := time.Now()
	res, err := r.Translator.Translate(tctx, c.English, r.Builder.Build(su.Ctx, c.English), opt)
	if ctx.Err() != nil {
		return rec, ctx.Err()
	}
	if res != nil {
		rec.fill(res)
	}
	if err != nil {
		rec.OK = false
		rec.Error = err.Error()
		if errors.Is(tctx.Err(), context.DeadlineExceeded) {
			rec.Error = fmt.Sprintf("timeout after %s", r.Timeout)
		}
		// The service's own time doesn't include the call that never came back.
		rec.DurationMS = time.Since(start).Milliseconds()
	}
	return rec, nil
}

func (rec *Record) fill(res *llm.Result) {
	rec.OK = res.OK
	rec.Attempts = len(res.Attempts)
	rec.FirstOK = len(res.Attempts) > 0 && res.Attempts[0].Check.OK()
	for _, a := range res.Attempts {
		rec.DurationMS += a.Duration.Milliseconds()
		rec.PromptTokens += a.PromptTokens
		rec.EvalTokens += a.EvalTokens
		for _, st := range a.Steps {
			rec.ToolCalls += len(st.Calls)
			for _, x := range st.Results {
				if strings.HasPrefix(x.Content, "repeated call:") {
					rec.Repeats++
				}
			}
		}
	}
	if last := res.Last(); last != nil {
		rec.Translation = last.Answer.Translation
		rec.Problems = last.Check.Problems
	}
}
