package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/olehmushka/lingua-nova/internal/lexicon"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		code   int
		stdout string // substring
		stderr string // substring
	}{
		{"ipa", []string{"ipa", "folha"}, 0, "f·o·lh·a   /ˈfo.ʎa/", ""},
		{"ipa proto", []string{"ipa", "*rifatia"}, 0, "*rifatia   r·i·f·a·t·i·a   /ɾi.faˈti.a/", ""},
		{"ipa bad letter", []string{"ipa", "kato", "lu"}, 1, "/lu/", `unknown letter 'k'`},
		{"ipa no vowel", []string{"ipa", "pr", "lu"}, 1, "/lu/", `"pr": word has no vowel`},
		{"ipa no words", []string{"ipa"}, 2, "", "usage: lnova ipa"},
		{"validate", []string{"validate"}, 0, "entries, 0 problems", ""},
		{"validate extra args", []string{"validate", "x"}, 2, "", "usage: lnova validate"},
		{"rules", []string{"rules"}, 0, "28   si > x / _V\n", ""},
		{"rules extra args", []string{"rules", "x"}, 2, "", "usage: lnova rules"},
		{"evolve", []string{"evolve"}, 0, "entries, 0 mismatches\n", ""},
		{"evolve extra args", []string{"evolve", "water"}, 2, "", "usage: lnova evolve"},
		{"evolve trace no args", []string{"evolve", "-trace"}, 2, "", "usage: lnova evolve"},
		{"evolve trace id", []string{"evolve", "--trace", "water"}, 0, "water: *esia → exa\n  28   si > x / _V   esia → exa\n", ""},
		{"evolve trace proto", []string{"evolve", "-trace", "*patta"}, 0, "*patta → pata\n  43   tt > t   patta → pata\n", ""},
		{"evolve trace geminate palatal", []string{"evolve", "-trace", "*luccio"}, 0, "*luccio → lucho\n", ""},
		{"evolve trace cluster palatal", []string{"evolve", "-trace", "*plia"}, 0, "*plia → lha\n", ""},
		{"evolve trace no change", []string{"evolve", "-trace", "*lirma"}, 0, "*lirma → lirma\n  (no rule applies)\n", ""},
		{"evolve trace unknown id", []string{"evolve", "-trace", "nope", "*lirma"}, 1, "*lirma → lirma", `no lexicon entry "nope"`},
		{"evolve trace bad letter", []string{"evolve", "-trace", "*kato"}, 1, "", `unknown letter 'k'`},
		{"gen", []string{"gen", "-n", "3", "-seed", "1"}, 0, "3 words, seed 1; rejected ", ""}, // words vary with the lexicon
		{"gen homophone", []string{"gen", "-n", "300", "-seed", "5"}, 0, " = ", ""},
		{"gen extra args", []string{"gen", "x"}, 2, "", "usage: lnova gen"},
		{"gen zero", []string{"gen", "-n", "0"}, 2, "", "usage: lnova gen"},
		{"inflect form", []string{"inflect", "see", "pst", "1sg"}, 0, "PST.1SG   naraxu   nar-a-x-u   see-TH-PST-1SG   /naˈɾa.ʃu/\n", ""},
		{"inflect verb paradigm", []string{"inflect", "see"}, 0, "PRS.3SG   nara       nar-a         see-TH.3SG       /ˈna.ɾa/\n", ""},
		{"inflect noun paradigm", []string{"inflect", "woman"}, 0, "PL.POSS   menahes   mena-h-es   woman-PL-POSS", ""},
		{"inflect missing form", []string{"inflect", "it"}, 0, "PL.SBJ    —\n", ""},
		{"inflect bad feature", []string{"inflect", "see", "PL"}, 1, "", `unexpected feature "PL"`},
		{"inflect unknown id", []string{"inflect", "nope"}, 1, "", `no lexicon entry "nope"`},
		{"inflect adjective", []string{"inflect", "white"}, 1, "", "white (adjective) doesn't inflect"},
		{"inflect no args", []string{"inflect"}, 2, "", "usage: lnova inflect"},
		{"no command", nil, 2, "", "usage: lnova"},
		{"unknown command", []string{"nope"}, 2, "", `unknown command "nope"`},
		{"bad lang dir", []string{"-lang", "/nonexistent", "ipa", "lu"}, 1, "", "no such file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := tt.args
			if len(args) == 0 || args[0] != "-lang" {
				args = append([]string{"-lang", "../../language"}, args...)
			}
			if code := run(args, &stdout, &stderr); code != tt.code {
				t.Errorf("exit code = %d, want %d (stderr: %s)", code, tt.code, stderr.String())
			}
			if !strings.Contains(stdout.String(), tt.stdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.stdout)
			}
			if !strings.Contains(stderr.String(), tt.stderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.stderr)
			}
		})
	}
}

// TestGen checks a batch: n word lines plus the summary, no proto form from
// the lexicon or repeated in the batch, the same output for the same seed.
func TestGen(t *testing.T) {
	gen := func(seed string) string {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"-lang", "../../language", "gen", "-n", "50", "-seed", seed}, &stdout, &stderr); code != 0 {
			t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
		}
		return stdout.String()
	}
	out := gen("7")
	if again := gen("7"); again != out {
		t.Errorf("same seed, different output:\n%s\n---\n%s", out, again)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 51 || !strings.HasPrefix(lines[50], "50 words, seed 7; rejected ") {
		t.Fatalf("want 50 words and a summary, got:\n%s", out)
	}
	lex, err := lexicon.Load("../../language/lexicon")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range lex.Entries {
		seen["*"+strings.TrimPrefix(e.Proto, "*")] = true
	}
	for _, l := range lines[:50] {
		proto := strings.Fields(l)[0]
		if seen[proto] {
			t.Errorf("%s is in the lexicon or generated twice", proto)
		}
		seen[proto] = true
		if l != strings.TrimRight(l, " ") {
			t.Errorf("trailing spaces in %q", l)
		}
	}
}

// TestValidateBroken runs validate on a copy of the phonology with one bad
// lexicon file.
func TestValidateBroken(t *testing.T) {
	dir := t.TempDir()
	phonology, err := os.ReadFile("../../language/phonology.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "phonology.yaml"), phonology, 0o644); err != nil {
		t.Fatal(err)
	}
	morphology := "verb:\n  persons: [1SG]\n  classes: [{name: I, infinitive: ar, theme: a}]\n  tenses: [{name: PRS, endings: {1SG: u}}]\n"
	if err := os.WriteFile(filepath.Join(dir, "morphology.yaml"), []byte(morphology), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "lexicon"), 0o755); err != nil {
		t.Fatal(err)
	}
	bad := "- id: see\n  proto: naxar\n  modern: naxo\n  pos: verb\n  gloss: see\n  definition: See.\n"
	file := filepath.Join(dir, "lexicon", "verbs.yaml")
	if err := os.WriteFile(file, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"-lang", dir, "validate"}, &stdout, &stderr); code != 1 {
		t.Errorf("exit code = %d, want 1 (stderr: %s)", code, stderr.String())
	}
	want := file + `:2: see: proto *naxar: "x" /ʃ/ is not a proto-language sound
` + file + `:3: see: verb modern "naxo" must end in -ar, -er or -ir
1 entries, 2 problems
`
	if stdout.String() != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", stdout.String(), want)
	}
}

// TestRulesBroken runs rules on a copy of the phonology with a bad rule file.
func TestRulesBroken(t *testing.T) {
	dir := t.TempDir()
	phonology, err := os.ReadFile("../../language/phonology.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "phonology.yaml"), phonology, 0o644); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "sound-changes.txt")
	if err := os.WriteFile(file, []byte("# rules\np > b / V_V\nk > g\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"-lang", dir, "rules"}, &stdout, &stderr); code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	want := file + `:3: target: unknown segment or category at "k"` + "\n"
	if stderr.String() != want {
		t.Errorf("stderr:\n%s\nwant:\n%s", stderr.String(), want)
	}
}

// TestEvolveBroken runs evolve on a copy of the phonology and rules with a
// lexicon entry whose modern form the rules don't produce.
func TestEvolveBroken(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"phonology.yaml", "sound-changes.txt"} {
		data, err := os.ReadFile(filepath.Join("../../language", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "lexicon"), 0o755); err != nil {
		t.Fatal(err)
	}
	lex := "- id: water\n  proto: esia\n  modern: esa\n  pos: noun\n  gender: f\n  gloss: water\n  definition: Water.\n" +
		"- id: white\n  proto: lirma\n  modern: lirma\n  pos: adjective\n  gloss: white\n  definition: White.\n"
	file := filepath.Join(dir, "lexicon", "words.yaml")
	if err := os.WriteFile(file, []byte(lex), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"-lang", dir, "evolve"}, &stdout, &stderr); code != 1 {
		t.Errorf("exit code = %d, want 1 (stderr: %s)", code, stderr.String())
	}
	want := file + ":3: water: *esia → exa, want esa\n2 entries, 1 mismatch\n"
	if stdout.String() != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", stdout.String(), want)
	}

	stdout.Reset()
	if code := run([]string{"-lang", dir, "evolve", "-trace", "water"}, &stdout, &stderr); code != 1 {
		t.Errorf("trace exit code = %d, want 1", code)
	}
	if !strings.HasSuffix(stdout.String(), "esia → exa\n  want esa\n") {
		t.Errorf("trace stdout = %q, want a want line", stdout.String())
	}
}
