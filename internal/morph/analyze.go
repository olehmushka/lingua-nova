package morph

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/olehmushka/lingua-nova/internal/lexicon"
	"github.com/olehmushka/lingua-nova/internal/phon"
)

// Analysis is one way to read a word form: which lexeme, which features.
type Analysis struct {
	Entry *lexicon.Entry // nil for a guess about a root not in the lexicon
	Lemma string         // citation form: narar, menah; a guess's is not marked
	POS   string
	Class string // verb class name: I; "" for other parts of speech
	Form  Form   // the morphemes, which re-inflect to the analyzed word
}

// Guess reports whether the root isn't in the lexicon.
func (a Analysis) Guess() bool { return a.Entry == nil }

// Analyzer runs Inflect in reverse: word form → root + features.
//
// It works by affix stripping and verification (analysis by synthesis):
// peel endings off the right edge using the morphology tables, which
// gives candidate stems and features, then inflect each candidate and keep
// it only if it gives back the word. Stripping is nondeterministic (the
// final -s of naras could be 2SG or POSS), so every path is tried.
type Analyzer struct {
	r        *Rules
	byBase   map[string][]*lexicon.Entry // base(e) → entries
	pronouns map[string][]pronounCell    // whole form → cells
}

type pronounCell struct {
	entry        *lexicon.Entry
	number, kase string
}

// NewAnalyzer indexes the lexicon. Pronoun forms go in a lookup table:
// suppletive forms (mu, mi, me) share no stem to strip down to.
func NewAnalyzer(r *Rules, lex *lexicon.Lexicon) *Analyzer {
	a := &Analyzer{r: r, byBase: map[string][]*lexicon.Entry{}, pronouns: map[string][]pronounCell{}}
	for _, e := range lex.Entries {
		a.byBase[base(e)] = append(a.byBase[base(e)], e)
		p, ok := r.Pronouns[e.ID]
		if e.POS != "pronoun" || !ok {
			continue
		}
		for _, n := range Numbers {
			for _, c := range Cases {
				if f := pronounForm(p, n, c); f != nil {
					w := f.Word()
					a.pronouns[w] = append(a.pronouns[w], pronounCell{e, n, c})
				}
			}
		}
	}
	return a
}

// candidate is a hypothesis from stripping, not yet verified.
type candidate struct {
	lemma, pos string
	class      *Class   // verbs
	feats      []string // as Inflect takes them
}

// Analyze returns every analysis of a word, best first: known roots before
// guesses, then verb, noun, pronoun, other, then paradigm order. Guesses
// are only made when no known root fits. An error means the word can't be
// spelled with the inventory's letters; a word with no analysis gives none.
func (a *Analyzer) Analyze(word string) ([]Analysis, error) {
	word = strings.ToLower(word)
	segs, err := a.r.inv.Tokenize(word)
	if err != nil {
		return nil, err
	}
	if len(segs) == 0 {
		return nil, fmt.Errorf("empty word")
	}

	var out []Analysis
	// Bare words: uninflected parts of speech, and verb infinitives.
	// Nouns and pronouns are found as paradigm cells below.
	for _, e := range a.byBase[word] {
		switch e.POS {
		case "noun":
		case "pronoun":
			if _, ok := a.r.Pronouns[e.ID]; !ok {
				out = append(out, Analysis{Entry: e, Lemma: word, POS: e.POS, Form: bare(e)})
			}
		case "verb":
			if c := a.r.classOf(word); c != nil {
				out = append(out, Analysis{Entry: e, Lemma: word, POS: e.POS, Class: c.Name, Form: infinitive(e.Gloss, word, c)})
			}
		default:
			out = append(out, Analysis{Entry: e, Lemma: word, POS: e.POS, Form: bare(e)})
		}
	}
	for _, c := range a.pronouns[word] {
		f := pronounForm(a.r.Pronouns[c.entry.ID], c.number, c.kase)
		out = append(out, Analysis{Entry: c.entry, Lemma: base(c.entry), POS: "pronoun", Form: *f})
	}

	cands := append(a.r.stripVerb(word), a.r.stripNoun(word)...)
	for _, c := range cands {
		for _, e := range a.byBase[c.lemma] {
			if e.POS != c.pos {
				continue
			}
			if an, ok := a.verify(e, c, word); ok {
				out = append(out, an)
			}
		}
	}

	if len(out) == 0 {
		out = a.guess(word, cands)
	}
	slices.SortStableFunc(out, a.r.compare)
	return slices.CompactFunc(out, func(x, y Analysis) bool { return a.r.compare(x, y) == 0 }), nil
}

// verify inflects e with the candidate's features and keeps the analysis
// only if that spells word and the morpheme breaks don't cut a digraph.
func (a *Analyzer) verify(e *lexicon.Entry, c candidate, word string) (Analysis, bool) {
	f, err := a.r.Inflect(e, c.feats...)
	if err != nil || f.Word() != word {
		return Analysis{}, false
	}
	segs, err := a.r.inv.Tokenize(word)
	if err != nil || a.r.mergesAcrossBoundary(f, segs) {
		return Analysis{}, false
	}
	an := Analysis{Entry: e, Lemma: base(e), POS: c.pos, Form: f}
	if c.class != nil {
		an.Class = c.class.Name
	}
	if c.pos == "noun" {
		f.Feats = nounFeats(f.Feats)
		an.Form = f
	}
	return an, true
}

// guess verifies each candidate against a made-up entry. The lemma must
// obey the modern phonotactics and its stem must have a vowel, so p-a-x-u
// isn't read as a verb p + PST.
func (a *Analyzer) guess(word string, cands []candidate) []Analysis {
	var out []Analysis
	if c := a.r.classOf(word); c != nil {
		cands = append(cands, candidate{lemma: word, pos: "verb", class: c, feats: []string{"INF"}})
	}
	for _, c := range cands {
		segs, err := a.r.inv.Tokenize(c.lemma)
		if err != nil || len(a.r.inv.CheckPhonotactics(segs)) > 0 {
			continue
		}
		stem := c.lemma
		if c.class != nil {
			stem = strings.TrimSuffix(c.lemma, c.class.Infinitive)
		}
		if !a.r.hasVowel(stem) {
			continue
		}
		e := &lexicon.Entry{Modern: c.lemma, POS: c.pos, Gloss: "?"}
		if slices.Equal(c.feats, []string{"INF"}) {
			out = append(out, Analysis{Lemma: word, POS: "verb", Class: c.class.Name, Form: infinitive("?", word, c.class)})
			continue
		}
		if an, ok := a.verify(e, c, word); ok {
			an.Entry = nil
			out = append(out, an)
		}
	}
	return out
}

func (r *Rules) hasVowel(s string) bool {
	segs, err := r.inv.Tokenize(s)
	return err == nil && slices.ContainsFunc(segs, func(s phon.Segment) bool { return s.Kind == phon.Vowel })
}

// stripVerb peels a verb right to left: person ending, tense marker, theme
// vowel. The theme vowel tells the class; a person without one (PRS 1SG
// naru) could be any class, so each is a candidate.
func (r *Rules) stripVerb(word string) []candidate {
	var out []candidate
	for _, t := range r.Verb.Tenses {
		for _, p := range r.Verb.Persons {
			rest, ok := strings.CutSuffix(word, t.Endings[p])
			if !ok {
				continue
			}
			if rest, ok = strings.CutSuffix(rest, t.Marker); !ok {
				continue
			}
			for i := range r.Verb.Classes {
				c := &r.Verb.Classes[i]
				stem := rest
				if !slices.Contains(t.NoTheme, p) {
					th := cmp.Or(t.Theme[c.Name][p], c.Theme)
					if stem, ok = strings.CutSuffix(rest, th); !ok {
						continue
					}
				}
				if stem != "" {
					out = append(out, candidate{lemma: stem + c.Infinitive, pos: "verb", class: c, feats: []string{t.Name, p}})
				}
			}
		}
	}
	return out
}

// stripNoun peels noun suffixes in reverse slot order (POSS, then PL),
// each optional, trying both shapes. menahes gives menahes, menah + POSS
// and mena + PL + POSS; verification throws out the wrong ones.
func (r *Rules) stripNoun(word string) []candidate {
	var out []candidate
	var peel func(slot int, rest string, feats []string)
	peel = func(slot int, rest string, feats []string) {
		if slot < 0 {
			if rest != "" {
				out = append(out, candidate{lemma: rest, pos: "noun", feats: slices.Clone(feats)})
			}
			return
		}
		peel(slot-1, rest, feats)
		s := r.Noun[slot]
		for _, shape := range []string{s.AfterVowel, s.AfterConsonant} {
			if stem, ok := strings.CutSuffix(rest, shape); ok {
				peel(slot-1, stem, append([]string{s.Name}, feats...))
			}
		}
	}
	peel(len(r.Noun)-1, word, nil)
	return out
}

// nounFeats names the number as Paradigm does: [] → [SG], [POSS] → [SG POSS].
func nounFeats(feats []string) []string {
	if slices.Contains(feats, "PL") {
		return feats
	}
	return append([]string{"SG"}, feats...)
}

// classOf finds a verb's class from its infinitive.
func (r *Rules) classOf(inf string) *Class {
	for i, c := range r.Verb.Classes {
		if len(inf) > len(c.Infinitive) && strings.HasSuffix(inf, c.Infinitive) {
			return &r.Verb.Classes[i]
		}
	}
	return nil
}

func bare(e *lexicon.Entry) Form {
	return Form{Morphemes: []Morpheme{{base(e), e.Gloss}}}
}

// infinitive is nar-ar, see-INF.
func infinitive(gloss, inf string, c *Class) Form {
	return Form{Feats: []string{"INF"}, Morphemes: []Morpheme{
		{strings.TrimSuffix(inf, c.Infinitive), gloss},
		{c.Infinitive, "INF"},
	}}
}

// compare orders analyses: known first; then, for guesses, the one that
// explains more of the word; then part of speech, paradigm order, lemma.
func (r *Rules) compare(x, y Analysis) int {
	posRank := func(pos string) int {
		if i := slices.Index([]string{"verb", "noun", "pronoun"}, pos); i >= 0 {
			return i
		}
		return 3
	}
	guess := func(a Analysis) int {
		if a.Guess() {
			return 1
		}
		return 0
	}
	c := cmp.Or(
		cmp.Compare(guess(x), guess(y)),
		// both guesses: more morphemes first; both known: 0 == 0
		cmp.Compare(len(y.Form.Morphemes)*guess(y), len(x.Form.Morphemes)*guess(x)),
		cmp.Compare(posRank(x.POS), posRank(y.POS)),
		slices.Compare(r.featRanks(x.Form.Feats), r.featRanks(y.Form.Feats)),
		cmp.Compare(x.Lemma, y.Lemma),
		cmp.Compare(x.Class, y.Class),
	)
	if c != 0 || x.Guess() {
		return c
	}
	return cmp.Compare(x.Entry.ID, y.Entry.ID)
}

// featRanks maps features to their position in the tables, so analyses
// sort like the paradigm: PRS before PST, 1SG before 2SG, SBJ before OBJ.
func (r *Rules) featRanks(feats []string) []int {
	order := append(append(append(r.tenseNames(), r.Verb.Persons...), Numbers...), Cases...)
	order = append(append(order, r.suffixNames()...), "INF")
	ranks := make([]int, len(feats))
	for i, f := range feats {
		ranks[i] = slices.Index(order, f)
	}
	return ranks
}
