package translate

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/olehmushka/lingua-nova/internal/earley"
	"github.com/olehmushka/lingua-nova/internal/english"
	"github.com/olehmushka/lingua-nova/internal/lexicon"
)

// clause is a tree reduced to what Rifatian needs: the roles, not the
// English word order.
type clause struct {
	wh        *earley.Tree // question word (WhP or WhAdv), fronted
	subj, obj *earley.Tree // NP nodes
	aux, neg  *earley.Tree
	verb      *earley.Tree
}

// read walks the tree in English order. The first NP before the verb is
// the subject and an NP after it is the object; the auxiliary and
// negation are found wherever English put them (did you not see / didn't
// you see / you didn't see).
func (c *clause) read(t *earley.Tree) {
	switch t.Label {
	case "NP":
		if c.verb == nil && c.subj == nil {
			c.subj = t
		} else {
			c.obj = t
		}
	case "Aux":
		c.aux = t
	case "Neg":
		c.neg = t
	case "V":
		c.verb = t
	case "WhP", "WhAdv":
		c.wh = t
	default:
		for _, k := range t.Children {
			c.read(k)
		}
	}
}

// gen builds one translation: a tree and a choice for each of its leaves.
type gen struct {
	t      *Translator
	s      english.Sentence
	pick   map[*earley.Tree]choice
	words  []Word
	notes  []string
	unknwn []string
}

func (t *Translator) generate(s english.Sentence, tree *earley.Tree, picks []choice) (Translation, error) {
	g := &gen{t: t, s: s, pick: map[*earley.Tree]choice{}}
	for i, l := range tree.Leaves() {
		g.pick[l] = picks[i]
	}
	var c clause
	c.read(tree)
	if c.verb == nil {
		return Translation{}, fmt.Errorf("no verb")
	}

	tense := g.pick[c.verb].r.Tense
	if c.aux != nil {
		tense = g.pick[c.aux].r.Tense // did see → PST: the auxiliary's tense moves to the verb
	}
	person := "3SG" // a question word as the subject: who saw…
	if c.subj != nil {
		person = g.person(c.subj)
	}
	// Double negation: a negative object needs na before the verb. A
	// negative subject doesn't (Zunhu zuvixe, like Portuguese Nada caiu).
	neg := c.neg != nil || (c.obj != nil && g.negative(c.obj))

	if c.wh != nil {
		g.plain(c.wh)
	}
	if c.subj != nil {
		if err := g.np(c.subj, "SBJ", false); err != nil {
			return Translation{}, err
		}
	}
	if neg {
		not := t.Tagger.Entry("not")
		if c.neg != nil {
			not = cmp.Or(g.pick[c.neg].e, not)
		}
		g.add(not, "")
	}
	if err := g.verbWord(c.verb, tense, person); err != nil {
		return Translation{}, err
	}
	if c.obj != nil {
		if err := g.np(c.obj, "OBJ", false); err != nil {
			return Translation{}, err
		}
	}

	if len(g.words) > 0 {
		g.words[0].Text = capitalize(g.words[0].Text)
	}
	texts := make([]string, len(g.words))
	for i, w := range g.words {
		texts[i] = w.Text
	}
	return Translation{
		Text:    strings.Join(texts, " ") + g.s.End,
		Words:   g.words,
		Tree:    tree,
		Notes:   g.notes,
		Unknown: g.unknwn,
	}, nil
}

func (g *gen) add(e *lexicon.Entry, text string) {
	if e != nil && text == "" {
		text = base(e)
	}
	g.words = append(g.words, Word{Text: text, Entry: e})
}

// base is an entry's modern word, uninflected.
func base(e *lexicon.Entry) string { return cmp.Or(e.Word, e.Modern) }

// unknown adds an English word with no lexicon entry as ?word.
func (g *gen) unknown(leaf *earley.Tree) {
	w := g.s.Tokens[leaf.Start].Text
	g.unknwn = append(g.unknwn, w)
	g.add(nil, "?"+strings.ToLower(w))
}

// plain adds a word that doesn't inflect: articles, adjectives, question
// words.
func (g *gen) plain(leaf *earley.Tree) {
	if c := g.pick[leaf]; c.e != nil {
		g.add(c.e, "")
	} else {
		g.unknown(leaf)
	}
}

func (g *gen) inflect(leaf *earley.Tree, feats ...string) error {
	c := g.pick[leaf]
	if c.e == nil {
		g.unknown(leaf)
		return nil
	}
	if c.e.POS == "pronoun" {
		if _, ok := g.t.Rules.Pronouns[c.e.ID]; !ok {
			g.add(c.e, "") // nothing, who, what: no paradigm
			return nil
		}
	}
	f, err := g.t.Rules.Inflect(c.e, feats...)
	if err != nil {
		return err
	}
	g.add(c.e, f.Word())
	return nil
}

func (g *gen) verbWord(leaf *earley.Tree, tense, person string) error {
	return g.inflect(leaf, tense, person)
}

// person is the verb person a subject NP takes: a pronoun's own (or the
// one it agrees as, polite sela → 3), a noun phrase's is 3rd, with the
// number of its head noun.
func (g *gen) person(np *earley.Tree) string {
	h := g.head(np)
	c := g.pick[h]
	if h.Label == "Pron" {
		if c.e == nil {
			return "3SG"
		}
		p, ok := g.t.Rules.Pronouns[c.e.ID]
		if !ok {
			return "3SG" // nothing
		}
		return cmp.Or(p.Agrees, p.Person) + c.r.Num
	}
	return "3" + cmp.Or(c.r.Num, "SG")
}

// head is the leaf that decides an NP's person and number: the pronoun,
// or the noun of the outermost Nom (the dog in "the woman's dog").
func (g *gen) head(np *earley.Tree) *earley.Tree {
	last := np.Children[len(np.Children)-1]
	switch last.Label {
	case "Pron", "N":
		return last
	case "Nom":
		return g.head(last)
	}
	return last
}

// negative reports whether an NP is a negative word (nothing).
func (g *gen) negative(np *earley.Tree) bool {
	h := g.head(np)
	e := g.pick[h].e
	return h.Label == "Pron" && e != nil && e.ID == "nothing"
}

// np adds a noun phrase. Rifatian keeps the determiner or possessor
// first and puts adjectives after the noun. possessor means the NP is
// followed by 's: its noun takes the POSS suffix.
func (g *gen) np(np *earley.Tree, kase string, possessor bool) error {
	k := np.Children
	switch {
	case len(k) == 1 && k[0].Label == "Pron":
		if possessor {
			kase = "POSS"
		}
		return g.inflect(k[0], g.pick[k[0]].r.Num, kase)
	case len(k) == 1: // Nom
		return g.nom(k[0], possessor)
	case k[0].Label == "Det":
		g.plain(k[0])
		return g.nom(k[1], possessor)
	case k[0].Label == "Poss":
		if err := g.inflect(k[0], g.pick[k[0]].r.Num, "POSS"); err != nil {
			return err
		}
		return g.nom(k[1], possessor)
	case k[0].Label == "NP": // NP Gen Nom: [the woman]'s dog
		if err := g.np(k[0], kase, true); err != nil {
			return err
		}
		return g.nom(k[2], possessor)
	}
	return fmt.Errorf("unexpected NP %v", k)
}

// nom adds a noun and its adjectives, noun first: big dog → huro gumo.
func (g *gen) nom(nom *earley.Tree, possessor bool) error {
	var adjs []*earley.Tree
	for nom.Children[0].Label == "Adj" {
		adjs = append(adjs, nom.Children[0])
		nom = nom.Children[1]
	}
	n := nom.Children[0]
	var feats []string
	if g.pick[n].r.Num == "PL" {
		feats = append(feats, "PL")
	}
	if possessor {
		feats = append(feats, "POSS") // on the noun, before its adjectives: lu menas lirma huro
	}
	if err := g.inflect(n, feats...); err != nil {
		return err
	}
	for _, a := range adjs {
		g.plain(a)
	}
	return nil
}
