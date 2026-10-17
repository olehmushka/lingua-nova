// Package earley parses token sequences with a context-free grammar, using
// Earley's algorithm (Earley 1970), and returns every parse tree.
//
// A grammar file has one rule per line, alternatives separated by |:
//
//	S   -> NP VP | Aux NP VP
//	NP  -> Det Nom | Pron
//
// The left-hand side of the first rule is the start symbol. Symbols that
// appear on some left-hand side are nonterminals; every other symbol is a
// preterminal: a category a token must carry (N, V, Det). Everything after
// # is a comment. Empty right-hand sides are not allowed, which keeps the
// algorithm simple: every item spans at least one token.
package earley

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"
)

// Rule is one alternative: LHS -> RHS.
type Rule struct {
	LHS  string
	RHS  []string
	Line int
}

func (r Rule) String() string { return r.LHS + " -> " + strings.Join(r.RHS, " ") }

// Grammar is a parsed grammar file.
type Grammar struct {
	File  string
	Start string
	Rules []Rule
	byLHS map[string][]int // nonterminal → indexes into Rules
}

// Error is a problem in the grammar file, at a line.
type Error struct {
	File string
	Line int
	Msg  string
}

func (e Error) Error() string { return fmt.Sprintf("%s:%d: %s", e.File, e.Line, e.Msg) }

// ErrorList is every problem found in a grammar file, in line order.
type ErrorList []Error

func (l ErrorList) Error() string {
	msgs := make([]string, len(l))
	for i, e := range l {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "\n")
}

// Load parses the grammar file at path.
func Load(path string) (*Grammar, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(path, data)
}

// Parse parses a grammar file's contents; name is used in errors.
func Parse(name string, data []byte) (*Grammar, error) {
	g := &Grammar{File: name, byLHS: map[string][]int{}}
	var errs ErrorList
	sc := bufio.NewScanner(bytes.NewReader(data))
	for n := 1; sc.Scan(); n++ {
		line, _, _ := strings.Cut(sc.Text(), "#")
		if strings.TrimSpace(line) == "" {
			continue
		}
		lhs, rhs, ok := strings.Cut(line, "->")
		lhsF := strings.Fields(lhs)
		switch {
		case !ok:
			errs = append(errs, Error{name, n, fmt.Sprintf("%q: want LHS -> RHS", strings.TrimSpace(line))})
			continue
		case len(lhsF) != 1:
			errs = append(errs, Error{name, n, fmt.Sprintf("want one symbol before ->, found %d", len(lhsF))})
			continue
		}
		for _, alt := range strings.Split(rhs, "|") {
			syms := strings.Fields(alt)
			if len(syms) == 0 {
				errs = append(errs, Error{name, n, fmt.Sprintf("%s: empty alternative", lhsF[0])})
				continue
			}
			g.byLHS[lhsF[0]] = append(g.byLHS[lhsF[0]], len(g.Rules))
			g.Rules = append(g.Rules, Rule{LHS: lhsF[0], RHS: syms, Line: n})
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(g.Rules) == 0 && len(errs) == 0 {
		errs = append(errs, Error{name, 1, "no rules"})
	}
	if len(errs) > 0 {
		return nil, errs
	}
	g.Start = g.Rules[0].LHS
	return g, nil
}

// Nonterminal reports whether sym is the left-hand side of some rule.
func (g *Grammar) Nonterminal(sym string) bool { return len(g.byLHS[sym]) > 0 }

// Preterminals lists the symbols tokens must carry, in order of first use.
func (g *Grammar) Preterminals() []string {
	var out []string
	seen := map[string]bool{}
	for _, r := range g.Rules {
		for _, s := range r.RHS {
			if !g.Nonterminal(s) && !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}
