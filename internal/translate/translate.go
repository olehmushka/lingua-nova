// Package translate turns simple English sentences into Rifatian.
//
// It's a transfer translator, the classic rule-based architecture (as in
// SYSTRAN): analyse the source into a tree, transform the tree into the
// target's structure, then generate the target words.
//
//  1. tag: english.Tagger gives every word its readings;
//  2. parse: an Earley parser builds every tree that grammar.cfg allows;
//  3. transfer: the tree becomes a clause (subject, verb, object, tense,
//     negation, question word), which is where English and Rifatian
//     differ: do-support and auxiliaries disappear, their tense moves onto
//     the verb, adjectives move after the noun, na goes before the verb;
//  4. generate: morph.Rules inflects each word, with the verb agreeing
//     with the subject and pronoun case coming from the position;
//  5. verify: the Rifatian glosser reads each output back (analysis by
//     synthesis, as in the morphology analyzer).
//
// English words are often ambiguous in Rifatian terms ("you" is four
// pronouns, "love" five verbs), so every combination is generated and
// ranked: trees that agree with the tagger first, then the first choice
// of each word (an exact gloss before a sense, informal before polite).
package translate

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/olehmushka/lingua-nova/internal/earley"
	"github.com/olehmushka/lingua-nova/internal/english"
	"github.com/olehmushka/lingua-nova/internal/gloss"
	"github.com/olehmushka/lingua-nova/internal/lexicon"
	"github.com/olehmushka/lingua-nova/internal/morph"
	"github.com/olehmushka/lingua-nova/internal/phon"
)

// Translator holds the English and Rifatian sides.
type Translator struct {
	Tagger  *english.Tagger
	Grammar *earley.Grammar
	Rules   *morph.Rules
	Glosser *gloss.Glosser
}

// MaxTrees and MaxCombos bound the work on very ambiguous input.
const (
	MaxTrees  = 50
	MaxCombos = 5000
)

// Parse is one tree of a sentence, scored by how many of its leaves have
// the category the statistical tagger chose.
type Parse struct {
	Tree  *earley.Tree
	Score int
}

// Parse tags a sentence and returns its trees, best first.
func (t *Translator) Parse(sentence string) (english.Sentence, []Parse) {
	s := t.Tagger.Tag(sentence)
	cats := make([][]string, len(s.Tokens))
	for i, tok := range s.Tokens {
		cats[i] = tok.Cats()
	}
	var out []Parse
	for _, tr := range t.Grammar.Parse(cats, MaxTrees) {
		p := Parse{Tree: tr}
		for _, l := range tr.Leaves() {
			if slices.ContainsFunc(s.Tokens[l.Start].Readings, func(r english.Reading) bool {
				return r.Cat == l.Label && r.Tagged
			}) {
				p.Score++
			}
		}
		out = append(out, p)
	}
	slices.SortStableFunc(out, func(a, b Parse) int { return b.Score - a.Score })
	return s, out
}

// Word is one word of a translation.
type Word struct {
	Text  string         // as it appears in the sentence: Naraxis
	Entry *lexicon.Entry // nil for an unknown English word (?dragon)
}

// Translation is one way to translate the sentence.
type Translation struct {
	Text    string
	Words   []Word
	Tree    *earley.Tree
	Gloss   *gloss.Result // nil if a word is unknown
	Notes   []string      // problems: unknown words, glosser notes
	Unknown []string      // English words with no lexicon entry
	rank    []int         // tree score (negated), choice sum, tree index
}

// Result is every translation of a sentence.
type Result struct {
	Input        english.Sentence
	Translations []Translation // best first, at most the max asked for
	Total        int           // distinct translations found
}

// Translate translates one sentence and keeps the best max translations
// (0 = all). An error means the sentence didn't parse.
func (t *Translator) Translate(sentence string, max int) (*Result, error) {
	s, parses := t.Parse(sentence)
	res := &Result{Input: s}
	if len(s.Tokens) == 0 {
		return res, fmt.Errorf("nothing to translate")
	}
	if len(parses) == 0 {
		return res, t.noParse(s)
	}
	var all []Translation
	for ti, p := range parses {
		leaves := p.Tree.Leaves()
		opts := make([][]choice, len(leaves))
		for i, l := range leaves {
			opts[i] = choices(s.Tokens[l.Start], l.Label)
		}
		combos(opts, MaxCombos, func(pick []choice, sum int) {
			tr, err := t.generate(s, p.Tree, pick)
			if err != nil {
				return // a combination morphology can't build (it has no plural)
			}
			tr.rank = []int{-p.Score, sum, ti}
			all = append(all, tr)
		})
	}
	slices.SortStableFunc(all, func(a, b Translation) int { return slices.Compare(a.rank, b.rank) })
	seen := map[string]bool{}
	for _, tr := range all {
		if seen[tr.Text] {
			continue
		}
		seen[tr.Text] = true
		res.Total++
		if max == 0 || len(res.Translations) < max {
			res.Translations = append(res.Translations, tr)
		}
	}
	if len(res.Translations) == 0 {
		return res, fmt.Errorf("no translation could be generated")
	}
	for i := range res.Translations {
		t.verify(&res.Translations[i])
	}
	return res, nil
}

// noParse explains a sentence the grammar rejects: the usual cause is a
// word whose category doesn't fit, so list them.
func (t *Translator) noParse(s english.Sentence) error {
	var ws []string
	for _, tok := range s.Tokens {
		cats := tok.Cats()
		if len(cats) == 0 {
			ws = append(ws, tok.Text+" (no category: "+tok.Tag+")")
		} else {
			ws = append(ws, tok.Text+"/"+strings.Join(cats, "/"))
		}
	}
	return fmt.Errorf("the grammar has no tree for: %s", strings.Join(ws, " "))
}

// choice is one reading of a leaf and one of its entries.
type choice struct {
	r english.Reading
	e *lexicon.Entry // nil: the reading has no entries
}

// choices are a leaf's alternatives, best first: the readings with the
// leaf's category, each with each of its entries.
func choices(tok english.Token, cat string) []choice {
	var out []choice
	for _, r := range tok.Readings {
		if r.Cat != cat {
			continue
		}
		if len(r.Entries) == 0 {
			out = append(out, choice{r, nil})
		}
		for _, e := range r.Entries {
			out = append(out, choice{r, e})
		}
	}
	return out
}

// combos calls f for each combination of one choice per slot (the
// cartesian product), at most limit times, with the sum of the choice
// indexes: 0 is everyone's first choice.
func combos(opts [][]choice, limit int, f func([]choice, int)) {
	idx := make([]int, len(opts))
	pick := make([]choice, len(opts))
	for n := 0; n < limit; n++ {
		sum := 0
		for i, j := range idx {
			pick[i] = opts[i][j]
			sum += j
		}
		f(slices.Clone(pick), sum)
		i := len(idx) - 1
		for ; i >= 0; i-- {
			if idx[i]++; idx[i] < len(opts[i]) {
				break
			}
			idx[i] = 0
		}
		if i < 0 {
			return
		}
	}
}

// verify glosses the translation and keeps the glosser's notes: a note
// means the generated sentence breaks a rule the glosser checks.
func (t *Translator) verify(tr *Translation) {
	for _, u := range tr.Unknown {
		tr.Notes = append(tr.Notes, fmt.Sprintf("%s: not in the lexicon", u))
	}
	if len(tr.Unknown) > 0 || t.Glosser == nil {
		return
	}
	tr.Gloss = t.Glosser.Gloss(tr.Text)
	tr.Notes = append(tr.Notes, tr.Gloss.Notes...)
}

// capitalize uppercases the first letter.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// New wires a translator from loaded data. The glosser verifies outputs;
// it learns the question words from words.yaml (WhP, WhAdv).
func New(g *earley.Grammar, w *english.Words, lex *lexicon.Lexicon, r *morph.Rules, a *morph.Analyzer) *Translator {
	gl := gloss.New(a)
	gl.Wh = map[string]bool{}
	for _, rs := range w.Words {
		for _, r := range rs {
			if r.Cat == "WhP" || r.Cat == "WhAdv" {
				for _, id := range r.IDs {
					gl.Wh[id] = true
				}
			}
		}
	}
	return &Translator{Tagger: english.New(w, lex), Grammar: g, Rules: r, Glosser: gl}
}

// Load reads everything a translator needs from a language directory:
// the Rifatian phonology, morphology and lexicon, and english/.
func Load(langDir string) (*Translator, error) {
	inv, err := phon.Load(filepath.Join(langDir, "phonology.yaml"))
	if err != nil {
		return nil, err
	}
	r, err := morph.Load(filepath.Join(langDir, "morphology.yaml"), inv)
	if err != nil {
		return nil, err
	}
	lex, err := lexicon.Load(filepath.Join(langDir, "lexicon"))
	if err != nil {
		return nil, err
	}
	g, w, err := LoadEnglish(langDir)
	if err != nil {
		return nil, err
	}
	return New(g, w, lex, r, morph.NewAnalyzer(r, lex)), nil
}

// LoadEnglish reads english/grammar.cfg and english/words.yaml.
func LoadEnglish(langDir string) (*earley.Grammar, *english.Words, error) {
	g, err := earley.Load(filepath.Join(langDir, "english", "grammar.cfg"))
	if err != nil {
		return nil, nil, err
	}
	w, err := english.LoadWords(filepath.Join(langDir, "english", "words.yaml"))
	if err != nil {
		return nil, nil, err
	}
	return g, w, nil
}

// Check reports words.yaml readings with unknown lexicon ids or with
// categories the grammar doesn't use.
func Check(g *earley.Grammar, w *english.Words, lex *lexicon.Lexicon) []lexicon.Problem {
	return w.Check(lex, g.Preterminals())
}
