package morph

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// show is one analysis in one line: see PST.1SG nar-a-x-u, ?pumar for a guess.
func show(a Analysis) string {
	lemma := a.Lemma
	if a.Guess() {
		lemma = "?" + lemma
	}
	return strings.Join(strings.Fields(fmt.Sprintf("%s %s %s %s", lemma, a.Class, a.Form.Label(), a.Form.Segmented())), " ")
}

func TestAnalyze(t *testing.T) {
	r, lex := load(t)
	a := NewAnalyzer(r, lex)
	tests := []struct {
		word string
		want []string // every analysis, in rank order
	}{
		{"naraxu", []string{"narar I PST.1SG nar-a-x-u"}},
		{"nara", []string{"narar I PRS.3SG nar-a"}},                // fusion: TH.3SG
		{"naru", []string{"narar I PRS.1SG nar-u"}},                // no theme vowel
		{"gilves", []string{"gilver II PRS.2SG gilv-e-s"}},         // class II
		{"siles", []string{"silir III PRS.2SG sil-e-s"}},           // class III theme override
		{"silimu", []string{"silir III PRS.1PL sil-i-mu"}},         // ...and the class theme elsewhere
		{"NARAXU", []string{"narar I PST.1SG nar-a-x-u"}},          // case-insensitive
		{"narar", []string{"narar I INF nar-ar"}},                  // infinitive
		{"exa", []string{"exa SG exa"}},                            // bare noun is the SG cell
		{"exahes", []string{"exa PL.POSS exa-h-es"}},               // both suffixes, slot order
		{"menahes", []string{"mena PL.POSS mena-h-es"}},            // -es after plural -h
		{"hani", []string{"hana SG.OBJ hani"}},                     // suppletion: table lookup
		{"sela", []string{"sela SG.SBJ sela", "sela SG.OBJ sela"}}, // ambiguous
		{"lirma", []string{"lirma lirma"}},                         // adjective: no features
		// Guesses: unknown roots, most morphemes first.
		{"pumaxu", []string{
			"?pumar I PST.1SG pum-a-x-u",
			"?pumaxar I PRS.1SG pumax-u",
			"?pumaxer II PRS.1SG pumax-u",
			"?pumaxir III PRS.1SG pumax-u",
			"?pumaxu SG pumaxu",
		}},
		{"pumar", []string{"?pumar I INF pum-ar", "?pumar SG pumar"}},
		{"xxxx", nil}, // no reading obeys the phonotactics
	}
	for _, tt := range tests {
		got, err := a.Analyze(tt.word)
		if err != nil {
			t.Errorf("Analyze(%q): %v", tt.word, err)
			continue
		}
		var lines []string
		for _, an := range got {
			lines = append(lines, show(an))
		}
		if !slices.Equal(lines, tt.want) {
			t.Errorf("Analyze(%q) =\n  %s\nwant\n  %s", tt.word, strings.Join(lines, "\n  "), strings.Join(tt.want, "\n  "))
		}
	}
}

func TestAnalyzeErrors(t *testing.T) {
	r, lex := load(t)
	a := NewAnalyzer(r, lex)
	for _, w := range []string{"", "naraq", "nara1"} {
		if _, err := a.Analyze(w); err == nil {
			t.Errorf("Analyze(%q): no error", w)
		}
	}
}

// TestAnalyzeRoundTrip runs every form of every paradigm in the lexicon
// back through the analyzer: the generator is the analyzer's test oracle.
func TestAnalyzeRoundTrip(t *testing.T) {
	r, lex := load(t)
	a := NewAnalyzer(r, lex)
	n := 0
	for _, e := range lex.Entries {
		tbl, err := r.Paradigm(e)
		if err != nil || tbl == nil {
			continue
		}
		for _, row := range tbl.Rows {
			for _, f := range row.Cells {
				if f == nil {
					continue
				}
				n++
				got, err := a.Analyze(f.Word())
				if err != nil {
					t.Errorf("%s %s %s: %v", e.ID, f.Label(), f.Word(), err)
					continue
				}
				if !slices.ContainsFunc(got, func(an Analysis) bool {
					return an.Entry == e && an.Form.Label() == f.Label() && an.Form.Segmented() == f.Segmented()
				}) {
					t.Errorf("%s %s %s: not among its analyses %v", e.ID, f.Label(), f.Word(), got)
				}
				for _, an := range got {
					if an.Guess() {
						t.Errorf("%s %s %s: guess %s next to a known root", e.ID, f.Label(), f.Word(), show(an))
					}
				}
			}
		}
	}
	if n < 2000 {
		t.Errorf("only %d forms checked", n)
	}
}
