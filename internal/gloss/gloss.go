// Package gloss turns a Rifatian sentence into an interlinear gloss: one
// column per word, the word split into morphemes over their glosses.
//
//	Lu   mena   nar-a       lu   trubo.
//	the  woman  see-TH.3SG  the  river
//
// The analyzer gives each word every reading its form allows. Glossing
// then needs one, so a small Constraint Grammar (Karlsson 1990) removes
// readings that don't fit the sentence: rules look at the context (word
// order, agreement) and REMOVE readings, but never a word's last one.
// Whatever is left after the rules is ranked as the analyzer ranked it.
package gloss

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/olehmushka/lingua-nova/internal/morph"
)

// Word is one word of a sentence and what's left of its readings.
type Word struct {
	Lead  string // punctuation before the word: "— ", «
	Text  string // as written: Lu
	Punct string // punctuation after it: . ? ,

	Readings []morph.Analysis // kept by the rules, best first; the first is glossed
	Removed  []Removal        // ruled out by the context
	Err      error            // not spelled with Rifatian letters
}

// Removal is a reading a rule ruled out, and the rule's reason.
type Removal struct {
	Reading morph.Analysis
	Rule    string // OBJ before the verb
}

// Top is the reading the gloss shows, or nil if the word has none.
func (w Word) Top() *morph.Analysis {
	if len(w.Readings) == 0 {
		return nil
	}
	return &w.Readings[0]
}

// Segmented is the first gloss line's column: the word with morpheme
// breaks and its punctuation (Nar-a-x-is?).
func (w Word) Segmented() string { return w.Lead + w.Morphs() + w.Punct }

// Morphs is the word with morpheme breaks, capitalized as written: Nar-a-x-is.
func (w Word) Morphs() string {
	t := w.Top()
	if t == nil {
		return w.Text
	}
	s := t.Form.Segmented()
	if r, _ := utf8.DecodeRuneInString(w.Text); unicode.IsUpper(r) {
		first, n := utf8.DecodeRuneInString(s)
		s = string(unicode.ToUpper(first)) + s[n:]
	}
	return s
}

// Gloss is the second line's column: see-TH-PST-2SG, ?-TH-PST-1SG for an
// unknown root, ??? for a word with no reading.
func (w Word) Gloss() string {
	if t := w.Top(); t != nil {
		return t.Form.Gloss()
	}
	return "???"
}

// Result is a glossed sentence.
type Result struct {
	Words []Word
	Notes []string // grammar problems the rules found: agreement
}

// Glosser glosses sentences with an analyzer.
type Glosser struct{ a *morph.Analyzer }

func New(a *morph.Analyzer) *Glosser { return &Glosser{a} }

// Gloss analyzes every word, then disambiguates clause by clause.
func (g *Glosser) Gloss(sentence string) *Result {
	res := &Result{Words: Tokenize(sentence)}
	for i := range res.Words {
		w := &res.Words[i]
		w.Readings, w.Err = g.a.Analyze(w.Text)
	}
	ws := res.Words
	for start := 0; start < len(ws); {
		end := start + 1
		for end < len(ws) && !boundary(ws, end-1) {
			end++
		}
		res.Notes = append(res.Notes, g.clause(ws[start:end])...)
		start = end
	}
	return res
}

// clauseEnd is punctuation that ends a clause after a word.
const clauseEnd = ".,?!;:—–…"

// boundary reports whether a clause ends after ws[i]: Naraxis hono? — Naraxu.
func boundary(ws []Word, i int) bool {
	return strings.ContainsAny(ws[i].Punct, clauseEnd) ||
		i+1 < len(ws) && strings.ContainsAny(ws[i+1].Lead, clauseEnd)
}

// clause runs the rules on one clause. Rifatian clauses are SVO with one
// finite verb, and the verb anchors everything else:
//
//  1. the verb is the first word with a finite reading (known roots
//     before guesses); no other word in the clause is a finite verb;
//  2. a subject pronoun comes before the verb, an object pronoun after;
//  3. the subject agrees with the verb in person and number. The subject
//     is the nearest word before the verb, skipping negation and
//     adjectives; a clause can have none (pro-drop: Naraxu chi).
//
// A clause without a verb (Na.) is left as the analyzer read it.
func (g *Glosser) clause(ws []Word) (notes []string) {
	v := verb(ws)
	if v < 0 {
		return nil
	}
	// A rule that would remove a word's only reading is a grammar
	// problem in the sentence (or a gap in the grammar): note it.
	check := func(w *Word, rule string, match func(morph.Analysis) bool) {
		if !w.remove(rule, match) {
			notes = append(notes, strings.ToLower(w.Text)+" is "+rule)
		}
	}
	for i := range ws {
		if i == v {
			continue
		}
		check(&ws[i], "a second verb in the clause", func(an morph.Analysis) bool {
			_, ok := an.Finite()
			return ok
		})
		if i < v {
			check(&ws[i], "OBJ before the verb", hasCase("OBJ"))
		} else {
			check(&ws[i], "SBJ after the verb", hasCase("SBJ"))
		}
	}

	s := subject(g.a, ws[:v])
	if s < 0 {
		return notes
	}
	sw, vw := &ws[s], &ws[v]
	verbPersons, subjPersons := persons(vw.Readings, morph.Analysis.Finite), persons(sw.Readings, g.a.Agreement)
	sw.remove(fmt.Sprintf("the verb %s is %s", vw.Text, strings.Join(verbPersons, "/")), func(an morph.Analysis) bool {
		p, ok := g.a.Agreement(an)
		return ok && !slices.Contains(verbPersons, p)
	})
	vw.remove(fmt.Sprintf("the subject %s is %s", sw.Text, strings.Join(subjPersons, "/")), func(an morph.Analysis) bool {
		p, ok := an.Finite()
		return ok && !slices.Contains(subjPersons, p)
	})

	sp, sok := g.a.Agreement(*sw.Top())
	vp, vok := vw.Top().Finite()
	if sok && vok && sp != vp {
		notes = append(notes, fmt.Sprintf("%s (%s) doesn't agree with %s (%s)", strings.ToLower(sw.Text), sp, strings.ToLower(vw.Text), vp))
	}
	return notes
}

// verb finds the clause's verb, or -1.
func verb(ws []Word) int {
	for _, known := range []bool{true, false} {
		for i, w := range ws {
			if slices.ContainsFunc(w.Readings, func(an morph.Analysis) bool {
				_, ok := an.Finite()
				return ok && an.Guess() != known
			}) {
				return i
			}
		}
	}
	return -1
}

// subject finds the subject among the words before the verb, or -1.
func subject(a *morph.Analyzer, before []Word) int {
	for i := len(before) - 1; i >= 0; i-- {
		rs := before[i].Readings
		skip := len(rs) > 0 && !slices.ContainsFunc(rs, func(an morph.Analysis) bool {
			return an.POS != "particle" && an.POS != "adjective"
		})
		if skip {
			continue
		}
		if slices.ContainsFunc(rs, func(an morph.Analysis) bool { _, ok := a.Agreement(an); return ok }) {
			return i
		}
		return -1
	}
	return -1
}

// persons collects the persons readings agree in: 1SG, 3SG.
func persons(rs []morph.Analysis, person func(morph.Analysis) (string, bool)) []string {
	var out []string
	for _, an := range rs {
		if p, ok := person(an); ok && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

func hasCase(kase string) func(morph.Analysis) bool {
	return func(an morph.Analysis) bool {
		f := an.Form.Feats
		return an.POS == "pronoun" && len(f) == 2 && f[1] == kase
	}
}

// remove rules out the readings that match, unless that would leave none:
// a rule never removes a word's last reading. It reports false then.
func (w *Word) remove(rule string, match func(morph.Analysis) bool) bool {
	var keep []morph.Analysis
	var out []Removal
	for _, an := range w.Readings {
		if match(an) {
			out = append(out, Removal{an, rule})
		} else {
			keep = append(keep, an)
		}
	}
	if len(keep) == 0 && len(out) > 0 {
		return false
	}
	w.Readings, w.Removed = keep, append(w.Removed, out...)
	return true
}

// Remarks explain the gloss, one line each: removed and leftover readings,
// guesses, words without a reading, then the notes.
func (r *Result) Remarks() []string {
	var out []string
	for _, w := range r.Words {
		word := strings.ToLower(w.Text)
		switch t := w.Top(); {
		case w.Err != nil:
			out = append(out, w.Err.Error())
			continue
		case t == nil:
			out = append(out, word+": no analysis")
			continue
		case t.Guess():
			s := fmt.Sprintf("%s: root not in the lexicon, guessed ?%s", word, t.Lemma)
			if n := len(w.Readings) - 1; n > 0 {
				s += fmt.Sprintf(" (%d other guesses)", n)
			}
			out = append(out, s)
		default:
			for _, an := range w.Readings[1:] {
				if !an.Guess() {
					out = append(out, fmt.Sprintf("%s: also %s", word, an.Form.Gloss()))
				}
			}
		}
		for _, rm := range w.Removed {
			out = append(out, fmt.Sprintf("%s: removed %s (%s)", word, describe(rm.Reading), rm.Rule))
		}
	}
	return append(out, r.Notes...)
}

// describe names a reading in a remark: its gloss, or for a guess the
// made-up lemma and cell (?pumaxar PRS.1SG), since guesses gloss alike.
func describe(an morph.Analysis) string {
	if an.Guess() {
		return strings.TrimSpace("?" + an.Lemma + " " + an.Form.Label())
	}
	return an.Form.Gloss()
}

// String is the two aligned gloss lines, then the remarks.
func (r *Result) String() string {
	var top, bottom strings.Builder
	for i, w := range r.Words {
		seg, gl := w.Segmented(), w.Gloss()
		if i < len(r.Words)-1 {
			width := max(utf8.RuneCountInString(seg), utf8.RuneCountInString(gl)) + 2
			seg += strings.Repeat(" ", width-utf8.RuneCountInString(seg))
			gl += strings.Repeat(" ", width-utf8.RuneCountInString(gl))
		}
		top.WriteString(seg)
		bottom.WriteString(gl)
	}
	s := top.String() + "\n" + bottom.String() + "\n"
	if rs := r.Remarks(); len(rs) > 0 {
		s += "\n" + strings.Join(rs, "\n") + "\n"
	}
	return s
}

// Tokenize splits a sentence into words. Punctuation right after a word
// is its Punct; other punctuation leads the next word (— Naraxu).
func Tokenize(s string) []Word {
	var ws []Word
	var lead strings.Builder
	rs := []rune(s)
	for i := 0; i < len(rs); {
		switch r := rs[i]; {
		case unicode.IsLetter(r):
			j := i
			for j < len(rs) && unicode.IsLetter(rs[j]) {
				j++
			}
			k := j
			for k < len(rs) && !unicode.IsLetter(rs[k]) && !unicode.IsSpace(rs[k]) {
				k++
			}
			ws = append(ws, Word{Lead: lead.String(), Text: string(rs[i:j]), Punct: string(rs[j:k])})
			lead.Reset()
			i = k
		case unicode.IsSpace(r):
			if lead.Len() > 0 && !strings.HasSuffix(lead.String(), " ") {
				lead.WriteByte(' ')
			}
			i++
		default:
			lead.WriteRune(r)
			i++
		}
	}
	if rest := strings.TrimSpace(lead.String()); rest != "" && len(ws) > 0 {
		ws[len(ws)-1].Punct += " " + rest
	}
	return ws
}
