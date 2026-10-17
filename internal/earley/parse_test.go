package earley

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

const toy = `# A toy English grammar.
S   -> NP VP
NP  -> Det N | NP PP | Pron   # left recursive
VP  -> V NP | VP PP
PP  -> P NP
`

// sentence splits "I/Pron saw/V the/Det man/N" into words and categories;
// a word can have several: her/Pron/Poss.
func sentence(s string) (words []string, cats [][]string) {
	for _, f := range strings.Fields(s) {
		parts := strings.Split(f, "/")
		words = append(words, parts[0])
		cats = append(cats, parts[1:])
	}
	return words, cats
}

func mustParse(t *testing.T, src string) *Grammar {
	t.Helper()
	g, err := Parse("test.cfg", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestParse(t *testing.T) {
	g := mustParse(t, toy)
	tests := []struct {
		name, in string
		want     []string
	}{
		{"simple", "I/Pron saw/V her/Pron", []string{
			"(S (NP (Pron I)) (VP (V saw) (NP (Pron her))))",
		}},
		// The classic attachment ambiguity: who has the telescope?
		{"PP attachment", "I/Pron saw/V the/Det man/N with/P the/Det telescope/N", []string{
			"(S (NP (Pron I)) (VP (V saw) (NP (NP (Det the) (N man)) (PP (P with) (NP (Det the) (N telescope))))))",
			"(S (NP (Pron I)) (VP (VP (V saw) (NP (Det the) (N man))) (PP (P with) (NP (Det the) (N telescope)))))",
		}},
		// A token with two categories: the grammar picks the one that fits.
		{"lexical ambiguity", "I/Pron saw/V/N her/Pron/Det", []string{
			"(S (NP (Pron I)) (VP (V saw) (NP (Pron her))))",
		}},
		{"no parse", "saw/V the/Det man/N", nil},
		{"incomplete", "I/Pron saw/V the/Det", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			words, cats := sentence(tt.in)
			var got []string
			for _, tr := range g.Parse(cats, 0) {
				got = append(got, tr.Bracketed(words))
			}
			slices.Sort(got)
			slices.Sort(tt.want)
			if !slices.Equal(got, tt.want) {
				t.Errorf("got\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(tt.want, "\n  "))
			}
		})
	}
}

func TestParseMax(t *testing.T) {
	g := mustParse(t, toy)
	// Two PPs: five attachments (a Catalan number), capped at 3.
	_, cats := sentence("I/Pron saw/V the/Det man/N with/P the/Det dog/N in/P the/Det park/N")
	if n := len(g.Parse(cats, 0)); n != 5 {
		t.Errorf("all parses: got %d, want 5", n)
	}
	if n := len(g.Parse(cats, 3)); n != 3 {
		t.Errorf("max 3: got %d", n)
	}
}

func TestUnitCycle(t *testing.T) {
	g := mustParse(t, "S -> A\nA -> B | x\nB -> A")
	_, cats := sentence("w/x")
	if n := len(g.Parse(cats, 0)); n == 0 {
		t.Error("no parse")
	}
}

func TestLeaves(t *testing.T) {
	g := mustParse(t, toy)
	words, cats := sentence("I/Pron saw/V the/Det man/N")
	trs := g.Parse(cats, 0)
	if len(trs) != 1 {
		t.Fatalf("got %d trees", len(trs))
	}
	var got []string
	for _, l := range trs[0].Leaves() {
		got = append(got, l.Label+":"+words[l.Start])
	}
	if want := "Pron:I V:saw Det:the N:man"; strings.Join(got, " ") != want {
		t.Errorf("got %v, want %s", got, want)
	}
}

func TestGrammarErrors(t *testing.T) {
	_, err := Parse("bad.cfg", []byte("S -> NP VP\nNP Det N\nVP -> V |\nA B -> C\n"))
	var errs ErrorList
	if !errors.As(err, &errs) {
		t.Fatalf("got %v, want ErrorList", err)
	}
	want := []string{
		`bad.cfg:2: "NP Det N": want LHS -> RHS`,
		"bad.cfg:3: VP: empty alternative",
		"bad.cfg:4: want one symbol before ->, found 2",
	}
	var got []string
	for _, e := range errs {
		got = append(got, e.Error())
	}
	if !slices.Equal(got, want) {
		t.Errorf("got\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	if _, err := Parse("empty.cfg", []byte("# nothing\n")); err == nil {
		t.Error("empty grammar: no error")
	}
}

func TestPreterminals(t *testing.T) {
	g := mustParse(t, toy)
	if got, want := g.Preterminals(), []string{"Det", "N", "Pron", "V", "P"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if g.Start != "S" {
		t.Errorf("start %q", g.Start)
	}
}
