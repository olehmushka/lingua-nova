package phon

import (
	"strings"
	"testing"
)

func TestCheckPhonotactics(t *testing.T) {
	inv := loadReal(t)
	tests := []struct {
		word string // segment spellings separated by spaces
		want string // fragment of one of the violations; "" = legal
	}{
		{"d u b r o", ""},
		{"b i l s a", ""},
		{"h e u j o", ""},
		{"l u", ""},
		{"h o i d a h", ""},
		{"t l a", `illegal onset "tl" in tla`},
		{"c u p p e", `double consonant "pp" in cup.pe`},
		{"a s s a", `double consonant "ss" in as.sa`},
		{"b i l s t a", `coda "ls" has more than one consonant in bils.ta`},
		{"g a t", `illegal word-final consonant "t" in gat`},
		{"g a r s", `word-final consonant "rs" has more than one consonant`},
		{"a n h a", "forbidden sequence [n h]"}, // "anha" would tokenize as ɲ
		{"a l h a", "forbidden sequence [l h]"},
		{"a i a", "forbidden sequence [a i a] (matches [V V V])"},
		{"p r", "word has no vowel"},
		{"d i a", ""}, // modern hiatus after any vowel
		{"a o", ""},
	}
	for _, tt := range tests {
		t.Run(strings.ReplaceAll(tt.word, " ", ""), func(t *testing.T) {
			checkViolations(t, inv.CheckPhonotactics(segsOf(t, inv, tt.word)), tt.want)
		})
	}
}

func TestCheckProtoPhonotactics(t *testing.T) {
	inv := loadReal(t)
	tests := []struct {
		word string // segment spellings separated by spaces
		want string // fragment of one of the violations; "" = legal
	}{
		{"c u p p e", ""},
		{"i t t u n", ""},
		{"b r e c c o", ""},
		{"c u t a n", ""}, // final n: legal in proto, deleted by rule 4
		{"b i m", ""},
		{"h e s t o", ""},
		{"g a r s o", ""},
		{"a m p a", `illegal coda "m" in am.pa`},
		{"a h t a", `illegal coda "h" in ah.ta`},
		{"a p t a", `illegal coda "p" in ap.ta`},               // not a geminate
		{"a p p r a", `illegal coda "p" in ap.pra`},            // onset isn't exactly [p]
		{"o r p p a", `coda "rp" has more than one consonant`}, // geminate after a coda
		{"a s s a", `double consonant "ss" in as.sa`},          // s is a legal coda, but not a geminate
		{"a l l a", `double consonant "ll" in al.la`},
		{"c u p p", `word-final consonant "pp" has more than one consonant in cupp`},
		{"p a s", `illegal word-final consonant "s" in pas`},
		{"t l a", `illegal onset "tl" in tla`},
		{"a i a", "forbidden sequence [a i a]"},
		{"u s i a", ""}, // hiatus after i
		{"b u z i o", ""},
		{"i e", ""},
		{"a a r", `hiatus "a.a" (only after i) in a.ar`},
		{"n e i v a o", `hiatus "a.o" (only after i) in nei.va.o`},
		{"a u m u i", `hiatus "u.i" (only after i) in au.mu.i`},
		{"t e o", `hiatus "e.o"`},
	}
	for _, tt := range tests {
		t.Run(strings.ReplaceAll(tt.word, " ", ""), func(t *testing.T) {
			checkViolations(t, inv.CheckProtoPhonotactics(segsOf(t, inv, tt.word)), tt.want)
		})
	}
}

func segsOf(t *testing.T, inv *Inventory, word string) []Segment {
	t.Helper()
	var segs []Segment
	for _, sp := range strings.Fields(word) {
		seg, ok := inv.Lookup(sp)
		if !ok {
			t.Fatalf("unknown segment %q", sp)
		}
		segs = append(segs, seg)
	}
	return segs
}

// checkViolations wants no violations if want is "", else one containing want.
func checkViolations(t *testing.T, got []Violation, want string) {
	t.Helper()
	if want == "" {
		if len(got) > 0 {
			t.Errorf("want legal, got %v", got)
		}
		return
	}
	for _, v := range got {
		if strings.Contains(v.Msg, want) {
			return
		}
	}
	t.Errorf("got %v, want a violation containing %q", got, want)
}
