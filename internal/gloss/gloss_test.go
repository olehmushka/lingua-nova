package gloss

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/olehmushka/lingua-nova/internal/lexicon"
	"github.com/olehmushka/lingua-nova/internal/morph"
	"github.com/olehmushka/lingua-nova/internal/phon"
)

const langDir = "../../language"

func glosser(t *testing.T) *Glosser {
	t.Helper()
	inv, err := phon.Load(filepath.Join(langDir, "phonology.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := morph.Load(filepath.Join(langDir, "morphology.yaml"), inv)
	if err != nil {
		t.Fatal(err)
	}
	lex, err := lexicon.Load(filepath.Join(langDir, "lexicon"))
	if err != nil {
		t.Fatal(err)
	}
	return New(morph.NewAnalyzer(r, lex))
}

// lines are the two gloss lines with single spaces, for comparing with
// hand-aligned text.
func lines(r *Result) (string, string) {
	l := strings.SplitN(r.String(), "\n", 3)
	return strings.Join(strings.Fields(l[0]), " "), strings.Join(strings.Fields(l[1]), " ")
}

// TestGrammarExamples glosses every interlinear example in grammar.md: a
// code block of segmented line, gloss line, "translation" (see Examples).
func TestGrammarExamples(t *testing.T) {
	g := glosser(t)
	md, err := os.ReadFile(filepath.Join(langDir, "grammar.md"))
	if err != nil {
		t.Fatal(err)
	}
	examples := Examples(string(md))
	if len(examples) < 5 {
		t.Fatalf("found %d examples in grammar.md, want at least 5", len(examples))
	}
	for _, ex := range examples {
		wantSeg, wantGloss := ex.Segmented, ex.Gloss
		res := g.Gloss(ex.Sentence())
		seg, gloss := lines(res)
		if seg != wantSeg || gloss != wantGloss {
			t.Errorf("Gloss(%q) =\n  %s\n  %s\nwant\n  %s\n  %s", ex.Sentence(), seg, gloss, wantSeg, wantGloss)
		}
		if rs := res.Remarks(); len(rs) > 0 {
			t.Errorf("Gloss(%q): unexpected remarks %q", wantSeg, rs)
		}
	}
}

func TestGloss(t *testing.T) {
	g := glosser(t)
	tests := []struct {
		sentence   string
		seg, gloss string
		remarks    []string
	}{
		// Case by position: the same form, read by where it stands.
		{"Sela naraxe.", "Sela nar-a-x-e.", "2SG.POL see-TH-PST-3SG",
			[]string{"sela: removed 2SG.POL.OBJ (OBJ before the verb)"}},
		{"Mu naraxu sela.", "Mu nar-a-x-u sela.", "1SG see-TH-PST-1SG 2SG.POL.OBJ",
			[]string{"sela: removed 2SG.POL (SBJ after the verb)"}},
		{"Selah narah?", "Selah nar-a-h?", "2PL.POL see-TH-3PL",
			[]string{"selah: removed 2PL.POL.OBJ (OBJ before the verb)"}},
		// Agreement: one reading left, so it stays, with a note.
		{"Mu nara.", "Mu nar-a.", "1SG see-TH.3SG",
			[]string{"mu (1SG) doesn't agree with nara (3SG)"}},
		// Polite sela takes a 3rd-person verb, not a 2nd.
		{"Sela naras.", "Sela nar-a-s.", "2SG.POL see-TH-2SG",
			[]string{"sela: removed 2SG.POL.OBJ (OBJ before the verb)", "sela (3SG) doesn't agree with naras (2SG)"}},
		{"Lu menah narah.", "Lu mena-h nar-a-h.", "the woman-PL see-TH-3PL", nil},
		{"Lu mena lirma nara.", "Lu mena lirma nar-a.", "the woman white see-TH.3SG", nil}, // adjective skipped
		{"Lu menas huro nara.", "Lu mena-s huro nar-a.", "the woman-POSS dog see-TH.3SG", nil},
		// A question word or zunhu as the subject takes a 3SG verb, but only
		// with an object: without one it can be a fronted object (whom did
		// you see?).
		{"Ifa naraxe lu dubroh?", "Ifa nar-a-x-e lu dubro-h?", "who see-TH-PST-3SG the man-PL", nil},
		{"Ifa naraxis lu dubroh?", "Ifa nar-a-x-is lu dubro-h?", "who see-TH-PST-2SG the man-PL",
			[]string{"ifa (3SG) doesn't agree with naraxis (2SG)"}},
		{"Ifa na naraxis lu dubroh?", "Ifa na nar-a-x-is lu dubro-h?", "who NEG see-TH-PST-2SG the man-PL",
			[]string{"ifa (3SG) doesn't agree with naraxis (2SG)"}},
		{"Ifa naraxis?", "Ifa nar-a-x-is?", "who see-TH-PST-2SG", nil},
		{"Itu naraxis?", "Itu nar-a-x-is?", "what see-TH-PST-2SG", nil},
		{"Itu chu naraxis?", "Itu chu nar-a-x-is?", "what 2SG see-TH-PST-2SG", nil},
		{"Zunhu zuvixe.", "Zunhu zuv-i-x-e.", "nothing fall-TH-PST-3SG", nil},
		// Pro-drop, and a clause without a verb: no subject, no note.
		{"Naraxu chi.", "Nar-a-x-u chi.", "see-TH-PST-1SG 2SG.OBJ", nil},
		{"Na.", "Na.", "NEG", nil},
		// Clauses: each answer is its own clause.
		{"Naraxis honi? — Naraxu.", "Nar-a-x-is honi? — Nar-a-x-u.", "see-TH-PST-2SG 3SG.M.OBJ see-TH-PST-1SG", nil},
		// A rule that can't remove the only reading: the sentence is wrong.
		{"Naraxis hono?", "Nar-a-x-is hono?", "see-TH-PST-2SG 3SG.M", []string{"hono is SBJ after the verb"}},
		{"Mi naraxu.", "Mi nar-a-x-u.", "1SG.OBJ see-TH-PST-1SG", []string{"mi is OBJ before the verb"}},
		{"Mu nara naraxu.", "Mu nar-a nar-a-x-u.", "1SG see-TH.3SG see-TH-PST-1SG",
			[]string{"naraxu is a second verb in the clause", "mu (1SG) doesn't agree with nara (3SG)"}},
		{"Mu na naraxu sela, na.", "Mu na nar-a-x-u sela, na.", "1SG NEG see-TH-PST-1SG 2SG.POL.OBJ NEG",
			[]string{"sela: removed 2SG.POL (SBJ after the verb)"}},
		// Unknown roots: the best guess, and a guess is never the verb when
		// a known verb is there.
		{"Mu pumaxu.", "Mu pum-a-x-u.", "1SG ?-TH-PST-1SG",
			[]string{"pumaxu: root not in the lexicon, guessed ?pumar (4 other guesses)"}},
		{"Lu pumaxu nara.", "Lu pumaxu nar-a.", "the ? see-TH.3SG",
			[]string{"pumaxu: root not in the lexicon, guessed ?pumaxu",
				"pumaxu: removed ?pumar PST.1SG (a second verb in the clause)",
				"pumaxu: removed ?pumaxar PRS.1SG (a second verb in the clause)",
				"pumaxu: removed ?pumaxer PRS.1SG (a second verb in the clause)",
				"pumaxu: removed ?pumaxir PRS.1SG (a second verb in the clause)",
			}},
		{"Mu xxxx.", "Mu xxxx.", "1SG ???", []string{"xxxx: no analysis"}},
		// A word with no reading isn't "a second verb" or "SBJ after the verb".
		{"Lu mena nara lu blomp.", "Lu mena nar-a lu blomp.", "the woman see-TH.3SG the ???", []string{"blomp: no analysis"}},
		{"Mu kato.", "Mu kato.", "1SG ???", []string{`"kato": unknown letter 'k' at position 0`}},
		{"", "", "", nil},
	}
	for _, tt := range tests {
		res := g.Gloss(tt.sentence)
		seg, gloss := lines(res)
		if seg != tt.seg || gloss != tt.gloss {
			t.Errorf("Gloss(%q) =\n  %s\n  %s\nwant\n  %s\n  %s", tt.sentence, seg, gloss, tt.seg, tt.gloss)
		}
		if rs := res.Remarks(); !slices.Equal(rs, tt.remarks) {
			t.Errorf("Gloss(%q) remarks =\n  %s\nwant\n  %s", tt.sentence, strings.Join(rs, "\n  "), strings.Join(tt.remarks, "\n  "))
		}
	}
}

// TestString checks the column layout: each column is as wide as the
// longer of its two lines, plus two spaces.
func TestString(t *testing.T) {
	got := glosser(t).Gloss("Lu mena nara lu trubo.").String()
	want := "Lu   mena   nar-a       lu   trubo.\nthe  woman  see-TH.3SG  the  river\n"
	if got != want {
		t.Errorf("String() =\n%s\nwant\n%s", got, want)
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		in   string
		want []string // lead|text|punct
	}{
		{"Lu mena nara.", []string{"|Lu|", "|mena|", "|nara|."}},
		{"Naraxis hono? — Naraxu.", []string{"|Naraxis|", "|hono|?", "— |Naraxu|."}},
		{"«Na», na!", []string{"«|Na|»,", "|na|!"}},
		{"  mu   nara  ", []string{"|mu|", "|nara|"}},
		{"nara .", []string{"|nara| ."}},
		{"", nil},
	}
	for _, tt := range tests {
		var got []string
		for _, w := range Tokenize(tt.in) {
			got = append(got, w.Lead+"|"+w.Text+"|"+w.Punct)
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("Tokenize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestClauses(t *testing.T) {
	g := glosser(t)
	g.Wh = map[string]bool{"who": true, "what": true}
	str := func(ts []Tok) string {
		var out []string
		for _, t := range ts {
			out = append(out, t.ID+":"+t.Feats)
		}
		return strings.Join(out, " ")
	}
	one := func(t *Tok) string {
		if t == nil {
			return "-"
		}
		return str([]Tok{*t})
	}
	tests := []struct {
		sentence string
		want     []string // per clause: wh | subject | neg | verb | object
	}{
		{"Lu mena nara lu trubo.", []string{"- | the: woman:SG | - | see:PRS.3SG | the: river:SG"}},
		{"Ifa naraxe lu dubroh?", []string{"who: |  | - | see:PST.3SG | the: man:PL"}},
		{"Itu chu naraxis?", []string{"what: | thou:SG.SBJ | - | see:PST.2SG | "}},
		{"Mu na naraxu zunhu.", []string{"- | i:SG.SBJ | not: | see:PST.1SG | nothing:"}},
		// No finite verb: everything is Subject.
		{"Huro baugir lu gurna.", []string{"- | dog:SG eat:INF the: bread:SG | - | - | "}},
		// na after the verb lands in the object.
		{"Hana nara na lu hoida.", []string{"- | she:SG.SBJ | - | see:PRS.3SG | not: the: house:SG"}},
		{"Naraxis honi? — Naraxu.", []string{"- |  | - | see:PST.2SG | he:SG.OBJ", "- |  | - | see:PST.1SG | "}},
	}
	for _, tt := range tests {
		var got []string
		for _, c := range g.Gloss(tt.sentence).Clauses() {
			got = append(got, strings.Join([]string{one(c.Wh), str(c.Subject), one(c.Neg), one(c.Verb), str(c.Object)}, " | "))
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("Clauses(%q) =\n  %s\nwant\n  %s", tt.sentence, strings.Join(got, "\n  "), strings.Join(tt.want, "\n  "))
		}
	}
}
