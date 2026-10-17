package translate

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/olehmushka/lingua-nova/internal/gloss"
	"github.com/olehmushka/lingua-nova/internal/lexicon"
)

const langDir = "../../language"

var (
	once     sync.Once
	shared   *Translator
	setupErr error
)

// translator is shared: loading the tagger's model takes a moment.
func translator(t *testing.T) *Translator {
	t.Helper()
	once.Do(func() { shared, setupErr = load() })
	if setupErr != nil {
		t.Fatal(setupErr)
	}
	return shared
}

func load() (*Translator, error) { return Load(langDir) }

// TestGrammarExamples translates the English line of every interlinear
// example in grammar.md: the example's Rifatian must be among the results.
func TestGrammarExamples(t *testing.T) {
	tr := translator(t)
	md, err := os.ReadFile(filepath.Join(langDir, "grammar.md"))
	if err != nil {
		t.Fatal(err)
	}
	examples := gloss.Examples(string(md))
	if len(examples) < 5 {
		t.Fatalf("found %d examples in grammar.md, want at least 5", len(examples))
	}
	for _, ex := range examples {
		res, err := tr.Translate(ex.Translation, 0)
		if err != nil {
			t.Errorf("%q: %v", ex.Translation, err)
			continue
		}
		var got []string
		for _, x := range res.Translations {
			got = append(got, x.Text)
		}
		if !slices.Contains(got, ex.Sentence()) {
			t.Errorf("%q: got %q, want it to include %q", ex.Translation, got, ex.Sentence())
		}
	}
}

func TestTranslate(t *testing.T) {
	tr := translator(t)
	tests := []struct {
		in    string
		first string // the best translation
		total int
	}{
		// Statements, tenses, plurals
		{"The woman sees the river.", "Lu mena nara lu trubo.", 1},
		{"The men will drink the water.", "Lu dubroh pulvareh lu exa.", 1},
		{"The woman saw the dogs", "Lu mena naraxe lu huroh", 1},
		{"Women see the sea.", "Menah narah lu zahor.", 1},
		{"I sleep.", "Mu silu.", 1},
		// Adjectives follow the noun; possessives precede it
		{"She sees my house.", "Hana nara me hoida.", 1},
		{"He saw the big black dog.", "Hono naraxe lu huro gumo bujo.", 1},
		{"The women's dog sees her.", "Lu menahes huro nara hani.", 1},
		{"They see the woman's dog's house.", "Honoh narah lu menas huros hoida.", 2},
		{"The white woman's dog sleeps.", "Lu menas lirma huro sile.", 1},
		// Negation: do-support, will not, double negation
		{"I didn't see thee.", "Mu na naraxu chi.", 1},
		{"The man does not drink water.", "Lu dubro na pulva exa.", 1},
		{"We won't sleep.", "Muh na siliremu.", 1},
		{"I saw nothing.", "Mu na naraxu zunhu.", 1},
		{"Nothing fell.", "Zunhu zuvixe.", 1},
		// Questions: no inversion, question word first
		{"Didst thou see the woman's dog?", "Chu naraxis lu menas huro?", 1},
		{"Did the man see thee?", "Lu dubro naraxe chi?", 1},
		{"Didn't he see her?", "Hono na naraxe hani?", 1},
		{"Who saw the dog?", "Ifa naraxe lu huro?", 1},
		{"What did she see?", "Itu hana naraxe?", 1},
		{"When didst thou see her?", "Cuda chu naraxis hani?", 1},
		// Ambiguity: four yous, five loves
		{"You see the dog.", "Chu naras lu huro.", 4},
		{"I love the woman.", "Mu eluvu lu mena.", 5},
		{"Thou killed him", "Chu hecaxis honi", 2},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			res, err := tr.Translate(tt.in, 0)
			if err != nil {
				t.Fatal(err)
			}
			if got := res.Translations[0].Text; got != tt.first {
				t.Errorf("first: got %q, want %q", got, tt.first)
			}
			if res.Total != tt.total {
				var all []string
				for _, x := range res.Translations {
					all = append(all, x.Text)
				}
				t.Errorf("total: got %d %q, want %d", res.Total, all, tt.total)
			}
			// Every output must read back cleanly: analysis by synthesis.
			for _, x := range res.Translations {
				if len(x.Notes) > 0 {
					t.Errorf("%q: notes %q", x.Text, x.Notes)
				}
				if x.Gloss == nil {
					t.Errorf("%q: not glossed", x.Text)
				}
			}
		})
	}
}

func TestTranslateMax(t *testing.T) {
	res, err := translator(t).Translate("You love them.", 3)
	if err != nil {
		t.Fatal(err)
	}
	// 4 yous × 5 loves × 2 thems
	if len(res.Translations) != 3 || res.Total != 40 {
		t.Errorf("got %d shown of %d, want 3 of 40", len(res.Translations), res.Total)
	}
}

func TestTranslateProblems(t *testing.T) {
	tr := translator(t)

	res, err := tr.Translate("The dragon eats bread.", 0)
	if err != nil {
		t.Fatal(err)
	}
	x := res.Translations[0]
	if x.Text != "Lu ?dragon bauge gurna." || !slices.Equal(x.Unknown, []string{"dragon"}) || x.Gloss != nil {
		t.Errorf("unknown word: got %q unknown %q", x.Text, x.Unknown)
	}

	for _, in := range []string{"Sees the dog the woman.", "the the", "", "Quickly!"} {
		if _, err := tr.Translate(in, 0); err == nil {
			t.Errorf("%q: no error", in)
		}
	}
	_, err = tr.Translate("Sees the dog the woman.", 0)
	if want := "the grammar has no tree for: Sees/V the/Det dog/N the/Det woman/N"; err == nil || err.Error() != want {
		t.Errorf("got %v, want %s", err, want)
	}
}

// TestParseScore: the tagger mis-tags eats and bread after an unknown
// word, but the tree that agrees with it most doesn't parse, so the
// translation still uses the verb reading.
func TestParseScore(t *testing.T) {
	s, ps := translator(t).Parse("The dragon eats bread")
	if len(ps) != 1 {
		t.Fatalf("got %d parses", len(ps))
	}
	if got := ps[0].Tree.Bracketed(s.Words()); got != "(S (NP (Det The) (Nom (N dragon))) (Pred (VP (V eats) (NP (Nom (N bread))))))" {
		t.Errorf("got %s", got)
	}
	if ps[0].Score != 2 {
		t.Errorf("score %d, want 2 (The, dragon)", ps[0].Score)
	}
}

func TestCombos(t *testing.T) {
	a, b := choice{}, choice{}
	a.r.Cat, b.r.Cat = "a", "b"
	var got []string
	combos([][]choice{{a, b}, {a}, {a, b}}, 100, func(p []choice, sum int) {
		s := ""
		for _, c := range p {
			s += c.r.Cat
		}
		got = append(got, fmt.Sprint(s, sum))
	})
	if want := []string{"aaa0", "aab1", "baa1", "bab2"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	got = nil
	combos([][]choice{{a, b}, {a, b}}, 3, func([]choice, int) { got = append(got, "") })
	if len(got) != 3 {
		t.Errorf("limit: %d calls", len(got))
	}
}

func TestCheck(t *testing.T) {
	g, w, err := LoadEnglish(langDir)
	if err != nil {
		t.Fatal(err)
	}
	lex, err := lexicon.Load(filepath.Join(langDir, "lexicon"))
	if err != nil {
		t.Fatal(err)
	}
	if ps := Check(g, w, lex); len(ps) > 0 {
		t.Errorf("english/: %v", ps)
	}
}
