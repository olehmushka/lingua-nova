package llm

import (
	"fmt"
	"slices"
	"strings"

	"github.com/olehmushka/lingua-nova/internal/english"
	"github.com/olehmushka/lingua-nova/internal/gloss"
)

// Expectation is what a right answer must mean, worked out once per
// English sentence. The glosser checks that an answer is Rifatian; this
// checks that it says what the English says.
//
// When the Phase 4 grammar parses the English, the rule-based translator
// gives reference translations, and the answer's clauses (who did what
// to whom, when, negated or not) must equal those of one of them. This
// compares meaning, not spelling: both sides go through the same glosser,
// so `Muh noidaremu` and `Muh eluviremu` differ in the verb slot, while a
// dropped subject pronoun still matches.
//
// When the English doesn't parse, there's no reference, only the words:
// each English content word needs one of its entries in the answer, and
// the answer may not add content words of its own.
type Expectation struct {
	English string
	Refs    []Reference // the frame check; empty when there's no parse
	Words   []Expected  // the lexical check, used when Refs is empty
	Why     string      // why there are no references
}

// Reference is a rule-based translation cut into clauses.
type Reference struct {
	Text    string
	Clauses []gloss.Clause
}

// Expected is an English content word and the entries it can be.
type Expected struct {
	Word string
	IDs  []string
}

// contentCats are the English categories whose words carry meaning the
// answer must keep; articles, auxiliaries and negation become grammar.
var contentCats = []string{"N", "V", "Adj", "Pron", "WhP", "WhAdv"}

// contentPOS are the Rifatian parts of speech an answer can't add freely.
var contentPOS = []string{"noun", "verb", "adjective", "pronoun", "adverb"}

// Expect works out what an answer to the English sentence must mean.
// Without a rule-based translator there's nothing to compare: nil.
func (k *Checker) Expect(sentence string) *Expectation {
	if k.Ref == nil {
		return nil
	}
	x := &Expectation{English: sentence}
	res, err := k.Ref.Translate(sentence, 0)
	if err == nil {
		for _, t := range res.Translations {
			if t.Gloss != nil && len(t.Notes) == 0 {
				x.Refs = append(x.Refs, Reference{t.Text, t.Gloss.Clauses()})
			}
		}
	}
	switch {
	case len(x.Refs) > 0:
		return x
	case err != nil:
		x.Why = err.Error()
	default:
		x.Why = "every translation has a word that isn't in the lexicon"
	}
	for _, tok := range k.Ref.Tagger.Tag(sentence).Tokens {
		if len(tok.Readings) > 0 && !slices.ContainsFunc(tok.Readings, func(r english.Reading) bool {
			return slices.Contains(contentCats, r.Cat)
		}) {
			continue
		}
		if ids := candidates(k.Lex, tok); len(ids) > 0 {
			x.Words = append(x.Words, Expected{tok.Text, ids})
		}
	}
	return x
}

// Kind says which check the expectation runs, for showing.
func (x *Expectation) Kind() string {
	switch {
	case x == nil:
		return "none"
	case len(x.Refs) > 0:
		return fmt.Sprintf("frame, %d reference(s)", len(x.Refs))
	case len(x.Words) > 0:
		return "lexical (" + x.Why + ")"
	}
	return "none (" + x.Why + ")"
}

// problems compares a glossed answer with the expectation. With
// references, the answer passes if its clauses equal one reference's;
// otherwise the problems are the slots that differ from the closest one.
func (x *Expectation) problems(g *gloss.Result) []string {
	if x == nil {
		return nil
	}
	if len(x.Refs) == 0 {
		return x.lexical(g)
	}
	got := g.Clauses()
	var best []string
	bestShared := 0
	for i, r := range x.Refs {
		d := diff(r.Clauses, got)
		if len(d) == 0 {
			return nil
		}
		// Ties go to the reference with more of the answer's entries:
		// for "we will love", Muh noidareh is closest to Muh noidaremu
		// (wrong ending), not to Muh eluviremu (wrong verb).
		n := shared(r.Clauses, got)
		if i == 0 || len(d) < len(best) || len(d) == len(best) && n > bestShared {
			best, bestShared = d, n
		}
	}
	return best
}

// shared counts the words of got whose entry is in the same slot of want.
func shared(want, got []gloss.Clause) int {
	n := 0
	ids := func(c gloss.Clause) [][]gloss.Tok {
		var v []gloss.Tok
		if c.Verb != nil {
			v = []gloss.Tok{*c.Verb}
		}
		return [][]gloss.Tok{c.Subject, v, c.Object}
	}
	for i := range min(len(want), len(got)) {
		w, g := ids(want[i]), ids(got[i])
		for j := range w {
			for _, t := range g[j] {
				if slices.ContainsFunc(w[j], func(u gloss.Tok) bool { return u.ID == t.ID }) {
					n++
				}
			}
		}
	}
	return n
}

// diff lists the slots where the answer's clauses differ from the
// reference's. It names words by their glosses, not their spelling: the
// model is told what to say, and still has to inflect it.
func diff(want, got []gloss.Clause) []string {
	if len(want) != len(got) {
		return []string{fmt.Sprintf("the English is %d clause(s); you have %d", len(want), len(got))}
	}
	var out []string
	for i, w := range want {
		g := got[i]
		pre := ""
		if len(want) > 1 {
			pre = fmt.Sprintf("clause %d: ", i+1)
		}
		add := func(f string, args ...any) { out = append(out, pre+fmt.Sprintf(f, args...)) }

		if !sameTok(w.Wh, g.Wh) {
			add("question word: the English needs %s; you have %s", glossOf(w.Wh), glossOf(g.Wh))
		}
		if !sameToks(w.Subject, g.Subject) && !proDrop(w, g) {
			add("subject: the English needs %s; you have %s", glosses(w.Subject), glosses(g.Subject))
		}
		switch {
		case w.Neg != nil && g.Neg == nil:
			add("negation: the English is negative; %s goes right before the verb", w.Neg.Gloss)
		case w.Neg == nil && g.Neg != nil:
			add("negation: the English isn't negative; drop %s", g.Neg.Gloss)
		}
		switch {
		case g.Verb == nil:
			add("verb: your clause has no conjugated verb; the English needs %s", glossOf(w.Verb))
		case !sameTok(w.Verb, g.Verb):
			add("verb: the English needs %s; you have %s", glossOf(w.Verb), glossOf(g.Verb))
		}
		if !sameToks(w.Object, g.Object) {
			add("object: the English needs %s, in this order; you have %s", glosses(w.Object), glosses(g.Object))
		}
	}
	return out
}

// proDrop reports whether the answer dropped a subject pronoun the
// reference has. The verb's ending carries the person, so nothing is lost.
func proDrop(want, got gloss.Clause) bool {
	return len(got.Subject) == 0 && len(want.Subject) == 1 && strings.HasSuffix(want.Subject[0].Feats, "SBJ")
}

func sameTok(a, b *gloss.Tok) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Same(*b)
}

func sameToks(a, b []gloss.Tok) bool {
	return slices.EqualFunc(a, b, gloss.Tok.Same)
}

func glossOf(t *gloss.Tok) string {
	if t == nil {
		return "none"
	}
	return t.Gloss
}

func glosses(ts []gloss.Tok) string {
	if len(ts) == 0 {
		return "nothing"
	}
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.Gloss
	}
	return `"` + strings.Join(out, " ") + `"`
}

// lexical checks the words alone: every English content word has one of
// its entries in the answer, and every content word of the answer comes
// from the English.
func (x *Expectation) lexical(g *gloss.Result) []string {
	have := map[string]bool{}
	for _, w := range g.Words {
		if t := w.Top(); t != nil && t.Entry != nil {
			have[t.Entry.ID] = true
		}
	}
	var out []string
	allowed := map[string]bool{}
	for _, e := range x.Words {
		for _, id := range e.IDs {
			allowed[id] = true
		}
		if !slices.ContainsFunc(e.IDs, func(id string) bool { return have[id] }) {
			out = append(out, fmt.Sprintf("%q is missing: none of its entries (%s) is in your sentence", e.Word, strings.Join(e.IDs, ", ")))
		}
	}
	for _, w := range g.Words {
		t := w.Top()
		if t != nil && t.Entry != nil && slices.Contains(contentPOS, t.POS) && !allowed[t.Entry.ID] {
			out = append(out, fmt.Sprintf("%s (%s): no English word means this", strings.ToLower(w.Text), t.Entry.ID))
		}
	}
	return out
}
