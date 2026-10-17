package english

import (
	"bytes"
	"fmt"
	"os"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/olehmushka/lingua-nova/internal/lexicon"
)

// Reading is one way to understand an English word: a grammar category
// and what it can translate to.
type Reading struct {
	Cat   string   `yaml:"cat"`   // preterminal of grammar.cfg: N, V, Pron, Aux…
	IDs   []string `yaml:"ids"`   // lexicon ids, best first (words.yaml only)
	Num   string   `yaml:"num"`   // SG or PL: pronouns, possessives, nouns
	Tense string   `yaml:"tense"` // PRS, PST or FUT: auxiliaries, verbs

	Lemma   string           `yaml:"-"` // English dictionary form: see, man
	Entries []*lexicon.Entry `yaml:"-"` // what it translates to, best first; empty if unknown
	Tagged  bool             `yaml:"-"` // the statistical tagger chose this category
}

// Words is language/english/words.yaml: function words, archaic forms
// and irregular inflections.
type Words struct {
	File   string
	Words  map[string][]Reading
	Past   map[string]string // irregular past → verb: saw → see
	Plural map[string]string // irregular plural → noun: men → man
	lines  map[string]int    // word → line, for problems
}

var (
	nums   = []string{"", "SG", "PL"}
	tenses = []string{"", "PRS", "PST", "FUT"}
)

// LoadWords reads a words file.
func LoadWords(path string) (*Words, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseWords(path, data)
}

// ParseWords parses a words file's contents; name is used in errors.
func ParseWords(name string, data []byte) (*Words, error) {
	var raw struct {
		Words  yaml.Node         `yaml:"words"`
		Past   map[string]string `yaml:"past"`
		Plural map[string]string `yaml:"plural"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	w := &Words{File: name, Words: map[string][]Reading{}, Past: raw.Past, Plural: raw.Plural, lines: map[string]int{}}
	n := &raw.Words
	if n.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s:%d: words: want a mapping", name, n.Line)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		var rs []Reading
		if err := checkKeys(v); err != nil {
			return nil, fmt.Errorf("%s:%d: %s: %w", name, k.Line, k.Value, err)
		}
		if err := v.Decode(&rs); err != nil {
			return nil, fmt.Errorf("%s:%d: %s: %w", name, k.Line, k.Value, err)
		}
		for _, r := range rs {
			switch {
			case r.Cat == "":
				return nil, fmt.Errorf("%s:%d: %s: a reading needs a cat", name, k.Line, k.Value)
			case !slices.Contains(nums, r.Num):
				return nil, fmt.Errorf("%s:%d: %s: num %q, want SG or PL", name, k.Line, k.Value, r.Num)
			case !slices.Contains(tenses, r.Tense):
				return nil, fmt.Errorf("%s:%d: %s: tense %q, want PRS, PST or FUT", name, k.Line, k.Value, r.Tense)
			}
		}
		w.Words[k.Value] = rs
		w.lines[k.Value] = k.Line
	}
	return w, nil
}

// Check reports ids that aren't in the lexicon and categories that the
// grammar doesn't use (cats nil: skip that check).
func (w *Words) Check(lex *lexicon.Lexicon, cats []string) []lexicon.Problem {
	ids := map[string]bool{}
	for _, e := range lex.Entries {
		ids[e.ID] = true
	}
	var out []lexicon.Problem
	words := make([]string, 0, len(w.Words))
	for word := range w.Words {
		words = append(words, word)
	}
	slices.SortFunc(words, func(a, b string) int { return w.lines[a] - w.lines[b] })
	for _, word := range words {
		for _, r := range w.Words[word] {
			if cats != nil && !slices.Contains(cats, r.Cat) {
				out = append(out, lexicon.Problem{File: w.File, Line: w.lines[word], ID: word,
					Msg: fmt.Sprintf("category %s isn't in the grammar", r.Cat)})
			}
			for _, id := range r.IDs {
				if !ids[id] {
					out = append(out, lexicon.Problem{File: w.File, Line: w.lines[word], ID: word,
						Msg: fmt.Sprintf("no lexicon entry %q", id)})
				}
			}
		}
	}
	return out
}

// checkKeys rejects unknown fields in a word's readings. Node.Decode
// doesn't apply the decoder's KnownFields, so this does it by hand.
func checkKeys(n *yaml.Node) error {
	for _, r := range n.Content {
		for i := 0; r.Kind == yaml.MappingNode && i < len(r.Content); i += 2 {
			if k := r.Content[i].Value; !slices.Contains(readingKeys, k) {
				return fmt.Errorf("unknown field %q (want %s)", k, strings.Join(readingKeys, ", "))
			}
		}
	}
	return nil
}

var readingKeys = []string{"cat", "ids", "num", "tense"}
