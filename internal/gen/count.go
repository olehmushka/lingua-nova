package gen

import (
	"fmt"
	"strings"

	"github.com/olehmushka/lingua-nova/internal/phon"
)

// Counts is how often each syllable count and each template per position
// occurs in a corpus: the raw material for the weights in phonology.yaml
// (proto: generator:). The maps use the same keys as that block.
type Counts struct {
	Syllables                    map[int]int
	Mono, Initial, Medial, Final map[string]int
}

// Count syllabifies every corpus word and counts its shapes. A geminate's
// first half is a coda (*pip.po: initial CVC, final CV), the same way the
// generator builds geminates.
func Count(inv *phon.Inventory, corpus [][]phon.Segment) (*Counts, error) {
	c := &Counts{
		Syllables: map[int]int{},
		Mono:      map[string]int{}, Initial: map[string]int{},
		Medial: map[string]int{}, Final: map[string]int{},
	}
	for _, word := range corpus {
		syls, err := inv.Syllabify(word)
		if err != nil {
			return nil, fmt.Errorf("corpus word %q: %v", key(phon.Spellings(word)), err)
		}
		c.Syllables[len(syls)]++
		for k, s := range syls {
			m := c.Medial
			switch {
			case len(syls) == 1:
				m = c.Mono
			case k == 0:
				m = c.Initial
			case k == len(syls)-1:
				m = c.Final
			}
			m[TemplateOf(s)]++
		}
	}
	return c, nil
}

// TemplateOf writes a syllable's shape: CCVC for *pri-, V for *-a.
func TemplateOf(s phon.Syllable) string {
	return strings.Repeat("C", len(s.Onset)) + strings.Repeat("V", len(s.Nucleus)) + strings.Repeat("C", len(s.Coda))
}
