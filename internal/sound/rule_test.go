package sound

import (
	"errors"
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

// show renders a parsed rule with every item separated, so the test sees
// how the line was tokenized: "li > lh / _V" → "l i > lh / _ V".
func show(r Rule) string {
	names := func(items []Item) string {
		s := make([]string, len(items))
		for i, it := range items {
			s[i] = it.Name
		}
		return strings.Join(s, " ")
	}
	repl := strings.Join(phon.Spellings(r.Replacement), " ")
	if repl == "" {
		repl = "∅"
	}
	out := names(r.Target) + " > " + repl
	if len(r.Left)+len(r.Right) > 0 {
		out += " / " + strings.TrimSpace(names(r.Left)+" _ "+names(r.Right))
	}
	return out
}

func TestParse(t *testing.T) {
	inv := loadPhon(t)
	tests := []struct {
		line string
		want string
	}{
		{"t > d / V_V", "t > d / V _ V"},
		{"li > lh / _V", "l i > lh / _ V"},       // digraph replacement
		{"si > x  / _V", "s i > x / _ V"},        // extra spaces
		{"pp > p", "p p > p"},                    // no environment
		{"n > ∅ / _#", "n > ∅ / _ #"},            // deletion at the word end
		{"C > h / #_", "C > h / # _"},            // category target, word start
		{"lh > l i / a _ V", "lh > l i / a _ V"}, // spaces between items
		{"Va > e", "V a > e"},                    // category then segment, no space
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			rules, err := Parse("test", strings.NewReader(tt.line), inv)
			if err != nil {
				t.Fatal(err)
			}
			if len(rules) != 1 {
				t.Fatalf("got %d rules, want 1", len(rules))
			}
			if got := show(rules[0]); got != tt.want {
				t.Errorf("parsed as %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	inv := loadPhon(t)
	tests := []struct {
		line string
		want string
	}{
		{"p b", "want exactly one >, found 0"},
		{"p > b > c", "want exactly one >, found 2"},
		{"p > b / V_ / _V", "want at most one /, found 2"},
		{"> b", "empty target"},
		{"p > ", "empty replacement"},
		{"p > b / V", "want exactly one _, found 0"},
		{"p > b / V_V_", "want exactly one _, found 2"},
		{"k > c", `target: unknown segment or category at "k"`},
		{"p > b / Q_", `environment: unknown segment or category at "Q"`},
		{"∅ > a", "target: ∅ is not allowed here"},
		{"p# > b", "target: # is not allowed here"},
		{"p > ∅ b", "replacement: ∅ is not allowed here"},
		{"p > V", "replacement: V is not allowed here"},
		{"p > # ", "replacement: # is not allowed here"},
		{"p > b / V#_", "environment: # is not allowed here"},
		{"p > b / _#V", "environment: # is not allowed here"},
		{"p > b / ∅_", "environment: ∅ is not allowed here"},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			// A comment and a blank line first, so the error is on line 3.
			src := "# comment\n\n" + tt.line + "\n"
			_, err := Parse("rules.txt", strings.NewReader(src), inv)
			var list ErrorList
			if !errors.As(err, &list) || len(list) != 1 {
				t.Fatalf("err = %v, want one error", err)
			}
			if !strings.HasPrefix(list[0].Error(), "rules.txt:3: ") {
				t.Errorf("error %q: want prefix rules.txt:3:", list[0])
			}
			if !strings.Contains(list[0].Msg, tt.want) {
				t.Errorf("error %q: want it to contain %q", list[0].Msg, tt.want)
			}
		})
	}
}

// TestParseCollectsErrors: one bad line doesn't stop the parse.
func TestParseCollectsErrors(t *testing.T) {
	src := "p > b / V_V\nk > c\nt > d / V_V\np > b > c\n"
	rules, err := Parse("rules.txt", strings.NewReader(src), loadPhon(t))
	var list ErrorList
	if !errors.As(err, &list) || len(list) != 2 {
		t.Fatalf("err = %v, want two errors", err)
	}
	if list[0].Line != 2 || list[1].Line != 4 {
		t.Errorf("error lines = %d, %d; want 2, 4", list[0].Line, list[1].Line)
	}
	if len(rules) != 2 || rules[0].Line != 1 || rules[1].Line != 3 {
		t.Errorf("rules = %v, want the ones on lines 1 and 3", rules)
	}
}

// TestRealRules: sound-changes.txt parses, one rule per line.
func TestRealRules(t *testing.T) {
	rules, err := Load(filepath.Join(langDir, "sound-changes.txt"), loadPhon(t))
	if err != nil {
		t.Fatal(err)
	}
	wantLines := []int{26, 27, 28, 29, 30, 31, 35, 36, 37, 42, 43, 44, 48, 49}
	if len(rules) != len(wantLines) {
		t.Fatalf("got %d rules, want %d", len(rules), len(wantLines))
	}
	for i, r := range rules {
		if r.Line != wantLines[i] {
			t.Errorf("rule %d (%s) on line %d, want %d", i, r, r.Line, wantLines[i])
		}
	}
	if got, want := rules[2].Text, "si > x / _V"; got != want {
		t.Errorf("rule text = %q, want %q", got, want)
	}
}
