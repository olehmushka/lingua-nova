package earley

import (
	"slices"
	"strings"
)

// Tree is a parse tree. A leaf is a preterminal over one token (Start);
// an inner node is a nonterminal over its children.
type Tree struct {
	Label      string
	Start, End int // token span [Start, End)
	Children   []*Tree
}

// Leaf reports whether t is a preterminal over a single token.
func (t *Tree) Leaf() bool { return len(t.Children) == 0 }

// Leaves returns the preterminals left to right, one per token.
func (t *Tree) Leaves() []*Tree {
	if t.Leaf() {
		return []*Tree{t}
	}
	var out []*Tree
	for _, c := range t.Children {
		out = append(out, c.Leaves()...)
	}
	return out
}

// Bracketed renders the tree with the words at its leaves:
// (S (NP (Pron I)) (VP (V see))).
func (t *Tree) Bracketed(words []string) string {
	var b strings.Builder
	t.write(&b, words)
	return b.String()
}

func (t *Tree) write(b *strings.Builder, words []string) {
	b.WriteString("(" + t.Label)
	if t.Leaf() {
		w := "?"
		if t.Start < len(words) {
			w = words[t.Start]
		}
		b.WriteString(" " + w + ")")
		return
	}
	for _, c := range t.Children {
		b.WriteByte(' ')
		c.write(b, words)
	}
	b.WriteByte(')')
}

// item is an Earley item: a rule, how much of its right-hand side has been
// matched (the dot), and where the match started.
type item struct {
	rule, dot, origin int
}

// span is a nonterminal that was completed over tokens [i, j).
type span struct {
	sym  string
	i, j int
}

// Parse returns the parse trees of a sentence, at most max (0 = no limit).
// cats[i] lists the categories token i can have: a word can be ambiguous
// ("her": Pron or Poss), and the grammar decides which reading fits.
//
// The recognizer fills a chart: chart[i] holds every item that is
// consistent with the first i tokens. Three operations grow it:
//
//   - predict: the dot is before a nonterminal X, so every rule for X
//     might start here;
//   - scan: the dot is before a preterminal that token i carries, so the
//     item moves on to chart[i+1];
//   - complete: an item is finished, so every item waiting for its LHS
//     where it started moves its dot past it.
//
// Then the trees are read back from the completed items.
func (g *Grammar) Parse(cats [][]string, max int) []*Tree {
	n := len(cats)
	if n == 0 {
		return nil
	}
	chart := make([][]item, n+1)
	seen := make([]map[item]bool, n+1)
	for i := range seen {
		seen[i] = map[item]bool{}
	}
	add := func(i int, it item) {
		if !seen[i][it] {
			seen[i][it] = true
			chart[i] = append(chart[i], it)
		}
	}
	done := map[span]bool{}
	for _, r := range g.byLHS[g.Start] {
		add(0, item{r, 0, 0})
	}
	for i := 0; i <= n; i++ {
		for k := 0; k < len(chart[i]); k++ { // chart[i] grows while we walk it
			it := chart[i][k]
			r := g.Rules[it.rule]
			if it.dot == len(r.RHS) { // complete
				done[span{r.LHS, it.origin, i}] = true
				for _, w := range chart[it.origin] {
					wr := g.Rules[w.rule]
					if w.dot < len(wr.RHS) && wr.RHS[w.dot] == r.LHS {
						add(i, item{w.rule, w.dot + 1, w.origin})
					}
				}
				continue
			}
			next := r.RHS[it.dot]
			if g.Nonterminal(next) { // predict
				for _, p := range g.byLHS[next] {
					add(i, item{p, 0, i})
				}
			} else if i < n && slices.Contains(cats[i], next) { // scan
				add(i+1, item{it.rule, it.dot + 1, it.origin})
			}
		}
	}
	if !done[span{g.Start, 0, n}] {
		return nil
	}
	b := &builder{g: g, cats: cats, done: done, memo: map[span][]*Tree{}, busy: map[span]bool{}, max: max}
	return b.trees(g.Start, 0, n)
}

// builder reads trees back from the chart: a nonterminal over [i, j) has
// one tree per rule and per way of splitting the span among the rule's
// symbols, where every piece was itself completed (or is a token with
// that category).
type builder struct {
	g    *Grammar
	cats [][]string
	done map[span]bool
	memo map[span][]*Tree
	busy map[span]bool // guards against unit cycles (A -> B, B -> A)
	max  int
}

func (b *builder) full(n int) bool { return b.max > 0 && n >= b.max }

func (b *builder) trees(sym string, i, j int) []*Tree {
	if !b.g.Nonterminal(sym) {
		if j == i+1 && slices.Contains(b.cats[i], sym) {
			return []*Tree{{Label: sym, Start: i, End: j}}
		}
		return nil
	}
	s := span{sym, i, j}
	if !b.done[s] || b.busy[s] {
		return nil
	}
	if ts, ok := b.memo[s]; ok {
		return ts
	}
	b.busy[s] = true
	var out []*Tree
	for _, r := range b.g.byLHS[sym] {
		for _, kids := range b.split(b.g.Rules[r].RHS, i, j) {
			out = append(out, &Tree{Label: sym, Start: i, End: j, Children: kids})
			if b.full(len(out)) {
				break
			}
		}
		if b.full(len(out)) {
			break
		}
	}
	b.busy[s] = false
	b.memo[s] = out
	return out
}

// split returns every way the symbols can cover [i, j), as child lists.
func (b *builder) split(syms []string, i, j int) [][]*Tree {
	if len(syms) == 1 {
		var out [][]*Tree
		for _, t := range b.trees(syms[0], i, j) {
			out = append(out, []*Tree{t})
		}
		return out
	}
	var out [][]*Tree
	// Each symbol covers at least one token, so leave one per symbol left.
	for k := i + 1; k <= j-(len(syms)-1); k++ {
		firsts := b.trees(syms[0], i, k)
		if len(firsts) == 0 {
			continue
		}
		for _, rest := range b.split(syms[1:], k, j) {
			for _, f := range firsts {
				out = append(out, append([]*Tree{f}, rest...))
				if b.full(len(out)) {
					return out
				}
			}
		}
	}
	return out
}
