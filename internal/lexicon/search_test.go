package lexicon

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"
)

func TestSearch(t *testing.T) {
	lex, err := Load(filepath.Join(langDir, "lexicon"))
	if err != nil {
		t.Fatal(err)
	}
	// hits formats results as "id field match" so a failure shows the list.
	hits := func(q string) []string {
		var out []string
		for _, h := range lex.Search(q) {
			out = append(out, fmt.Sprintf("%s %s %s", h.Entry.ID, h.Field, h.Match))
		}
		return out
	}
	tests := []struct {
		q     string
		first []string // the first results, in order
		has   []string // also somewhere in the results
		not   []string // ids that must not appear
	}{
		{q: "water", first: []string{"water gloss exact"}, has: []string{"spring gloss word"}},
		{q: "exa", first: []string{"water modern exact"}},
		{q: "  EXA ", first: []string{"water modern exact"}},
		{q: "*esia", first: []string{"water proto exact"}},
		{q: "*exa", first: nil},
		{q: "accident", first: []string{"kill_accident gloss word"}},
		{q: "lie down", first: []string{"lie gloss exact"}},
		{q: "love", has: []string{"love_doing gloss word"}, not: nil},
		{q: "ate", not: []string{"water"}},
		{q: "", first: nil},
		{q: "*", first: nil},
	}
	for _, tt := range tests {
		got := hits(tt.q)
		if tt.first == nil && len(tt.has) == 0 && len(tt.not) == 0 {
			if len(got) > 0 {
				t.Errorf("Search(%q) = %v, want nothing", tt.q, got)
			}
			continue
		}
		if len(got) < len(tt.first) || !slices.Equal(got[:len(tt.first)], tt.first) {
			t.Errorf("Search(%q) starts %v, want %v", tt.q, got[:min(len(got), len(tt.first)+2)], tt.first)
		}
		for _, h := range tt.has {
			if !slices.Contains(got, h) {
				t.Errorf("Search(%q) = %v, want it to contain %q", tt.q, got, h)
			}
		}
		for _, id := range tt.not {
			for _, h := range lex.Search(tt.q) {
				if h.Entry.ID == id {
					t.Errorf("Search(%q) found %s (%s %s)", tt.q, id, h.Field, h.Match)
				}
			}
		}
	}
}

func TestSearchLoveFamily(t *testing.T) {
	lex, err := Load(filepath.Join(langDir, "lexicon"))
	if err != nil {
		t.Fatal(err)
	}
	var glosses []string
	for _, h := range lex.Search("lov") {
		if h.Match == MatchPrefix && h.Field == "gloss" {
			glosses = append(glosses, h.Entry.Gloss)
		}
	}
	for _, g := range []string{"love.agape", "love.eros", "love.philia", "love.storge", "love.doing"} {
		if !slices.Contains(glosses, g) {
			t.Errorf("Search(lov) gloss prefix hits = %v, missing %s", glosses, g)
		}
	}
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"Water; any liquid.": "water any liquid",
		"kill.by.accident":   "kill by accident",
		" *Esia ":            "*esia",
		"you.POL":            "you pol",
		"":                   "",
	} {
		if got := normalize(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}
