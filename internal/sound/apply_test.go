package sound

import (
	"strings"
	"testing"

	"github.com/olehmushka/lingua-nova/internal/phon"
)

func TestApply(t *testing.T) {
	inv := loadPhon(t)
	tests := []struct {
		rule, word, want string
	}{
		{"t > d / V_V", "ihitu", "ihidu"},
		{"t > d / V_V", "patta", "patta"}, // neither t is between vowels
		{"p > b / V_V", "haupa", "hauba"}, // the u of a diphthong is still V
		{"c > g / V_V", "nauca", "nauga"},
		{"si > x / _V", "esia", "exa"}, // two segments become one
		{"si > x / _V", "sizio", "sizio"},
		{"ti > ch / _V", "rifatia", "rifacha"},
		{"n > ∅ / _#", "hulen", "hule"},
		{"n > ∅ / _#", "frinia", "frinia"}, // n not at the end
		{"pp > p", "pippo", "pipo"},
		{"C > h / #_", "pata", "hata"},
		{"a > e / _a", "aaaa", "eeea"}, // every a before an a, in the input
		{"pp > p", "ppp", "pp"},        // matches don't overlap
		{"pp > p", "pppp", "pp"},
	}
	for _, tt := range tests {
		t.Run(tt.rule+" "+tt.word, func(t *testing.T) {
			rules, err := Parse("test", strings.NewReader(tt.rule), inv)
			if err != nil {
				t.Fatal(err)
			}
			word, err := inv.Tokenize(tt.word)
			if err != nil {
				t.Fatal(err)
			}
			out, changed := rules[0].Apply(inv, word)
			got := strings.Join(phon.Spellings(out), "")
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
			if changed != (tt.word != tt.want) {
				t.Errorf("changed = %v", changed)
			}
		})
	}
}

// TestApplySimultaneous shows the application semantics. With a > e / e_,
// only the a right after the original e changes. Iterative application
// would let each new e trigger the next change and give eeee.
func TestApplySimultaneous(t *testing.T) {
	inv := loadPhon(t)
	rules, err := Parse("test", strings.NewReader("a > e / e_"), inv)
	if err != nil {
		t.Fatal(err)
	}
	word, _ := inv.Tokenize("eaaa")
	out, _ := rules[0].Apply(inv, word)
	if got := strings.Join(phon.Spellings(out), ""); got != "eeaa" {
		t.Errorf("got %q, want eeaa (simultaneous; iterative would give eeee)", got)
	}
}
