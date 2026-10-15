package phon

import (
	"errors"
	"slices"
	"testing"
)

func TestSyllabify(t *testing.T) {
	inv := loadReal(t)
	tests := []struct {
		word  string
		spell string // syllables in spelling
		ipa   string
	}{
		{"folha", "fo.lha", "ˈfo.ʎa"},
		{"hoida", "hoi.da", "ˈxoi.da"},     // diphthong = one nucleus
		{"hoidah", "hoi.dah", "ˈxoi.dax"},  // -h is a coda, not a syllable
		{"naraxu", "na.ra.xu", "naˈɾa.ʃu"}, // stress mark replaces the dot
		{"dubro", "du.bro", "ˈdu.bɾo"},     // br is a legal onset
		{"bilsa", "bil.sa", "ˈbil.sa"},     // ls is not
		{"zolche", "zol.che", "ˈzol.tʃe"},  // ch is one segment
		{"cuppe", "cup.pe", "ˈkup.pe"},     // illegal coda, split anyway
		{"tuilar", "tu.i.lar", "tuˈi.laɾ"}, // ui is not a diphthong
		{"dia", "di.a", "ˈdi.a"},           // hiatus
		{"rifatia", "ri.fa.ti.a", "ɾi.faˈti.a"},
		{"eila", "ei.la", "ˈei.la"}, // no onset
		{"zou", "zou", "zou"},       // monosyllable: no mark
		{"lu", "lu", "lu"},
		{"a", "a", "a"},
	}
	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			segs, err := inv.Tokenize(tt.word)
			if err != nil {
				t.Fatal(err)
			}
			syls, err := inv.Syllabify(segs)
			if err != nil {
				t.Fatal(err)
			}
			if got := SyllableSpelling(syls); got != tt.spell {
				t.Errorf("syllables = %s, want %s", got, tt.spell)
			}
			if got := SyllableIPA(syls, Stress(syls)); got != tt.ipa {
				t.Errorf("IPA = %s, want %s", got, tt.ipa)
			}
		})
	}
}

func TestSyllabifyNoVowel(t *testing.T) {
	inv := loadReal(t)
	for _, w := range []string{"", "h", "pr"} {
		segs, err := inv.Tokenize(w)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := inv.Syllabify(segs); !errors.Is(err, ErrNoVowel) {
			t.Errorf("%q: err = %v, want ErrNoVowel", w, err)
		}
	}
}

// TestLexiconSyllabifies: every lexicon form splits into syllables that each
// have a nucleus and that join back into the original word.
func TestLexiconSyllabifies(t *testing.T) {
	inv := loadReal(t)
	labels, forms := lexiconForms(t)
	for i, form := range forms {
		segs, err := inv.Tokenize(form)
		if err != nil {
			t.Fatalf("%s: %v", labels[i], err)
		}
		syls, err := inv.Syllabify(segs)
		if err != nil {
			t.Errorf("%s: %s: %v", labels[i], form, err)
			continue
		}
		var joined []Segment
		for _, s := range syls {
			if len(s.Nucleus) == 0 {
				t.Errorf("%s: %s: syllable without nucleus", labels[i], form)
			}
			joined = append(joined, s.Segments()...)
		}
		if !slices.Equal(joined, segs) {
			t.Errorf("%s: %s: syllables %s don't join back to the word", labels[i], form, SyllableSpelling(syls))
		}
	}
}
