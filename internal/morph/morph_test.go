package morph

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/olehmushka/lingua-nova/internal/lexicon"
	"github.com/olehmushka/lingua-nova/internal/phon"
)

const langDir = "../../language"

func load(t *testing.T) (*Rules, *lexicon.Lexicon) {
	t.Helper()
	inv, err := phon.Load(filepath.Join(langDir, "phonology.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := Load(filepath.Join(langDir, "morphology.yaml"), inv)
	if err != nil {
		t.Fatal(err)
	}
	lex, err := lexicon.Load(filepath.Join(langDir, "lexicon"))
	if err != nil {
		t.Fatal(err)
	}
	return r, lex
}

func entry(t *testing.T, lex *lexicon.Lexicon, id string) *lexicon.Entry {
	t.Helper()
	for _, e := range lex.Entries {
		if e.ID == id {
			return e
		}
	}
	t.Fatalf("no entry %q", id)
	return nil
}

// TestInflect checks the forms given in grammar.md.
func TestInflect(t *testing.T) {
	r, lex := load(t)
	tests := []struct {
		id, feats         string
		word, seg, glossd string
	}{
		// Present: 1sg has no theme vowel; 3sg fuses theme and person.
		{"see", "PRS 1SG", "naru", "nar-u", "see-1SG"},
		{"see", "PRS 2SG", "naras", "nar-a-s", "see-TH-2SG"},
		{"see", "PRS 3SG", "nara", "nar-a", "see-TH.3SG"},
		{"see", "PRS 1PL", "naramu", "nar-a-mu", "see-TH-1PL"},
		{"see", "PRS 2PL", "naradi", "nar-a-di", "see-TH-2PL"},
		{"see", "PRS 3PL", "narah", "nar-a-h", "see-TH-3PL"},
		{"know", "PRS 1SG", "gilvu", "gilv-u", "know-1SG"},
		{"know", "PRS 2SG", "gilves", "gilv-e-s", "know-TH-2SG"},
		{"know", "PRS 1PL", "gilvemu", "gilv-e-mu", "know-TH-1PL"},
		// Class III: e in the singular and 3pl, i in 1pl/2pl.
		{"sleep", "PRS 1SG", "silu", "sil-u", "sleep-1SG"},
		{"sleep", "PRS 2SG", "siles", "sil-e-s", "sleep-TH-2SG"},
		{"sleep", "PRS 3SG", "sile", "sil-e", "sleep-TH.3SG"},
		{"sleep", "PRS 1PL", "silimu", "sil-i-mu", "sleep-TH-1PL"},
		{"sleep", "PRS 2PL", "silidi", "sil-i-di", "sleep-TH-2PL"},
		{"sleep", "PRS 3PL", "sileh", "sil-e-h", "sleep-TH-3PL"},
		// Past and future: the infinitive's vowel, then x or r.
		{"see", "PST 1SG", "naraxu", "nar-a-x-u", "see-TH-PST-1SG"},
		{"see", "PST 2SG", "naraxis", "nar-a-x-is", "see-TH-PST-2SG"},
		{"know", "PST 3SG", "gilvexe", "gilv-e-x-e", "know-TH-PST-3SG"},
		{"sleep", "PST 3PL", "silixeh", "sil-i-x-eh", "sleep-TH-PST-3PL"},
		{"see", "FUT 1SG", "nararu", "nar-a-r-u", "see-TH-FUT-1SG"},
		{"sleep", "FUT 3SG", "silire", "sil-i-r-e", "sleep-TH-FUT-3SG"},
		{"drink", "FUT 3PL", "pulvareh", "pulv-a-r-eh", "drink-TH-FUT-3PL"},
		{"see", "1sg pst", "naraxu", "nar-a-x-u", "see-TH-PST-1SG"}, // any order and case
		// Nouns: -h/-eh and -s/-es by the last sound.
		{"house", "", "hoida", "hoida", "house"},
		{"house", "PL", "hoidah", "hoida-h", "house-PL"},
		{"sea", "PL", "zahoreh", "zahor-eh", "sea-PL"},
		{"woman", "POSS", "menas", "mena-s", "woman-POSS"},
		{"sea", "POSS", "zahores", "zahor-es", "sea-POSS"},
		{"woman", "POSS PL", "menahes", "mena-h-es", "woman-PL-POSS"}, // slot order
		{"man", "PL", "dubroh", "dubro-h", "man-PL"},
		// Pronouns: whole forms.
		{"i", "", "mu", "mu", "1SG"},
		{"thou", "OBJ", "chi", "chi", "2SG.OBJ"},
		{"i", "POSS", "me", "me", "1SG.POSS"},
		{"she", "", "hana", "hana", "3SG.F"},
		{"she", "PL OBJ", "hanih", "hanih", "3PL.F.OBJ"},
		{"you_polite", "PL POSS", "selahes", "selahes", "2PL.POL.POSS"},
	}
	for _, tt := range tests {
		t.Run(tt.id+" "+tt.feats, func(t *testing.T) {
			f, err := r.Inflect(entry(t, lex, tt.id), strings.Fields(tt.feats)...)
			if err != nil {
				t.Fatal(err)
			}
			if got := [3]string{f.Word(), f.Segmented(), f.Gloss()}; got != [3]string{tt.word, tt.seg, tt.glossd} {
				t.Errorf("got %q, want %q", got, [3]string{tt.word, tt.seg, tt.glossd})
			}
		})
	}
}

func TestInflectErrors(t *testing.T) {
	r, lex := load(t)
	tests := []struct{ id, feats, want string }{
		{"see", "PST", "needs a tense"},
		{"see", "PST FUT 1SG", `unexpected feature "FUT"`},
		{"see", "PST 1SG PL", `unexpected feature "PL"`},
		{"house", "PST", `unexpected feature "PST"`},
		{"house", "PL PL", `unexpected feature "PL"`},
		{"it", "PL", "no PL form"},
		{"who", "", "no pronoun table"},
		{"water", "PL", ""}, // control: works
	}
	for _, tt := range tests {
		t.Run(tt.id+" "+tt.feats, func(t *testing.T) {
			_, err := r.Inflect(entry(t, lex, tt.id), strings.Fields(tt.feats)...)
			switch {
			case tt.want == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
				t.Fatalf("error %v, want %q", err, tt.want)
			}
		})
	}
}

// render shows a table as "row: cell cell; row: …", with - for a missing cell.
func render(tb *Table) string {
	var rows []string
	for _, row := range tb.Rows {
		cells := []string{row.Label + ":"}
		for _, c := range row.Cells {
			if c == nil {
				cells = append(cells, "-")
			} else {
				cells = append(cells, c.Word())
			}
		}
		rows = append(rows, strings.Join(cells, " "))
	}
	return strings.Join(tb.Cols, ",") + " | " + strings.Join(rows, "; ")
}

func TestParadigm(t *testing.T) {
	r, lex := load(t)
	tests := []struct{ id, want string }{
		{"see", "PRS,PST,FUT | 1SG: naru naraxu nararu; 2SG: naras naraxis nararis; 3SG: nara naraxe narare; " +
			"1PL: naramu naraxemu nararemu; 2PL: naradi naraxedi nararedi; 3PL: narah naraxeh narareh"},
		{"sea", ",POSS | SG: zahor zahores; PL: zahoreh zahorehes"},
		{"he", "SBJ,OBJ,POSS | SG: hono honi hone; PL: honoh honih honeh"},
		{"it", "SBJ,OBJ,POSS | SG: hunu huni hune; PL: - - -"},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			tb, err := r.Paradigm(entry(t, lex, tt.id))
			if err != nil {
				t.Fatal(err)
			}
			if got := render(tb); got != tt.want {
				t.Errorf("got\n  %s\nwant\n  %s", got, tt.want)
			}
		})
	}
	for _, id := range []string{"who", "white"} { // pronoun without a table, adjective
		if tb, err := r.Paradigm(entry(t, lex, id)); tb != nil || err != nil {
			t.Errorf("%s: got table %v, err %v; want none", id, tb, err)
		}
	}
}

// TestLexiconInflects inflects every word in the lexicon: all forms must
// obey the modern phonotactics.
func TestLexiconInflects(t *testing.T) {
	r, lex := load(t)
	for _, p := range Check(lex, r) {
		t.Error(p)
	}
}

func TestParseErrors(t *testing.T) {
	inv, err := phon.Load(filepath.Join(langDir, "phonology.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	const verb = `
verb:
  persons: [1SG, 2SG]
  classes: [{name: I, infinitive: ar, theme: a}]
  tenses:
`
	tests := []struct{ name, yaml, want string }{
		{"unknown key", "verbs: {}", "field verbs not found"},
		{"missing person", verb + "    - {name: PRS, endings: {1SG: u}}", "no ending for 2SG"},
		{"unknown person", verb + "    - {name: PRS, endings: {1SG: u, 2SG: s, 3SG: a}}", `unknown person "3SG"`},
		{"unknown class", verb + "    - {name: PRS, endings: {1SG: u, 2SG: s}, theme: {II: {1SG: e}}}", `unknown class "II"`},
		{"pronoun row", verb + "    - {name: PRS, endings: {1SG: u, 2SG: s}}\npronoun:\n  i: {person: '1', SG: [mu, mi]}", "need 3 forms"},
		{"ok", verb + "    - {name: PRS, endings: {1SG: u, 2SG: s}}", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.yaml), inv)
			switch {
			case tt.want == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
				t.Fatalf("error %v, want %q", err, tt.want)
			}
		})
	}
}

// TestCheck runs Check on a made-up lexicon and rules, so each kind of
// problem shows up once.
func TestCheck(t *testing.T) {
	inv, err := phon.Load(filepath.Join(langDir, "phonology.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	// 1SG -hu glues onto stem pul as pulhu = p·u·lh·u; 2SG -nt ends in an
	// illegal final cluster.
	r, err := Parse([]byte(`
verb:
  persons: [1SG, 2SG]
  classes: [{name: I, infinitive: ar, theme: a}]
  tenses: [{name: PRS, endings: {1SG: hu, 2SG: nt}, no_theme: [1SG]}]
pronoun:
  i: {person: "1", SG: [mu, mi, me]}
  ghost: {person: "3", SG: [ba, bi, be]}
`), inv)
	if err != nil {
		t.Fatal(err)
	}
	r.File = "morphology.yaml"
	lex := &lexicon.Lexicon{Entries: []*lexicon.Entry{
		{ID: "drink", Modern: "pular", POS: "verb", Gloss: "drink", File: "verbs.yaml", Line: 1},
		{ID: "i", Modern: "mo", POS: "pronoun", Gloss: "I", File: "pronouns.yaml", Line: 1},
		{ID: "bad", Modern: "nax", POS: "verb", Gloss: "x", File: "verbs.yaml", Line: 9}, // Validate reports it
	}}
	var got []string
	for _, p := range Check(lex, r) {
		got = append(got, p.String())
	}
	want := []string{
		`morphology.yaml:8: ghost: pronoun table has no pronoun entry in the lexicon`,
		`pronouns.yaml:1: i: morphology.yaml subject form "mu" differs from "mo"`,
		`verbs.yaml:1: drink: PRS.1SG pulhu (pul-hu): spelling reads as p·u·lh·u across a morpheme boundary`,
		`verbs.yaml:1: drink: PRS.2SG pulant (pul-a-nt): `,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d problems, want %d:\n%s", len(got), len(want), strings.Join(got, "\n"))
	}
	for i := range want {
		if !strings.HasPrefix(got[i], want[i]) {
			t.Errorf("problem %d:\n  got  %s\n  want %s…", i, got[i], want[i])
		}
	}
}
