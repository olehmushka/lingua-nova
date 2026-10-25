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

	wh map[string]bool // the glosser's question words, for Clauses
}

// Glosser glosses sentences with an analyzer.
type Glosser struct {
	a *morph.Analyzer
	// Wh holds the entry ids of the question words (who, where…), which
	// Result.Clauses puts in their own slot. The lexicon doesn't mark
	// them, so the caller says which they are.
	Wh map[string]bool
}

func New(a *morph.Analyzer) *Glosser { return &Glosser{a: a} }

// Gloss analyzes every word, then disambiguates clause by clause.
func (g *Glosser) Gloss(sentence string) *Result {
	res := &Result{Words: Tokenize(sentence), wh: g.Wh}
	for i := range res.Words {
		w := &res.Words[i]
		w.Readings, w.Err = g.a.Analyze(w.Text)
	}
	complementizers(res.Words)
	for _, c := range clauses(res.Words) {
		res.Notes = append(res.Notes, g.clause(c)...)
	}
	return res
}

// complementizers decides each complementizer-or-not word (itu: COMP
// or "what") before the sentence is cut into clauses, because a
// complementizer is itself a clause boundary. After a finite verb in the
// same clause it's COMP and opens a subordinate clause (Mu sozu itu lu
// trubo iha grepo); anywhere else it isn't (Itu lu mena beuzexe?).
func complementizers(ws []Word) {
	isComp := func(an morph.Analysis) bool { return an.POS == "complementizer" }
	verb := false
	for i := range ws {
		w := &ws[i]
		if slices.ContainsFunc(w.Readings, isComp) {
			if verb {
				w.remove("a COMP after the verb", func(an morph.Analysis) bool { return !isComp(an) })
				verb = false
			} else {
				w.remove("no verb before it to be its COMP", isComp)
			}
		} else if slices.ContainsFunc(w.Readings, func(an morph.Analysis) bool { _, ok := an.Finite(); return ok }) {
			verb = true
		}
		if boundary(ws, i) {
			verb = false
		}
	}
}

// comp reports whether w was read as a complementizer.
func comp(w Word) bool {
	t := w.Top()
	return t != nil && t.POS == "complementizer"
}

// prep reports whether w was read as a preposition.
func prep(w Word) bool {
	t := w.Top()
	return t != nil && t.POS == "preposition"
}

// clauses splits words at clause boundaries; the clauses share ws's
// backing array, so the rules can change their readings in place.
func clauses(ws []Word) [][]Word {
	var out [][]Word
	for start := 0; start < len(ws); {
		end := start + 1
		for end < len(ws) && !boundary(ws, end-1) {
			end++
		}
		out = append(out, ws[start:end])
		start = end
	}
	return out
}

// clauseEnd is punctuation that ends a clause after a word.
const clauseEnd = ".,?!;:—–…"

// boundary reports whether a clause ends after ws[i]: Naraxis hono? — Naraxu.
// A complementizer starts a clause too: Mu sozu | itu lu trubo iha grepo.
func boundary(ws []Word, i int) bool {
	return strings.ContainsAny(ws[i].Punct, clauseEnd) ||
		i+1 < len(ws) && (strings.ContainsAny(ws[i+1].Lead, clauseEnd) || comp(ws[i+1]))
}

// clause runs the rules on one clause. Rifatian clauses are SVO with one
// finite verb, and the verb anchors everything else:
//
//  1. the verb is the first word with a finite reading (known roots
//     before guesses); no other word in the clause is a finite verb;
//  2. a subject pronoun comes before the verb, an object pronoun after;
//     after the copula ihar a pronoun is in the subject form too (Hunu
//     iha mu, "It is I", like Portuguese Sou eu); after a preposition
//     it is in the object form wherever it is (gu honi, "with him");
//  3. the subject agrees with the verb in person and number. The subject
//     is the nearest word before the verb, skipping negation and
//     adjectives; a clause can have none (pro-drop: Naraxu chi). A pronoun
//     without case forms (ifa who, itu what, zunhu nothing) agrees in 3SG,
//     as in English, but only when the verb has an object: otherwise it
//     can be a fronted object with the subject dropped (Ifa naraxis? =
//     whom did you see?).
//
// A subordinate clause's complementizer isn't part of it for these rules.
// A clause without a verb (Na.) is left as the analyzer read it.
func (g *Glosser) clause(ws []Word) (notes []string) {
	if len(ws) > 0 && comp(ws[0]) {
		if len(ws) == 1 {
			return []string{strings.ToLower(ws[0].Text) + " is a COMP with no clause after it"}
		}
		ws = ws[1:]
	}
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
	copula := slices.ContainsFunc(ws[v].Readings, func(an morph.Analysis) bool {
		_, ok := an.Finite()
		return ok && an.Entry != nil && an.Entry.ID == copulaID
	})
	for i := range ws {
		if i == v {
			continue
		}
		check(&ws[i], "a second verb in the clause", func(an morph.Analysis) bool {
			_, ok := an.Finite()
			return ok
		})
		switch {
		case i > 0 && prep(ws[i-1]):
			check(&ws[i], "SBJ after a preposition", hasCase("SBJ"))
		case i < v:
			check(&ws[i], "OBJ before the verb", hasCase("OBJ"))
		case copula:
			check(&ws[i], "OBJ after the copula", hasCase("OBJ"))
		default:
			check(&ws[i], "SBJ after the verb", hasCase("SBJ"))
		}
	}

	agree := g.a.Agreement
	if slices.ContainsFunc(ws[v+1:], isArgument) {
		agree = func(an morph.Analysis) (string, bool) {
			if bare(an) {
				return "3SG", true
			}
			return g.a.Agreement(an)
		}
	}
	s := subject(agree, ws[:v])
	if s < 0 {
		return notes
	}
	sw, vw := &ws[s], &ws[v]
	verbPersons, subjPersons := persons(vw.Readings, morph.Analysis.Finite), persons(sw.Readings, agree)
	sw.remove(fmt.Sprintf("the verb %s is %s", vw.Text, strings.Join(verbPersons, "/")), func(an morph.Analysis) bool {
		p, ok := agree(an)
		return ok && !slices.Contains(verbPersons, p)
	})
	vw.remove(fmt.Sprintf("the subject %s is %s", sw.Text, strings.Join(subjPersons, "/")), func(an morph.Analysis) bool {
		p, ok := an.Finite()
		return ok && !slices.Contains(subjPersons, p)
	})

	sp, sok := agree(*sw.Top())
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
// agree is the person a reading can be the subject of.
func subject(agree func(morph.Analysis) (string, bool), before []Word) int {
	for i := len(before) - 1; i >= 0; i-- {
		rs := before[i].Readings
		skip := len(rs) > 0 && !slices.ContainsFunc(rs, func(an morph.Analysis) bool {
			return an.POS != "particle" && an.POS != "adjective"
		})
		if skip {
			continue
		}
		if slices.ContainsFunc(rs, func(an morph.Analysis) bool { _, ok := agree(an); return ok }) {
			return i
		}
		return -1
	}
	return -1
}

// bare reports whether an is a pronoun without case forms: ifa, itu, zunhu.
func bare(an morph.Analysis) bool {
	return an.POS == "pronoun" && an.Entry != nil && len(an.Form.Feats) == 0
}

// isArgument reports whether w can be a verb's object: a noun or pronoun.
func isArgument(w Word) bool {
	return slices.ContainsFunc(w.Readings, func(an morph.Analysis) bool {
		return an.POS == "noun" || an.POS == "pronoun"
	})
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

// copulaID is the lexicon id of ihar "be", whose complement pronoun is
// in the subject form.
const copulaID = "be"

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
