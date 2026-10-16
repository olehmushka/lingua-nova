package phon

import (
	"fmt"
	"slices"
	"strings"
)

// Violation is one way a word breaks the phonotactics.
type Violation struct {
	Msg string
}

func (v Violation) Error() string  { return v.Msg }
func (v Violation) String() string { return v.Msg }

// Matches reports whether a pattern item (a segment spelling or a category
// name) matches the segment spelled spell.
func (inv *Inventory) Matches(item, spell string) bool {
	if set, ok := inv.catSet[item]; ok {
		return set[spell]
	}
	return item == spell
}

// CheckForbidden returns a violation for every place where a forbidden sequence
// occurs. The scan ignores syllable boundaries: l·h is forbidden in al.ha
// as much as inside a syllable.
func (inv *Inventory) CheckForbidden(segs []Segment) []Violation {
	var out []Violation
	for _, pat := range inv.Forbidden {
		for i := 0; i+len(pat) <= len(segs); i++ {
			ok := true
			for j, item := range pat {
				if !inv.Matches(item, segs[i+j].Spell) {
					ok = false
					break
				}
			}
			if ok {
				got := strings.Join(Spellings(segs[i:i+len(pat)]), " ")
				out = append(out, Violation{fmt.Sprintf("forbidden sequence [%s] (matches %v)", got, pat)})
			}
		}
	}
	return out
}

// CheckPhonotactics returns every way the word breaks the modern
// phonotactics. The shape is (C)(C)V(C):
//
//   - an onset is one consonant or a listed cluster;
//   - a coda is at most one consonant, from codas (inside the word) or
//     finals (at the end);
//   - no double consonants: a coda that repeats the next onset (as.sa);
//   - no forbidden sequence occurs anywhere.
//
// Onsets and codas are checked per syllable, because "is this consonant
// allowed here?" depends on its position. Forbidden sequences are checked on
// the flat segment list, because they cross syllable boundaries.
func (inv *Inventory) CheckPhonotactics(segs []Segment) []Violation {
	return inv.check(segs, rules{codas: inv.Codas, finals: inv.Finals})
}

// CheckProtoPhonotactics is CheckPhonotactics with the proto-language codas
// and finals, plus geminates: a double consonant is legal when it is in
// Geminates (*cup.pe), even if it isn't a legal coda. Since a coda
// follows a nucleus and a one-consonant onset precedes one, that means
// "only between vowels". Hiatus (a syllable starting with a vowel right
// after a vowel) is legal only after a vowel in HiatusAfter (*u.si.a, not
// *a.ar). It doesn't check for modern-only segments.
func (inv *Inventory) CheckProtoPhonotactics(segs []Segment) []Violation {
	return inv.check(segs, rules{
		codas:       inv.ProtoCodas,
		finals:      inv.ProtoFinals,
		geminates:   inv.Geminates,
		hiatusAfter: inv.HiatusAfter,
	})
}

// rules are the sets that differ between proto and modern phonotactics.
type rules struct {
	codas, finals, geminates []string
	hiatusAfter              []string // nil = hiatus after any vowel
}

func (inv *Inventory) check(segs []Segment, r rules) []Violation {
	syls, err := inv.Syllabify(segs)
	if err != nil {
		return []Violation{{err.Error()}}
	}
	word := SyllableSpelling(syls)
	var out []Violation
	bad := func(format string, args ...any) {
		out = append(out, Violation{fmt.Sprintf(format, args...) + " in " + word})
	}
	for k, s := range syls {
		// A diphthong before the hiatus is a V V V run, reported by forbidden.
		if r.hiatusAfter != nil && k > 0 && len(s.Onset) == 0 {
			prev := syls[k-1]
			if len(prev.Coda) == 0 && len(prev.Nucleus) == 1 && !slices.Contains(r.hiatusAfter, prev.Nucleus[0].Spell) {
				bad("hiatus %q (only after %s)", prev.Nucleus[0].Spell+"."+s.Nucleus[0].Spell, strings.Join(r.hiatusAfter, " "))
			}
		}
		on := Spellings(s.Onset)
		if len(on) > 1 && !inv.onsets[seqKey(on)] {
			bad("illegal onset %q", strings.Join(on, ""))
		}
		co := Spellings(s.Coda)
		last := k == len(syls)-1
		allowed, what := r.codas, "coda"
		if last {
			allowed, what = r.finals, "word-final consonant"
		}
		double := len(co) == 1 && !last && slices.Equal(Spellings(syls[k+1].Onset), co)
		switch {
		case len(co) > 1:
			bad("%s %q has more than one consonant", what, strings.Join(co, ""))
		case double && slices.Contains(r.geminates, co[0]):
		case double:
			bad("double consonant %q", co[0]+co[0])
		case len(co) == 1 && !slices.Contains(allowed, co[0]):
			bad("illegal %s %q", what, co[0])
		}
	}
	return append(out, inv.CheckForbidden(segs)...)
}
