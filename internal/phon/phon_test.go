package phon

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

const langDir = "../../language"

func loadReal(t *testing.T) *Inventory {
	t.Helper()
	inv, err := Load(filepath.Join(langDir, "phonology.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return inv
}

func TestTokenize(t *testing.T) {
	inv := loadReal(t)
	tests := []struct {
		word string
		segs string // spellings joined by ·
		ipa  string
	}{
		{"folha", "f·o·lh·a", "foʎa"},
		{"frinha", "f·r·i·nh·a", "fɾiɲa"},
		{"gucho", "g·u·ch·o", "gutʃo"},
		{"exa", "e·x·a", "eʃa"},
		{"hoidah", "h·o·i·d·a·h", "xoidax"},
		{"rifacha", "r·i·f·a·ch·a", "ɾifatʃa"},
		{"cuda", "c·u·d·a", "kuda"},
		{"lu", "l·u", "lu"},
		{"", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			segs, err := inv.Tokenize(tt.word)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(Spellings(segs), "·"); got != tt.segs {
				t.Errorf("segments = %s, want %s", got, tt.segs)
			}
			if got := IPA(segs); got != tt.ipa {
				t.Errorf("IPA = %s, want %s", got, tt.ipa)
			}
		})
	}
}

func TestTokenizeUnknownLetter(t *testing.T) {
	inv := loadReal(t)
	tests := []struct {
		word   string
		pos    int
		letter rune
	}{
		{"kato", 0, 'k'},
		{"aqua", 1, 'q'},
		{"café", 3, 'é'},
		{"Mena", 0, 'M'},
	}
	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			_, err := inv.Tokenize(tt.word)
			var ule *UnknownLetterError
			if !errors.As(err, &ule) {
				t.Fatalf("err = %v, want UnknownLetterError", err)
			}
			if ule.Pos != tt.pos || ule.Letter != tt.letter {
				t.Errorf("got %q at %d, want %q at %d", ule.Letter, ule.Pos, tt.letter, tt.pos)
			}
		})
	}
}

func TestLoadReal(t *testing.T) {
	inv := loadReal(t)
	if got := len(inv.Categories["V"]); got != 5 {
		t.Errorf("V has %d members, want 5", got)
	}
	if got := len(inv.Categories["C"]); got != 20 {
		t.Errorf("C has %d members, want 20", got)
	}
	var modernOnly []string
	for _, s := range inv.Segments {
		if !s.Proto {
			modernOnly = append(modernOnly, s.Spell)
		}
	}
	if want := []string{"lh", "nh", "x", "j", "ch"}; !slices.Equal(modernOnly, want) {
		t.Errorf("modern-only segments = %v, want %v", modernOnly, want)
	}
}

func TestParseErrors(t *testing.T) {
	const base = `
consonants:
  - {spell: p, ipa: p}
  - {spell: r, ipa: ɾ}
vowels:
  - {spell: a, ipa: a}
  - {spell: i, ipa: i}
`
	tests := []struct {
		name  string
		extra string
		want  string // substring of the error
	}{
		{"duplicate", "  - {spell: a, ipa: a}\n", "defined twice"},
		{"uppercase", "  - {spell: A, ipa: a}\n", "lowercase"},
		{"empty ipa", "  - {spell: o}\n", "empty"},
		{"unknown field", "foo: 1\n", "foo"},
		{"onset unknown", "onsets: [[p, l]]\n", `unknown segment "l"`},
		{"onset vowel", "onsets: [[p, a]]\n", "is a vowel"},
		{"onset single", "onsets: [[p]]\n", "only clusters"},
		{"diphthong consonant", "diphthongs: [[a, r]]\n", "is a consonant"},
		{"coda vowel", "codas: [a]\n", "is a vowel"},
		{"builtin category", "categories: {V: [a]}\n", "built in"},
		{"forbidden unknown", "forbidden: [[p, h]]\n", `unknown segment or category "h"`},
		{"forbidden unknown category", "forbidden: [[V, Stop]]\n", `unknown segment or category "Stop"`},
		{"lowercase category", "categories: {stop: [p]}\n", "uppercase"},
		{"forbidden single", "forbidden: [[p]]\n", "2 or more"},
		{"proto coda vowel", "proto: {codas: [a]}\n", "is a vowel"},
		{"proto geminate unknown", "proto: {geminates: [t]}\n", `unknown segment "t"`},
		{"proto hiatus consonant", "proto: {hiatus_after: [p]}\n", `"p" is a consonant, want vowel`},
		{"generator no syllables", "proto: {generator: {final: {CV: 1}}}\n", "syllables is empty"},
		{"generator zero count", "proto: {generator: {syllables: {0: 1}}}\n", "want 1 or more"},
		{"generator zero weight", "proto: {generator: {syllables: {1: 0}}}\n", "weight 0"},
		{"generator no mono", "proto: {generator: {syllables: {1: 1}}}\n", "mono: needed"},
		{"generator no medial", "proto: {generator: {syllables: {3: 1}, initial: {CV: 1}, final: {CV: 1}}}\n", "medial: needed"},
		{"generator long coda", "proto: {generator: {syllables: {1: 1}, mono: {CVCC: 1}}}\n", `mono "CVCC": coda of more`},
		{"generator long onset", "proto: {generator: {syllables: {1: 1}, mono: {CCCV: 1}}}\n", `mono "CCCV": onset of more`},
		{"generator long nucleus", "proto: {generator: {syllables: {1: 1}, mono: {CVVV: 1}}}\n", "nucleus of more"},
		{"generator no vowel", "proto: {generator: {syllables: {1: 1}, mono: {CC: 1}}}\n", "no vowel"},
		{"generator bad letter", "proto: {generator: {syllables: {1: 1}, mono: {CVX: 1}}}\n", "want C, CC"},
		{"generator template order", "proto: {generator: {syllables: {1: 1}, mono: {VCV: 1}}}\n", "want C, CC"},
		{"generator template weight", "proto: {generator: {syllables: {1: 1}, mono: {CV: -1}}}\n", `mono "CV": weight -1`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(base + tt.extra))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want it to contain %q", err, tt.want)
			}
		})
	}
	if _, err := Parse([]byte(base)); err != nil {
		t.Errorf("base: %v", err)
	}
	modernOnly := strings.Replace(base, "vowels:", "  - {spell: x, ipa: ʃ, modern_only: true}\nvowels:", 1)
	if _, err := Parse([]byte(modernOnly + "proto: {finals: [r, x]}\n")); err == nil || !strings.Contains(err.Error(), `"x" is not a proto-language sound`) {
		t.Errorf("modern-only proto final: err = %v", err)
	}
	if _, err := Parse([]byte(base + "categories: {Stop: [p]}\nforbidden: [[V, V, V], [Stop, r]]\n")); err != nil {
		t.Errorf("categories in forbidden: %v", err)
	}
}

func TestParseGenerator(t *testing.T) {
	g := loadReal(t).Generator
	if g == nil {
		t.Fatal("real phonology has no generator block")
	}
	if got := len(g.Syllables); got != 4 || g.Syllables[0] != (Weighted[int]{1, 10}) {
		t.Errorf("syllables = %v, want 4 counts starting at {1 10}", g.Syllables)
	}
	want := map[string]Template{
		"CCVC": {Text: "CCVC", Onset: 2, Nucleus: 1, Coda: 1, Weight: 4},
		"VV":   {Text: "VV", Onset: 0, Nucleus: 2, Coda: 0, Weight: 2},
		"CV":   {Text: "CV", Onset: 1, Nucleus: 1, Coda: 0, Weight: 36},
	}
	for _, tmpl := range g.Initial {
		if w, ok := want[tmpl.Text]; ok && tmpl != w {
			t.Errorf("initial %s = %+v, want %+v", tmpl.Text, tmpl, w)
		}
	}
	if !slices.IsSortedFunc(g.Initial, func(a, b Template) int { return strings.Compare(a.Text, b.Text) }) {
		t.Errorf("initial templates not sorted: %v", g.Initial)
	}
}

// lexiconForms returns every proto and modern form in the lexicon, labelled
// "file: id", for tests that use the lexicon as a fixture.
func lexiconForms(t *testing.T) (labels, forms []string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(langDir, "lexicon", "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no lexicon files: %v", err)
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var entries []struct {
			ID     string `yaml:"id"`
			Proto  string `yaml:"proto"`
			Modern string `yaml:"modern"`
		}
		if err := yaml.Unmarshal(data, &entries); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		for _, e := range entries {
			for _, form := range []string{e.Proto, e.Modern} {
				labels = append(labels, filepath.Base(file)+": "+e.ID)
				forms = append(forms, form)
			}
		}
	}
	return labels, forms
}

// TestLexiconTokenizes uses the lexicon as a fixture: every proto and modern
// form must be spelled with known segments. (The full checks are in
// internal/lexicon.)
func TestLexiconTokenizes(t *testing.T) {
	inv := loadReal(t)
	labels, forms := lexiconForms(t)
	for i, form := range forms {
		if _, err := inv.Tokenize(form); err != nil {
			t.Errorf("%s: %v", labels[i], err)
		}
	}
}
