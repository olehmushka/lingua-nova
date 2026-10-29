// Package fixture builds a large synthetic lexicon for scale tests: the
// real entries plus many made-up ones, so retrieval can be measured at
// the size the dictionary is meant to reach (~300k entries) long before
// it gets there.
//
// The made-up entries are distractors, of three kinds:
//
//   - senses (Options.Senses): new senses of a real word's gloss head
//     (water.cold, see.far): the polysemy a big dictionary has, where
//     "water" has dozens of entries and the prompt can only hold a few;
//   - homonyms (Options.Homonyms): entries whose gloss is exactly a real
//     word's, with another meaning (water, the verb "to water");
//   - the rest: other concepts, with made-up English glosses (splices of
//     real words: watness), the way most of a big dictionary is words a
//     given sentence doesn't use.
//
// All definitions are built from the words of the real glosses and
// definitions, so every kind competes with the real entries in a search
// of definitions. The real entries stay the gold answers, so recall on
// the fixture says whether the right entry still gets through.
package fixture

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"unicode"

	"github.com/olehmushka/lingua-nova/internal/lexicon"
)

// Options says how many entries to make and how.
type Options struct {
	N        int     // synthetic entries
	Senses   float64 // share of them that are new senses of a real entry's gloss head
	Homonyms float64 // share that have a real entry's gloss head as their whole gloss
}

// Former returns a proto root and its modern form for a part of speech
// (verbs need an infinitive ending).
type Former func(pos string) (proto, modern string)

// IDPrefix starts every synthetic id.
const IDPrefix = "syn_"

// openPOS are the parts of speech a synthetic entry can have: the open
// classes, weighted roughly as in a big dictionary.
var openPOS = []weightedPOS{{"noun", 0.6}, {"verb", 0.25}, {"adjective", 0.15}}

type weightedPOS struct {
	pos    string
	weight float64
}

// stopwords are left out of the word pool: they say nothing about meaning.
var stopwords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "from": true, "that": true,
	"this": true, "into": true, "its": true, "not": true, "are": true, "was": true,
	"one": true, "any": true, "who": true, "which": true, "what": true, "has": true,
	"have": true, "been": true, "but": true, "out": true, "off": true, "use": true,
}

// Entries returns opt.N synthetic entries over the real ones. The same
// rng seed and inputs give the same entries.
func Entries(real []*lexicon.Entry, form Former, opt Options, rng *rand.Rand) []*lexicon.Entry {
	pool := Pool(real)
	var heads []*lexicon.Entry // real open-class entries
	for _, e := range real {
		if slices.ContainsFunc(openPOS, func(p weightedPOS) bool { return p.pos == e.POS }) {
			heads = append(heads, e)
		}
	}
	word := func(not ...string) string {
		for {
			w := pool[rng.IntN(len(pool))]
			if !slices.Contains(not, w) {
				return w
			}
		}
	}

	real1 := map[string]bool{} // real gloss heads and pool words: not for made-up glosses
	for _, w := range pool {
		real1[w] = true
	}
	for _, e := range real {
		h, _, _ := strings.Cut(strings.ToLower(e.Gloss), ".")
		real1[h] = true
	}
	madeUp := func() string {
		for {
			a, b := []rune(pool[rng.IntN(len(pool))]), []rune(pool[rng.IntN(len(pool))])
			w := string(a[:min(len(a), 2+rng.IntN(3))]) + string(b[max(0, len(b)-3-rng.IntN(2)):])
			if !real1[w] {
				return w
			}
		}
	}

	out := make([]*lexicon.Entry, 0, opt.N)
	for i := range opt.N {
		e := &lexicon.Entry{ID: fmt.Sprintf("%s%06d", IDPrefix, i+1)}
		x := rng.Float64()
		switch {
		case len(heads) > 0 && x < opt.Senses:
			r := heads[rng.IntN(len(heads))]
			head, _, _ := strings.Cut(r.Gloss, ".")
			mod := word(head)
			e.POS, e.Gloss = r.POS, head+"."+mod
			e.Definition = fmt.Sprintf("%s, as in %s %s.", capital(head), mod, word(head, mod))
		case len(heads) > 0 && x < opt.Senses+opt.Homonyms:
			r := heads[rng.IntN(len(heads))]
			head, _, _ := strings.Cut(r.Gloss, ".")
			e.POS, e.Gloss = pickPOS(rng), head
			e.Definition = define(e.POS, head, word(head), word(head))
		default:
			e.POS = pickPOS(rng)
			e.Gloss = madeUp()
			if rng.IntN(2) == 0 {
				e.Gloss += "." + word()
			}
			e.Definition = define(e.POS, word(), word(), word())
		}
		if e.POS == "noun" {
			e.Gender = []string{"m", "f"}[rng.IntN(2)]
		}
		e.Proto, e.Modern = form(e.POS)
		out = append(out, e)
	}
	return out
}

// define writes a definition in the style of the part of speech.
func define(pos, w1, w2, w3 string) string {
	switch pos {
	case "noun":
		return fmt.Sprintf("A %s %s of the %s.", w1, w2, w3)
	case "verb":
		return fmt.Sprintf("To %s with %s %s.", w1, w2, w3)
	}
	return fmt.Sprintf("%s and %s, like %s.", capital(w1), w2, w3)
}

// Pool is the English words of the real glosses and definitions: at least
// three letters, lowercase, no stopwords, sorted and unique.
func Pool(real []*lexicon.Entry) []string {
	seen := map[string]bool{}
	for _, e := range real {
		for _, w := range strings.FieldsFunc(e.Gloss+" "+e.Definition, func(r rune) bool { return !unicode.IsLetter(r) }) {
			w = strings.ToLower(w)
			if len(w) >= 3 && !stopwords[w] {
				seen[w] = true
			}
		}
	}
	pool := make([]string, 0, len(seen))
	for w := range seen {
		pool = append(pool, w)
	}
	slices.Sort(pool)
	return pool
}

func pickPOS(rng *rand.Rand) string {
	x := rng.Float64()
	for _, p := range openPOS {
		if x < p.weight {
			return p.pos
		}
		x -= p.weight
	}
	return openPOS[0].pos
}

func capital(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
