package semantic

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/olehmushka/lingua-nova/internal/lexicon"
)

// fake embeds a text as counts of a few topic words, so related texts
// share dimensions: [water, fire, animal].
type fake struct {
	calls [][]string // texts per call
	kinds []string
	err   error
}

var topics = [][]string{{"sea", "water", "river", "wave"}, {"fire", "flame", "light", "lamp", "lantern"}, {"goat", "wolf", "animal"}}

func (f *fake) Embed(_ context.Context, kind string, texts []string) ([][]float32, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.calls = append(f.calls, texts)
	f.kinds = append(f.kinds, kind)
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v := make([]float32, len(topics))
		for _, w := range strings.FieldsFunc(strings.ToLower(t), func(r rune) bool { return r < 'a' || r > 'z' }) {
			for d, ws := range topics {
				if slices.Contains(ws, w) {
					v[d]++
				}
			}
		}
		out[i] = v
	}
	return out, nil
}

func entries() []*lexicon.Entry {
	return []*lexicon.Entry{
		{ID: "water", POS: "noun", Gloss: "water", Definition: "Water; any liquid."},
		{ID: "river", POS: "noun", Gloss: "river", Definition: "A river."},
		{ID: "fire", POS: "noun", Gloss: "fire", Definition: "Fire, flame."},
		{ID: "goat", POS: "noun", Gloss: "goat", Definition: "A goat."},
		{ID: "burn", POS: "verb", Gloss: "burn", Definition: "To burn with fire."},
		{ID: "nothing", POS: "noun", Gloss: "thing"},
	}
}

func TestText(t *testing.T) {
	for _, tc := range []struct {
		e    lexicon.Entry
		want string
	}{
		{lexicon.Entry{Gloss: "lantern", POS: "noun", Definition: "A lamp in a case."}, "lantern (noun): A lamp in a case."},
		{lexicon.Entry{Gloss: "kill.by.accident", POS: "verb"}, "kill by accident (verb)"},
		{lexicon.Entry{Gloss: "lie.down", POS: "verb", Definition: "  "}, "lie down (verb)"},
	} {
		if got := Text(&tc.e); got != tc.want {
			t.Errorf("Text(%q) = %q, want %q", tc.e.Gloss, got, tc.want)
		}
	}
	if got := QueryText("lie down", "verb", ""); got != "lie down (verb)" {
		t.Errorf("QueryText = %q", got)
	}
}

func ids(hits []Hit) []string {
	var out []string
	for _, h := range hits {
		out = append(out, h.Entry.ID)
	}
	return out
}

func TestSearch(t *testing.T) {
	f := &fake{}
	idx, st, err := Build(context.Background(), f, "m", entries(), "")
	if err != nil {
		t.Fatal(err)
	}
	if st.Embedded != 6 || st.Cached != 0 || f.kinds[0] != Document {
		t.Fatalf("stats %+v, kinds %v", st, f.kinds)
	}
	for _, tc := range []struct {
		query string
		k     int
		keep  func(*lexicon.Entry) bool
		want  []string
	}{
		{"the sea", 2, nil, []string{"water", "river"}},
		{"lantern light", 2, nil, []string{"fire", "burn"}},
		{"lantern light", 1, func(e *lexicon.Entry) bool { return e.POS == "verb" }, []string{"burn"}},
		{"wolf", 1, nil, []string{"goat"}},
	} {
		hits, err := idx.Search(context.Background(), tc.query, tc.k, tc.keep)
		if err != nil {
			t.Fatal(err)
		}
		if got := ids(hits); !slices.Equal(got, tc.want) {
			t.Errorf("Search(%q) = %v, want %v", tc.query, got, tc.want)
		}
	}
	if f.kinds[len(f.kinds)-1] != Query {
		t.Errorf("query embedded as %q", f.kinds[len(f.kinds)-1])
	}
	// A zero vector ("thing" has no topic word) scores 0, not NaN.
	hits, _ := idx.Search(context.Background(), "sea", 0, nil)
	for _, h := range hits {
		if math.IsNaN(h.Score) {
			t.Errorf("%s: NaN score", h.Entry.ID)
		}
	}
	if _, err := idx.Search(context.Background(), " ", 3, nil); err == nil {
		t.Error("empty query: no error")
	}
}

func TestCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "emb.bin")
	ctx := context.Background()
	es := entries()

	f := &fake{}
	if _, st, err := Build(ctx, f, "m", es, path); err != nil || st.Embedded != 6 {
		t.Fatalf("first build: %+v %v", st, err)
	}

	f = &fake{}
	if _, st, err := Build(ctx, f, "m", es, path); err != nil || st.Embedded != 0 || st.Cached != 6 || len(f.calls) != 0 {
		t.Fatalf("second build: %+v %v, calls %v", st, err, f.calls)
	}

	es[1].Definition = "A river or stream."
	f = &fake{}
	idx, st, err := Build(ctx, f, "m", es, path)
	if err != nil || st.Embedded != 1 || st.Cached != 5 {
		t.Fatalf("after edit: %+v %v", st, err)
	}
	if !slices.Equal(f.calls[0], []string{"river (noun): A river or stream."}) {
		t.Errorf("re-embedded %v", f.calls[0])
	}
	if hits, _ := idx.Search(ctx, "sea", 2, nil); !slices.Equal(ids(hits), []string{"water", "river"}) {
		t.Errorf("cached search = %v", ids(hits))
	}

	// Another model doesn't use these vectors.
	f = &fake{}
	if _, st, _ := Build(ctx, f, "other", es, path); st.Embedded != 6 {
		t.Errorf("other model: %+v", st)
	}
}

func TestBuildError(t *testing.T) {
	_, _, err := Build(context.Background(), &fake{err: errors.New("down")}, "m", entries(), "")
	if err == nil || err.Error() != "down" {
		t.Errorf("err = %v", err)
	}
}

func TestCachePath(t *testing.T) {
	if got, want := CachePath("language", "nomic-embed-text:latest"), filepath.Join(".cache", "embeddings-nomic-embed-text-latest.bin"); got != want {
		t.Errorf("CachePath = %q, want %q", got, want)
	}
	if got, want := CachePath("/x/repo/language/", "m"), "/x/repo/.cache/embeddings-m.bin"; got != want {
		t.Errorf("CachePath = %q, want %q", got, want)
	}
}

func TestRecall(t *testing.T) {
	es := entries()
	hits := []Hit{{Entry: es[0]}, {Entry: es[2]}}
	r, missing := Recall(hits, []string{"water", "river", "fire", "burn"})
	if r != 0.5 || !slices.Equal(missing, []string{"river", "burn"}) {
		t.Errorf("Recall = %v %v", r, missing)
	}
}

func TestLoadEval(t *testing.T) {
	lex := &lexicon.Lexicon{Entries: entries()}
	dir := t.TempDir()
	write := func(s string) string {
		p := filepath.Join(dir, "e.yaml")
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	qs, err := LoadEval(write("- query: sea\n  expect: [water, river]\n"), lex)
	if err != nil || len(qs) != 1 || qs[0].Expect[1] != "river" {
		t.Errorf("LoadEval = %v, %v", qs, err)
	}
	if _, err := LoadEval(write("- query: sea\n  expect: [ocean]\n"), lex); err == nil || !strings.Contains(err.Error(), "no entry with id ocean") {
		t.Errorf("unknown id: err = %v", err)
	}
	if _, err := LoadEval(write("- query: sea\n"), lex); err == nil {
		t.Error("no expect: no error")
	}
}

// The repository's eval file names only real entries.
func TestRepoEval(t *testing.T) {
	lex, err := lexicon.Load("../../language/lexicon")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadEval("../../evals/similar.yaml", lex); err != nil {
		t.Error(err)
	}
}
