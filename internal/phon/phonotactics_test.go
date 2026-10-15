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
		{"c u p p e", `illegal coda "p" in cup.pe`},
		{"b i l s t a", `coda "ls" has more than one consonant in bils.ta`},
		{"g a t", `illegal word-final consonant "t" in gat`},
		{"g a r s", `word-final consonant "rs" has more than one consonant`},
		{"a n h a", "forbidden sequence [n h]"}, // "anha" would tokenize as ɲ
		{"a l h a", "forbidden sequence [l h]"},
		{"a i a", "forbidden sequence [a i a] (matches [V V V])"},
		{"p r", "word has no vowel"},
	}
	for _, tt := range tests {
		t.Run(strings.ReplaceAll(tt.word, " ", ""), func(t *testing.T) {
			var segs []Segment
			for _, sp := range strings.Fields(tt.word) {
				seg, ok := inv.Lookup(sp)
				if !ok {
					t.Fatalf("unknown segment %q", sp)
				}
				segs = append(segs, seg)
			}
			got := inv.CheckPhonotactics(segs)
			if tt.want == "" {
				if len(got) > 0 {
					t.Errorf("want legal, got %v", got)
				}
				return
			}
			for _, v := range got {
				if strings.Contains(v.Msg, tt.want) {
					return
				}
			}
			t.Errorf("got %v, want a violation containing %q", got, tt.want)
		})
	}
}
