package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
