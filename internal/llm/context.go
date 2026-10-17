package llm

import (
	"cmp"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/olehmushka/lingua-nova/internal/english"
	"github.com/olehmushka/lingua-nova/internal/gloss"
	"github.com/olehmushka/lingua-nova/internal/lexicon"
)

// Context is what the model is told about Rifatian.
type Context struct {
	Grammar    string    `json:"grammar"`
	Morphology string    `json:"morphology"`
	Entries    []Entry   `json:"entries"`
	Examples   []Example `json:"examples,omitempty"`
}

// Entry is a dictionary entry as the prompt shows it.
type Entry struct {
	ID         string `json:"id"`
	Word       string `json:"word"`
	POS        string `json:"pos"`
	Gender     string `json:"gender"`
	Gloss      string `json:"gloss"`
	Definition string `json:"definition"`
}

// Example is a worked translation for the prompt (few-shot).
type Example struct {
	English  string `json:"english"`
	Rifatian string `json:"rifatian"`
	Gloss    string `json:"gloss"`
}

// Size is the context's length in bytes, for choosing the model's
// context window.
func (c Context) Size() int {
	n := len(c.Grammar) + len(c.Morphology)
	for _, e := range c.Entries {
		n += len(e.ID) + len(e.Word) + len(e.POS) + len(e.Gender) + len(e.Gloss) + len(e.Definition) + 16
	}
	for _, x := range c.Examples {
		n += len(x.English) + len(x.Rifatian) + len(x.Gloss) + 32
	}
	return n
}

// Modes of building a context.
const (
	Full      = "full"      // every entry
	Retrieved = "retrieved" // the entries the sentence points to
)

// Builder builds contexts from the language files.
type Builder struct {
	Grammar    string // grammar.md
	Morphology string // morphology.yaml
	Lex        *lexicon.Lexicon
	Tagger     *english.Tagger
	Examples   []Example
}

// closedFiles hold the closed classes (articles, na, pronouns, question
// words), which a retrieved context always includes: the English words
// don't reliably point to them (do-support, pro-drop, possessive 's).
var closedFiles = []string{"function.yaml", "pronouns.yaml"}

// NewBuilder reads grammar.md and morphology.yaml from langDir. The
// grammar's interlinear examples become the prompt's worked examples.
func NewBuilder(langDir string, lex *lexicon.Lexicon, t *english.Tagger) (*Builder, error) {
	g, err := os.ReadFile(filepath.Join(langDir, "grammar.md"))
	if err != nil {
		return nil, err
	}
	m, err := os.ReadFile(filepath.Join(langDir, "morphology.yaml"))
	if err != nil {
		return nil, err
	}
	b := &Builder{Grammar: string(g), Morphology: string(m), Lex: lex, Tagger: t}
	for _, x := range gloss.Examples(b.Grammar) {
		b.Examples = append(b.Examples, Example{English: x.Translation, Rifatian: x.Sentence(), Gloss: x.Gloss})
	}
	return b, nil
}

// Build returns the context for a sentence in the given mode.
func (b *Builder) Build(mode, sentence string) Context {
	if mode == Retrieved {
		return b.Retrieved(sentence)
	}
	return b.Full()
}

// Full has every entry: about 16K tokens with the grammar, and identical
// for every sentence, so the model server can reuse its cached prefix.
func (b *Builder) Full() Context {
	return b.context(func(*lexicon.Entry) bool { return true })
}

// Retrieved has the closed classes plus the entries the sentence's
// words point to, found the way the rule-based translator finds them:
// tag, lemmatize, look up by gloss. A word with no entry falls back to
// the dictionary search, which also matches definitions (big → large).
// This is retrieval-augmented generation in miniature: the prompt carries
// only what the question needs.
func (b *Builder) Retrieved(sentence string) Context {
	want := map[string]bool{}
	for _, tok := range b.Tagger.Tag(sentence).Tokens {
		for _, id := range candidates(b.Lex, tok) {
			want[id] = true
		}
	}
	return b.context(func(e *lexicon.Entry) bool {
		return want[e.ID] || slices.Contains(closedFiles, filepath.Base(e.File))
	})
}

// candidates are the entries an English token can translate to: its
// readings' entries, or for a word with none, the top dictionary hits.
func candidates(lex *lexicon.Lexicon, tok english.Token) []string {
	var ids []string
	for _, r := range tok.Readings {
		for _, e := range r.Entries {
			if !slices.Contains(ids, e.ID) {
				ids = append(ids, e.ID)
			}
		}
	}
	if len(ids) > 0 {
		return ids
	}
	lemma := strings.ToLower(tok.Text)
	if len(tok.Readings) > 0 {
		lemma = tok.Readings[0].Lemma
	}
	for i, h := range lex.Search(lemma) {
		if i == 3 || h.Match > lexicon.MatchWord {
			break
		}
		ids = append(ids, h.Entry.ID)
	}
	return ids
}

func (b *Builder) context(keep func(*lexicon.Entry) bool) Context {
	c := Context{Grammar: b.Grammar, Morphology: b.Morphology, Entries: []Entry{}, Examples: b.Examples}
	for _, e := range b.Lex.Entries {
		if keep(e) {
			c.Entries = append(c.Entries, Entry{
				ID: e.ID, Word: cmp.Or(e.Word, e.Modern), POS: e.POS, Gender: e.Gender,
				Gloss: e.Gloss, Definition: e.Definition,
			})
		}
	}
	return c
}
