// Package morph inflects lexicon entries using the tables in
// language/morphology.yaml: root + features → word form.
//
// A form is built item-and-arrangement style, as a sequence of morphemes
// (nar + a + x + u), each with a Leipzig gloss label (see + TH + PST + 1SG).
// Paradigm lays the forms out word-and-paradigm style, as a table.
package morph

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/olehmushka/lingua-nova/internal/lexicon"
	"github.com/olehmushka/lingua-nova/internal/phon"
)

// ErrNoClass means a verb's infinitive has none of the class endings.
// lexicon.Validate already reports those, so Check skips them.
var ErrNoClass = errors.New("no class ending")

// Morpheme is one meaningful piece of a word form.
type Morpheme struct {
	Form  string // spelling: "x"
	Gloss string // label: "PST", or the entry's gloss for a stem
}

// Form is an inflected word.
type Form struct {
	Feats     []string // the features asked for: [PST 1SG]
	Morphemes []Morpheme
}

// Word is the spelled form: naraxu.
func (f Form) Word() string {
	var b strings.Builder
	for _, m := range f.Morphemes {
		b.WriteString(m.Form)
	}
	return b.String()
}

// Segmented is the form with morpheme breaks: nar-a-x-u.
func (f Form) Segmented() string { return f.join(func(m Morpheme) string { return m.Form }) }

// Gloss is the Leipzig gloss: see-TH-PST-1SG.
func (f Form) Gloss() string { return f.join(func(m Morpheme) string { return m.Gloss }) }

// Label names the cell: PST.1SG.
func (f Form) Label() string { return strings.Join(f.Feats, ".") }

func (f Form) join(part func(Morpheme) string) string {
	parts := make([]string, len(f.Morphemes))
	for i, m := range f.Morphemes {
		parts[i] = part(m)
	}
	return strings.Join(parts, "-")
}

// Table is a paradigm: rows × columns of forms. A nil cell is a form the
// language doesn't have (hunu has no plural).
type Table struct {
	Cols []string
	Rows []Row
}

// Row is one row of a paradigm.
type Row struct {
	Label string
	Cells []*Form
}

// Rules is the parsed morphology.yaml.
type Rules struct {
	File     string
	Verb     VerbRules
	Noun     []Suffix
	Pronouns map[string]Pronoun

	pronounLine map[string]int // lexicon id → line in File
	inv         *phon.Inventory
}

// VerbRules: stem + theme vowel + tense marker + person ending.
type VerbRules struct {
	Persons []string `yaml:"persons"`
	Classes []Class  `yaml:"classes"`
	Tenses  []Tense  `yaml:"tenses"`
}

// Class is a conjugation class, recognized by its infinitive ending.
type Class struct {
	Name       string `yaml:"name"`
	Infinitive string `yaml:"infinitive"` // ending that marks the class: ar
	Theme      string `yaml:"theme"`
}

// Tense is one tense's endings.
type Tense struct {
	Name    string                       `yaml:"name"`
	Marker  string                       `yaml:"marker"`   // x, r; empty for the present
	Endings map[string]string            `yaml:"endings"`  // person → ending; "" fuses with TH
	NoTheme []string                     `yaml:"no_theme"` // persons without a theme vowel
	Theme   map[string]map[string]string `yaml:"theme"`    // class → person → theme vowel
}

// Suffix is a noun suffix with two allomorphs, chosen by the preceding sound.
type Suffix struct {
	Name           string `yaml:"name"`
	AfterVowel     string `yaml:"after_vowel"`
	AfterConsonant string `yaml:"after_consonant"`
}

// Pronoun is a suppletive pronoun: whole forms, not built by rule.
type Pronoun struct {
	Person string   `yaml:"person"`
	Mark   string   `yaml:"mark"`
	SG     []string `yaml:"SG"` // subject, object, possessive
	PL     []string `yaml:"PL"` // empty if there is no plural
}

// Numbers and Cases are the pronoun paradigm's dimensions, in table order.
var (
	Numbers = []string{"SG", "PL"}
	Cases   = []string{"SBJ", "OBJ", "POSS"}
)

type fileYAML struct {
	Verb    VerbRules          `yaml:"verb"`
	Noun    []Suffix           `yaml:"noun"`
	Pronoun map[string]Pronoun `yaml:"pronoun"`
}

// Load reads and checks a morphology file. inv decides which sounds are
// vowels when choosing allomorphs.
func Load(path string, inv *phon.Inventory) (*Rules, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	r, err := Parse(data, inv)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	r.File = path
	return r, nil
}

// Parse decodes and checks morphology YAML.
func Parse(data []byte, inv *phon.Inventory) (*Rules, error) {
	var f fileYAML
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return nil, err
	}
	r := &Rules{Verb: f.Verb, Noun: f.Noun, Pronouns: f.Pronoun, inv: inv, pronounLine: pronounLines(data)}
	if err := r.check(); err != nil {
		return nil, err
	}
	return r, nil
}

// pronounLines finds the line of each key under pronoun:, for problems
// about a pronoun table entry.
func pronounLines(data []byte) map[string]int {
	lines := map[string]int{}
	var doc yaml.Node
	if yaml.Unmarshal(data, &doc) != nil || len(doc.Content) == 0 {
		return lines
	}
	root := doc.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "pronoun" {
			continue
		}
		m := root.Content[i+1]
		for j := 0; j+1 < len(m.Content); j += 2 {
			lines[m.Content[j].Value] = m.Content[j].Line
		}
	}
	return lines
}

// check catches table mistakes that YAML decoding can't: a missing person,
// an unknown class, a pronoun row of the wrong length.
func (r *Rules) check() error {
	v := r.Verb
	if len(v.Persons) == 0 || len(v.Classes) == 0 || len(v.Tenses) == 0 {
		return fmt.Errorf("verb: persons, classes and tenses are required")
	}
	classes := map[string]bool{}
	for _, c := range v.Classes {
		if c.Name == "" || c.Infinitive == "" || c.Theme == "" {
			return fmt.Errorf("verb class %q: name, infinitive and theme are required", c.Name)
		}
		classes[c.Name] = true
	}
	for _, t := range v.Tenses {
		if t.Name == "" {
			return fmt.Errorf("verb tense without a name")
		}
		for _, p := range v.Persons {
			end, ok := t.Endings[p]
			if !ok {
				return fmt.Errorf("verb tense %s: no ending for %s", t.Name, p)
			}
			if end == "" && slices.Contains(t.NoTheme, p) && t.Marker == "" {
				return fmt.Errorf("verb tense %s %s: no theme, marker or ending", t.Name, p)
			}
		}
		for p := range t.Endings {
			if !slices.Contains(v.Persons, p) {
				return fmt.Errorf("verb tense %s: unknown person %q", t.Name, p)
			}
		}
		for c, byPerson := range t.Theme {
			if !classes[c] {
				return fmt.Errorf("verb tense %s: theme for unknown class %q", t.Name, c)
			}
			for p := range byPerson {
				if !slices.Contains(v.Persons, p) {
					return fmt.Errorf("verb tense %s: theme for unknown person %q", t.Name, p)
				}
			}
		}
	}
	for _, s := range r.Noun {
		if s.Name == "" || s.AfterVowel == "" || s.AfterConsonant == "" {
			return fmt.Errorf("noun suffix %q: name, after_vowel and after_consonant are required", s.Name)
		}
	}
	for id, p := range r.Pronouns {
		if p.Person == "" {
			return fmt.Errorf("pronoun %s: person is required", id)
		}
		if len(p.SG) != len(Cases) || (p.PL != nil && len(p.PL) != len(Cases)) {
			return fmt.Errorf("pronoun %s: SG and PL need %d forms (%s)", id, len(Cases), strings.Join(Cases, ", "))
		}
	}
	return nil
}

// base is the form inflection starts from: word if set, else modern.
func base(e *lexicon.Entry) string { return cmp.Or(e.Word, e.Modern) }

// Inflect builds one form of an entry. Features are case-insensitive and
// in any order:
//
//   - verb: a tense and a person (PST 1SG);
//   - noun: none, PL, POSS or both;
//   - pronoun: a number (default SG) and a case (default SBJ).
func (r *Rules) Inflect(e *lexicon.Entry, feats ...string) (Form, error) {
	up := make([]string, len(feats))
	for i, f := range feats {
		up[i] = strings.ToUpper(f)
	}
	switch e.POS {
	case "verb":
		return r.inflectVerb(e, up)
	case "noun":
		return r.inflectNoun(e, up)
	case "pronoun":
		return r.inflectPronoun(e, up)
	}
	return Form{}, fmt.Errorf("%s: a %s doesn't inflect", e.ID, e.POS)
}

func (r *Rules) inflectVerb(e *lexicon.Entry, feats []string) (Form, error) {
	var tense *Tense
	person := ""
	for _, f := range feats {
		if i := slices.IndexFunc(r.Verb.Tenses, func(t Tense) bool { return t.Name == f }); i >= 0 && tense == nil {
			tense = &r.Verb.Tenses[i]
		} else if slices.Contains(r.Verb.Persons, f) && person == "" {
			person = f
		} else {
			return Form{}, fmt.Errorf("%s: unexpected feature %q (a verb takes one of %s and one of %s)",
				e.ID, f, strings.Join(r.tenseNames(), " "), strings.Join(r.Verb.Persons, " "))
		}
	}
	if tense == nil || person == "" {
		return Form{}, fmt.Errorf("%s: a verb needs a tense (%s) and a person (%s)",
			e.ID, strings.Join(r.tenseNames(), " "), strings.Join(r.Verb.Persons, " "))
	}
	return r.verbForm(e, *tense, person)
}

func (r *Rules) tenseNames() []string {
	names := make([]string, len(r.Verb.Tenses))
	for i, t := range r.Verb.Tenses {
		names[i] = t.Name
	}
	return names
}

func (r *Rules) verbForm(e *lexicon.Entry, t Tense, person string) (Form, error) {
	inf := base(e)
	var class *Class
	for i, c := range r.Verb.Classes {
		if len(inf) > len(c.Infinitive) && strings.HasSuffix(inf, c.Infinitive) {
			class = &r.Verb.Classes[i]
			break
		}
	}
	if class == nil {
		return Form{}, fmt.Errorf("%s: %q: %w", e.ID, inf, ErrNoClass)
	}
	stem := strings.TrimSuffix(inf, class.Infinitive)

	ms := []Morpheme{{stem, e.Gloss}}
	ending := t.Endings[person]
	if !slices.Contains(t.NoTheme, person) {
		th := cmp.Or(t.Theme[class.Name][person], class.Theme)
		if ending == "" && t.Marker == "" {
			// Fusion: the theme vowel alone carries the person (nar-a).
			ms = append(ms, Morpheme{th, "TH." + person})
		} else {
			ms = append(ms, Morpheme{th, "TH"})
		}
	}
	if t.Marker != "" {
		ms = append(ms, Morpheme{t.Marker, t.Name})
	}
	if ending != "" {
		ms = append(ms, Morpheme{ending, person})
	}
	return Form{Feats: []string{t.Name, person}, Morphemes: ms}, nil
}

func (r *Rules) inflectNoun(e *lexicon.Entry, feats []string) (Form, error) {
	want := map[string]bool{}
	for _, f := range feats {
		if !slices.ContainsFunc(r.Noun, func(s Suffix) bool { return s.Name == f }) || want[f] {
			return Form{}, fmt.Errorf("%s: unexpected feature %q (a noun takes %s)", e.ID, f, strings.Join(r.suffixNames(), ", "))
		}
		want[f] = true
	}
	ms := []Morpheme{{base(e), e.Gloss}}
	var names []string
	for _, s := range r.Noun { // slot order, not argument order
		if !want[s.Name] {
			continue
		}
		vowel, err := r.endsInVowel(Form{Morphemes: ms}.Word())
		if err != nil {
			return Form{}, fmt.Errorf("%s: %w", e.ID, err)
		}
		suf := s.AfterConsonant
		if vowel {
			suf = s.AfterVowel
		}
		ms = append(ms, Morpheme{suf, s.Name})
		names = append(names, s.Name)
	}
	return Form{Feats: names, Morphemes: ms}, nil
}

func (r *Rules) suffixNames() []string {
	names := make([]string, len(r.Noun))
	for i, s := range r.Noun {
		names[i] = s.Name
	}
	return names
}

// endsInVowel tests the last segment, not the last letter, so a word
// ending in a digraph like lh counts as consonant-final.
func (r *Rules) endsInVowel(word string) (bool, error) {
	segs, err := r.inv.Tokenize(word)
	if err != nil {
		return false, err
	}
	if len(segs) == 0 {
		return false, fmt.Errorf("empty word")
	}
	return segs[len(segs)-1].Kind == phon.Vowel, nil
}

func (r *Rules) inflectPronoun(e *lexicon.Entry, feats []string) (Form, error) {
	p, ok := r.Pronouns[e.ID]
	if !ok {
		return Form{}, fmt.Errorf("%s: no pronoun table in morphology.yaml", e.ID)
	}
	number, kase := "", ""
	for _, f := range feats {
		switch {
		case slices.Contains(Numbers, f) && number == "":
			number = f
		case slices.Contains(Cases, f) && kase == "":
			kase = f
		default:
			return Form{}, fmt.Errorf("%s: unexpected feature %q (a pronoun takes %s and %s)",
				e.ID, f, strings.Join(Numbers, "/"), strings.Join(Cases, "/"))
		}
	}
	f := pronounForm(p, cmp.Or(number, "SG"), cmp.Or(kase, "SBJ"))
	if f == nil {
		return Form{}, fmt.Errorf("%s: no %s form", e.ID, cmp.Or(number, "SG"))
	}
	return *f, nil
}

// pronounForm returns nil if the pronoun has no such number.
func pronounForm(p Pronoun, number, kase string) *Form {
	forms := p.SG
	if number == "PL" {
		forms = p.PL
	}
	if len(forms) == 0 {
		return nil
	}
	gloss := p.Person + number
	if p.Mark != "" {
		gloss += "." + p.Mark
	}
	if kase != "SBJ" {
		gloss += "." + kase
	}
	return &Form{
		Feats:     []string{number, kase},
		Morphemes: []Morpheme{{forms[slices.Index(Cases, kase)], gloss}},
	}
}

// Paradigm returns every form of an entry as a table, or nil if the entry
// doesn't inflect (adjectives, particles, pronouns without a table).
//
//   - verb: rows are persons, columns tenses;
//   - noun: rows SG and PL, columns plain and POSS;
//   - pronoun: rows SG and PL, columns SBJ, OBJ, POSS.
func (r *Rules) Paradigm(e *lexicon.Entry) (*Table, error) {
	switch e.POS {
	case "verb":
		t := &Table{Cols: r.tenseNames()}
		for _, p := range r.Verb.Persons {
			row := Row{Label: p}
			for _, tense := range r.Verb.Tenses {
				f, err := r.verbForm(e, tense, p)
				if err != nil {
					return nil, err
				}
				row.Cells = append(row.Cells, &f)
			}
			t.Rows = append(t.Rows, row)
		}
		return t, nil
	case "noun":
		var others []string
		for _, s := range r.Noun {
			if s.Name != "PL" {
				others = append(others, s.Name)
			}
		}
		t := &Table{Cols: append([]string{""}, others...)}
		for _, number := range Numbers {
			row := Row{Label: number}
			for _, col := range t.Cols {
				var feats []string
				if number == "PL" {
					feats = append(feats, "PL")
				}
				if col != "" {
					feats = append(feats, col)
				}
				f, err := r.inflectNoun(e, feats)
				if err != nil {
					return nil, err
				}
				f.Feats = []string{number}
				if col != "" {
					f.Feats = append(f.Feats, col)
				}
				row.Cells = append(row.Cells, &f)
			}
			t.Rows = append(t.Rows, row)
		}
		return t, nil
	case "pronoun":
		p, ok := r.Pronouns[e.ID]
		if !ok {
			return nil, nil
		}
		t := &Table{Cols: Cases}
		for _, number := range Numbers {
			row := Row{Label: number}
			for _, c := range Cases {
				row.Cells = append(row.Cells, pronounForm(p, number, c))
			}
			t.Rows = append(t.Rows, row)
		}
		return t, nil
	}
	return nil, nil
}

// mergesAcrossBoundary reports whether the spelled word tokenizes
// differently from its morphemes one by one: stem pul + suffix hu would
// spell pulhu, which reads back as p·u·lh·u.
func (r *Rules) mergesAcrossBoundary(f Form, word []phon.Segment) bool {
	var parts []string
	for _, m := range f.Morphemes {
		segs, err := r.inv.Tokenize(m.Form)
		if err != nil {
			return false // the whole word's tokenizing reports it
		}
		parts = append(parts, phon.Spellings(segs)...)
	}
	return !slices.Equal(parts, phon.Spellings(word))
}

// Check inflects every entry and reports forms that break the modern
// phonotactics or whose spelling merges two morphemes' letters into one
// sound, entries that can't be inflected, and pronoun tables that
// don't match the lexicon. Problems point at the entry, or at the
// morphology file for a table without an entry.
func Check(lex *lexicon.Lexicon, r *Rules) []lexicon.Problem {
	var probs []lexicon.Problem
	seen := map[string]bool{}
	for _, e := range lex.Entries {
		bad := func(format string, args ...any) {
			probs = append(probs, lexicon.Problem{File: e.File, Line: e.Line, ID: e.ID, Msg: fmt.Sprintf(format, args...)})
		}
		if e.POS == "pronoun" {
			if p, ok := r.Pronouns[e.ID]; ok {
				seen[e.ID] = true
				if p.SG[0] != base(e) {
					bad("morphology.yaml subject form %q differs from %q", p.SG[0], base(e))
				}
			}
		}
		t, err := r.Paradigm(e)
		if errors.Is(err, ErrNoClass) {
			continue
		}
		if err != nil {
			bad("can't inflect: %v", err)
			continue
		}
		if t == nil {
			continue
		}
		for _, row := range t.Rows {
			for _, f := range row.Cells {
				if f == nil {
					continue
				}
				w := f.Word()
				segs, err := r.inv.Tokenize(w)
				if err != nil {
					bad("%s %s: %v", f.Label(), w, err)
					continue
				}
				if r.mergesAcrossBoundary(*f, segs) {
					bad("%s %s (%s): spelling reads as %s across a morpheme boundary",
						f.Label(), w, f.Segmented(), strings.Join(phon.Spellings(segs), "·"))
				}
				for _, v := range r.inv.CheckPhonotactics(segs) {
					bad("%s %s (%s): %s", f.Label(), w, f.Segmented(), v.Msg)
				}
			}
		}
	}
	for id := range r.Pronouns {
		if !seen[id] {
			probs = append(probs, lexicon.Problem{File: r.File, Line: r.pronounLine[id], ID: id, Msg: "pronoun table has no pronoun entry in the lexicon"})
		}
	}
	slices.SortStableFunc(probs, func(a, b lexicon.Problem) int {
		return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line))
	})
	return probs
}
