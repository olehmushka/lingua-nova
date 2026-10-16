// Package phon loads the machine-readable phonology (language/phonology.yaml)
// and splits spelled words into segments.
//
// All linguistic logic works on segments (sounds), not letters: "folha" is
// four segments f·o·lh·a, because the digraph lh spells the single sound /ʎ/.
package phon

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// Kind is the broad class of a segment.
type Kind int

const (
	Consonant Kind = iota + 1
	Vowel
)

func (k Kind) String() string {
	switch k {
	case Consonant:
		return "consonant"
	case Vowel:
		return "vowel"
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// Segment is one sound of the language.
type Segment struct {
	Spell string // romanized spelling: "a", "lh"
	IPA   string // phonetic value: "a", "ʎ"
	Kind  Kind
	Proto bool // exists in the proto-language; every segment exists in the modern language
}

// Inventory is the parsed phonology. Sequences (diphthongs, onsets,
// forbidden) are slices of segment spellings.
type Inventory struct {
	Segments   []Segment
	Categories map[string][]string // includes the built-in V and C
	Diphthongs [][]string
	Onsets     [][]string // multi-consonant onsets; any single consonant is also legal
	Codas      []string
	Finals     []string   // consonants allowed word-finally; vowels always are
	Forbidden  [][]string // items are segment spellings or category names

	// Proto-language phonotactics. Onsets, diphthongs and forbidden are
	// shared with the modern language.
	ProtoCodas  []string
	ProtoFinals []string
	Geminates   []string       // consonants that may double between vowels in proto (*cup.pe)
	Generator   *GeneratorSpec // word generator templates; nil if the file has none

	bySpell    map[string]Segment
	maxLen     int // longest spelling in bytes, for longest-match tokenizing
	diphthongs map[[2]string]bool
	onsets     map[string]bool // clusters keyed by seqKey
	catSet     map[string]map[string]bool
}

// seqKey turns a sequence of spellings into a map key. The space keeps
// [l, h] and [lh] apart.
func seqKey(spells []string) string { return strings.Join(spells, " ") }

type segmentYAML struct {
	Spell      string `yaml:"spell"`
	IPA        string `yaml:"ipa"`
	ModernOnly bool   `yaml:"modern_only"`
}

type fileYAML struct {
	Consonants []segmentYAML       `yaml:"consonants"`
	Vowels     []segmentYAML       `yaml:"vowels"`
	Categories map[string][]string `yaml:"categories"`
	Diphthongs [][]string          `yaml:"diphthongs"`
	Onsets     [][]string          `yaml:"onsets"`
	Codas      []string            `yaml:"codas"`
	Finals     []string            `yaml:"finals"`
	Forbidden  [][]string          `yaml:"forbidden"`
	Proto      protoYAML           `yaml:"proto"`
}

type protoYAML struct {
	Codas     []string       `yaml:"codas"`
	Finals    []string       `yaml:"finals"`
	Geminates []string       `yaml:"geminates"`
	Generator *generatorYAML `yaml:"generator"`
}

// Load reads and validates a phonology file.
func Load(path string) (*Inventory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	inv, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return inv, nil
}

// Parse decodes and validates phonology YAML.
func Parse(data []byte) (*Inventory, error) {
	var f fileYAML
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return nil, err
	}

	inv := &Inventory{
		Categories:  map[string][]string{},
		Diphthongs:  f.Diphthongs,
		Onsets:      f.Onsets,
		Codas:       f.Codas,
		Finals:      f.Finals,
		Forbidden:   f.Forbidden,
		ProtoCodas:  f.Proto.Codas,
		ProtoFinals: f.Proto.Finals,
		Geminates:   f.Proto.Geminates,
		bySpell:     map[string]Segment{},
		diphthongs:  map[[2]string]bool{},
		onsets:      map[string]bool{},
	}
	add := func(s segmentYAML, k Kind) error {
		if s.Spell == "" || s.IPA == "" {
			return fmt.Errorf("%s with empty spell or ipa", k)
		}
		for _, r := range s.Spell {
			if r < 'a' || r > 'z' {
				return fmt.Errorf("segment %q: spelling must be lowercase a-z", s.Spell)
			}
		}
		if _, dup := inv.bySpell[s.Spell]; dup {
			return fmt.Errorf("segment %q defined twice", s.Spell)
		}
		seg := Segment{Spell: s.Spell, IPA: s.IPA, Kind: k, Proto: !s.ModernOnly}
		inv.Segments = append(inv.Segments, seg)
		inv.bySpell[s.Spell] = seg
		inv.maxLen = max(inv.maxLen, len(s.Spell))
		return nil
	}
	for _, s := range f.Consonants {
		if err := add(s, Consonant); err != nil {
			return nil, err
		}
		inv.Categories["C"] = append(inv.Categories["C"], s.Spell)
	}
	for _, s := range f.Vowels {
		if err := add(s, Vowel); err != nil {
			return nil, err
		}
		inv.Categories["V"] = append(inv.Categories["V"], s.Spell)
	}
	if len(inv.Categories["V"]) == 0 || len(inv.Categories["C"]) == 0 {
		return nil, fmt.Errorf("need at least one consonant and one vowel")
	}

	for name, members := range f.Categories {
		if name == "V" || name == "C" {
			return nil, fmt.Errorf("category %s is built in; don't define it", name)
		}
		if name == "" || name[0] < 'A' || name[0] > 'Z' {
			return nil, fmt.Errorf("category %q: name must start with an uppercase letter", name)
		}
		if err := inv.checkSeq("category "+name, members, 0); err != nil {
			return nil, err
		}
		inv.Categories[name] = members
	}
	inv.catSet = map[string]map[string]bool{}
	for name, members := range inv.Categories {
		set := map[string]bool{}
		for _, m := range members {
			set[m] = true
		}
		inv.catSet[name] = set
	}
	for _, d := range inv.Diphthongs {
		if err := inv.checkSeq("diphthong", d, Vowel); err != nil {
			return nil, err
		}
		if len(d) != 2 {
			return nil, fmt.Errorf("diphthong %v: want 2 vowels", d)
		}
		inv.diphthongs[[2]string{d[0], d[1]}] = true
	}
	for _, o := range inv.Onsets {
		if err := inv.checkSeq("onset", o, Consonant); err != nil {
			return nil, err
		}
		if len(o) < 2 {
			return nil, fmt.Errorf("onset %v: list only clusters; single consonants are always legal", o)
		}
		inv.onsets[seqKey(o)] = true
	}
	if err := inv.checkSeq("codas", inv.Codas, Consonant); err != nil {
		return nil, err
	}
	if err := inv.checkSeq("finals", inv.Finals, Consonant); err != nil {
		return nil, err
	}
	for _, p := range []struct {
		what   string
		spells []string
	}{
		{"proto codas", inv.ProtoCodas},
		{"proto finals", inv.ProtoFinals},
		{"proto geminates", inv.Geminates},
	} {
		if err := inv.checkSeq(p.what, p.spells, Consonant); err != nil {
			return nil, err
		}
		for _, sp := range p.spells {
			if !inv.bySpell[sp].Proto {
				return nil, fmt.Errorf("%s %v: %q is not a proto-language sound", p.what, p.spells, sp)
			}
		}
	}
	for _, s := range inv.Forbidden {
		if err := inv.checkPattern("forbidden", s); err != nil {
			return nil, err
		}
		if len(s) < 2 {
			return nil, fmt.Errorf("forbidden %v: want a sequence of 2 or more segments", s)
		}
	}
	if f.Proto.Generator != nil {
		g, err := parseGenerator(f.Proto.Generator)
		if err != nil {
			return nil, err
		}
		inv.Generator = g
	}
	return inv, nil
}

// checkSeq checks that every spelling is a known segment and, if want != 0,
// of that kind. An empty list is fine: a language may have no codas at all.
func (inv *Inventory) checkSeq(what string, spells []string, want Kind) error {
	for _, sp := range spells {
		seg, ok := inv.bySpell[sp]
		if !ok {
			return fmt.Errorf("%s %v: unknown segment %q", what, spells, sp)
		}
		if want != 0 && seg.Kind != want {
			return fmt.Errorf("%s %v: %q is a %s, want %s", what, spells, sp, seg.Kind, want)
		}
	}
	return nil
}

// checkPattern is checkSeq for sequences whose items may also be category
// names (uppercase, so they never clash with a segment spelling).
func (inv *Inventory) checkPattern(what string, items []string) error {
	for _, it := range items {
		if _, ok := inv.bySpell[it]; ok {
			continue
		}
		if _, ok := inv.Categories[it]; ok {
			continue
		}
		return fmt.Errorf("%s %v: unknown segment or category %q", what, items, it)
	}
	return nil
}

// Lookup returns the segment with the given spelling.
func (inv *Inventory) Lookup(spell string) (Segment, bool) {
	seg, ok := inv.bySpell[spell]
	return seg, ok
}

// UnknownLetterError reports a letter that starts no segment spelling.
type UnknownLetterError struct {
	Word   string
	Pos    int // byte offset in Word
	Letter rune
}

func (e *UnknownLetterError) Error() string {
	return fmt.Sprintf("%q: unknown letter %q at position %d", e.Word, e.Letter, e.Pos)
}

// Tokenize splits a spelled word into segments by greedy longest match:
// at each position it takes the longest spelling in the inventory, so
// "folha" is f·o·lh·a, not f·o·l·h·a. The forbidden l+h, n+h, c+h sequences
// are what make that greedy reading always correct.
func (inv *Inventory) Tokenize(word string) ([]Segment, error) {
	var segs []Segment
	for i := 0; i < len(word); {
		matched := false
		for n := min(inv.maxLen, len(word)-i); n > 0; n-- {
			if seg, ok := inv.bySpell[word[i:i+n]]; ok {
				segs = append(segs, seg)
				i += n
				matched = true
				break
			}
		}
		if !matched {
			r, _ := utf8.DecodeRuneInString(word[i:])
			return nil, &UnknownLetterError{Word: word, Pos: i, Letter: r}
		}
	}
	return segs, nil
}

// IPA joins the segments' phonetic values: f·o·lh·a → "foʎa".
func IPA(segs []Segment) string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteString(s.IPA)
	}
	return b.String()
}

// Spellings returns each segment's spelling.
func Spellings(segs []Segment) []string {
	out := make([]string, len(segs))
	for i, s := range segs {
		out[i] = s.Spell
	}
	return out
}
