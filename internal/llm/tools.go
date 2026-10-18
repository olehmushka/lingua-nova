package llm

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/olehmushka/lingua-nova/internal/english"
	"github.com/olehmushka/lingua-nova/internal/lexicon"
	"github.com/olehmushka/lingua-nova/internal/morph"
)

// Toolbox is the Go functions the model can call: the dictionary, the
// inflector, the analyzer and the validator. The model never runs them
// itself. It asks for a call (function calling), Go runs it, and the
// result goes back into the conversation as a tool message.
//
// Results are short plain text, not JSON: a small model reads text
// better. A bad call (unknown id, wrong features) returns "error: …" as
// its result, so the model can correct it; it never ends the loop.
type Toolbox struct {
	Lex      *lexicon.Lexicon
	Rules    *morph.Rules
	Analyzer *morph.Analyzer
	Tagger   *english.Tagger
	Checker  *Checker

	byID   map[string]*lexicon.Entry
	byWord map[string][]*lexicon.Entry // citation form → entries (homophones)
}

// NewToolbox returns the tools over a lexicon and its morphology.
func NewToolbox(lex *lexicon.Lexicon, r *morph.Rules, t *english.Tagger, k *Checker) *Toolbox {
	b := &Toolbox{Lex: lex, Rules: r, Analyzer: morph.NewAnalyzer(r, lex), Tagger: t, Checker: k,
		byID: map[string]*lexicon.Entry{}, byWord: map[string][]*lexicon.Entry{}}
	for _, e := range lex.Entries {
		b.byID[e.ID] = e
		w := cmp.Or(e.Word, e.Modern)
		b.byWord[w] = append(b.byWord[w], e)
	}
	return b
}

// params is a JSON schema for an object of string arguments, all
// required; a name ending in [] is an array of strings.
func params(names ...string) map[string]any {
	props := map[string]any{}
	var req []string
	for _, n := range names {
		desc := ""
		n, desc, _ = strings.Cut(n, ":")
		if base, ok := strings.CutSuffix(n, "[]"); ok {
			props[base] = map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": desc}
			n = base
		} else {
			props[n] = map[string]any{"type": "string", "description": desc}
		}
		req = append(req, n)
	}
	return map[string]any{"type": "object", "properties": props, "required": req}
}

// toolParams is each tool's argument names in order, for showing a call.
var toolParams = map[string][]string{
	"lookup":  {"english"},
	"inflect": {"id", "features"},
	"analyze": {"word"},
	"check":   {"sentence"},
}

// Specs describes the tools for the model.
func (b *Toolbox) Specs() []Tool {
	return []Tool{
		{"lookup", "Find the Rifatian dictionary entries for an English word. Returns id | Rifatian word | part of speech | gender | gloss | definition, one per line.",
			params("english:One English word, e.g. dog or saw.")},
		{"inflect", "Build an inflected form of a dictionary entry. Verbs need a tense and a person (PRS 3SG, PST 1PL, FUT 2SG); nouns take none, PL, POSS or both; pronouns a number and a case (SG SBJ, PL OBJ, SG POSS).",
			params("id:The dictionary id, e.g. see (or its Rifatian word, narar).", "features[]:The features, e.g. [\"PST\", \"3SG\"].")},
		{"analyze", "Read a Rifatian word back: which dictionary entry it is a form of, and with which features.",
			params("word:One Rifatian word.")},
		{"check", "Run the Rifatian analyzer on a draft translation of the sentence. Returns OK, or how it reads the draft and what is wrong.",
			params("sentence:The whole Rifatian sentence.")},
	}
}

// Run runs one call. x is what the sentence being translated must mean,
// for check.
func (b *Toolbox) Run(c ToolCall, x *Expectation) ToolResult {
	var out string
	switch c.Name {
	case "lookup":
		out = b.lookup(c.str("english"))
	case "inflect":
		out = b.inflect(c.str("id"), c.list("features"))
	case "analyze":
		out = b.analyze(c.str("word"))
	case "check":
		out = b.check(c.str("sentence"), x)
	default:
		out = fmt.Sprintf("error: no tool %q; the tools are lookup, inflect, analyze and check", c.Name)
	}
	return ToolResult{Name: c.Name, Content: out}
}

// runOnce runs a call unless the same call (same tool, same arguments as
// String prints them) already ran for this sentence. Then it returns the
// earlier result with a note instead. The tools are deterministic, so a
// repeat can't learn anything new. A small model stuck on one idea sends
// the same call round after round (the "doom loop"); saying so gives it a
// reason to change course, where a fresh copy of the result doesn't.
// seen maps String() to the earlier result and is updated.
func (b *Toolbox) runOnce(c ToolCall, x *Expectation, seen map[string]string) ToolResult {
	key := c.String()
	if prev, ok := seen[key]; ok {
		return ToolResult{Name: c.Name, Content: fmt.Sprintf(
			"repeated call: you already called %s and got:\n%s\nIt will give the same result again. Use it, or call something different.",
			key, prev)}
	}
	r := b.Run(c, x)
	seen[key] = r.Content
	return r
}

// lookup finds entries the way retrieval does (tag, lemmatize, look up
// by gloss, then the dictionary search), for each word given.
func (b *Toolbox) lookup(word string) string {
	if strings.TrimSpace(word) == "" {
		return "error: give an English word"
	}
	var lines []string
	seen := map[string]bool{}
	for _, tok := range b.Tagger.Tag(word).Tokens {
		for _, id := range candidates(b.Lex, tok) {
			if e := b.byID[id]; e != nil && !seen[id] {
				seen[id] = true
				lines = append(lines, fmt.Sprintf("%s | %s | %s | %s | %s | %s",
					e.ID, cmp.Or(e.Word, e.Modern), e.POS, cmp.Or(e.Gender, "-"), e.Gloss, e.Definition))
			}
		}
	}
	if len(lines) == 0 {
		return fmt.Sprintf("no entry for %q; try a synonym", word)
	}
	return strings.Join(lines, "\n")
}

// inflect takes an id or a dictionary word: models tend to send the
// word they just looked up (baugir) where the id (eat) is asked for.
func (b *Toolbox) inflect(id string, feats []string) string {
	e := b.byID[id]
	if e == nil {
		switch es := b.byWord[strings.ToLower(id)]; len(es) {
		case 0:
			return fmt.Sprintf("error: no entry with id %q; use lookup to find the id", id)
		case 1:
			e = es[0]
		default:
			var ids []string
			for _, e := range es {
				ids = append(ids, fmt.Sprintf("%s (%s)", e.ID, e.Gloss))
			}
			return fmt.Sprintf("error: %s is the word of %d entries: %s; give the id", id, len(es), strings.Join(ids, ", "))
		}
	}
	switch e.POS {
	case "verb", "noun", "pronoun":
	default:
		return fmt.Sprintf("%s is a %s and doesn't inflect; use it as is: %s", id, e.POS, cmp.Or(e.Word, e.Modern))
	}
	f, err := b.Rules.Inflect(e, feats...)
	if err != nil {
		return "error: " + err.Error()
	}
	return fmt.Sprintf("%s  (%s)", f.Word(), f.Gloss())
}

func (b *Toolbox) analyze(word string) string {
	word = strings.ToLower(clean(word))
	as, err := b.Analyzer.Analyze(word)
	if err != nil {
		return "error: " + err.Error()
	}
	var lines []string
	for _, a := range as {
		if a.Guess() {
			lines = append(lines, fmt.Sprintf("not in the dictionary (looks like a form of an unknown %s %s)", a.POS, a.Lemma))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s = %s, %s %s  (%s)", word, a.Entry.ID, a.POS, label(a.Form.Feats), a.Form.Gloss()))
	}
	if len(lines) == 0 {
		return fmt.Sprintf("%s is not a form of any dictionary word", word)
	}
	return strings.Join(lines, "\n")
}

// check is the validator the loop runs on the answer, run on a draft.
// Only problems count; remarks are about the answer's word list, which a
// draft doesn't have.
func (b *Toolbox) check(sentence string, x *Expectation) string {
	c := b.Checker.Verify(Answer{Translation: sentence}, x)
	if c.OK() {
		return "OK"
	}
	return c.Feedback()
}

// str is a string argument; models sometimes send a number or a list.
func (c ToolCall) str(name string) string {
	switch v := c.Arguments[name].(type) {
	case nil:
		return ""
	case string:
		return v
	case []any:
		return strings.Join(c.list(name), " ")
	default:
		return fmt.Sprint(v)
	}
}

// list is a list-of-strings argument; a string is split on spaces,
// commas and dots ("PST.3SG").
func (c ToolCall) list(name string) []string {
	var out []string
	add := func(s string) {
		out = append(out, strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' || r == '.' })...)
	}
	switch v := c.Arguments[name].(type) {
	case string:
		add(v)
	case []any:
		for _, x := range v {
			add(fmt.Sprint(x))
		}
	}
	return out
}

// String shows the call: inflect(see, [PST 3SG]). Features print the same
// however they were sent, so it's also the key for spotting repeated calls.
func (c ToolCall) String() string {
	var args []string
	for _, p := range toolParams[c.Name] {
		switch v := c.Arguments[p].(type) {
		case []any:
			args = append(args, "["+strings.Join(c.list(p), " ")+"]")
		case string:
			switch {
			case p == "features": // "PST.3SG" is the same features as ["PST", "3SG"]
				v = "[" + strings.Join(c.list(p), " ") + "]"
			case c.Name == "check":
				v = fmt.Sprintf("%q", v)
			}
			args = append(args, v)
		case nil:
		default:
			args = append(args, fmt.Sprint(v))
		}
	}
	return c.Name + "(" + strings.Join(args, ", ") + ")"
}
