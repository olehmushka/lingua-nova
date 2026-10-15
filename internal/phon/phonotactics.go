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
//   - no forbidden sequence occurs anywhere.
//
// Onsets and codas are checked per syllable, because "is this consonant
// allowed here?" depends on its position. Forbidden sequences are checked on
// the flat segment list, because they cross syllable boundaries.
func (inv *Inventory) CheckPhonotactics(segs []Segment) []Violation {
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
		on := Spellings(s.Onset)
		if len(on) > 1 && !inv.onsets[seqKey(on)] {
			bad("illegal onset %q", strings.Join(on, ""))
		}
		co := Spellings(s.Coda)
		allowed, what := inv.Codas, "coda"
		if k == len(syls)-1 {
			allowed, what = inv.Finals, "word-final consonant"
		}
		switch {
		case len(co) > 1:
			bad("%s %q has more than one consonant", what, strings.Join(co, ""))
		case len(co) == 1 && !slices.Contains(allowed, co[0]):
			bad("illegal %s %q", what, co[0])
		}
	}
	return append(out, inv.CheckForbidden(segs)...)
}
