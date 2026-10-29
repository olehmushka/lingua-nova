package retrieve

import (
	"slices"
	"testing"

	"github.com/olehmushka/lingua-nova/internal/lexicon"
)

func entry(id string) *lexicon.Entry {
	return &lexicon.Entry{ID: id, Modern: id, POS: "noun", Gloss: id, Definition: "A thing."}
}

func pickIDs(ps []Pick) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Entry.ID)
	}
	return out
}

func TestTop(t *testing.T) {
	a, b, c, d := entry("a"), entry("b"), entry("c"), entry("d")
	tests := []struct {
		name  string
		lists [][]ranked
		k     int
		want  []string
	}{
		// b is second in both lists: agreement beats one first place.
		{"agreement", [][]ranked{{{e: a, rank: 0}, {e: b, rank: 1}}, {{e: c, rank: 0}, {e: b, rank: 1}}}, 3, []string{"b", "a", "c"}},
		{"k", [][]ranked{{{e: a, rank: 0}, {e: b, rank: 1}, {e: c, rank: 2}}}, 2, []string{"a", "b"}},
		// a tier: a, b, c tie in the first list; the second list's score
		// breaks the tie between b and c, which it ranks equal.
		{"tie on score", [][]ranked{
			{{e: a, rank: 0}, {e: b, rank: 0}, {e: c, rank: 0}},
			{{e: a, rank: 0, score: 9}, {e: c, rank: 1, score: 5}, {e: b, rank: 1, score: 4}},
		}, 3, []string{"a", "c", "b"}},
		{"depth", [][]ranked{{{e: a, rank: 0}, {e: d, rank: depth}}}, 3, []string{"a"}},
		// The same entry from two copies of a lexicon counts once, by id.
		{"by id", [][]ranked{{{e: a, rank: 0}}, {{e: entry("a"), rank: 0}}}, 3, []string{"a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pickIDs(top("w", tt.lists, tt.k)); !slices.Equal(got, tt.want) {
				t.Errorf("top = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBudget(t *testing.T) {
	cost := Tokens(entry("aa"))
	picks := []Pick{
		{Entry: entry("aa"), Word: "x", Rank: 0}, {Entry: entry("ab"), Word: "x", Rank: 1}, {Entry: entry("ac"), Word: "x", Rank: 2},
		{Entry: entry("ba"), Word: "y", Rank: 0}, {Entry: entry("bb"), Word: "y", Rank: 1},
	}
	tests := []struct {
		budget int
		want   []string
	}{
		{0, []string{"aa", "ab", "ac", "ba", "bb"}},
		// Room for 3: both best entries, then the best second.
		{3 * cost, []string{"aa", "ab", "ba"}},
		// Room for 1: every word still keeps its best.
		{cost, []string{"aa", "ba"}},
	}
	for _, tt := range tests {
		r := &Retriever{Budget: tt.budget}
		if got := pickIDs(r.budget(picks)); !slices.Equal(got, tt.want) {
			t.Errorf("budget %d: %v, want %v", tt.budget, got, tt.want)
		}
	}
}
