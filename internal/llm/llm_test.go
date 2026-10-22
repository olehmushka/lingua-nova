package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/olehmushka/lingua-nova/internal/lexicon"
	"github.com/olehmushka/lingua-nova/internal/translate"
)

const langDir = "../../language"

var (
	once     sync.Once
	sBuilder *Builder
	sChecker *Checker
	sTools   *Toolbox
	setupErr error
)

// setup is shared: loading the tagger's model takes a moment.
func setup(t *testing.T) (*Builder, *Checker) {
	t.Helper()
	once.Do(func() {
		tr, err := translate.Load(langDir)
		if err != nil {
			setupErr = err
			return
		}
		lex, err := lexicon.Load(filepath.Join(langDir, "lexicon"))
		if err != nil {
			setupErr = err
			return
		}
		sBuilder, setupErr = NewBuilder(langDir, lex, tr.Tagger)
		ids := map[string]bool{}
		for _, e := range lex.Entries {
			ids[e.ID] = true
		}
		sChecker = &Checker{Glosser: tr.Glosser, Known: func(id string) bool { return ids[id] }, Ref: tr, Lex: lex}
		sTools = NewToolbox(lex, tr.Rules, tr.Tagger, sChecker)
	})
	if setupErr != nil {
		t.Fatal(setupErr)
	}
	return sBuilder, sChecker
}

func ids(c Context) []string {
	var out []string
	for _, e := range c.Entries {
		out = append(out, e.ID)
	}
	return out
}

func TestContext(t *testing.T) {
	b, _ := setup(t)
	full := b.Full()
	if len(full.Entries) != len(b.Lex.Entries) {
		t.Errorf("full: %d entries, want all %d", len(full.Entries), len(b.Lex.Entries))
	}
	if len(full.Examples) < 3 {
		t.Errorf("full: %d examples, want grammar.md's", len(full.Examples))
	}
	tests := []struct {
		sentence string
		want     []string
		not      []string
	}{
		{"The woman sees the river.", []string{"the", "woman", "see", "river", "i", "not"}, []string{"kill", "dog"}},
		{"The men will drink the water.", []string{"man", "drink", "water"}, []string{"woman"}},
		{"Didst thou see the woman's dog?", []string{"thou", "see", "woman", "dog"}, []string{"river"}},
	}
	for _, tt := range tests {
		got := ids(b.Retrieved(tt.sentence))
		for _, id := range tt.want {
			if !slices.Contains(got, id) {
				t.Errorf("%q: retrieved %v, missing %s", tt.sentence, got, id)
			}
		}
		for _, id := range tt.not {
			if slices.Contains(got, id) {
				t.Errorf("%q: retrieved %s, which the sentence doesn't need", tt.sentence, id)
			}
		}
	}
	r := b.Retrieved("The woman sees the river.")
	if len(r.Entries) >= len(full.Entries)/3 {
		t.Errorf("retrieved %d of %d entries; retrieval should cut most of them", len(r.Entries), len(full.Entries))
	}
	if none := b.Build(None, "The woman sees the river."); len(none.Entries) != 0 || none.Grammar == "" {
		t.Errorf("none: %d entries, grammar %d bytes; want no entries, the grammar", len(none.Entries), len(none.Grammar))
	}
	if n, m := numCtx(r, false), numCtx(full, false); n%1024 != 0 || m%1024 != 0 || n >= m {
		t.Errorf("numCtx: retrieved %d, full %d; want multiples of 1024, retrieved smaller", n, m)
	}
}

func w(text, id string, feats ...string) AnswerWord {
	return AnswerWord{Text: text, LexiconID: id, Features: feats}
}

func TestVerify(t *testing.T) {
	_, k := setup(t)
	good := []AnswerWord{w("Lu", "the"), w("mena", "woman"), w("nara", "see", "PRS", "3SG"), w("lu", "the"), w("trubo.", "river")}
	tests := []struct {
		name    string
		answer  Answer
		problem string // "" = must pass
		remark  string
	}{
		{"good", Answer{Translation: "Lu mena nara lu trubo.", Words: good}, "", ""},
		{"no word list", Answer{Translation: "Lu mena nara lu trubo."}, "", "missing from the answer's word list"},
		{"made-up word", Answer{Translation: "Lu mena nara lu blomp."}, "blomp: not a form of any dictionary word", ""},
		{"guessed root", Answer{Translation: "Lu mena nara lu pumo."}, "pumo: not in the dictionary", ""},
		{"wrong word", Answer{Translation: "Lu huro nara lu trubo.", Words: []AnswerWord{w("huro", "woman")}},
			`huro is a form of "dog"`, ""},
		{"made-up id", Answer{Translation: "Lu mena nara lu trubo.", Words: []AnswerWord{w("mena", "lady")}},
			"", `claimed id "lady" is not in the dictionary`},
		{"wrong features", Answer{Translation: "Lu mena nara lu trubo.", Words: []AnswerWord{w("nara", "see", "PST", "3SG")}},
			"", "claimed PST.3SG, the analyzer says PRS.3SG"},
		{"agreement", Answer{Translation: "Mu nara lu trubo."}, "agree", ""},
		{"empty", Answer{}, "empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k.Verify(tt.answer, nil)
			probs, rems := strings.Join(c.Problems, "\n"), strings.Join(c.Remarks, "\n")
			if tt.problem == "" && !c.OK() {
				t.Errorf("want pass, got problems:\n%s", probs)
			}
			if tt.problem != "" && !strings.Contains(probs, tt.problem) {
				t.Errorf("want a problem containing %q, got:\n%s", tt.problem, probs)
			}
			if tt.remark != "" && !strings.Contains(rems, tt.remark) {
				t.Errorf("want a remark containing %q, got:\n%s", tt.remark, rems)
			}
		})
	}
}

// Every interlinear example in grammar.md is a known-good answer, in
// form and in meaning.
func TestVerifyGrammarExamples(t *testing.T) {
	b, k := setup(t)
	for _, ex := range b.Examples {
		if c := k.Verify(Answer{Translation: ex.Rifatian}, k.Expect(ex.English)); !c.OK() {
			t.Errorf("%q: %v", ex.Rifatian, c.Problems)
		}
	}
}

// TestMeaning: answers that are good Rifatian but say something else. The
// bad answers are the local model's from the first runs (SESSION-LOG
// 2026-10-17); each passed the form check alone.
func TestMeaning(t *testing.T) {
	_, k := setup(t)
	tests := []struct {
		english, answer string
		kind            string   // prefix of Expectation.Kind
		want            []string // substrings of the meaning problems; none = must pass
	}{
		// Right, including a dropped subject pronoun.
		{"The dog eats the bread.", "Lu huro bauge lu gurna.", "frame", nil},
		{"We will love the god.", "Muh noidaremu lu ouri.", "frame", nil},
		{"We will love the god.", "Noidaremu lu ouri.", "frame", nil},
		{"I saw nothing.", "Na naraxu zunhu.", "frame", nil},
		{"Who saw the men?", "Ifa naraxe lu dubroh?", "frame", nil},
		// Infinitive for the verb, no article.
		{"The dog eats the bread.", "Huro baugir lu gurna.", "frame",
			[]string{"no conjugated verb; the English needs eat-TH.3SG"}},
		// Possessive as subject, present for past, adjective first, no article.
		{"She didn't see the big house.", "Hane na nara gumo hoida.", "frame",
			[]string{`subject: the English needs "3SG.F"; you have "3SG.F.POSS"`,
				"verb: the English needs see-TH-PST-3SG; you have see-TH.3SG",
				`object: the English needs "the house big", in this order; you have "big house"`}},
		// na after the verb, white for big.
		{"She didn't see the big house.", "Hana naraxe na lu hoida lirma.", "frame",
			[]string{"negation: the English is negative; NEG goes right before the verb",
				`you have "NEG the house white"`}},
		// Changed subject to satisfy agreement.
		{"We will love the god.", "Selah noidareh lu ouri.", "frame",
			[]string{`subject: the English needs "1PL"; you have "2PL.POL"`}},
		// Wrong ending on one of the five "love" verbs: the feedback is
		// about the ending, not about choosing another verb.
		{"We will love the god.", "Muh noidareh lu ouri.", "frame",
			[]string{"verb: the English needs love.storge-TH-FUT-1PL; you have love.storge-TH-FUT-3PL"}},
		// Wrong tense only.
		{"Who saw the men?", "Ifa nara lu dubroh?", "frame", []string{"verb: the English needs see-TH-PST-3SG"}},
		// Copula, prepositions, "of" as possessive, a subordinate clause.
		{"The warrior's sword is long.", "Lu gurzos halto iha estra.", "frame", nil},
		{"The king was a warrior.", "Lu dauzo ihaxe ne gurzo.", "frame", nil},
		{"The children of the king sleep in the house.", "Lu dauzos orloh sileh bi lu hoida.", "frame", nil},
		{"The birds fly over the mountain.", "Lu frinhah letah fai lu velda.", "frame", nil},
		{"The birds fly over the mountain.", "Lu frinhah letah lu velda.", "frame", []string{`object: the English needs "over the mountain"`}},
		{"I think that the river is cold.", "Mu sozu itu lu trubo iha grepo.", "frame", nil},
		{"The woman gave bread to the child.", "Lu mena hibaxe gurna epe lu orlo.", "frame", nil},
		// A PP after a noun isn't in the English grammar: no parse, so
		// the lexical check, which wants the preposition too.
		{"The birds over the mountain fly.", "Lu frinhah letah.", "lexical", []string{`"over" is missing`}},
		// No parse (an adverb): only the words are checked.
		{"The dog sleeps quickly.", "Lu huro sile.", "lexical", nil},
		{"The dog sleeps quickly.", "Lu mena sile.", "lexical",
			[]string{`"dog" is missing`, "mena (woman): no English word means this"}},
	}
	for _, tt := range tests {
		t.Run(tt.english+" "+tt.answer, func(t *testing.T) {
			x := k.Expect(tt.english)
			if !strings.HasPrefix(x.Kind(), tt.kind) {
				t.Fatalf("kind %q, want %s", x.Kind(), tt.kind)
			}
			c := k.Verify(Answer{Translation: tt.answer}, x)
			got := strings.Join(c.Meaning, "\n")
			if len(tt.want) == 0 && !c.OK() {
				t.Errorf("want pass, got problems:\n%s", strings.Join(c.Problems, "\n"))
			}
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("want a meaning problem containing %q, got:\n%s", w, got)
				}
			}
		})
	}
}

// TestExpectWords: the content words the lexical check requires, for
// sentences the grammar doesn't parse (a PP after a noun). Prepositions
// and the copula count, "of" doesn't (it's -s); is/was are be, not
// entries that mention "is" in their definition.
func TestExpectWords(t *testing.T) {
	_, k := setup(t)
	tests := []struct {
		english string
		want    string // word:ids … in order
		unknown string
	}{
		{"The dog in the house sleeps.", "dog:dog in:in house:house sleeps:sleep", ""},
		{"The children of the king in the house sleep.", "children:child king:king in:in house:house sleep:sleep", ""},
		{"The woman in the house gave bread to the child.", "woman:woman in:in house:house gave:give bread:bread to:to child:child", ""},
		{"The dog in the house is big.", "dog:dog in:in house:house is:be big:big", ""},
		{"The men in the house were strong.", "men:man in:in house:house were:be strong:strong", ""},
		{"The birds over the mountain fly.", "birds:bird over:over mountain:mountain fly:fly_insect,fly", ""}, // the tagger: fly/NN
		{"The dragon sleeps.", "sleeps:sleep", "dragon"},
	}
	for _, tt := range tests {
		t.Run(tt.english, func(t *testing.T) {
			x := k.Expect(tt.english)
			if !strings.HasPrefix(x.Kind(), "lexical") {
				t.Fatalf("kind %q, want lexical", x.Kind())
			}
			var got []string
			for _, w := range x.Words {
				got = append(got, w.Word+":"+strings.Join(w.IDs, ","))
			}
			if g := strings.Join(got, " "); g != tt.want {
				t.Errorf("words %q, want %q", g, tt.want)
			}
			if g := strings.Join(x.Unknown, " "); g != tt.unknown {
				t.Errorf("unknown %q, want %q", g, tt.unknown)
			}
		})
	}
}

func TestFeedback(t *testing.T) {
	_, k := setup(t)
	fb := k.Verify(Answer{Translation: "Lu mena nara lu blomp."}, nil).Feedback()
	for _, want := range []string{"see-TH.3SG", "Problems:", "- blomp: not a form of any dictionary word"} {
		if !strings.Contains(fb, want) {
			t.Errorf("feedback missing %q:\n%s", want, fb)
		}
	}
}

func call(name string, args ...any) ToolCall {
	c := ToolCall{Name: name, Arguments: map[string]any{}}
	for i := 0; i < len(args); i += 2 {
		c.Arguments[args[i].(string)] = args[i+1]
	}
	return c
}

func TestTools(t *testing.T) {
	_, k := setup(t)
	x := k.Expect("The dog eats the bread.")
	tests := []struct {
		call ToolCall
		want []string // substrings of the result
		not  string
	}{
		{call("lookup", "english", "dog"), []string{"dog | huro | noun | m | dog"}, ""},
		{call("lookup", "english", "dogs"), []string{"dog | huro"}, ""},
		{call("lookup", "english", "zxqv"), []string{`no entry for "zxqv"`}, ""},
		{call("inflect", "id", "see", "features", []any{"PST", "3SG"}), []string{"naraxe  (see-TH-PST-3SG)"}, ""},
		{call("inflect", "id", "eat", "features", "PRS.3SG"), []string{"bauge  (eat-TH.3SG)"}, ""},
		{call("inflect", "id", "dog", "features", []any{"PL"}), []string{"huroh"}, ""},
		{call("inflect", "id", "see", "features", []any{"FOO", "3SG"}), []string{"error:", "PRS PST FUT"}, ""},
		{call("inflect", "id", "baugir", "features", []any{"PRS", "3SG"}), []string{"bauge  (eat-TH.3SG)"}, ""},
		{call("inflect", "id", "lady", "features", []any{}), []string{`error: no entry with id "lady"`, "lookup"}, ""},
		{call("inflect", "id", "the", "features", []any{}), []string{"doesn't inflect; use it as is: lu"}, ""},
		{call("analyze", "word", "Naraxe,"), []string{"naraxe = see, verb PST.3SG  (see-TH-PST-3SG)"}, ""},
		{call("analyze", "word", "pumo"), []string{"not in the dictionary"}, ""},
		{call("check", "sentence", "Lu huro bauge lu gurna."), []string{"OK"}, "Problems"},
		{call("check", "sentence", "Huro baugir lu gurna."), []string{"Problems:", "eat-TH.3SG"}, ""},
		{call("fly"), []string{`error: no tool "fly"`}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.call.String(), func(t *testing.T) {
			got := sTools.Run(tt.call, x)
			if got.Name != tt.call.Name {
				t.Errorf("result name %q", got.Name)
			}
			for _, w := range tt.want {
				if !strings.Contains(got.Content, w) {
					t.Errorf("result:\n%s\nwant it to contain %q", got.Content, w)
				}
			}
			if tt.not != "" && strings.Contains(got.Content, tt.not) {
				t.Errorf("result:\n%s\nshould not contain %q", got.Content, tt.not)
			}
		})
	}
	if s := call("inflect", "id", "see", "features", []any{"PST", "3SG"}).String(); s != "inflect(see, [PST 3SG])" {
		t.Errorf("String() = %q", s)
	}
	for _, sp := range sTools.Specs() {
		if _, ok := toolParams[sp.Name]; !ok {
			t.Errorf("spec %s has no toolParams", sp.Name)
		}
	}
}

// fakeService answers with scripted replies, one per request, and
// records the requests. A reply "→name args…" is a tool call (args as
// key=value), "…" is a tool round with no calls; anything else is an
// answer.
type fakeService struct {
	answers []string
	reqs    []Request
}

func (f *fakeService) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	data, _ := io.ReadAll(r.Body)
	if strings.Contains(string(data), `"history":null`) {
		http.Error(rw, "history must be a list or absent", http.StatusUnprocessableEntity)
		return
	}
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}
	f.reqs = append(f.reqs, req)
	text := f.answers[min(len(f.reqs), len(f.answers))-1]
	if text == "!malformed" {
		rw.WriteHeader(http.StatusBadGateway)
		rw.Write([]byte(`{"detail": "model reply is not a valid answer"}`))
		return
	}
	if name, ok := strings.CutPrefix(text, "→"); ok {
		f := strings.Fields(name)
		c := ToolCall{Name: f[0], Arguments: map[string]any{}}
		for _, kv := range f[1:] {
			k, v, _ := strings.Cut(kv, "=")
			c.Arguments[k] = strings.ReplaceAll(v, "_", " ")
		}
		json.NewEncoder(rw).Encode(Response{Calls: []ToolCall{c}, PromptTokens: 100, EvalTokens: 10, DurationMS: 5})
		return
	}
	if text == "…" {
		json.NewEncoder(rw).Encode(Response{Content: "done", PromptTokens: 100, EvalTokens: 10, DurationMS: 5})
		return
	}
	json.NewEncoder(rw).Encode(Response{Answer: &Answer{Translation: text}, PromptTokens: 100, EvalTokens: 10, DurationMS: 5})
}

func TestTranslateLoop(t *testing.T) {
	b, k := setup(t)
	const bad, good = "Lu mena nara lu blomp.", "Lu mena nara lu trubo."
	const blomp = "blomp: not a form of any dictionary word"
	tests := []struct {
		name     string
		answers  []string
		attempts int
		ok       bool
		err      string
		feedback string // in every retry
	}{
		{"first try", []string{good}, 1, true, "", ""},
		{"fixed after feedback", []string{bad, good}, 2, true, "", blomp},
		{"gives up", []string{bad}, 3, false, "", blomp},
		{"malformed reply", []string{bad, "!malformed"}, 1, false, "not a valid answer", blomp},
		// Good Rifatian, wrong meaning (the dog, not the woman).
		{"fixed after meaning feedback", []string{"Lu huro nara lu trubo.", good}, 2, true, "",
			`subject: the English needs "the woman"; you have "the dog"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeService{answers: tt.answers}
			srv := httptest.NewServer(f)
			defer srv.Close()
			tr := &Translator{Client: NewClient(srv.URL), Checker: k}
			ctx := b.Retrieved("The woman sees the river.")
			res, err := tr.Translate(context.Background(), "The woman sees the river.", ctx, Options{})
			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("err = %v, want %q", err, tt.err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if len(res.Attempts) != tt.attempts || res.OK != tt.ok {
				t.Fatalf("%d attempts, ok=%v; want %d, %v", len(res.Attempts), res.OK, tt.attempts, tt.ok)
			}
			for i, req := range f.reqs {
				if len(req.History) != i {
					t.Errorf("request %d has %d history turns, want %d", i, len(req.History), i)
				}
				if req.Model != DefaultModel || req.NumCtx == 0 || len(req.Context.Entries) != len(ctx.Entries) {
					t.Errorf("request %d: model %q, num_ctx %d, %d entries", i, req.Model, req.NumCtx, len(req.Context.Entries))
				}
				if i > 0 && !strings.Contains(req.History[i-1].Feedback, tt.feedback) {
					t.Errorf("request %d feedback:\n%s", i, req.History[i-1].Feedback)
				}
			}
		})
	}
}

// TestSampling: temperature and seed reach every call of a sentence,
// tool rounds and retries included; without a seed none is sent.
func TestSampling(t *testing.T) {
	b, k := setup(t)
	seed := 7
	tests := []struct {
		name string
		opt  Options
		temp float64
		seed *int
	}{
		{"greedy", Options{}, 0, nil},
		{"sampled", Options{Temperature: 0.7, Seed: &seed}, 0.7, &seed},
		{"sampled with tools", Options{Temperature: 0.7, Seed: &seed, Tools: sTools}, 0.7, &seed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeService{answers: []string{"→lookup english=dog", "…", "Lu huro bauge lu blomp.", "Lu huro bauge lu gurna."}}
			if tt.opt.Tools == nil {
				f.answers = f.answers[2:]
			}
			srv := httptest.NewServer(f)
			defer srv.Close()
			tr := &Translator{Client: NewClient(srv.URL), Checker: k}
			en := "The dog eats the bread."
			if _, err := tr.Translate(context.Background(), en, b.Retrieved(en), tt.opt); err != nil {
				t.Fatal(err)
			}
			if len(f.reqs) < 2 {
				t.Fatalf("%d requests", len(f.reqs))
			}
			for i, req := range f.reqs {
				if req.Temperature != tt.temp || (req.Seed == nil) != (tt.seed == nil) || (req.Seed != nil && *req.Seed != *tt.seed) {
					t.Errorf("request %d: temperature %v, seed %v", i, req.Temperature, req.Seed)
				}
			}
		})
	}
}

func TestToolLoop(t *testing.T) {
	b, k := setup(t)
	const en = "The dog eats the bread."
	const good = "Lu huro bauge lu gurna."
	tests := []struct {
		name     string
		answers  []string
		rounds   int
		reqs     int   // requests sent
		steps    []int // steps per attempt
		finals   []int // indexes of the requests with final set
		ok       bool
		contains string // in some tool result
	}{
		{"inflect then answer", []string{"→inflect id=eat features=PRS_3SG", "…", good}, 0, 3, []int{1}, []int{2}, true, "bauge"},
		{"check then answer", []string{"→check sentence=Lu_huro_bauge_lu_gurna.", "…", good}, 0, 3, []int{1}, []int{2}, true, "OK"},
		{"no calls", []string{"…", good}, 0, 2, []int{0}, []int{1}, true, ""},
		{"round cap forces final", []string{"→lookup english=dog", "→lookup english=dog", good}, 2, 3, []int{2}, []int{2}, true, "huro"},
		{"tools again after feedback", []string{"…", "Huro baugir lu gurna.", "→analyze word=baugir", "…", good}, 0, 5, []int{0, 1}, []int{1, 4}, true, "eat"},
		{"repeated call", []string{"→lookup english=dog", "→lookup english=dog", "…", good}, 0, 4, []int{2}, []int{3}, true,
			"repeated call: you already called lookup(dog) and got:\ndog | huro"},
		{"repeated across attempts", []string{"→analyze word=baugir", "…", "Huro baugir lu gurna.", "→analyze word=baugir", "…", good}, 0, 6, []int{1, 1}, []int{2, 5}, true,
			"repeated call: you already called analyze(baugir)"},
		// Loose input: "PRS 3SG" and "PRS.3SG" are the same features.
		{"repeated with other spelling", []string{"→inflect id=eat features=PRS_3SG", "→inflect id=eat features=PRS.3SG", "…", good}, 0, 4, []int{2}, []int{3}, true,
			"repeated call: you already called inflect(eat, [PRS 3SG])"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeService{answers: tt.answers}
			srv := httptest.NewServer(f)
			defer srv.Close()
			tr := &Translator{Client: NewClient(srv.URL), Checker: k}
			opt := Options{Tools: sTools, MaxToolRounds: tt.rounds, MaxAttempts: len(tt.steps)}
			res, err := tr.Translate(context.Background(), en, b.Build(None, en), opt)
			if err != nil {
				t.Fatal(err)
			}
			if len(f.reqs) != tt.reqs {
				t.Fatalf("%d requests, want %d", len(f.reqs), tt.reqs)
			}
			if res.OK != tt.ok {
				t.Errorf("ok = %v, want %v", res.OK, tt.ok)
			}
			var finals, steps []int
			var results []string
			tokens := 0
			for i, req := range f.reqs {
				if len(req.Tools) != 4 {
					t.Errorf("request %d has %d tools", i, len(req.Tools))
				}
				if req.Final {
					finals = append(finals, i)
				}
			}
			for _, a := range res.Attempts {
				steps = append(steps, len(a.Steps))
				for _, st := range a.Steps {
					for _, r := range st.Results {
						results = append(results, r.Content)
					}
				}
				tokens += a.PromptTokens
			}
			if tokens != 100*len(f.reqs) {
				t.Errorf("attempts add up to %d prompt tokens, want 100 per request = %d", tokens, 100*len(f.reqs))
			}
			if !slices.Equal(steps, tt.steps) || !slices.Equal(finals, tt.finals) {
				t.Errorf("steps per attempt %v, final requests %v; want %v, %v", steps, finals, tt.steps, tt.finals)
			}
			if tt.contains != "" && !strings.Contains(strings.Join(results, "\n"), tt.contains) {
				t.Errorf("tool results %q, want one containing %q", results, tt.contains)
			}
			// The final request of an attempt carries all of its steps, and
			// a retry carries the failed attempt's steps in its history.
			last := f.reqs[len(f.reqs)-1]
			if n := len(res.Attempts[len(res.Attempts)-1].Steps); len(last.Steps) != n {
				t.Errorf("last request has %d steps, want %d", len(last.Steps), n)
			}
			for i, h := range last.History {
				if len(h.Steps) != len(res.Attempts[i].Steps) {
					t.Errorf("history turn %d has %d steps, want %d", i, len(h.Steps), len(res.Attempts[i].Steps))
				}
			}
		})
	}
}

func TestServiceDown(t *testing.T) {
	_, k := setup(t)
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	tr := &Translator{Client: NewClient(url), Checker: k}
	_, err := tr.Translate(context.Background(), "Hi.", Context{}, Options{})
	if err == nil || !strings.Contains(err.Error(), "uv run uvicorn") {
		t.Fatalf("err = %v, want the start hint", err)
	}
}

func TestServiceTimeout(t *testing.T) {
	_, k := setup(t)
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-block }))
	defer srv.Close()
	defer close(block)
	c := NewClient(srv.URL)
	c.HTTP.Timeout = 50 * time.Millisecond
	_, err := (&Translator{Client: c, Checker: k}).Translate(context.Background(), "Hi.", Context{}, Options{})
	if err == nil || !strings.Contains(err.Error(), "timed out") || strings.Contains(err.Error(), "uv run uvicorn") {
		t.Fatalf("err = %v, want a timeout, not the start hint", err)
	}
}
