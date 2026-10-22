package gloss

import "strings"

// Clause is a glossed clause cut into the slots of its predicate–argument
// structure: who did what to whom, when, negated or not. Two sentences
// with equal clauses say the same thing, whatever their words look like,
// so a translation can be checked against a reference by comparing
// clauses instead of strings.
//
// The slots follow Rifatian word order (grammar.md): question word
// first, then subject, na, the verb, and the object. A slot keeps its
// words in order, so `lu hoida gumo` and `gumo hoida` differ.
type Clause struct {
	Comp    *Tok  // the complementizer opening a subordinate clause: itu
	Wh      *Tok  // a clause-initial question word: Ifa
	Subject []Tok // the words before the verb, without Wh and Neg
	Neg     *Tok  // a particle right before the verb: na
	Verb    *Tok  // the finite verb; nil if the clause has none
	Object  []Tok // the words after the verb
}

// Tok is a word in a slot: what it is (entry and features), not how it's
// spelled.
type Tok struct {
	ID    string // lexicon entry; "" for a word without a known reading
	Feats string // PST.3SG, PL, SG.SBJ; "" when it doesn't inflect
	Gloss string // the gloss line's column: see-TH-PST-3SG
}

// Same reports whether two words are the same entry with the same
// features.
func (t Tok) Same(u Tok) bool { return t.ID == u.ID && t.Feats == u.Feats }

// Clauses cuts each clause of the result into its slots. A clause
// without a finite verb has all its words (after a question word) as
// Subject and a nil Verb.
func (r *Result) Clauses() []Clause {
	var out []Clause
	for _, ws := range clauses(r.Words) {
		var c Clause
		if len(ws) > 0 && comp(ws[0]) {
			t := tok(ws[0])
			c.Comp, ws = &t, ws[1:]
		}
		if len(ws) > 0 && r.wh[tok(ws[0]).ID] {
			t := tok(ws[0])
			c.Wh, ws = &t, ws[1:]
		}
		v := -1
		for i, w := range ws {
			if t := w.Top(); t != nil {
				if _, ok := t.Finite(); ok {
					v = i
					break
				}
			}
		}
		if v < 0 {
			c.Subject = toks(ws)
			out = append(out, c)
			continue
		}
		before := ws[:v]
		if n := len(before); n > 0 && particle(before[n-1]) {
			t := tok(before[n-1])
			c.Neg, before = &t, before[:n-1]
		}
		vt := tok(ws[v])
		c.Subject, c.Verb, c.Object = toks(before), &vt, toks(ws[v+1:])
		out = append(out, c)
	}
	return out
}

func tok(w Word) Tok {
	t := w.Top()
	if t == nil || t.Entry == nil {
		return Tok{Gloss: w.Gloss()}
	}
	return Tok{ID: t.Entry.ID, Feats: strings.Join(t.Form.Feats, "."), Gloss: w.Gloss()}
}

func toks(ws []Word) []Tok {
	out := make([]Tok, len(ws))
	for i, w := range ws {
		out[i] = tok(w)
	}
	return out
}

func particle(w Word) bool {
	t := w.Top()
	return t != nil && t.POS == "particle"
}
