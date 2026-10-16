package sound

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/olehmushka/lingua-nova/internal/lexicon"
	"github.com/olehmushka/lingua-nova/internal/phon"
)

func spell(segs []phon.Segment) string { return strings.Join(phon.Spellings(segs), "") }

// trace renders the steps as "line:before>after" items, space separated.
func trace(steps []Step) string {
	var parts []string
	for _, s := range steps {
		parts = append(parts, fmt.Sprintf("%d:%s>%s", s.Rule.Line, spell(s.Before), spell(s.After)))
	}
	return strings.Join(parts, " ")
}

func evolve(t *testing.T, inv *phon.Inventory, rules, word string) (string, string) {
	t.Helper()
	rs, err := Parse("test", strings.NewReader(rules), inv)
	if err != nil {
		t.Fatal(err)
	}
	segs, err := inv.Tokenize(word)
	if err != nil {
		t.Fatal(err)
	}
	out, steps := Evolve(inv, rs, segs)
	return spell(out), trace(steps)
}

func TestEvolve(t *testing.T) {
	inv := loadPhon(t)
	tests := []struct {
		name, rules, word, want, trace string
	}{
		{"no rules", "", "pata", "pata", ""},
		{"nothing matches", "t > d / V_V\nn > ∅ / _#", "lirma", "lirma", ""},
		{"one rule", "t > d / V_V\nn > ∅ / _#", "pata", "pada", "1:pata>pada"},
		{"two rules", "t > d / V_V\nn > ∅ / _#", "saten", "sade", "1:saten>saden 2:saden>sade"},
		// The t of *rifatia is between vowels, but palatalization runs first
		// and turns ti into ch, so voicing finds no t (rules 1 and 2 in the
		// real file).
		{"palatal then voicing", "ti > ch / _V\nt > d / V_V", "rifatia", "rifacha", "1:rifatia>rifacha"},
		// Feeding: rule 1 creates the intervocalic t that rule 2 voices.
		{"feeding", "tt > t\nt > d / V_V", "patta", "pada", "1:patta>pata 2:pata>pada"},
		// Bleeding: rule 1 (voicing) can't touch tt, and rule 2 then removes
		// the geminate.
		{"bleeding", "t > d / V_V\ntt > t", "patta", "pata", "2:patta>pata"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, tr := evolve(t, inv, tt.rules, tt.word)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
			if tr != tt.trace {
				t.Errorf("trace %q, want %q", tr, tt.trace)
			}
		})
	}
}

// TestEvolveOrder shows why the order in sound-changes.txt matters: the same
// two rules, swapped, give a different word.
func TestEvolveOrder(t *testing.T) {
	inv := loadPhon(t)
	if got, _ := evolve(t, inv, "t > d / V_V\ntt > t", "patta"); got != "pata" {
		t.Errorf("voicing before degemination: got %q, want pata", got)
	}
	if got, _ := evolve(t, inv, "tt > t\nt > d / V_V", "patta"); got != "pada" {
		t.Errorf("degemination before voicing: got %q, want pada", got)
	}
}

// TestLexiconEvolves: the lexicon is the oracle. Every proto form, run
// through the real sound changes, must give its modern form.
func TestLexiconEvolves(t *testing.T) {
	inv := loadPhon(t)
	rules, err := Load(filepath.Join(langDir, "sound-changes.txt"), inv)
	if err != nil {
		t.Fatal(err)
	}
	lex, err := lexicon.Load(filepath.Join(langDir, "lexicon"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range lex.Entries {
		t.Run(e.ID, func(t *testing.T) {
			segs, err := inv.Tokenize(strings.TrimPrefix(e.Proto, "*"))
			if err != nil {
				t.Fatalf("%s:%d: %v", e.File, e.LineOf("proto"), err)
			}
			out, steps := Evolve(inv, rules, segs)
			if got := spell(out); got != e.Modern {
				t.Errorf("%s:%d: *%s → %s, want %s (trace: %s)",
					e.File, e.LineOf("modern"), e.Proto, got, e.Modern, trace(steps))
			}
		})
	}
}
