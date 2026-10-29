package fixture

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/olehmushka/lingua-nova/internal/lexicon"
)

var real = []*lexicon.Entry{
	{ID: "water", POS: "noun", Gender: "f", Gloss: "water", Definition: "Water; any liquid."},
	{ID: "see", POS: "verb", Gloss: "see", Definition: "To see with the eyes."},
	{ID: "big", POS: "adjective", Gloss: "big", Definition: "Large."},
	{ID: "the", POS: "article", Gloss: "the", Definition: "The definite article."},
}

func form(pos string) (string, string) {
	if pos == "verb" {
		return "tadar", "tadar"
	}
	return "tada", "tada"
}

func TestPool(t *testing.T) {
	// "any" and "the" are stopwords; "to" is too short.
	want := []string{"article", "big", "definite", "eyes", "large", "liquid", "see", "water"}
	if got := Pool(real); !slices.Equal(got, want) {
		t.Errorf("Pool = %v, want %v", got, want)
	}
}

func TestEntries(t *testing.T) {
	gen := func(senses float64) []*lexicon.Entry {
		return Entries(real, form, Options{N: 2000, Senses: senses, Homonyms: 0.2}, rand.New(rand.NewPCG(1, 0)))
	}
	es := gen(0.3)
	if len(es) != 2000 {
		t.Fatalf("got %d entries, want 2000", len(es))
	}
	ids := map[string]bool{}
	senses, homonyms := 0, 0
	heads := []string{"water", "see", "big"}
	pool := Pool(real)
	for _, e := range es {
		if !strings.HasPrefix(e.ID, IDPrefix) || ids[e.ID] {
			t.Fatalf("bad or repeated id %q", e.ID)
		}
		ids[e.ID] = true
		if e.Gloss == "" || e.Definition == "" || e.Proto == "" || e.Modern == "" {
			t.Fatalf("%s: empty field: %+v", e.ID, e)
		}
		if (e.POS == "noun") != (e.Gender != "") {
			t.Errorf("%s: pos %s with gender %q", e.ID, e.POS, e.Gender)
		}
		if e.POS == "article" {
			t.Errorf("%s: closed class %s", e.ID, e.POS)
		}
		if e.POS == "verb" && e.Modern != "tadar" {
			t.Errorf("%s: verb formed as %s", e.ID, e.Modern)
		}
		head, _, ok := strings.Cut(e.Gloss, ".")
		switch {
		case ok && slices.Contains(heads, head) && strings.HasPrefix(e.Definition, strings.ToUpper(head[:1])+head[1:]+", as in"):
			senses++
		case !ok && slices.Contains(heads, head):
			homonyms++
		case slices.Contains(pool, head) || slices.Contains(heads, head):
			t.Errorf("%s: made-up gloss %q is a real word", e.ID, e.Gloss)
		}
	}
	// About 30% senses and 20% homonyms; allow sampling noise.
	if senses < 500 || senses > 700 {
		t.Errorf("%d new senses of real heads, want about 600", senses)
	}
	if homonyms < 320 || homonyms > 480 {
		t.Errorf("%d homonyms of real heads, want about 400", homonyms)
	}
	if !slices.EqualFunc(es, gen(0.3), func(a, b *lexicon.Entry) bool {
		return a.ID == b.ID && a.Gloss == b.Gloss && a.Definition == b.Definition && a.POS == b.POS
	}) {
		t.Error("same seed, different entries")
	}
}
