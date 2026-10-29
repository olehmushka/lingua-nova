package llm

import (
	"cmp"
	"context"
	"os"
	"path/filepath"
	"slices"

	"github.com/olehmushka/lingua-nova/internal/english"
	"github.com/olehmushka/lingua-nova/internal/gloss"
	"github.com/olehmushka/lingua-nova/internal/lexicon"
	"github.com/olehmushka/lingua-nova/internal/retrieve"
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

// Tokens estimates the context's length in tokens: about 3 bytes per
// token for this mix of English, Rifatian and markup.
func (c Context) Tokens() int { return c.Size() / 3 }

// Modes of building a context.
const (
	Full      = "full"      // every entry
	Retrieved = "retrieved" // the entries the sentence points to
	None      = "none"      // no entries: the model looks words up with tools
)

// Builder builds contexts from the language files.
type Builder struct {
	Grammar    string // grammar.md
	Morphology string // morphology.yaml
	Lex        *lexicon.Lexicon
	Tagger     *english.Tagger
	Examples   []Example
	// Retriever picks a retrieved context's open-class entries; its K and
	// Budget can be changed, and a Dense channel added (package semantic).
	Retriever *retrieve.Retriever
}

// closedFiles hold the closed classes (articles, na, pronouns, question
// words), which a retrieved context always includes: the English words
// don't reliably point to them (do-support, pro-drop, possessive 's).
var closedFiles = []string{"function.yaml", "pronouns.yaml"}

// closed says whether an entry is in a closed class.
func closed(e *lexicon.Entry) bool { return slices.Contains(closedFiles, filepath.Base(e.File)) }

// NewBuilder reads grammar.md and morphology.yaml from langDir. The
// grammar's interlinear examples become the prompt's worked examples.
// The retriever gets a BM25 index of the lexicon, the default limits and
// no dense channel.
func NewBuilder(langDir string, lex *lexicon.Lexicon, t *english.Tagger) (*Builder, error) {
	g, err := os.ReadFile(filepath.Join(langDir, "grammar.md"))
	if err != nil {
		return nil, err
	}
	m, err := os.ReadFile(filepath.Join(langDir, "morphology.yaml"))
	if err != nil {
		return nil, err
	}
	b := &Builder{Grammar: string(g), Morphology: string(m), Lex: lex, Tagger: t,
		Retriever: &retrieve.Retriever{Tagger: t, BM25: retrieve.NewBM25(lex.Entries), Always: closed, Budget: retrieve.DefaultBudget}}
	for _, x := range gloss.Examples(b.Grammar) {
		b.Examples = append(b.Examples, Example{English: x.Translation, Rifatian: x.Sentence(), Gloss: x.Gloss})
	}
	return b, nil
}

// Build returns the context for a sentence in the given mode. An error
// comes only from a retrieved context's dense channel, and the context
// is still usable then: it has the lexical channels' entries.
func (b *Builder) Build(ctx context.Context, mode, sentence string) (Context, error) {
	switch mode {
	case Retrieved:
		return b.Retrieved(ctx, sentence)
	case None:
		return b.context(func(*lexicon.Entry) bool { return false }), nil
	}
	return b.Full(), nil
}

// Full has every entry: about 16K tokens with the grammar for the real
// lexicon, and identical for every sentence, so the model server can
// reuse its cached prefix. It grows with the lexicon; the translator
// refuses one that doesn't fit (Options.MaxNumCtx).
func (b *Builder) Full() Context {
	return b.context(func(*lexicon.Entry) bool { return true })
}

// Retrieved has the closed classes plus the entries the retriever picks
// for the sentence's words: a few per word, fused from the tagger's
// readings, a BM25 search and (if set) search by meaning, within a token
// budget. This is retrieval-augmented generation: the prompt carries only
// what the question needs, so its size doesn't grow with the lexicon.
func (b *Builder) Retrieved(ctx context.Context, sentence string) (Context, error) {
	picks, err := b.Retriever.Sentence(ctx, sentence)
	want := map[string]bool{} // by id: the tagger may hold another copy of the lexicon
	for _, p := range picks {
		want[p.Entry.ID] = true
	}
	return b.context(func(e *lexicon.Entry) bool { return want[e.ID] || closed(e) }), err
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
