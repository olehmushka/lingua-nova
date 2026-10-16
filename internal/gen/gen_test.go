package gen

import (
	"maps"
	"math/rand/v2"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/olehmushka/lingua-nova/internal/lexicon"
	"github.com/olehmushka/lingua-nova/internal/phon"
)

const langDir = "../../language"

func newReal(t *testing.T, seed uint64) (*Generator, *phon.Inventory) {
	t.Helper()
	inv, err := phon.Load(filepath.Join(langDir, "phonology.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	lex, err := lexicon.Load(filepath.Join(langDir, "lexicon"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus [][]phon.Segment
	for _, e := range lex.Entries {
		segs, err := inv.Tokenize(strings.TrimPrefix(e.Proto, "*"))
		if err != nil {
			t.Fatal(err)
		}
		corpus = append(corpus, segs)
	}
	g, err := New(inv, corpus, rand.New(rand.NewPCG(seed, 0)))
	if err != nil {
		t.Fatal(err)
	}
	return g, inv
}

func spell(segs []phon.Segment) string { return strings.Join(phon.Spellings(segs), "") }

func TestSeedIsReproducible(t *testing.T) {
	a, _ := newReal(t, 42)
	b, _ := newReal(t, 42)
	c, _ := newReal(t, 43)
	same, differ := true, false
	for range 50 {
		wa, wb, wc := spell(a.Word()), spell(b.Word()), spell(c.Word())
		same = same && wa == wb
		differ = differ || wa != wc
	}
	if !same {
		t.Error("same seed gave different words")
	}
	if !differ {
		t.Error("different seeds gave the same 50 words")
	}
}

// TestDistribution generates many words and checks that they follow the
// spec: every syllable count appears in roughly its share, geminates stand
// only between vowels, and most candidates pass the proto phonotactics
// (a low pass rate means a slot pool or template is broken). Other double
// consonants (a coda l before an onset l: *al.la) do occur; the validator
// rejects them.
func TestDistribution(t *testing.T) {
	g, inv := newReal(t, 1)
	const total = 5000
	syllables := map[int]int{}
	legal := 0
	for range total {
		w := g.Word()
		syls, err := inv.Syllabify(w)
		if err != nil {
			t.Fatalf("%s: %v", spell(w), err)
		}
		syllables[len(syls)]++
		if len(inv.CheckProtoPhonotactics(w)) == 0 {
			legal++
		}
		for i := 1; i < len(w); i++ {
			if w[i].Spell != w[i-1].Spell || !slices.Contains(inv.Geminates, w[i].Spell) {
				continue
			}
			ok := i >= 2 && w[i-2].Kind == phon.Vowel &&
				i+1 < len(w) && w[i+1].Kind == phon.Vowel
			if !ok {
				t.Errorf("geminate not between vowels in %s", spell(w))
			}
		}
	}

	// Syllabify can merge a template-V with a neighbouring vowel into a
	// diphthong (CV + V: *ta.i → *tai), so allow some drift.
	weights := 0
	for _, s := range inv.Generator.Syllables {
		weights += s.Weight
	}
	for _, s := range inv.Generator.Syllables {
		want := float64(s.Weight) / float64(weights)
		got := float64(syllables[s.Value]) / total
		if syllables[s.Value] == 0 || got < want*0.7 || got > want*1.3 {
			t.Errorf("%d syllables: share %.3f, want about %.3f", s.Value, got, want)
		}
	}
	if rate := float64(legal) / total; rate < 0.8 {
		t.Errorf("only %.0f%% of candidates pass the proto phonotactics", rate*100)
	}
}

func TestNewErrors(t *testing.T) {
	const base = `
consonants:
  - {spell: p, ipa: p}
vowels:
  - {spell: a, ipa: a}
`
	tests := []struct {
		name  string
		extra string
		want  string
	}{
		{"no generator", "", "no proto: generator: block"},
		{"no clusters", "proto: {generator: {syllables: {1: 1}, mono: {CCV: 1}}}\n", "CCV needs onset clusters"},
		{"no diphthongs", "proto: {generator: {syllables: {1: 1}, mono: {CVV: 1}}}\n", "CVV needs diphthongs"},
		{"no finals", "proto: {generator: {syllables: {1: 1}, mono: {CVC: 1}}}\n", "CVC needs proto finals"},
		{"no codas", "proto: {finals: [p], generator: {syllables: {2: 1}, initial: {CVC: 1}, final: {CV: 1}}}\n", "CVC needs proto codas"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv, err := phon.Parse([]byte(base + tt.extra))
			if err != nil {
				t.Fatal(err)
			}
			_, err = New(inv, nil, rand.New(rand.NewPCG(1, 0)))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

// TestGeminateNeedsOnset: when no next template has a one-consonant onset,
// a geminate can't be completed, so the generator must not pick one.
func TestGeminateNeedsOnset(t *testing.T) {
	inv, err := phon.Parse([]byte(`
consonants:
  - {spell: p, ipa: p}
  - {spell: r, ipa: ɾ}
vowels:
  - {spell: a, ipa: a}
onsets: [[p, r]]
proto:
  codas: [r]
  geminates: [p]
  generator: {syllables: {2: 1}, initial: {CVC: 1}, final: {CCV: 1, V: 1}}
`))
	if err != nil {
		t.Fatal(err)
	}
	g, err := New(inv, nil, rand.New(rand.NewPCG(1, 0)))
	if err != nil {
		t.Fatal(err)
	}
	for range 200 {
		if w := spell(g.Word()); strings.Contains(w, "pp") {
			t.Fatalf("generated %s: geminate with no one-consonant onset to follow", w)
		}
	}
}

func TestCount(t *testing.T) {
	_, inv := newReal(t, 1)
	tests := []struct {
		words []string
		want  Counts
	}{
		{[]string{"pippo"}, Counts{Syllables: map[int]int{2: 1}, Initial: map[string]int{"CVC": 1}, Final: map[string]int{"CV": 1}}},
		{[]string{"acua"}, Counts{Syllables: map[int]int{3: 1}, Initial: map[string]int{"V": 1}, Medial: map[string]int{"CV": 1}, Final: map[string]int{"V": 1}}},
		{[]string{"rifatia"}, Counts{Syllables: map[int]int{4: 1}, Initial: map[string]int{"CV": 1}, Medial: map[string]int{"CV": 2}, Final: map[string]int{"V": 1}}},
		{[]string{"sai", "lirma"}, Counts{Syllables: map[int]int{1: 1, 2: 1}, Mono: map[string]int{"CVV": 1}, Initial: map[string]int{"CVC": 1}, Final: map[string]int{"CV": 1}}},
	}
	for _, tt := range tests {
		var corpus [][]phon.Segment
		for _, w := range tt.words {
			segs, err := inv.Tokenize(w)
			if err != nil {
				t.Fatal(err)
			}
			corpus = append(corpus, segs)
		}
		got, err := Count(inv, corpus)
		if err != nil {
			t.Fatal(err)
		}
		for name, pair := range map[string][2]map[string]int{
			"mono": {got.Mono, tt.want.Mono}, "initial": {got.Initial, tt.want.Initial},
			"medial": {got.Medial, tt.want.Medial}, "final": {got.Final, tt.want.Final},
		} {
			if !maps.Equal(pair[0], pair[1]) && len(pair[0])+len(pair[1]) > 0 {
				t.Errorf("%v: %s = %v, want %v", tt.words, name, pair[0], pair[1])
			}
		}
		if !maps.Equal(got.Syllables, tt.want.Syllables) {
			t.Errorf("%v: syllables = %v, want %v", tt.words, got.Syllables, tt.want.Syllables)
		}
	}
}
