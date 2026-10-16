package phon

import (
	"fmt"
	"maps"
	"slices"
)

// GeneratorSpec is the proto: generator: block of phonology.yaml: how many
// syllables a generated word has, and which syllable shapes appear in each
// position. Weights are relative; the slices are sorted by template so a
// seeded generator is reproducible.
type GeneratorSpec struct {
	Syllables []Weighted[int] // syllable count → weight, ascending count
	Mono      []Template      // the only syllable of a one-syllable word
	Initial   []Template
	Medial    []Template
	Final     []Template
}

// Weighted is a value with a relative weight.
type Weighted[T any] struct {
	Value  T
	Weight int
}

// Template is one syllable shape, like CCVC: the number of consonants in the
// onset (0–2), vowels in the nucleus (1 = simple, 2 = diphthong) and
// consonants in the coda (0–1).
type Template struct {
	Text                 string
	Onset, Nucleus, Coda int
	Weight               int
}

type generatorYAML struct {
	Syllables map[int]int    `yaml:"syllables"`
	Mono      map[string]int `yaml:"mono"`
	Initial   map[string]int `yaml:"initial"`
	Medial    map[string]int `yaml:"medial"`
	Final     map[string]int `yaml:"final"`
}

func parseGenerator(y *generatorYAML) (*GeneratorSpec, error) {
	g := &GeneratorSpec{}
	if len(y.Syllables) == 0 {
		return nil, fmt.Errorf("generator: syllables is empty")
	}
	needMono, needMulti, needMedial := false, false, false
	for _, n := range slices.Sorted(maps.Keys(y.Syllables)) {
		w := y.Syllables[n]
		if n < 1 {
			return nil, fmt.Errorf("generator syllables %d: want 1 or more", n)
		}
		if w <= 0 {
			return nil, fmt.Errorf("generator syllables %d: weight %d, want > 0", n, w)
		}
		g.Syllables = append(g.Syllables, Weighted[int]{n, w})
		needMono = needMono || n == 1
		needMulti = needMulti || n >= 2
		needMedial = needMedial || n >= 3
	}
	for _, p := range []struct {
		name string
		src  map[string]int
		dst  *[]Template
		need bool
	}{
		{"mono", y.Mono, &g.Mono, needMono},
		{"initial", y.Initial, &g.Initial, needMulti},
		{"medial", y.Medial, &g.Medial, needMedial},
		{"final", y.Final, &g.Final, needMulti},
	} {
		if p.need && len(p.src) == 0 {
			return nil, fmt.Errorf("generator %s: needed by the syllable counts, but empty", p.name)
		}
		for _, text := range slices.Sorted(maps.Keys(p.src)) {
			t, err := parseTemplate(text)
			if err != nil {
				return nil, fmt.Errorf("generator %s %q: %v", p.name, text, err)
			}
			if t.Weight = p.src[text]; t.Weight <= 0 {
				return nil, fmt.Errorf("generator %s %q: weight %d, want > 0", p.name, text, t.Weight)
			}
			*p.dst = append(*p.dst, t)
		}
	}
	return g, nil
}

// parseTemplate reads C{0,2} V{1,2} C{0,1}.
func parseTemplate(s string) (Template, error) {
	t := Template{Text: s}
	i := 0
	count := func(r byte, limit int) int {
		n := 0
		for i < len(s) && s[i] == r {
			n++
			i++
		}
		return min(n, limit+1)
	}
	t.Onset = count('C', 2)
	t.Nucleus = count('V', 2)
	t.Coda = count('C', 1)
	switch {
	case i < len(s):
		return t, fmt.Errorf("want C, CC or nothing, then V or VV, then C or nothing")
	case t.Onset > 2:
		return t, fmt.Errorf("onset of more than 2 consonants")
	case t.Nucleus == 0:
		return t, fmt.Errorf("no vowel")
	case t.Nucleus > 2:
		return t, fmt.Errorf("nucleus of more than 2 vowels")
	case t.Coda > 1:
		return t, fmt.Errorf("coda of more than 1 consonant")
	}
	return t, nil
}
