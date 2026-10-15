// Package lexicon loads the dictionary (language/lexicon/*.yaml) and checks
// it against the phonology.
//
// Entries remember the file and line they came from, so problems can be
// reported like compiler errors: "verbs.yaml:42: ruccar: ...".
package lexicon

import (
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

// Example is a usage example in an entry.
type Example struct {
	Text        string `yaml:"text"`
	Translation string `yaml:"translation"`
}

// Entry is one dictionary entry. The format is described in
// language/README.md.
type Entry struct {
	ID         string    `yaml:"id"`
	Proto      string    `yaml:"proto"`
	Modern     string    `yaml:"modern"`
	Word       string    `yaml:"word"`
	POS        string    `yaml:"pos"`
	Gender     string    `yaml:"gender"`
	Gloss      string    `yaml:"gloss"`
	Definition string    `yaml:"definition"`
	Examples   []Example `yaml:"examples"`
	Notes      string    `yaml:"notes"`

	File      string         `yaml:"-"`
	Line      int            `yaml:"-"` // line of the entry's first key
	fieldLine map[string]int // YAML key → line
}

// LineOf returns the line of the given YAML key, or the entry's line if the
// key is absent.
func (e *Entry) LineOf(field string) int {
	if l, ok := e.fieldLine[field]; ok {
		return l
	}
	return e.Line
}

// knownFields are the YAML keys an entry may have.
var knownFields = map[string]bool{
	"id": true, "proto": true, "modern": true, "word": true, "pos": true,
	"gender": true, "gloss": true, "definition": true, "examples": true, "notes": true,
}

// Problem is one thing wrong with the lexicon, at a file and line.
type Problem struct {
	File string
	Line int
	ID   string // entry id, if known
	Msg  string
}

func (p Problem) String() string {
	if p.ID == "" {
		return fmt.Sprintf("%s:%d: %s", p.File, p.Line, p.Msg)
	}
	return fmt.Sprintf("%s:%d: %s: %s", p.File, p.Line, p.ID, p.Msg)
}

// Lexicon is every entry, in file order, plus the problems found while
// loading (unknown keys). Validate finds the rest.
type Lexicon struct {
	Entries  []*Entry
	Problems []Problem
}

// Load reads every *.yaml file in dir, in name order. A file that isn't
// valid YAML or isn't a list of mappings is an error; content problems are
// collected in Lexicon.Problems.
func Load(dir string) (*Lexicon, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%s: no lexicon files", dir)
	}
	lex := &Lexicon{}
	for _, f := range files { // Glob returns sorted names
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		if err := lex.ParseFile(f, data); err != nil {
			return nil, err
		}
	}
	return lex, nil
}

// ParseFile adds the entries in one file's data. name is used in positions.
//
// It decodes into a yaml.Node first, not straight into []Entry, because only
// the node tree keeps line numbers.
func (lex *Lexicon) ParseFile(name string, data []byte) error {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if len(doc.Content) == 0 {
		return nil // empty file
	}
	list := doc.Content[0]
	if list.Kind != yaml.SequenceNode {
		return fmt.Errorf("%s:%d: want a list of entries", name, list.Line)
	}
	for _, item := range list.Content {
		if item.Kind != yaml.MappingNode {
			return fmt.Errorf("%s:%d: want an entry (a mapping)", name, item.Line)
		}
		e := &Entry{File: name, Line: item.Line, fieldLine: map[string]int{}}
		if err := item.Decode(e); err != nil {
			return fmt.Errorf("%s:%d: %w", name, item.Line, err)
		}
		// Mapping content alternates key, value.
		for i := 0; i+1 < len(item.Content); i += 2 {
			key := item.Content[i]
			e.fieldLine[key.Value] = key.Line
			if !knownFields[key.Value] {
				lex.Problems = append(lex.Problems, Problem{name, key.Line, e.ID, fmt.Sprintf("unknown field %q", key.Value)})
			}
		}
		lex.Entries = append(lex.Entries, e)
	}
	return nil
}
