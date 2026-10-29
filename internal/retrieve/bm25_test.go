package retrieve

import (
	"slices"
	"testing"

	"github.com/olehmushka/lingua-nova/internal/lexicon"
)

func TestStem(t *testing.T) {
	tests := []struct{ in, want string }{
		{"eyes", "eye"}, {"houses", "house"}, {"berries", "berry"}, {"dog", "dog"},
		{"glass", "glass"}, {"virus", "virus"}, {"goes", "goe"}, {"toes", "toe"},
		{"gas", "gas"}, {"series", "sery"},
	}
	for _, tt := range tests {
		if got := Stem(tt.in); got != tt.want {
			t.Errorf("Stem(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestTerms(t *testing.T) {
	got := Terms("kill.by.accident: To kill a man's dogs.")
	want := []string{"kill", "accident", "kill", "man", "s", "dog"}
	if !slices.Equal(got, want) {
		t.Errorf("Terms = %q, want %q", got, want)
	}
}

var entries = []*lexicon.Entry{
	{ID: "water", POS: "noun", Gloss: "water", Definition: "Water; any liquid."},
	{ID: "river", POS: "noun", Gloss: "river", Definition: "A river; flowing water."},
	{ID: "large", POS: "adjective", Gloss: "large", Definition: "Large; big."},
	{ID: "big_house", POS: "noun", Gloss: "big.house", Definition: "A hall."},
	{ID: "drink", POS: "verb", Gloss: "drink", Definition: "To drink water or any liquid."},
	{ID: "eye", POS: "noun", Gloss: "eye", Definition: "The eye."},
	{ID: "see", POS: "verb", Gloss: "see", Definition: "To see with the eyes."},
}

func ids(hs []Hit) []string {
	var out []string
	for _, h := range hs {
		out = append(out, h.Entry.ID)
	}
	return out
}

func TestBM25Search(t *testing.T) {
	x := NewBM25(entries)
	tests := []struct {
		name  string
		query string
		k     int
		keep  func(*lexicon.Entry) bool
		want  []string
	}{
		// The gloss match wins; then the shorter definition (river) over
		// the longer one (drink).
		{"gloss first", "water", 0, nil, []string{"water", "river", "drink"}},
		{"k", "water", 1, nil, []string{"water"}},
		// big is only in a gloss as a modifier and in large's definition:
		// the gloss weight puts big.house first.
		{"definition match", "big", 0, nil, []string{"big_house", "large"}},
		{"plural in definition", "eye", 0, nil, []string{"eye", "see"}},
		{"keep", "water", 0, func(e *lexicon.Entry) bool { return e.POS == "verb" }, []string{"drink"}},
		{"stopwords only", "the of a", 0, nil, nil},
		{"no match", "goat", 0, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ids(x.Search(tt.query, tt.k, tt.keep)); !slices.Equal(got, tt.want) {
				t.Errorf("Search(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// Rarer terms count for more (idf): an entry matching "goat" beats one
// matching "animal", which many entries have.
func TestBM25IDF(t *testing.T) {
	es := []*lexicon.Entry{
		{ID: "a1", Gloss: "x", Definition: "animal"},
		{ID: "a2", Gloss: "y", Definition: "animal"},
		{ID: "a3", Gloss: "z", Definition: "animal"},
		{ID: "g", Gloss: "w", Definition: "goat"},
	}
	got := ids(NewBM25(es).Search("animal goat", 0, nil))
	if got[0] != "g" {
		t.Errorf("Search = %v, want g first", got)
	}
}
