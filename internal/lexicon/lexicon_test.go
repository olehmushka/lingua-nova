package lexicon

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/olehmushka/lingua-nova/internal/phon"
)

const langDir = "../../language"

func loadPhon(t *testing.T) *phon.Inventory {
	t.Helper()
	inv, err := phon.Load(filepath.Join(langDir, "phonology.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return inv
}

// TestRealLexicon: the lexicon is a test fixture, and it must be clean.
func TestRealLexicon(t *testing.T) {
	lex, err := Load(filepath.Join(langDir, "lexicon"))
	if err != nil {
		t.Fatal(err)
	}
	if len(lex.Entries) < 100 {
		t.Errorf("loaded %d entries, want at least 100", len(lex.Entries))
	}
	for _, p := range Validate(lex, loadPhon(t)) {
		t.Error(p)
	}
}

// ok is a valid noun entry; tests start from it and break one thing.
const ok = `- id: water
  proto: acia
  modern: agua
  pos: noun
  gender: f
  gloss: water
  definition: Water.
`

func TestValidate(t *testing.T) {
	inv := loadPhon(t)
	tests := []struct {
		name string
		yaml string
		want []string // full Problem strings, in order; nil = valid
	}{
		{"valid", ok, nil},
		{"valid verb", "- {id: see, proto: narar, modern: narar, pos: verb, gloss: see, definition: See.}\n", nil},
		{"valid word override", strings.Replace(ok, "  gloss:", "  word: aga\n  gloss:", 1), nil},
		{"missing fields", "- id: x\n  pos: adverb\n", []string{
			"t.yaml:1: x: missing proto",
			"t.yaml:1: x: missing modern",
			"t.yaml:1: x: missing gloss",
			"t.yaml:1: x: missing definition",
		}},
		{"duplicate id", ok + ok, []string{
			"t.yaml:8: water: duplicate id (first defined at t.yaml:1)",
		}},
		{"unknown field", strings.Replace(ok, "  gloss:", "  glos: x\n  gloss:", 1), []string{
			`t.yaml:6: water: unknown field "glos"`,
		}},
		{"unknown pos", strings.Replace(strings.Replace(ok, "noun", "nown", 1), "  gender: f\n", "", 1), []string{
			`t.yaml:4: water: unknown pos "nown" (want one of adjective, adverb, article, complementizer, conjunction, determiner, noun, numeral, particle, preposition, pronoun, verb)`,
		}},
		{"noun without gender", strings.Replace(ok, "  gender: f\n", "", 1), []string{
			"t.yaml:4: water: noun without gender",
		}},
		{"bad gender", strings.Replace(ok, "gender: f", "gender: n", 1), []string{
			`t.yaml:5: water: gender "n": want m or f`,
		}},
		{"gender on non-noun", strings.Replace(ok, "pos: noun", "pos: adjective", 1), []string{
			"t.yaml:5: water: gender on a adjective; only nouns and pronouns have gender",
		}},
		{"gender on pronoun", strings.Replace(ok, "pos: noun", "pos: pronoun", 1), nil},
		{"bad gender on pronoun", strings.Replace(strings.Replace(ok, "pos: noun", "pos: pronoun", 1), "gender: f", "gender: x", 1), []string{
			`t.yaml:5: water: gender "x": want m or f`,
		}},
		{"verb ending", "- {id: v, proto: naro, modern: naro, pos: verb, gloss: v, definition: V.}\n", []string{
			`t.yaml:1: v: verb proto "naro" must end in -ar, -er or -ir`,
			`t.yaml:1: v: verb modern "naro" must end in -ar, -er or -ir`,
		}},
		{"palatal in proto", strings.Replace(ok, "proto: acia", "proto: axia", 1), []string{
			`t.yaml:2: water: proto *axia: "x" /ʃ/ is not a proto-language sound`,
		}},
		{"forbidden in proto", strings.Replace(ok, "proto: acia", "proto: aiuca", 1), []string{
			`t.yaml:2: water: proto *aiuca: forbidden sequence [a i u] (matches [V V V])`,
		}},
		{"geminate allowed in proto", strings.Replace(ok, "proto: acia", "proto: accia", 1), nil},
		{"final nasal allowed in proto", strings.Replace(ok, "proto: acia", "proto: acian", 1), nil},
		{"bad coda in proto", strings.Replace(ok, "proto: acia", "proto: ampia", 1), []string{
			`t.yaml:2: water: proto *ampia: illegal coda "m" in am.pi.a`,
		}},
		{"hiatus in proto", strings.Replace(ok, "proto: acia", "proto: acua", 1), []string{
			`t.yaml:2: water: proto *acua: hiatus "u.a" (only after i) in a.cu.a`,
		}},
		{"bad geminate in proto", strings.Replace(ok, "proto: acia", "proto: assia", 1), []string{
			`t.yaml:2: water: proto *assia: double consonant "ss" in as.si.a`,
		}},
		{"bad final in proto", strings.Replace(ok, "proto: acia", "proto: acias", 1), []string{
			`t.yaml:2: water: proto *acias: illegal word-final consonant "s" in a.ci.as`,
		}},
		{"double consonant in modern", strings.Replace(ok, "modern: agua", "modern: aggua", 1), []string{
			`t.yaml:3: water: modern "aggua": double consonant "gg" in ag.gu.a`,
		}},
		{"bad coda in modern", strings.Replace(ok, "modern: agua", "modern: agpua", 1), []string{
			`t.yaml:3: water: modern "agpua": illegal coda "g" in ag.pu.a`,
		}},
		{"bad final in word", strings.Replace(ok, "  gloss:", "  word: agut\n  gloss:", 1), []string{
			`t.yaml:6: water: word "agut": illegal word-final consonant "t" in a.gut`,
		}},
		{"unknown letter", strings.Replace(ok, "modern: agua", "modern: aqua", 1), []string{
			`t.yaml:3: water: modern "aqua": "aqua": unknown letter 'q' at position 1`,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lex := &Lexicon{}
			if err := lex.ParseFile("t.yaml", []byte(tt.yaml)); err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, p := range Validate(lex, inv) {
				got = append(got, p.String())
			}
			if strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Errorf("got:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(tt.want, "\n  "))
			}
		})
	}
}

func TestParseFileErrors(t *testing.T) {
	tests := []struct{ yaml, want string }{
		{"id: x\n", "t.yaml:1: want a list of entries"},
		{"- x\n", "t.yaml:1: want an entry (a mapping)"},
		{"- id: [\n", "t.yaml: yaml:"},
	}
	for _, tt := range tests {
		err := (&Lexicon{}).ParseFile("t.yaml", []byte(tt.yaml))
		if err == nil || !strings.HasPrefix(err.Error(), tt.want) {
			t.Errorf("%q: got %v, want prefix %q", tt.yaml, err, tt.want)
		}
	}
}
