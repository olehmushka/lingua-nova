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

	"github.com/olehmushka/lingua-nova/internal/lexicon"
	"github.com/olehmushka/lingua-nova/internal/translate"
)

const langDir = "../../language"

var (
	once     sync.Once
	sBuilder *Builder
	sChecker *Checker
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
		sChecker = &Checker{Glosser: tr.Glosser, Known: func(id string) bool { return ids[id] }}
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
	if n, m := numCtx(r), numCtx(full); n%1024 != 0 || m%1024 != 0 || n >= m {
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
			c := k.Verify(tt.answer)
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

// Every interlinear example in grammar.md is a known-good answer.
func TestVerifyGrammarExamples(t *testing.T) {
	b, k := setup(t)
	for _, ex := range b.Examples {
		if c := k.Verify(Answer{Translation: ex.Rifatian}); !c.OK() {
			t.Errorf("%q: %v", ex.Rifatian, c.Problems)
		}
	}
}

func TestFeedback(t *testing.T) {
	_, k := setup(t)
	fb := k.Verify(Answer{Translation: "Lu mena nara lu blomp."}).Feedback()
	for _, want := range []string{"see-TH.3SG", "Problems:", "- blomp: not a form of any dictionary word"} {
		if !strings.Contains(fb, want) {
			t.Errorf("feedback missing %q:\n%s", want, fb)
		}
	}
}

// fakeService answers with scripted translations, one per attempt, and
// records the requests.
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
	json.NewEncoder(rw).Encode(Response{Answer: Answer{Translation: text}, PromptTokens: 100, EvalTokens: 10, DurationMS: 5})
}

func TestTranslateLoop(t *testing.T) {
	b, k := setup(t)
	const bad, good = "Lu mena nara lu blomp.", "Lu mena nara lu trubo."
	tests := []struct {
		name     string
		answers  []string
		attempts int
		ok       bool
		err      string
	}{
		{"first try", []string{good}, 1, true, ""},
		{"fixed after feedback", []string{bad, good}, 2, true, ""},
		{"gives up", []string{bad}, 3, false, ""},
		{"malformed reply", []string{bad, "!malformed"}, 1, false, "not a valid answer"},
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
				if i > 0 && !strings.Contains(req.History[i-1].Feedback, "blomp: not a form of any dictionary word") {
					t.Errorf("request %d feedback:\n%s", i, req.History[i-1].Feedback)
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
