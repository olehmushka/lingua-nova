package eval

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/olehmushka/lingua-nova/internal/gloss"
	"github.com/olehmushka/lingua-nova/internal/llm"
)

// The retrieval eval measures the retriever alone, without a model: does
// the context built for a sentence hold the entries a correct translation
// needs? A translation can only be as good as its context, so this is
// recall, the share of the needed entries that got in. And at what cost:
// the context's size in entries and tokens. It's deterministic and needs
// no model, so it runs in a second and can be a test.

// Gold is what a context must hold for a sentence, from the rule-based
// translator's expectation (llm.Checker.Expect). Either Refs: the entry ids
// of each reference translation, any one of which is enough; or, when the
// sentence has no parse, Words: per content word, the ids it can be, any
// one of which is enough.
type Gold struct {
	Refs  [][]string
	Words [][]string
}

// GoldOf is the gold for an expectation; nil gives an empty Gold.
func GoldOf(x *llm.Expectation) Gold {
	var g Gold
	if x == nil {
		return g
	}
	for _, r := range x.Refs {
		var ids []string
		add := func(t gloss.Tok) {
			if t.ID != "" && !slices.Contains(ids, t.ID) {
				ids = append(ids, t.ID)
			}
		}
		for _, c := range r.Clauses {
			for _, t := range []*gloss.Tok{c.Comp, c.Wh, c.Neg, c.Verb} {
				if t != nil {
					add(*t)
				}
			}
			for _, t := range append(slices.Clone(c.Subject), c.Object...) {
				add(t)
			}
		}
		if len(ids) > 0 {
			g.Refs = append(g.Refs, ids)
		}
	}
	for _, w := range x.Words {
		g.Words = append(g.Words, w.IDs)
	}
	return g
}

// Recall is the share of the gold the context holds, and what's missing:
// with references, the best reference's share and its missing ids; else
// the share of words with an id in the context, and the words' first ids.
// An empty gold has recall 1: nothing is needed.
func (g Gold) Recall(c llm.Context) (float64, []string) {
	in := map[string]bool{}
	for _, e := range c.Entries {
		in[e.ID] = true
	}
	if len(g.Refs) > 0 {
		best, missing := -1.0, []string(nil)
		for _, ids := range g.Refs {
			var miss []string
			for _, id := range ids {
				if !in[id] {
					miss = append(miss, id)
				}
			}
			if r := float64(len(ids)-len(miss)) / float64(len(ids)); r > best {
				best, missing = r, miss
			}
		}
		return best, missing
	}
	if len(g.Words) == 0 {
		return 1, nil
	}
	var missing []string
	for _, ids := range g.Words {
		if !slices.ContainsFunc(ids, func(id string) bool { return in[id] }) && len(ids) > 0 {
			missing = append(missing, ids[0])
		}
	}
	return float64(len(g.Words)-len(missing)) / float64(len(g.Words)), missing
}

// RetrievalRecord is one sentence's result.
type RetrievalRecord struct {
	English  string
	Recall   float64
	Missing  []string
	Entries  int
	Tokens   int
	Duration time.Duration
}

// Retrieval builds the context of every case and scores it against its
// gold. gold and build are separate so the gold can come from another
// lexicon: on a big fixture the reference translator would also use the
// synthetic entries, but the gold must be the real ones.
func Retrieval(cases []Case, gold func(string) Gold, build func(string) llm.Context) []RetrievalRecord {
	var out []RetrievalRecord
	for _, c := range cases {
		g := gold(c.English)
		start := time.Now()
		ctx := build(c.English)
		d := time.Since(start)
		r, missing := g.Recall(ctx)
		out = append(out, RetrievalRecord{c.English, r, missing, len(ctx.Entries), ctx.Tokens(), d})
	}
	return out
}

// RetrievalTotals are the means over the records, and the slowest build.
type RetrievalTotals struct {
	Recall          float64
	Entries, Tokens float64
	Max             time.Duration
}

// Totals sums up a retrieval run.
func Totals(recs []RetrievalRecord) RetrievalTotals {
	var t RetrievalTotals
	if len(recs) == 0 {
		return t
	}
	for _, r := range recs {
		t.Recall += r.Recall
		t.Entries += float64(r.Entries)
		t.Tokens += float64(r.Tokens)
		t.Max = max(t.Max, r.Duration)
	}
	n := float64(len(recs))
	t.Recall, t.Entries, t.Tokens = t.Recall/n, t.Entries/n, t.Tokens/n
	return t
}

// ReportRetrieval prints one line per sentence and the means:
//
//	sentence                    recall  entries  tokens  time   missing
//	The woman sees the river.   100%    29       7142    2ms
func ReportRetrieval(w io.Writer, recs []RetrievalRecord) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "sentence\trecall\tentries\ttokens\ttime\tmissing")
	for _, r := range recs {
		fmt.Fprintf(tw, "%s\t%.0f%%\t%d\t%d\t%s\t%s\n", r.English, 100*r.Recall, r.Entries, r.Tokens,
			r.Duration.Round(time.Millisecond), strings.Join(r.Missing, " "))
	}
	t := Totals(recs)
	fmt.Fprintf(tw, "mean (%d sentences)\t%.1f%%\t%.0f\t%.0f\tmax %s\t\n", len(recs), 100*t.Recall, t.Entries, t.Tokens,
		t.Max.Round(time.Millisecond))
	tw.Flush()
}
