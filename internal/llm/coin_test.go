package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/olehmushka/lingua-nova/internal/lexicon"
	"github.com/olehmushka/lingua-nova/internal/phon"
	"github.com/olehmushka/lingua-nova/internal/semantic"
	"github.com/olehmushka/lingua-nova/internal/sound"
)

func coiner(t *testing.T, client *Client) *Coiner {
	t.Helper()
	inv, err := phon.Load(filepath.Join(langDir, "phonology.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	rules, err := sound.Load(filepath.Join(langDir, "sound-changes.txt"), inv)
	if err != nil {
		t.Fatal(err)
	}
	lex, err := lexicon.Load(filepath.Join(langDir, "lexicon"))
	if err != nil {
		t.Fatal(err)
	}
	return NewCoiner(client, inv, rules, lex)
}

var (
	lantern = Concept{English: "lantern", POS: "noun", Gender: "f"}
	whisper = Concept{English: "whisper", POS: "verb"}
)

func TestCoinCheck(t *testing.T) {
	c := coiner(t, nil)
	noRules := NewCoiner(nil, c.Inv, nil, c.Lex) // proto = modern: modern finals are stricter
	tests := []struct {
		name    string
		c       *Coiner
		proto   string
		concept Concept
		taken   map[string]string
		modern  string
		kind    string // first rejection; "" = accepted
		msg     string
	}{
		{"legal noun", c, "hurula", lantern, nil, "hurula", "", ""},
		{"normalized", c, " *Hurula", lantern, nil, "hurula", "", ""},
		{"legal verb", c, "tarar", whisper, nil, "tarar", "", ""},
		{"sound change", c, "patar", whisper, nil, "padar", "", ""},
		{"unknown letter", c, "hurwla", lantern, nil, "", RejSound, "unknown letter 'w'"},
		{"k for c", c, "kilir", whisper, nil, "", RejSound, "unknown letter 'k' at position 0 (write /k/ as c)"},
		{"q for c", c, "squilar", whisper, nil, "", RejSound, "(write /k/ as c)"},
		{"modern-only sound", c, "xalo", lantern, nil, "", RejSound, `"x" /ʃ/ is not a proto sound`},
		{"proto phonotactics", c, "tamsa", lantern, nil, "", RejProto, `illegal coda "m"`},
		{"verb ending", c, "parto", whisper, nil, "", RejVerb, "-ar, -er or -ir"},
		{"existing root", c, "nauca", lantern, nil, "nauga", RejExisting, `already a root: "fish"`},
		{"homophone", c, "nauga", lantern, nil, "nauga", RejHomophone, `modern nauga already means "fish" (fish)`},
		{"homophone of a candidate", c, "hurula", lantern, map[string]string{"hurula": "hurulla"}, "hurula", RejHomophone, "same word as your *hurulla"},
		{"modern phonotactics", noRules, "hurulan", lantern, nil, "hurulan", RejModern, `modern hurulan: illegal word-final consonant "n"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := tt.c.Check(tt.proto, tt.concept, tt.taken)
			if k.Modern != tt.modern {
				t.Errorf("modern = %q, want %q", k.Modern, tt.modern)
			}
			if tt.kind == "" {
				if !k.OK() {
					t.Errorf("rejected: %v", k.Rejections)
				}
				return
			}
			if k.OK() {
				t.Fatalf("accepted, want %s", tt.kind)
			}
			if r := k.Rejections[0]; r.Kind != tt.kind || !strings.Contains(r.Msg, tt.msg) {
				t.Errorf("rejection = %v, want %s containing %q", r, tt.kind, tt.msg)
			}
		})
	}
}

func TestCoinRemarks(t *testing.T) {
	c := coiner(t, nil)
	if k := c.Check("falio", lantern, nil); !k.OK() || len(k.Remarks) != 1 || !strings.Contains(k.Remarks[0], "feminine noun in -o") {
		t.Errorf("falio (f): %v %v, want accepted with a gender remark", k.Rejections, k.Remarks)
	}
	if k := c.Check("hurula", lantern, nil); len(k.Remarks) != 0 {
		t.Errorf("hurula (f): remarks %v, want none", k.Remarks)
	}
}

// TestCoinEntryYAML: an accepted candidate's YAML loads back as a lexicon
// entry that passes lnova validate, with modern = the derivation.
func TestCoinEntryYAML(t *testing.T) {
	c := coiner(t, nil)
	k := c.Check("pattuca", lantern, nil)
	if !k.OK() {
		t.Fatal(k.Rejections)
	}
	e := c.Entry(k, lantern, "lantern", `A lamp: a "fire" in a box.`, "qwen")
	y := EntryYAML(e)
	if !strings.Contains(y, "*pattuca → patuga (rule 2: c > g / V_V; rule 3: tt > t)") || !strings.Contains(y, "Coined by qwen") {
		t.Errorf("notes don't give the derivation:\n%s", y)
	}
	lex := &lexicon.Lexicon{}
	if err := lex.ParseFile("coined.yaml", []byte(y)); err != nil {
		t.Fatalf("%v\n%s", err, y)
	}
	if probs := lexicon.Validate(lex, c.Inv); len(probs) > 0 {
		t.Errorf("validate: %v\n%s", probs, y)
	}
	got := lex.Entries[0]
	if got.ID != "lantern" || got.Proto != "pattuca" || got.Modern != k.Modern || got.Gender != "f" ||
		got.Definition != e.Definition || got.Notes != e.Notes {
		t.Errorf("round trip: got %+v\nwant %+v", got, e)
	}
	segs, _ := c.Inv.Tokenize(got.Proto)
	if out, _ := sound.Evolve(c.Inv, c.Rules, segs); phon.Spell(out) != got.Modern {
		t.Errorf("modern %q, evolve gives %q", got.Modern, phon.Spell(out))
	}
}

func TestYAMLScalar(t *testing.T) {
	for _, s := range []string{"owl", "A bird.", "a: b", "-x", "yes", "12", "#tag", `say "hi"`, "*x → y (rule 1: a > b / _#)"} {
		var got string
		if err := yamlUnmarshal(yamlScalar(s), &got); err != nil || got != s {
			t.Errorf("yamlScalar(%q) = %s reads back as %q (%v)", s, yamlScalar(s), got, err)
		}
	}
	if yamlScalar("A bird.") != "A bird." {
		t.Errorf("plain text quoted: %s", yamlScalar("A bird."))
	}
}

// yamlUnmarshal reads one scalar through the lexicon loader's YAML parser.
func yamlUnmarshal(scalar string, out *string) error {
	lex := &lexicon.Lexicon{}
	if err := lex.ParseFile("x.yaml", []byte("- id: x\n  gloss: "+scalar+"\n")); err != nil {
		return err
	}
	*out = lex.Entries[0].Gloss
	return nil
}

func TestCoinPrompt(t *testing.T) {
	c := coiner(t, nil)
	p := c.Phonology()
	for _, want := range []string{
		"Consonants: p b t d c g f v s z h m n l r",
		"Never in a proto root (only sound change creates them): lh nh x j ch",
		"Word-final consonant: only r n m",
		"l+h, n+h, c+h, V+V+V",
		"- first: CVC (",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("phonology lacks %q:\n%s", want, p)
		}
	}
	ex := c.Examples(lantern, 30, nil)
	if len(ex) != 30 {
		t.Fatalf("%d examples, want 30", len(ex))
	}
	for i, r := range ex[:20] {
		if r.POS != "noun" {
			t.Errorf("example %d: %v, want the first 20 to be nouns", i, r)
		}
	}
	for _, r := range ex {
		if strings.Contains(r.Proto, "*") || r.Proto == "naucanuzio" {
			t.Errorf("example %v: want roots, no * and no compounds", r)
		}
	}
	if !slices.Equal(ex, c.Examples(lantern, 30, nil)) {
		t.Error("examples differ between calls")
	}
}

func TestCoinExisting(t *testing.T) {
	c := coiner(t, nil)
	if got := c.Existing(Concept{English: "fish"}); len(got) == 0 || got[0].ID != "fish" {
		t.Errorf("Existing(fish) = %v", got)
	}
	if got := c.Existing(Concept{English: "Lie down"}); len(got) != 1 || got[0].ID != "lie" {
		t.Errorf("Existing(lie down) = %v, want the entry glossed lie.down", got)
	}
	if got := c.Existing(lantern); len(got) != 0 {
		t.Errorf("Existing(lantern) = %v, want none", got)
	}
}

// fakeCoin answers each /coin request with the next scripted list of
// roots; "!" answers with an error.
type fakeCoin struct {
	answers [][]string
	reqs    []CoinRequest
}

func (f *fakeCoin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req CoinRequest
	json.NewDecoder(r.Body).Decode(&req)
	f.reqs = append(f.reqs, req)
	a := f.answers[min(len(f.reqs)-1, len(f.answers)-1)]
	if len(a) == 1 && a[0] == "!" {
		http.Error(w, `{"detail": "model reply is not a valid coinage"}`, http.StatusBadGateway)
		return
	}
	var cands []Candidate
	for _, p := range a {
		cands = append(cands, Candidate{Proto: p, Reason: "sounds right"})
	}
	json.NewEncoder(w).Encode(CoinResponse{Coinage: Coinage{Definition: "A lamp in a box.", Candidates: cands},
		PromptTokens: 1000, EvalTokens: 50, DurationMS: 2000})
}

func TestCoinLoop(t *testing.T) {
	tests := []struct {
		name     string
		answers  [][]string
		rounds   int
		accepted []string
		feedback string // in the second request's history
		err      string
	}{
		{"first round", [][]string{{"hurula", "pattura", "fiulo"}}, 1, []string{"hurula", "pattura", "fiulo"}, "", ""},
		{"fixed after feedback", [][]string{{"hurula", "tamsa", "xalo"}, {"tamsa", "pattura", "fiulo"}}, 2,
			[]string{"hurula", "pattura", "fiulo"}, `*tamsa: rejected: illegal coda "m"`, ""},
		{"gives up", [][]string{{"nauca", "xalo"}}, 3, nil, `*nauca: rejected: *nauca is already a root`, ""},
		{"accepted once", [][]string{{"hurula"}, {"hurula", "pattura"}, {"fiulo"}}, 3,
			[]string{"hurula", "pattura", "fiulo"}, "The accepted roots are taken now", ""},
		{"service error", [][]string{{"hurula"}, {"!"}}, 1, []string{"hurula"}, "", "not a valid coinage"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeCoin{answers: tt.answers}
			srv := httptest.NewServer(f)
			defer srv.Close()
			c := coiner(t, NewClient(srv.URL))
			res, err := c.Coin(context.Background(), lantern, CoinOptions{})
			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("err = %v, want %q", err, tt.err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if len(res.Rounds) != tt.rounds {
				t.Errorf("%d rounds, want %d", len(res.Rounds), tt.rounds)
			}
			var got []string
			for _, k := range res.Accepted {
				got = append(got, k.Proto)
			}
			if !slices.Equal(got, tt.accepted) {
				t.Errorf("accepted %v, want %v", got, tt.accepted)
			}
			for i, req := range f.reqs {
				if len(req.History) != i || req.N != 5 || req.Model != DefaultModel || len(req.Examples) != 30 || req.Phonology == "" {
					t.Errorf("request %d: %d history rounds, n=%d, model %q, %d examples", i, len(req.History), req.N, req.Model, len(req.Examples))
				}
			}
			if tt.feedback != "" && (len(f.reqs) < 2 || !strings.Contains(f.reqs[1].History[0].Feedback, tt.feedback)) {
				t.Errorf("second request's feedback doesn't contain %q: %+v", tt.feedback, f.reqs)
			}
			if res.Definition() != "A lamp in a box." {
				t.Errorf("definition %q", res.Definition())
			}
		})
	}
}

func TestCoinRepeatAndStats(t *testing.T) {
	f := &fakeCoin{answers: [][]string{{"tamsa", "xalo", "nauca"}, {"tamsa", "hurula"}, {"hurula", "pattura", "fiulo"}}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := coiner(t, NewClient(srv.URL))
	res, err := c.Coin(context.Background(), lantern, CoinOptions{Want: 2})
	if err != nil {
		t.Fatal(err)
	}
	if k := res.Rounds[1].Checked[0]; k.Rejections[0].Kind != RejRepeat {
		t.Errorf("second tamsa: %v, want a repeat", k.Rejections)
	}
	want := map[string]int{RejProto: 1, RejSound: 1, RejExisting: 1, RejRepeat: 2}
	if got := res.Rejected(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("Rejected() = %v, want %v", got, want)
	}
	if got := res.RejectedKinds(); got[0] != RejRepeat {
		t.Errorf("RejectedKinds() = %v, want repeat first", got)
	}
	if len(res.Accepted) != 3 || len(res.Rounds) != 3 {
		t.Errorf("%d accepted in %d rounds, want 3 in 3", len(res.Accepted), len(res.Rounds))
	}
}

// lightEmbedder puts texts about light and fire on one axis and
// everything else on another, so a lantern's neighbours are known.
// With failQuery, building works and every search fails.
type lightEmbedder struct{ failQuery bool }

func (f lightEmbedder) Embed(_ context.Context, kind string, texts []string) ([][]float32, error) {
	if f.failQuery && kind == semantic.Query {
		return nil, fmt.Errorf("embed down")
	}
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = []float32{0, 1}
		for _, w := range []string{"lantern", "fire", "light", "burn"} {
			if strings.HasPrefix(t, w+" ") {
				out[i] = []float32{1, 0.1}
			}
		}
	}
	return out, nil
}

func TestCoinRelatedExamples(t *testing.T) {
	c := coiner(t, nil)
	idx, _, err := semantic.Build(context.Background(), lightEmbedder{}, "m", c.Lex.Entries, "")
	if err != nil {
		t.Fatal(err)
	}
	c.Index = idx
	related, err := c.Related(context.Background(), lantern, 3, 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range related {
		got = append(got, e.ID)
	}
	slices.Sort(got)
	if want := []string{"burn", "fire", "light"}; !slices.Equal(got, want) {
		t.Errorf("Related(lantern) = %v, want %v (firefly is derived)", got, want)
	}

	ex := c.Examples(lantern, 30, related)
	if len(ex) != 30 {
		t.Fatalf("%d examples, want 30", len(ex))
	}
	seen := map[string]bool{}
	for i, r := range ex {
		if seen[r.Proto] {
			t.Errorf("example %d: *%s twice", i, r.Proto)
		}
		seen[r.Proto] = true
	}
	for i, e := range related {
		if ex[i].Gloss != e.Gloss {
			t.Errorf("example %d = %v, want the related %s first", i, ex[i], e.ID)
		}
	}
}

func TestCoinRelatedFallback(t *testing.T) {
	f := &fakeCoin{answers: [][]string{{"hurula", "pattura", "fiulo"}}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := coiner(t, NewClient(srv.URL))
	idx, _, err := semantic.Build(context.Background(), lightEmbedder{}, "m", c.Lex.Entries, "")
	if err != nil {
		t.Fatal(err)
	}
	c.Index = idx

	res, err := c.Coin(context.Background(), lantern, CoinOptions{Related: 10})
	if err != nil || res.Related != 10 || res.RelatedErr != nil || len(f.reqs[0].Examples) != 30 {
		t.Fatalf("related run: err %v, related %d (%v), %d examples", err, res.Related, res.RelatedErr, len(f.reqs[0].Examples))
	}

	// With a margin only the light words are close enough to the best.
	res, err = c.Coin(context.Background(), lantern, CoinOptions{Related: 10, RelatedMargin: 0.05})
	if err != nil || res.Related != 3 || len(res.Examples) != 30 {
		t.Fatalf("margin: err %v, related %d, %d examples", err, res.Related, len(res.Examples))
	}

	// The embedding model fails: coin still runs, on the spread examples.
	c.Index, _, _ = semantic.Build(context.Background(), lightEmbedder{failQuery: true}, "m", c.Lex.Entries, "")
	res, err = c.Coin(context.Background(), lantern, CoinOptions{Related: 10})
	if err != nil || res.Related != 0 || res.RelatedErr == nil || len(res.Examples) != 30 {
		t.Fatalf("fallback: err %v, related %d, relatedErr %v, %d examples", err, res.Related, res.RelatedErr, len(res.Examples))
	}
}
