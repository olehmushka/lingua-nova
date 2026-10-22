package llm

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/olehmushka/lingua-nova/internal/lexicon"
	"github.com/olehmushka/lingua-nova/internal/phon"
	"github.com/olehmushka/lingua-nova/internal/semantic"
	"github.com/olehmushka/lingua-nova/internal/sound"
)

// Coining new words: the model proposes proto roots for a concept the
// lexicon lacks, and the phonology, the sound changes and the lexicon
// validator accept or reject each one. Rejections go back to the model,
// as in the translation loop.
//
// It is generate-and-test once more, and for a reason: a language model
// reads tokens (chunks of letters), not sounds, so "no l before h" or
// "only r n m at the end" in the prompt is a weak constraint. The
// validator is the strong one. The acceptance rate per candidate measures
// how much of the phonology the model picks up from the prompt alone.

// Concept is a meaning to coin a root for.
type Concept struct {
	English    string `json:"english"` // the gloss: "owl", "lie down"
	POS        string `json:"pos"`
	Gender     string `json:"gender"`
	Definition string `json:"definition"` // "": the model writes one
}

// ID is the lexicon id a coined entry gets: the English with spaces as _.
func (c Concept) ID() string { return strings.Join(strings.Fields(strings.ToLower(c.English)), "_") }

// Gloss is the English with spaces as dots, the lexicon's gloss style.
func (c Concept) Gloss() string { return strings.Join(strings.Fields(strings.ToLower(c.English)), ".") }

// Candidate is one proposed root (the /coin answer schema in llm/app.py).
type Candidate struct {
	Proto  string `json:"proto"`
	Reason string `json:"reason"`
}

// Coinage is the model's answer to one /coin request.
type Coinage struct {
	Definition string      `json:"definition"`
	Candidates []Candidate `json:"candidates"`
}

// Root is an existing entry shown to the model as an example.
type Root struct {
	Proto  string `json:"proto"`
	Modern string `json:"modern"`
	POS    string `json:"pos"`
	Gloss  string `json:"gloss"`
}

// Round is an earlier answer and the feedback on it.
type Round struct {
	Coinage  Coinage `json:"coinage"`
	Feedback string  `json:"feedback"`
}

// CoinRequest is one /coin call.
type CoinRequest struct {
	Concept     Concept `json:"concept"`
	Phonology   string  `json:"phonology"`
	Examples    []Root  `json:"examples"`
	History     []Round `json:"history,omitempty"`
	N           int     `json:"n"` // candidates to ask for
	Model       string  `json:"model"`
	NumCtx      int     `json:"num_ctx"`
	Temperature float64 `json:"temperature"`
	Seed        *int    `json:"seed,omitempty"`
}

// CoinResponse is the service's reply to a CoinRequest.
type CoinResponse struct {
	Coinage      Coinage `json:"coinage"`
	Model        string  `json:"model"`
	PromptTokens int     `json:"prompt_tokens"`
	EvalTokens   int     `json:"eval_tokens"`
	DurationMS   int     `json:"duration_ms"`
}

// Coin sends one /coin call.
func (c *Client) Coin(ctx context.Context, req CoinRequest) (*CoinResponse, error) {
	var out CoinResponse
	if err := c.post(ctx, "/coin", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Rejection kinds, in the order Check tests them.
const (
	RejSound     = "sound"               // a letter that isn't a proto sound
	RejProto     = "proto-phonotactics"  // an illegal onset, coda, sequence...
	RejVerb      = "verb-ending"         // a verb not in -ar/-er/-ir
	RejModern    = "modern-phonotactics" // the sound changes give an illegal modern word
	RejExisting  = "existing-proto"      // the root is already in the lexicon
	RejHomophone = "homophone"           // the modern word is already a word
	RejEntry     = "entry"               // anything else lexicon.Validate finds
	RejRepeat    = "repeat"              // proposed before for this concept
)

// Rejection is one reason a candidate was rejected.
type Rejection struct {
	Kind string
	Msg  string
}

// Coined is the verdict on one candidate: its modern form and derivation
// if it got that far, and why it was rejected (nothing: accepted).
// Remarks don't reject.
type Coined struct {
	Proto      string // normalized: lower case, no *
	Modern     string
	Steps      []sound.Step
	Reason     string // the model's
	Rejections []Rejection
	Remarks    []string
}

// OK reports whether the candidate was accepted.
func (k Coined) OK() bool { return len(k.Rejections) == 0 }

// Coiner runs the coin loop and checks candidates.
type Coiner struct {
	Client *Client
	Inv    *phon.Inventory
	Rules  []sound.Rule
	Lex    *lexicon.Lexicon
	Index  *semantic.Index // nil: examples chosen without regard to meaning

	byProto  map[string]*lexicon.Entry
	byModern map[string]*lexicon.Entry // modern form and word override
}

// NewCoiner returns a coiner for the language.
func NewCoiner(client *Client, inv *phon.Inventory, rules []sound.Rule, lex *lexicon.Lexicon) *Coiner {
	c := &Coiner{Client: client, Inv: inv, Rules: rules, Lex: lex,
		byProto: map[string]*lexicon.Entry{}, byModern: map[string]*lexicon.Entry{}}
	for _, e := range lex.Entries {
		if p := strings.TrimPrefix(e.Proto, "*"); c.byProto[p] == nil {
			c.byProto[p] = e
		}
		for _, m := range []string{e.Modern, e.Word} {
			if m != "" && c.byModern[m] == nil {
				c.byModern[m] = e
			}
		}
	}
	return c
}

// Existing returns the entries that already have the concept's id or
// gloss (the whole gloss, or its part before the first dot, as the
// translator reads it).
func (c *Coiner) Existing(concept Concept) []*lexicon.Entry {
	id, gloss := concept.ID(), concept.Gloss()
	var out []*lexicon.Entry
	for _, e := range c.Lex.Entries {
		head, _, _ := strings.Cut(e.Gloss, ".")
		if e.ID == id || e.Gloss == gloss || head == gloss {
			out = append(out, e)
		}
	}
	return out
}

// Check runs one candidate through the checks, stopping at the first
// stage that fails: its sounds, the proto phonotactics and verb ending,
// then (on the modern form the sound changes give) the modern
// phonotactics, the lexicon (an existing root or homophone, also against
// taken: modern → proto of the candidates already accepted), and last the
// full lexicon validator on the entry it would become.
func (c *Coiner) Check(proto string, concept Concept, taken map[string]string) Coined {
	k := Coined{Proto: strings.ToLower(strings.TrimPrefix(strings.TrimSpace(proto), "*"))}
	reject := func(kind, format string, args ...any) {
		k.Rejections = append(k.Rejections, Rejection{kind, fmt.Sprintf(format, args...)})
	}
	segs, err := c.Inv.Tokenize(k.Proto)
	if err != nil {
		// The model's English spelling habit (*kilir, *squil); the
		// spelling is c for /k/, always.
		if strings.ContainsAny(k.Proto, "kq") {
			reject(RejSound, "%v (write /k/ as c)", err)
		} else {
			reject(RejSound, "%v", err)
		}
		return k
	}
	for _, s := range segs {
		if !s.Proto {
			reject(RejSound, "%q /%s/ is not a proto sound (only sound change creates it)", s.Spell, s.IPA)
		}
	}
	if !k.OK() {
		return k
	}
	for _, v := range c.Inv.CheckProtoPhonotactics(segs) {
		reject(RejProto, "%s", v.Msg)
	}
	if concept.POS == "verb" && !lexicon.HasVerbEnding(k.Proto) {
		reject(RejVerb, "a verb must end in -ar, -er or -ir")
	}
	if !k.OK() {
		return k
	}

	out, steps := sound.Evolve(c.Inv, c.Rules, segs)
	k.Modern, k.Steps = phon.Spell(out), steps
	for _, v := range c.Inv.CheckPhonotactics(out) {
		reject(RejModern, "modern %s: %s", k.Modern, v.Msg)
	}
	if e := c.byProto[k.Proto]; e != nil {
		reject(RejExisting, "*%s is already a root: %q (%s)", k.Proto, e.Gloss, e.ID)
	}
	if e := c.byModern[k.Modern]; e != nil {
		reject(RejHomophone, "modern %s already means %q (%s)", k.Modern, e.Gloss, e.ID)
	} else if p, ok := taken[k.Modern]; ok {
		reject(RejHomophone, "modern %s is the same word as your *%s", k.Modern, p)
	}
	if !k.OK() {
		return k
	}

	e := c.Entry(k, concept, concept.ID(), cmp.Or(concept.Definition, "-"), "")
	for _, p := range lexicon.Validate(&lexicon.Lexicon{Entries: []*lexicon.Entry{e}}, c.Inv) {
		reject(RejEntry, "%s", p.Msg)
	}
	if concept.POS == "noun" {
		switch {
		case concept.Gender == "m" && strings.HasSuffix(k.Modern, "a"):
			k.Remarks = append(k.Remarks, "masculine noun in -a (grammar: -a is typically feminine)")
		case concept.Gender == "f" && strings.HasSuffix(k.Modern, "o"):
			k.Remarks = append(k.Remarks, "feminine noun in -o (grammar: -o is typically masculine)")
		}
	}
	return k
}

// Entry is the lexicon entry an accepted candidate becomes. Its notes give
// the derivation in the lexicon's style and, with a model, who coined it.
func (c *Coiner) Entry(k Coined, concept Concept, id, definition, model string) *lexicon.Entry {
	var rules []string
	for _, s := range k.Steps {
		rules = append(rules, fmt.Sprintf("rule %d: %s", s.Rule.Group, s.Rule.Text))
	}
	notes := fmt.Sprintf("*%s → %s", k.Proto, k.Modern)
	if len(rules) > 0 {
		notes += " (" + strings.Join(rules, "; ") + ")"
	}
	notes += "."
	if model != "" {
		notes += fmt.Sprintf(" Coined by %s (lnova coin).", model)
	}
	return &lexicon.Entry{ID: id, Proto: k.Proto, Modern: k.Modern, POS: concept.POS, Gender: concept.Gender,
		Gloss: concept.Gloss(), Definition: definition, Notes: notes}
}

// EntryYAML formats an entry as a lexicon file item, in the field order
// of language/README.md.
func EntryYAML(e *lexicon.Entry) string {
	var b strings.Builder
	field := func(name, val string) {
		if val != "" {
			fmt.Fprintf(&b, "  %s: %s\n", name, yamlScalar(val))
		}
	}
	fmt.Fprintf(&b, "- id: %s\n", yamlScalar(e.ID))
	field("proto", e.Proto)
	field("modern", e.Modern)
	field("word", e.Word)
	field("pos", e.POS)
	field("gender", e.Gender)
	field("gloss", e.Gloss)
	field("definition", e.Definition)
	field("notes", e.Notes)
	return b.String()
}

// yamlScalar writes s plain when YAML reads it back as the same string,
// else double-quoted (Go's quoting is valid YAML there).
func yamlScalar(s string) string {
	plain := s != "" && strings.TrimSpace(s) == s &&
		!strings.ContainsAny(s[:1], "*&!|>%@`'\"#{}[],?:-") &&
		!strings.Contains(s, ": ") && !strings.Contains(s, " #") && !strings.ContainsAny(s, "\n\t\"")
	if plain {
		if _, err := strconv.ParseFloat(s, 64); err == nil {
			plain = false
		}
		switch strings.ToLower(s) {
		case "true", "false", "yes", "no", "on", "off", "null", "~":
			plain = false
		}
	}
	if plain {
		return s
	}
	return strconv.Quote(s)
}

// Phonology describes the proto sounds and phonotactics for the prompt,
// from phonology.yaml: what lnova validate checks, in words.
func (c *Coiner) Phonology() string {
	inv := c.Inv
	var cons, vows, special, modern []string
	for _, s := range inv.Segments {
		switch {
		case !s.Proto:
			modern = append(modern, s.Spell)
		case s.Kind == phon.Consonant:
			cons = append(cons, s.Spell)
		default:
			vows = append(vows, s.Spell)
		}
		if s.Proto && s.Spell != s.IPA {
			special = append(special, fmt.Sprintf("%s = /%s/", s.Spell, s.IPA))
		}
	}
	seqs := func(ss [][]string, sep, between string) string {
		out := make([]string, len(ss))
		for i, s := range ss {
			out[i] = strings.Join(s, sep)
		}
		return strings.Join(out, between)
	}
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	line("Consonants: %s", strings.Join(cons, " "))
	line("Vowels: %s", strings.Join(vows, " "))
	if len(special) > 0 {
		line("Letters with a special sound: %s", strings.Join(special, ", "))
	}
	if len(modern) > 0 {
		line("Never in a proto root (only sound change creates them): %s", strings.Join(modern, " "))
	}
	line("Diphthongs (one syllable): %s. Any other two vowels are two syllables.", seqs(inv.Diphthongs, "", " "))
	line("A syllable is (onset) vowel-or-diphthong (coda).")
	line("Onset: none, one consonant, or one of these clusters: %s.", seqs(inv.Onsets, "", " "))
	coda := fmt.Sprintf("Coda inside a word: only %s", strings.Join(inv.ProtoCodas, " "))
	if len(inv.Geminates) > 0 {
		var g []string
		for _, s := range inv.Geminates {
			g = append(g, s+s)
		}
		coda += fmt.Sprintf(", or the first half of a double %s between two vowels (*cup.pe)", strings.Join(g, " "))
	}
	line("%s. No other double consonants.", coda)
	line("Word-final consonant: only %s; otherwise the word ends in a vowel.", strings.Join(inv.ProtoFinals, " "))
	if len(inv.HiatusAfter) > 0 {
		line("Two vowels in separate syllables (hiatus) only after %s, as in *u.si.a.", strings.Join(inv.HiatusAfter, " "))
	}
	line("Never anywhere in a word: %s (V = any vowel, C = any consonant).", seqs(inv.Forbidden, "+", ", "))
	if g := inv.Generator; g != nil {
		var syl []phon.Weighted[string]
		for _, w := range g.Syllables {
			syl = append(syl, phon.Weighted[string]{Value: strconv.Itoa(w.Value), Weight: w.Weight})
		}
		line("Syllables per word: %s.", percents(syl))
		line("Syllable shapes (C consonant, V vowel, CC cluster, VV diphthong):")
		for _, p := range []struct {
			name string
			ts   []phon.Template
		}{{"first", g.Initial}, {"middle", g.Medial}, {"last", g.Final}, {"one-syllable word", g.Mono}} {
			var ws []phon.Weighted[string]
			for _, t := range p.ts {
				ws = append(ws, phon.Weighted[string]{Value: t.Text, Weight: t.Weight})
			}
			line("- %s: %s", p.name, percents(ws))
		}
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// percents lists values by weight, biggest first: "2 (76%), 3 (20%)".
func percents(ws []phon.Weighted[string]) string {
	ws = slices.Clone(ws)
	slices.SortStableFunc(ws, func(a, b phon.Weighted[string]) int { return cmp.Compare(b.Weight, a.Weight) })
	total := 0
	for _, w := range ws {
		total += w.Weight
	}
	out := make([]string, len(ws))
	for i, w := range ws {
		p := fmt.Sprintf("%d%%", (w.Weight*100+total/2)/total)
		if p == "0%" {
			p = "<1%"
		}
		out[i] = w.Value + " (" + p + ")"
	}
	return strings.Join(out, ", ")
}

// Examples picks up to n roots to show the model the style: first the
// related ones (by meaning, from Related), then entries of the concept's
// part of speech (and gender, for a noun), spread evenly over the
// lexicon, topped up with other roots. Derived words (notes with "+":
// compounds, diminutives) are left out, because a new word is a fresh root
// and the long compounds would skew its length.
func (c *Coiner) Examples(concept Concept, n int, related []*lexicon.Entry) []Root {
	root := func(e *lexicon.Entry) Root {
		return Root{Proto: strings.TrimPrefix(e.Proto, "*"), Modern: cmp.Or(e.Word, e.Modern), POS: e.POS, Gloss: e.Gloss}
	}
	var out []Root
	seen := map[*lexicon.Entry]bool{}
	for _, e := range related[:min(n, len(related))] {
		out = append(out, root(e))
		seen[e] = true
	}
	var same, other []*lexicon.Entry
	for _, e := range c.Lex.Entries {
		switch {
		case seen[e] || derived(e):
		case e.POS == concept.POS && (concept.Gender == "" || e.Gender == concept.Gender):
			same = append(same, e)
		default:
			other = append(other, e)
		}
	}
	spread := func(es []*lexicon.Entry, k int) {
		if k <= 0 || len(es) == 0 {
			return
		}
		for i := range min(k, len(es)) {
			out = append(out, root(es[i*len(es)/min(k, len(es))]))
		}
	}
	spread(same, (n-len(out))*2/3)
	spread(other, n-len(out))
	return out
}

// derived reports whether an entry is built from other words (its notes
// show the parts with "+"), not a root.
func derived(e *lexicon.Entry) bool { return strings.Contains(e.Notes, "+") }

// Related returns up to k roots closest in meaning to the concept, by the
// semantic index: the concept is embedded like an entry ("lantern (noun):
// A lamp in a case."), and derived words are left out as in Examples. Any
// part of speech counts: "burn" and "fire" both tell the model what a
// lantern is about. Without an index it returns nothing.
//
// With margin > 0, only roots scoring within margin of the best one are
// kept. Scores are relative to the model and the query: when the lexicon
// has few words near a concept, the best hit stands out and the rest
// cluster just below it (lantern: light 0.64, then squid, lion, whale
// around 0.55), so a fixed cut-off would fit one model and query only.
// The margin is still a number to tune per embedding model.
func (c *Coiner) Related(ctx context.Context, concept Concept, k int, margin float64) ([]*lexicon.Entry, error) {
	if c.Index == nil || k <= 0 {
		return nil, nil
	}
	q := semantic.QueryText(concept.English, concept.POS, concept.Definition)
	hits, err := c.Index.Search(ctx, q, k, func(e *lexicon.Entry) bool { return !derived(e) })
	if err != nil {
		return nil, err
	}
	var out []*lexicon.Entry
	for _, h := range hits {
		if margin > 0 && h.Score < hits[0].Score-margin {
			break
		}
		out = append(out, h.Entry)
	}
	return out, nil
}

// CoinOptions configure Coin.
type CoinOptions struct {
	Model     string // "" = DefaultModel
	N         int    // candidates per round; 0 = 5
	Want      int    // stop after this many are accepted; 0 = 3
	MaxRounds int    // 0 = 3
	Examples  int    // example roots in the prompt; 0 = 30
	Related   int    // how many of them are chosen by meaning (needs Coiner.Index); 0 = none
	// RelatedMargin keeps only related roots within this score of the
	// best one, so fewer than Related when few are close; 0 = no cut-off.
	RelatedMargin float64
	NumCtx        int // 0 = 8192
	Temperature   float64
	Seed          *int
}

// CoinRound is one model call and the verdict on each candidate.
type CoinRound struct {
	Coinage      Coinage
	Checked      []Coined
	PromptTokens int
	EvalTokens   int
	Duration     time.Duration
}

// Feedback tells the model which of its candidates passed and why the
// others failed.
func (r CoinRound) Feedback() string {
	var b strings.Builder
	accepted := false
	for _, k := range r.Checked {
		if k.OK() {
			accepted = true
			fmt.Fprintf(&b, "- *%s → %s: accepted.\n", k.Proto, k.Modern)
			continue
		}
		msgs := make([]string, len(k.Rejections))
		for i, rj := range k.Rejections {
			msgs[i] = rj.Msg
		}
		fmt.Fprintf(&b, "- *%s: rejected: %s.\n", k.Proto, strings.Join(msgs, "; "))
	}
	if accepted {
		b.WriteString("The accepted roots are taken now: make the new ones different.\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// CoinResult is every round for one concept; Accepted lists the
// accepted candidates in the order they came.
type CoinResult struct {
	Concept    Concept
	Examples   []Root // the first Related of them chosen by meaning
	Related    int
	RelatedErr error // the semantic search failed; examples without it
	Rounds     []CoinRound
	Accepted   []Coined
}

// Definition is the concept's definition, or else the model's first one.
func (r *CoinResult) Definition() string {
	if r.Concept.Definition != "" {
		return r.Concept.Definition
	}
	for _, rd := range r.Rounds {
		if d := strings.TrimSpace(rd.Coinage.Definition); d != "" {
			return d
		}
	}
	return ""
}

// Rejected counts the rejected candidates by the kind of their first
// rejection.
func (r *CoinResult) Rejected() map[string]int {
	out := map[string]int{}
	for _, rd := range r.Rounds {
		for _, k := range rd.Checked {
			if !k.OK() {
				out[k.Rejections[0].Kind]++
			}
		}
	}
	return out
}

// RejectedKinds is Rejected's keys, most frequent first.
func (r *CoinResult) RejectedKinds() []string {
	counts := r.Rejected()
	return slices.SortedStableFunc(maps.Keys(counts), func(a, b string) int {
		return cmp.Or(cmp.Compare(counts[b], counts[a]), cmp.Compare(a, b))
	})
}

// Coin asks the model for roots for the concept, checks them, and sends
// the verdicts back until Want roots are accepted or MaxRounds is
// reached. An error ends the loop; the rounds so far are in the result.
func (c *Coiner) Coin(ctx context.Context, concept Concept, opt CoinOptions) (*CoinResult, error) {
	opt.Model = cmp.Or(opt.Model, DefaultModel)
	opt.N = cmp.Or(opt.N, 5)
	opt.Want = cmp.Or(opt.Want, 3)
	opt.MaxRounds = cmp.Or(opt.MaxRounds, 3)
	opt.Examples = cmp.Or(opt.Examples, 30)
	opt.NumCtx = cmp.Or(opt.NumCtx, 8192)

	res := &CoinResult{Concept: concept}
	related, err := c.Related(ctx, concept, min(opt.Related, opt.Examples), opt.RelatedMargin)
	if err != nil {
		res.RelatedErr = err
	}
	res.Examples, res.Related = c.Examples(concept, opt.Examples, related), len(related)
	req := CoinRequest{Concept: concept, Phonology: c.Phonology(), Examples: res.Examples, N: opt.N,
		Model: opt.Model, NumCtx: opt.NumCtx, Temperature: opt.Temperature, Seed: opt.Seed}
	tried := map[string]bool{}
	taken := map[string]string{} // modern → proto of the accepted candidates
	for range opt.MaxRounds {
		resp, err := c.Client.Coin(ctx, req)
		if err != nil {
			return res, err
		}
		rd := CoinRound{Coinage: resp.Coinage, PromptTokens: resp.PromptTokens, EvalTokens: resp.EvalTokens,
			Duration: time.Duration(resp.DurationMS) * time.Millisecond}
		for _, cand := range resp.Coinage.Candidates {
			k := c.Check(cand.Proto, concept, taken)
			if tried[k.Proto] {
				k = Coined{Proto: k.Proto, Rejections: []Rejection{{RejRepeat, "proposed before"}}}
			}
			k.Reason = cand.Reason
			tried[k.Proto] = true
			if k.OK() {
				taken[k.Modern] = k.Proto
				res.Accepted = append(res.Accepted, k)
			}
			rd.Checked = append(rd.Checked, k)
		}
		res.Rounds = append(res.Rounds, rd)
		if len(res.Accepted) >= opt.Want {
			break
		}
		req.History = append(req.History, Round{Coinage: resp.Coinage, Feedback: rd.Feedback()})
	}
	return res, nil
}
