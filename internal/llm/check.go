package llm

import (
	"fmt"
	"slices"
	"strings"

	"github.com/olehmushka/lingua-nova/internal/gloss"
	"github.com/olehmushka/lingua-nova/internal/morph"
)

// Check is the validator's verdict on one answer.
type Check struct {
	Gloss    *gloss.Result
	Problems []string // fail the answer; sent back to the model
	Remarks  []string // worth showing, but don't fail it
}

// OK reports whether the answer passed.
func (c *Check) OK() bool { return len(c.Problems) == 0 }

// Checker verifies answers.
type Checker struct {
	Glosser *gloss.Glosser
	Known   func(id string) bool // is id a dictionary entry? nil: every id is
}

// Verify glosses the answer's translation and compares the result with
// what the model claimed about each word.
//
// What fails an answer:
//   - a word that isn't spelled with Rifatian letters, has no analysis,
//     or only analyzes as an unknown root (the model made it up);
//   - a grammar note from the glosser (agreement, two verbs);
//   - a word the model says is a form of entry X, when the analyzer finds
//     it's a form of some other entry Y and never of X.
//
// A claimed id that isn't in the lexicon, or claimed features that don't
// match, are only remarks: the sentence can be right with its labels wrong.
// What this can't catch: a valid sentence that means something else.
// The glosser checks form, not meaning.
func (k *Checker) Verify(a Answer) *Check {
	c := &Check{}
	if strings.TrimSpace(a.Translation) == "" {
		c.Problems = append(c.Problems, "the translation is empty")
		return c
	}
	c.Gloss = k.Glosser.Gloss(a.Translation)
	used := make([]bool, len(a.Words))
	for _, w := range c.Gloss.Words {
		word := strings.ToLower(w.Text)
		t := w.Top()
		switch {
		case w.Err != nil:
			c.Problems = append(c.Problems, w.Err.Error())
			continue
		case t == nil:
			c.Problems = append(c.Problems, fmt.Sprintf("%s: not a form of any dictionary word", word))
			continue
		case t.Guess():
			c.Problems = append(c.Problems, fmt.Sprintf("%s: not in the dictionary (it looks like a form of an unknown %s)", word, t.Lemma))
			continue
		}
		i := slices.IndexFunc(a.Words, func(aw AnswerWord) bool { return strings.EqualFold(clean(aw.Text), word) })
		for i >= 0 && used[i] { // the same word twice: take the next claim
			j := slices.IndexFunc(a.Words[i+1:], func(aw AnswerWord) bool { return strings.EqualFold(clean(aw.Text), word) })
			if j < 0 {
				i = -1
				break
			}
			i += 1 + j
		}
		if i < 0 {
			c.Remarks = append(c.Remarks, fmt.Sprintf("%s: missing from the answer's word list", word))
			continue
		}
		used[i] = true
		k.claim(c, word, w.Readings, a.Words[i])
	}
	c.Problems = append(c.Problems, c.Gloss.Notes...)
	return c
}

// claim compares the model's claim about a word with its readings.
func (k *Checker) claim(c *Check, word string, rs []morph.Analysis, aw AnswerWord) {
	t := rs[0]
	match := slices.IndexFunc(rs, func(r morph.Analysis) bool { return r.Entry != nil && r.Entry.ID == aw.LexiconID })
	switch {
	case match < 0 && k.Known != nil && !k.Known(aw.LexiconID):
		c.Remarks = append(c.Remarks, fmt.Sprintf("%s: claimed id %q is not in the dictionary; it is a form of %q", word, aw.LexiconID, t.Entry.ID))
		return
	case match < 0:
		c.Problems = append(c.Problems, fmt.Sprintf("%s is a form of %q (%s), not of %q", word, t.Entry.ID, t.Lemma, aw.LexiconID))
		return
	}
	r := rs[match]
	if !sameFeats(r.Form.Feats, aw.Features) {
		c.Remarks = append(c.Remarks, fmt.Sprintf("%s: claimed %s, the analyzer says %s", word, label(aw.Features), label(r.Form.Feats)))
	}
}

func sameFeats(have, claimed []string) bool {
	if len(have) != len(claimed) {
		return false
	}
	for i := range have {
		if !strings.EqualFold(have[i], claimed[i]) {
			return false
		}
	}
	return true
}

func label(fs []string) string {
	if len(fs) == 0 {
		return "no features"
	}
	return strings.Join(fs, ".")
}

// clean drops punctuation around a claimed word ("nara." → "nara").
func clean(s string) string {
	return strings.TrimFunc(s, func(r rune) bool { return strings.ContainsRune(`.,?!;:—–…"«»'()`, r) || r == ' ' })
}

// Feedback is what the model is told about a failed answer: the gloss
// lines (so it sees how its sentence was read) and the problems.
func (c *Check) Feedback() string {
	var b strings.Builder
	if c.Gloss != nil {
		lines := strings.SplitN(c.Gloss.String(), "\n", 3)
		b.WriteString("How the analyzer reads your sentence (word, then gloss):\n")
		b.WriteString(lines[0] + "\n" + lines[1] + "\n\n")
	}
	b.WriteString("Problems:\n")
	for _, p := range c.Problems {
		b.WriteString("- " + p + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
