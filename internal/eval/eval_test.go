package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/olehmushka/lingua-nova/internal/lexicon"
	"github.com/olehmushka/lingua-nova/internal/llm"
	"github.com/olehmushka/lingua-nova/internal/translate"
)

const langDir = "../../language"

// TestEvalSet loads the real eval set; every content word of every
// sentence must be in the lexicon, or the meaning check can't see it.
func TestEvalSet(t *testing.T) {
	cases, err := Load(filepath.Join(langDir, "evals.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) < 20 {
		t.Errorf("%d sentences", len(cases))
	}
	_, _, k := setup(t)
	for _, c := range cases {
		if len(c.Tags) == 0 {
			t.Errorf("line %d: %q has no tags", c.Line, c.English)
		}
		if x := k.Expect(c.English); len(x.Unknown) > 0 {
			t.Errorf("line %d: %q: not in the lexicon: %v", c.Line, c.English, x.Unknown)
		}
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct{ name, yaml, err string }{
		{"not a list", "en: x\n", "want a list"},
		{"no en", "- tags: [a]\n", ":1: no en"},
		{"duplicate", "- en: The dog sleeps.\n- en: The dog sleeps.\n", `:2: "The dog sleeps." is already on line 1`},
		{"bad field type", "- en: x\n  tags: 3\n", ":1:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "evals.yaml")
			os.WriteFile(p, []byte(tt.yaml), 0o644)
			_, err := Load(p)
			if err == nil || !strings.Contains(err.Error(), tt.err) {
				t.Errorf("err = %v, want %q", err, tt.err)
			}
		})
	}
}

func TestParseSetups(t *testing.T) {
	tests := []struct{ in, want, err string }{
		{"retrieved", "retrieved", ""},
		{"retrieved, retrieved+tools,none+tools", "retrieved retrieved+tools none+tools", ""},
		{"full+tools", "full+tools", ""},
		{"some", "", `setup "some"`},
		{"none,none", "", "twice"},
	}
	for _, tt := range tests {
		su, err := ParseSetups(tt.in)
		if tt.err != "" {
			if err == nil || !strings.Contains(err.Error(), tt.err) {
				t.Errorf("%q: err = %v, want %q", tt.in, err, tt.err)
			}
			continue
		}
		var got []string
		for _, s := range su {
			got = append(got, s.String())
		}
		if err != nil || strings.Join(got, " ") != tt.want {
			t.Errorf("%q: %v %v, want %s", tt.in, got, err, tt.want)
		}
	}
}

func TestWilson(t *testing.T) {
	tests := []struct {
		k, n   int
		lo, hi float64 // percent, rounded
	}{
		{5, 5, 57, 100},
		{0, 5, 0, 43},
		{3, 5, 23, 88},
		{50, 100, 40, 60},
		{0, 0, 0, 100},
	}
	for _, tt := range tests {
		r := Wilson(tt.k, tt.n)
		if lo, hi := round(100*r.Lo), round(100*r.Hi); lo != tt.lo || hi != tt.hi {
			t.Errorf("%d/%d: [%v, %v], want [%v, %v]", tt.k, tt.n, lo, hi, tt.lo, tt.hi)
		}
	}
	if a, b := Wilson(5, 5), Wilson(0, 5); a.Overlaps(b) {
		t.Errorf("5/5 %v and 0/5 %v overlap", a, b)
	}
	if a, b := Wilson(3, 5), Wilson(4, 5); !a.Overlaps(b) {
		t.Errorf("3/5 %v and 4/5 %v don't overlap", a, b)
	}
}

func round(f float64) float64 { return float64(int(f + 0.5)) }

var (
	once     sync.Once
	sBuilder *llm.Builder
	sTools   *llm.Toolbox
	sChecker *llm.Checker
	setupErr error
)

func setup(t *testing.T) (*llm.Builder, *llm.Toolbox, *llm.Checker) {
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
		sBuilder, setupErr = llm.NewBuilder(langDir, lex, tr.Tagger)
		sChecker = &llm.Checker{Glosser: tr.Glosser, Ref: tr, Lex: lex}
		sTools = llm.NewToolbox(lex, tr.Rules, tr.Tagger, sChecker)
	})
	if setupErr != nil {
		t.Fatal(setupErr)
	}
	return sBuilder, sTools, sChecker
}

// fakeService answers each sentence the same way every time: the right
// translation, a wrong one, wrong then right after feedback, or never.
type fakeService struct {
	mu    sync.Mutex
	seeds []*int
	temps []float64
}

var answers = map[string]string{
	"The woman sees the river.": "Lu mena nara lu trubo.",
	"The dog eats the bread.":   "Lu huro baugir lu gurna.",
}

func (f *fakeService) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var req llm.Request
	json.NewDecoder(r.Body).Decode(&req)
	f.mu.Lock()
	f.seeds, f.temps = append(f.seeds, req.Seed), append(f.temps, req.Temperature)
	f.mu.Unlock()
	text, ok := answers[req.English]
	switch {
	case req.English == "Who saw the men?":
		text = "Ifa nara lu dubroh?" // wrong tense first
		if len(req.History) > 0 {
			text = "Ifa naraxe lu dubroh?"
		}
	case !ok:
		<-r.Context().Done()
		return
	}
	json.NewEncoder(rw).Encode(llm.Response{Answer: &llm.Answer{Translation: text}, PromptTokens: 100, EvalTokens: 10, DurationMS: 20})
}

func TestRun(t *testing.T) {
	b, tools, k := setup(t)
	f := &fakeService{}
	srv := httptest.NewServer(f)
	defer srv.Close()
	cases := []Case{
		{English: "The woman sees the river."}, // passes
		{English: "The dog eats the bread."},   // fails 3 times
		{English: "Who saw the men?"},          // passes on the 2nd attempt
		{English: "The men sleep."},            // the service never answers
	}
	var progress bytes.Buffer
	r := &Runner{
		Translator: &llm.Translator{Client: llm.NewClient(srv.URL), Checker: k},
		Builder:    b, Tools: tools,
		Options:  llm.Options{Temperature: 0.7},
		Timeout:  200 * time.Millisecond,
		Progress: &progress,
	}
	recs, err := r.Run(context.Background(), cases, []Setup{{Ctx: llm.Retrieved}}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 8 {
		t.Fatalf("%d records, want 8", len(recs))
	}
	type want struct {
		ok, first bool
		attempts  int
		err       string
	}
	wants := []want{{true, true, 1, ""}, {false, false, 3, ""}, {true, false, 2, ""}, {false, false, 0, "timeout after 200ms"}}
	for i, rec := range recs {
		w := wants[i%4]
		if rec.OK != w.ok || rec.FirstOK != w.first || rec.Attempts != w.attempts || rec.Error != w.err {
			t.Errorf("%s run %d: ok %v first %v attempts %d err %q; want %+v", rec.English, rec.Run, rec.OK, rec.FirstOK, rec.Attempts, rec.Error, w)
		}
		if rec.Run != i/4+1 || rec.Seed == nil || *rec.Seed != rec.Run {
			t.Errorf("record %d: run %d seed %v", i, rec.Run, rec.Seed)
		}
	}
	for i, s := range f.seeds {
		if s == nil || f.temps[i] != 0.7 {
			t.Errorf("request %d: seed %v temperature %v", i, s, f.temps[i])
		}
	}
	if !strings.Contains(progress.String(), "[8/8] run 2 retrieved  The men sleep. →   ! timeout after 200ms") {
		t.Errorf("progress:\n%s", progress.String())
	}

	sums := Summarize(recs)
	if len(sums) != 1 {
		t.Fatalf("%d summaries", len(sums))
	}
	s := sums[0]
	if s.Pass.K != 4 || s.Pass.N != 8 || s.First.K != 2 || s.Errors != 2 {
		t.Errorf("summary %+v", s)
	}
	var out bytes.Buffer
	Report(&out, recs)
	for _, w := range []string{"The woman sees the river.  2/2", "The dog eats the bread.    0/2", "pass rate [95% CI]", "50% [22–78]", "first attempt", "25% [7–59]"} {
		if !strings.Contains(out.String(), w) {
			t.Errorf("report has no %q:\n%s", w, out.String())
		}
	}
}

// TestRunCancelled: a cancelled run returns what's done, without the
// translation in progress.
func TestRunCancelled(t *testing.T) {
	b, tools, k := setup(t)
	srv := httptest.NewServer(&fakeService{})
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	r := &Runner{
		Translator: &llm.Translator{Client: llm.NewClient(srv.URL), Checker: k},
		Builder:    b, Tools: tools,
	}
	time.AfterFunc(200*time.Millisecond, cancel)
	recs, err := r.Run(ctx, []Case{{English: "The woman sees the river."}, {English: "The men sleep."}}, []Setup{{Ctx: llm.None}}, 3)
	if err == nil || len(recs) != 1 || !recs[0].OK || recs[0].Seed != nil {
		t.Errorf("err %v, %+v", err, recs)
	}
}

func TestSaveDiff(t *testing.T) {
	rec := func(en, setup string, ok bool) Record { return Record{English: en, Setup: setup, OK: ok} }
	var oldRecs, newRecs []Record
	for i := range 10 {
		oldRecs = append(oldRecs, rec("A.", "retrieved", i < 2), rec("B.", "retrieved", true), rec("A.", "none+tools", false))
		newRecs = append(newRecs, rec("A.", "retrieved", i < 9), rec("B.", "retrieved", true), rec("A.", "none+tools", true))
	}
	old := &Results{Meta: Meta{Model: "m1", EvalSet: Hash([]Case{{English: "A."}})}, Records: oldRecs}
	p := filepath.Join(t.TempDir(), "old.json")
	if err := Save(p, old); err != nil {
		t.Fatal(err)
	}
	old, err := LoadResults(p)
	if err != nil || len(old.Records) != 30 {
		t.Fatalf("%v, %d records", err, len(old.Records))
	}
	nw := &Results{Meta: Meta{Model: "m2", EvalSet: old.Meta.EvalSet}, Records: newRecs}
	var out bytes.Buffer
	Diff(&out, old, nw)
	for _, w := range []string{
		"differs: model m1 → m2",
		// One sentence went 2/10 → 9/10, but over the setup's 20 runs
		// that's not enough to rule out noise.
		"retrieved   60% [39–78]  95% [76–99]    intervals overlap: could be noise",
		"none+tools  0% [0–28]    100% [72–100]  better: intervals don't overlap",
		"↑ retrieved   A.  2/10 → 9/10",
		"↑ none+tools  A.  0/10 → 10/10",
	} {
		if !strings.Contains(out.String(), w) {
			t.Errorf("diff has no %q:\n%s", w, out.String())
		}
	}
	if strings.Contains(out.String(), "eval set") || strings.Contains(out.String(), "B.  ") {
		t.Errorf("diff shows what didn't change:\n%s", out.String())
	}
}

// TestRunResume: records in Done aren't translated again, and Saved
// sees every record so far after each new one.
func TestRunResume(t *testing.T) {
	b, tools, k := setup(t)
	f := &fakeService{}
	srv := httptest.NewServer(f)
	defer srv.Close()
	done := Record{English: "The woman sees the river.", Setup: "retrieved", Run: 1, OK: true}
	var saves []int
	r := &Runner{
		Translator: &llm.Translator{Client: llm.NewClient(srv.URL), Checker: k},
		Builder:    b, Tools: tools,
		Done:  []Record{done},
		Saved: func(recs []Record) { saves = append(saves, len(recs)) },
	}
	cases := []Case{{English: "The woman sees the river."}, {English: "The dog eats the bread."}}
	recs, err := r.Run(context.Background(), cases, []Setup{{Ctx: llm.Retrieved}}, 2)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, rec := range recs {
		got = append(got, fmt.Sprintf("%d %s", rec.Run, rec.English))
	}
	want := []string{"1 The woman sees the river.", "1 The dog eats the bread.", "2 The woman sees the river.", "2 The dog eats the bread."}
	if !slices.Equal(got, want) {
		t.Errorf("records %q, want %q", got, want)
	}
	if !slices.Equal(saves, []int{2, 3, 4}) {
		t.Errorf("saves %v", saves)
	}
	// 3 translations: 1 + 3 attempts (the dog fails), twice, minus the done one.
	if len(f.seeds) != 1+3+1+3-1 {
		t.Errorf("%d requests", len(f.seeds))
	}
}

func TestMismatch(t *testing.T) {
	m := Meta{Model: "m", Temperature: 0.7, MaxAttempts: 3, MaxToolRounds: 8, EvalSet: "abc", Runs: 5, Commit: "x"}
	more := m
	more.Runs, more.Commit, more.Setups = 10, "y", []string{"full"}
	if why := Mismatch(m, more); why != "" {
		t.Errorf("more runs, other commit and setups: %q", why)
	}
	other := m
	other.Temperature, other.EvalSet = 0, "def"
	if why := Mismatch(m, other); why != "temperature 0.7 ≠ 0, eval set abc ≠ def" {
		t.Errorf("got %q", why)
	}
}
