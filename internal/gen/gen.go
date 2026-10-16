// Package gen generates random proto-language roots.
//
// A word is built top-down: a syllable count, then a syllable template per
// position (CV, CVV, CCVC, ...), then a sound for each slot of each
// template. Counts and templates come from phonology.yaml (proto: generator:);
// which sound fills a slot is weighted by how often it fills that slot in
// the lexicon.
//
// The generator only knows syllable shapes, not the rest of the
// phonotactics: a word can still contain a forbidden sequence across a
// syllable boundary (CVV + V gives three vowels in a row, *hau.a; CVC + CV
// can give *l.h, *bil.ha). Callers filter candidates
// with phon.CheckProtoPhonotactics, which is the validator from lnova
// validate.
package gen

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"

	"github.com/olehmushka/lingua-nova/internal/phon"
)

// pool is the weighted choice for one kind of slot. Each option is a
// sequence of segments: one consonant, an onset cluster, a diphthong.
type pool []phon.Weighted[[]phon.Segment]

// Generator makes random proto roots. It is not safe for concurrent use.
type Generator struct {
	inv  *phon.Inventory
	spec *phon.GeneratorSpec
	rng  *rand.Rand

	onset1, onset2 pool // single consonant, cluster
	nuc1, nuc2     pool // vowel, diphthong
	coda, final    pool // inside the word (incl. geminate halves), word-final
	geminate       map[string]bool
}

// New builds a generator for the inventory's generator spec. The corpus
// (normally every lexicon proto form) sets the sound weights: each legal
// option gets 1 plus the number of times it fills that slot in the corpus,
// so sounds the corpus never uses in a slot are rare but possible.
func New(inv *phon.Inventory, corpus [][]phon.Segment, rng *rand.Rand) (*Generator, error) {
	if inv.Generator == nil {
		return nil, errors.New("phonology has no proto: generator: block")
	}
	g := &Generator{inv: inv, spec: inv.Generator, rng: rng, geminate: map[string]bool{}}

	var cons, vows [][]string
	for _, s := range inv.Segments {
		if !s.Proto {
			continue
		}
		if s.Kind == phon.Consonant {
			cons = append(cons, []string{s.Spell})
		} else {
			vows = append(vows, []string{s.Spell})
		}
	}
	codas := single(inv.ProtoCodas)
	for _, c := range inv.Geminates {
		g.geminate[c] = true
		if !slices.Contains(inv.ProtoCodas, c) {
			codas = append(codas, []string{c})
		}
	}

	counts := map[string]map[string]int{}
	count := func(slot string, segs []phon.Segment) {
		if counts[slot] == nil {
			counts[slot] = map[string]int{}
		}
		counts[slot][key(phon.Spellings(segs))]++
	}
	for _, word := range corpus {
		syls, err := inv.Syllabify(word)
		if err != nil {
			return nil, fmt.Errorf("corpus word %q: %v", key(phon.Spellings(word)), err)
		}
		for k, s := range syls {
			switch len(s.Onset) {
			case 1:
				count("onset1", s.Onset)
			case 2:
				count("onset2", s.Onset)
			}
			if len(s.Nucleus) == 1 {
				count("nuc1", s.Nucleus)
			} else {
				count("nuc2", s.Nucleus)
			}
			if len(s.Coda) == 1 {
				if k == len(syls)-1 {
					count("final", s.Coda)
				} else {
					count("coda", s.Coda)
				}
			}
		}
	}

	var err error
	build := func(slot string, options [][]string) pool {
		var p pool
		for _, o := range options {
			segs, e := lookup(inv, o)
			if e != nil {
				err = e
			}
			p = append(p, phon.Weighted[[]phon.Segment]{Value: segs, Weight: 1 + counts[slot][key(o)]})
		}
		return p
	}
	g.onset1 = build("onset1", cons)
	g.onset2 = build("onset2", inv.Onsets)
	g.nuc1 = build("nuc1", vows)
	g.nuc2 = build("nuc2", inv.Diphthongs)
	g.coda = build("coda", codas)
	g.final = build("final", single(inv.ProtoFinals))
	if err != nil {
		return nil, err
	}
	return g, g.checkPools()
}

// checkPools reports a template slot that has nothing to fill it, like a
// CVC template in a language with no codas.
func (g *Generator) checkPools() error {
	need := func(name string, p pool, ts []phon.Template, used func(phon.Template) bool) error {
		if len(p) > 0 {
			return nil
		}
		for _, t := range ts {
			if used(t) {
				return fmt.Errorf("generator template %s needs %s, but the phonology has none", t.Text, name)
			}
		}
		return nil
	}
	s := g.spec
	all := slices.Concat(s.Mono, s.Initial, s.Medial, s.Final)
	for _, c := range []struct {
		name string
		p    pool
		ts   []phon.Template
		used func(phon.Template) bool
	}{
		{"onset clusters", g.onset2, all, func(t phon.Template) bool { return t.Onset == 2 }},
		{"diphthongs", g.nuc2, all, func(t phon.Template) bool { return t.Nucleus == 2 }},
		{"proto codas", g.coda, slices.Concat(s.Initial, s.Medial), func(t phon.Template) bool { return t.Coda == 1 }},
		{"proto finals", g.final, slices.Concat(s.Mono, s.Final), func(t phon.Template) bool { return t.Coda == 1 }},
	} {
		if err := need(c.name, c.p, c.ts, c.used); err != nil {
			return err
		}
	}
	return nil
}

// Word returns one random candidate root. It follows the templates but may
// still break the phonotactics; see the package comment.
func (g *Generator) Word() []phon.Segment {
	n := pick(g.rng, g.spec.Syllables)
	var word []phon.Segment
	var forced []phon.Segment // onset fixed by a geminate in the previous coda
	for k := range n {
		last := k == n-1
		ts := g.spec.Medial
		switch {
		case n == 1:
			ts = g.spec.Mono
		case k == 0:
			ts = g.spec.Initial
		case last:
			ts = g.spec.Final
		}
		if forced != nil {
			ts = withOnset1(ts)
		}
		t := pickTemplate(g.rng, ts)

		switch {
		case forced != nil:
			word = append(word, forced...)
			forced = nil
		case t.Onset == 1:
			word = append(word, pick(g.rng, g.onset1)...)
		case t.Onset == 2:
			word = append(word, pick(g.rng, g.onset2)...)
		}
		if t.Nucleus == 1 {
			word = append(word, pick(g.rng, g.nuc1)...)
		} else {
			word = append(word, pick(g.rng, g.nuc2)...)
		}
		if t.Coda == 0 {
			continue
		}
		if last {
			word = append(word, pick(g.rng, g.final)...)
			continue
		}
		codas := g.coda
		next := g.spec.Medial
		if k+1 == n-1 {
			next = g.spec.Final
		}
		if len(withOnset1(next)) == 0 {
			// No next template can start with the geminate's second half.
			codas = slices.DeleteFunc(slices.Clone(codas), func(o phon.Weighted[[]phon.Segment]) bool {
				return g.geminate[o.Value[0].Spell]
			})
		}
		c := pick(g.rng, codas)
		word = append(word, c...)
		if g.geminate[c[0].Spell] {
			forced = c
		}
	}
	return word
}

// withOnset1 returns the templates whose onset is one consonant: the only
// ones that can follow a geminate's first half (*cup.pe).
func withOnset1(ts []phon.Template) []phon.Template {
	return slices.DeleteFunc(slices.Clone(ts), func(t phon.Template) bool { return t.Onset != 1 })
}

func pickTemplate(rng *rand.Rand, ts []phon.Template) phon.Template {
	ws := make([]phon.Weighted[phon.Template], len(ts))
	for i, t := range ts {
		ws[i] = phon.Weighted[phon.Template]{Value: t, Weight: t.Weight}
	}
	return pick(rng, ws)
}

// pick returns a random value with probability weight / total weight.
func pick[T any](rng *rand.Rand, ws []phon.Weighted[T]) T {
	total := 0
	for _, w := range ws {
		total += w.Weight
	}
	r := rng.IntN(total)
	for _, w := range ws {
		if r < w.Weight {
			return w.Value
		}
		r -= w.Weight
	}
	panic("unreachable")
}

func single(spells []string) [][]string {
	out := make([][]string, len(spells))
	for i, s := range spells {
		out[i] = []string{s}
	}
	return out
}

func lookup(inv *phon.Inventory, spells []string) ([]phon.Segment, error) {
	segs := make([]phon.Segment, len(spells))
	for i, sp := range spells {
		seg, ok := inv.Lookup(sp)
		if !ok {
			return nil, fmt.Errorf("unknown segment %q", sp)
		}
		segs[i] = seg
	}
	return segs, nil
}

func key(spells []string) string { return fmt.Sprint(spells) }
