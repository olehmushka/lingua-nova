package eval

import (
	"bytes"
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/olehmushka/lingua-nova/internal/gloss"
	"github.com/olehmushka/lingua-nova/internal/llm"
)

func ctxOf(ids ...string) llm.Context {
	c := llm.Context{}
	for _, id := range ids {
		c.Entries = append(c.Entries, llm.Entry{ID: id})
	}
	return c
}

func TestGoldOf(t *testing.T) {
	v := &gloss.Tok{ID: "see"}
	x := &llm.Expectation{Refs: []llm.Reference{{Clauses: []gloss.Clause{
		{Subject: []gloss.Tok{{ID: "the"}, {ID: "woman"}}, Verb: v, Object: []gloss.Tok{{ID: "the"}, {ID: "river"}, {}}},
	}}}}
	g := GoldOf(x)
	if want := [][]string{{"see", "the", "woman", "river"}}; !slices.EqualFunc(g.Refs, want, slices.Equal) {
		t.Errorf("Refs = %v, want %v", g.Refs, want)
	}
	g = GoldOf(&llm.Expectation{Words: []llm.Expected{{Word: "fly", IDs: []string{"fly", "fly_insect"}}}})
	if len(g.Refs) != 0 || len(g.Words) != 1 {
		t.Errorf("GoldOf(words) = %+v", g)
	}
}

func TestRecall(t *testing.T) {
	tests := []struct {
		name    string
		gold    Gold
		ctx     llm.Context
		recall  float64
		missing []string
	}{
		{"empty gold", Gold{}, ctxOf(), 1, nil},
		{"all in", Gold{Refs: [][]string{{"a", "b"}}}, ctxOf("a", "b", "c"), 1, nil},
		{"half", Gold{Refs: [][]string{{"a", "b"}}}, ctxOf("a"), 0.5, []string{"b"}},
		{"best reference", Gold{Refs: [][]string{{"a", "x"}, {"a", "b"}}}, ctxOf("a", "b"), 1, nil},
		{"any id of a word", Gold{Words: [][]string{{"fly", "fly_insect"}, {"sea"}}}, ctxOf("fly_insect"), 0.5, []string{"sea"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, missing := tt.gold.Recall(tt.ctx)
			if r != tt.recall || !slices.Equal(missing, tt.missing) {
				t.Errorf("Recall = %v %v, want %v %v", r, missing, tt.recall, tt.missing)
			}
		})
	}
}

// TestRetrievalEval runs the retrieval eval on the real lexicon: every
// sentence's context must hold every entry its rule-based translation
// uses, and stay small.
func TestRetrievalEval(t *testing.T) {
	cases, err := Load(filepath.Join(langDir, "evals.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	b, _, k := setup(t)
	recs := Retrieval(cases,
		func(s string) Gold { return GoldOf(k.Expect(s)) },
		func(s string) llm.Context {
			c, err := b.Build(context.Background(), llm.Retrieved, s)
			if err != nil {
				t.Fatal(err)
			}
			return c
		})
	for _, r := range recs {
		if r.Recall < 1 {
			t.Errorf("%q: recall %.0f%%, missing %v", r.English, 100*r.Recall, r.Missing)
		}
	}
	if tot := Totals(recs); tot.Entries > 40 {
		t.Errorf("mean %.0f entries per context, want ≤ 40", tot.Entries)
	}
	var out bytes.Buffer
	ReportRetrieval(&out, recs)
	if !strings.Contains(out.String(), "mean (") {
		t.Errorf("report has no totals line:\n%s", out.String())
	}
}
