// Package sound parses and applies sound change rules (sound-changes.txt).
//
// A rule is one line:
//
//	target > replacement / left_right
//
// The environment ("/ left_right") is optional. Items are segment spellings
// (li, lh), category names from the phonology (V, C), # for a word edge in
// the environment, and ∅ for an empty replacement. Lines whose first
// non-space character is # are comments; there are no trailing comments,
// because # inside a rule is the word boundary.
package sound

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/olehmushka/lingua-nova/internal/phon"
)

// ItemKind says what a pattern item matches.
type ItemKind int

const (
	Seg      ItemKind = iota + 1 // one segment, by spelling
	Cat                          // any segment of a category
	Boundary                     // word edge (#)

	null // ∅; only seen while parsing
)

// Item is one position in a target or environment.
type Item struct {
	Kind ItemKind
	Name string // segment spelling or category name; "#" for Boundary
}

// Rule is one parsed sound change.
type Rule struct {
	Line        int
	Text        string // the source line with spaces normalized: "si > x / _V"
	Target      []Item // segments and categories, never empty
	Replacement []phon.Segment
	Left, Right []Item // environment around _; # only at the outer edges
}

func (r Rule) String() string { return r.Text }

// Error is a problem in the rule file, at a line.
type Error struct {
	File string
	Line int
	Msg  string
}

func (e Error) Error() string { return fmt.Sprintf("%s:%d: %s", e.File, e.Line, e.Msg) }

// ErrorList is every problem found in a rule file, in line order.
type ErrorList []Error

func (l ErrorList) Error() string {
	msgs := make([]string, len(l))
	for i, e := range l {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "\n")
}

// Load parses the rule file at path.
func Load(path string, inv *phon.Inventory) ([]Rule, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(path, f, inv)
}

// Parse reads rules from r; name is used in error messages. It reports every
// bad line, as an ErrorList, and returns the rules that did parse.
func Parse(name string, r io.Reader, inv *phon.Inventory) ([]Rule, error) {
	p := newParser(inv)
	var rules []Rule
	var errs ErrorList
	sc := bufio.NewScanner(r)
	for line := 1; sc.Scan(); line++ {
		text := strings.Join(strings.Fields(sc.Text()), " ")
		if text == "" || text[0] == '#' {
			continue
		}
		rule, err := p.rule(text)
		if err != nil {
			errs = append(errs, Error{name, line, err.Error()})
			continue
		}
		rule.Line = line
		rules = append(rules, rule)
	}
	if err := sc.Err(); err != nil {
		return rules, err
	}
	if len(errs) > 0 {
		return rules, errs
	}
	return rules, nil
}

type parser struct {
	inv    *phon.Inventory
	maxLen int // longest segment spelling or category name, in bytes
}

func newParser(inv *phon.Inventory) *parser {
	p := &parser{inv: inv}
	for _, s := range inv.Segments {
		p.maxLen = max(p.maxLen, len(s.Spell))
	}
	for name := range inv.Categories {
		p.maxLen = max(p.maxLen, len(name))
	}
	return p
}

// rule parses one non-comment line.
func (p *parser) rule(text string) (Rule, error) {
	r := Rule{Text: text}
	if n := strings.Count(text, ">"); n != 1 {
		return r, fmt.Errorf("%q: want exactly one >, found %d", text, n)
	}
	lhs, rhs, _ := strings.Cut(text, ">")
	if n := strings.Count(rhs, "/"); n > 1 {
		return r, fmt.Errorf("%q: want at most one /, found %d", text, n)
	}
	repl, env, hasEnv := strings.Cut(rhs, "/")

	// Target: segments and categories.
	target, err := p.items(lhs)
	if err != nil {
		return r, fmt.Errorf("target: %w", err)
	}
	if len(target) == 0 {
		return r, fmt.Errorf("%q: empty target", text)
	}
	for _, it := range target {
		if it.Kind != Seg && it.Kind != Cat {
			return r, fmt.Errorf("target: %s is not allowed here", it.Name)
		}
	}
	r.Target = target

	// Replacement: segments, or ∅ alone.
	items, err := p.items(repl)
	if err != nil {
		return r, fmt.Errorf("replacement: %w", err)
	}
	if len(items) == 0 {
		return r, fmt.Errorf("%q: empty replacement (write ∅ for deletion)", text)
	}
	if len(items) == 1 && items[0].Kind == null {
		items = nil
	}
	for _, it := range items {
		if it.Kind != Seg {
			return r, fmt.Errorf("replacement: %s is not allowed here (only segments, or ∅ alone)", it.Name)
		}
		seg, _ := p.inv.Lookup(it.Name)
		r.Replacement = append(r.Replacement, seg)
	}

	if !hasEnv {
		return r, nil
	}
	// Environment: left _ right; # only at the outer edges.
	if n := strings.Count(env, "_"); n != 1 {
		return r, fmt.Errorf("environment %q: want exactly one _, found %d", strings.TrimSpace(env), n)
	}
	l, rt, _ := strings.Cut(env, "_")
	if r.Left, err = p.items(l); err != nil {
		return r, fmt.Errorf("environment: %w", err)
	}
	if r.Right, err = p.items(rt); err != nil {
		return r, fmt.Errorf("environment: %w", err)
	}
	for i, it := range r.Left {
		if it.Kind == null || it.Kind == Boundary && i != 0 {
			return r, fmt.Errorf("environment: %s is not allowed here", it.Name)
		}
	}
	for i, it := range r.Right {
		if it.Kind == null || it.Kind == Boundary && i != len(r.Right)-1 {
			return r, fmt.Errorf("environment: %s is not allowed here", it.Name)
		}
	}
	return r, nil
}

// items tokenizes one part of a rule, skipping spaces. Segment spellings and
// category names are read by greedy longest match, as in phon.Tokenize;
// spellings are lowercase and category names start uppercase, so "V_V" and
// "Va" split as expected.
func (p *parser) items(s string) ([]Item, error) {
	var out []Item
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case unicode.IsSpace(r):
			i += size
			continue
		case r == '#':
			out = append(out, Item{Boundary, "#"})
			i += size
			continue
		case r == '∅':
			out = append(out, Item{null, "∅"})
			i += size
			continue
		}
		matched := false
		for n := min(p.maxLen, len(s)-i); n > 0; n-- {
			name := s[i : i+n]
			if _, ok := p.inv.Lookup(name); ok {
				out = append(out, Item{Seg, name})
			} else if _, ok := p.inv.Categories[name]; ok {
				out = append(out, Item{Cat, name})
			} else {
				continue
			}
			i += n
			matched = true
			break
		}
		if !matched {
			return nil, fmt.Errorf("unknown segment or category at %q", strings.TrimSpace(s[i:]))
		}
	}
	return out, nil
}
