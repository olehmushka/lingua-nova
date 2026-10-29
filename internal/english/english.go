// Package english is the source-language side of the translator: it
// splits an English sentence into tokens and gives each token its
// possible readings (category, lemma, number or tense, and the Rifatian
// entries it can translate to).
//
// Categories come from two places:
//
//   - words.yaml, for function words and archaic forms (the, her, didst).
//     These are a closed class, so a table is exact;
//   - for everything else, the prose part-of-speech tagger: an averaged
//     perceptron trained on the Penn Treebank (Wall Street Journal text).
//     It scores each tag from features of the word and its neighbours.
//     It has never seen "thou" or "didst", which is why the table wins.
//
// An open-class word also gets a reading for every category under which
// its lemma is in the lexicon, so a tagger mistake ("fish" as a verb)
// doesn't block the parse. The tagger's choice is kept as Reading.Tagged,
// and the translator ranks parses by how many of their leaves agree with it.
package english

import (
	"slices"
	"strings"

	"github.com/jdkato/prose/tag"
	"github.com/jdkato/prose/tokenize"

	"github.com/olehmushka/lingua-nova/internal/lexicon"
)

// Token is one word of a sentence with its possible readings.
type Token struct {
	Text     string // as written: Didst
	Tag      string // the tagger's Penn Treebank tag: NNP
	Readings []Reading
}

// Cats lists the token's categories, for the parser.
func (t Token) Cats() []string {
	var out []string
	for _, r := range t.Readings {
		if !slices.Contains(out, r.Cat) {
			out = append(out, r.Cat)
		}
	}
	return out
}

// Sentence is a tagged sentence without its punctuation.
type Sentence struct {
	Tokens []Token
	End    string // final punctuation: . ? ! or ""
}

// Words returns the tokens' text, for printing trees.
func (s Sentence) Words() []string {
	out := make([]string, len(s.Tokens))
	for i, t := range s.Tokens {
		out[i] = t.Text
	}
	return out
}

// Tagger tags English sentences.
type Tagger struct {
	words  *Words
	byID   map[string]*lexicon.Entry
	byHead map[string][]*lexicon.Entry // POS + " " + gloss head → entries
	tok    *tokenize.TreebankWordTokenizer
	tag    *tag.PerceptronTagger
}

// openCats maps the categories of open-class words to lexicon parts of
// speech, in the order readings are tried.
var openCats = []struct{ cat, pos string }{
	{"N", "noun"}, {"V", "verb"}, {"Adj", "adjective"},
}

// New indexes the lexicon by gloss and loads the tagger's model.
func New(w *Words, lex *lexicon.Lexicon) *Tagger {
	t := &Tagger{
		words:  w,
		byID:   map[string]*lexicon.Entry{},
		byHead: map[string][]*lexicon.Entry{},
		tok:    tokenize.NewTreebankWordTokenizer(),
		tag:    tag.NewPerceptronTagger(),
	}
	for _, e := range lex.Entries {
		t.byID[e.ID] = e
		k := e.POS + " " + head(e.Gloss)
		t.byHead[k] = append(t.byHead[k], e)
	}
	// An exact gloss (kill) ranks before a sense (kill.by.accident).
	for k, es := range t.byHead {
		_, g, _ := strings.Cut(k, " ")
		slices.SortStableFunc(es, func(a, b *lexicon.Entry) int {
			return exact(a, g) - exact(b, g)
		})
	}
	return t
}

// head is the part of a gloss before the first dot, lowercased:
// love.eros → love, God → god.
func head(gloss string) string {
	h, _, _ := strings.Cut(gloss, ".")
	return strings.ToLower(h)
}

func exact(e *lexicon.Entry, g string) int {
	if strings.EqualFold(e.Gloss, g) {
		return 0
	}
	return 1
}

// Lookup returns the entries whose gloss (or its head, before the first
// dot) is lemma, with part of speech pos, best first.
func (t *Tagger) Lookup(pos, lemma string) []*lexicon.Entry {
	return t.byHead[pos+" "+strings.ToLower(lemma)]
}

// Tag tokenizes and tags one sentence.
func (t *Tagger) Tag(sentence string) Sentence {
	var s Sentence
	toks := t.tok.Tokenize(sentence)
	for _, tt := range t.tag.Tag(toks) {
		switch tt.Tag {
		case ".":
			s.End = tt.Text
			continue
		case ",", ":", "``", "''", "(", ")", "#", "$":
			continue
		}
		s.Tokens = append(s.Tokens, Token{Text: tt.Text, Tag: tt.Tag, Readings: t.readings(tt.Text, tt.Tag)})
	}
	return s
}

// tagCat is the category a Penn Treebank tag stands for, if it's an open
// class.
func tagCat(ptb string) string {
	switch {
	case strings.HasPrefix(ptb, "NN"):
		return "N"
	case strings.HasPrefix(ptb, "VB"):
		return "V"
	case strings.HasPrefix(ptb, "JJ"):
		return "Adj"
	}
	return ""
}

func (t *Tagger) readings(word, ptb string) []Reading {
	lower := strings.ToLower(word)
	if rs, ok := t.words.Words[lower]; ok {
		out := make([]Reading, len(rs))
		for i, r := range rs {
			r.Lemma = lower
			r.Tagged = true
			for _, id := range r.IDs {
				if e := t.byID[id]; e != nil {
					r.Entries = append(r.Entries, e)
				}
			}
			out[i] = r
		}
		return out
	}

	want := tagCat(ptb)
	var out []Reading
	for _, oc := range openCats {
		for _, c := range t.lemmas(oc.cat, lower) {
			if es := t.Lookup(oc.pos, c.lemma); len(es) > 0 {
				out = append(out, t.open(oc.cat, c, ptb, es))
				break
			}
		}
	}
	if !slices.ContainsFunc(out, func(r Reading) bool { return r.Cat == want }) && want != "" {
		// Unknown word: keep the tagger's category so the sentence still
		// parses, and translate it as ?word.
		out = append(out, t.open(want, t.lemmas(want, lower)[0], ptb, nil))
	}
	// The tagger's category first.
	slices.SortStableFunc(out, func(a, b Reading) int { return b.tagged() - a.tagged() })
	return out
}

func (r Reading) tagged() int {
	if r.Tagged {
		return 1
	}
	return 0
}

func (t *Tagger) open(cat string, c candidate, ptb string, es []*lexicon.Entry) Reading {
	r := Reading{Cat: cat, Lemma: c.lemma, Entries: es, Tagged: tagCat(ptb) == cat}
	switch cat {
	case "N":
		r.Num = "SG"
		if c.inflected || ptb == "NNS" || ptb == "NNPS" {
			r.Num = "PL"
		}
	case "V":
		r.Tense = "PRS"
		if c.inflected || ptb == "VBD" { // cut, put: the tagger tells
			r.Tense = "PST"
		}
	}
	return r
}

// Listed says whether words.yaml lists the word: its readings are then
// complete as given (function words, irregular forms), not found by gloss.
func (t *Tagger) Listed(word string) bool {
	_, ok := t.words.Words[strings.ToLower(word)]
	return ok
}

// Entry returns the lexicon entry with an id, or nil.
func (t *Tagger) Entry(id string) *lexicon.Entry { return t.byID[id] }
