package semantic

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/olehmushka/lingua-nova/internal/lexicon"
)

// records counts a cache file's records.
func records(t *testing.T, path string) int {
	t.Helper()
	_, n, err := readCache(path, "m", func(key, record) {})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// failAfter embeds n calls, then fails.
type failAfter struct {
	fake
	n int
}

func (f *failAfter) Embed(ctx context.Context, kind string, texts []string) ([][]float32, error) {
	if f.n == 0 {
		return nil, errors.New("down")
	}
	f.n--
	return f.fake.Embed(ctx, kind, texts)
}

// An interrupted build keeps its finished batches: the next build embeds
// only the rest.
func TestCacheResume(t *testing.T) {
	path := filepath.Join(t.TempDir(), "emb.bin")
	ctx := context.Background()
	es := entries()
	_, _, err := BuildWith(ctx, &failAfter{n: 2}, "m", es, path, Options{Batch: 2})
	if err == nil {
		t.Fatal("no error from the failing embedder")
	}
	if n := records(t, path); n != 4 {
		t.Fatalf("%d records after 2 batches of 2, want 4", n)
	}
	f := &fake{}
	_, st, err := BuildWith(ctx, f, "m", es, path, Options{Batch: 2})
	if err != nil || st.Cached != 4 || st.Embedded != 2 || len(f.calls) != 1 {
		t.Fatalf("resume: %+v %v, calls %v", st, err, f.calls)
	}
	if n := records(t, path); n != 6 {
		t.Errorf("%d records, want 6", n)
	}
}

// Records of edited or deleted entries are dropped.
func TestCacheCompact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "emb.bin")
	ctx := context.Background()
	es := entries()
	if _, _, err := Build(ctx, &fake{}, "m", es, path); err != nil {
		t.Fatal(err)
	}
	es[1].Definition = "A river or stream."
	if _, _, err := Build(ctx, &fake{}, "m", es[:4], path); err != nil {
		t.Fatal(err)
	}
	if n := records(t, path); n != 4 {
		t.Errorf("%d records, want the 4 live ones", n)
	}
}

// A record cut off by a killed build is ignored; the rest is used.
func TestCacheCutOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "emb.bin")
	ctx := context.Background()
	es := entries()
	if _, _, err := Build(ctx, &fake{}, "m", es, path); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte("half a record"))
	f.Close()
	if _, st, err := Build(ctx, &fake{}, "m", es, path); err != nil || st.Cached != 6 {
		t.Errorf("after a cut-off record: %+v %v", st, err)
	}
}

// The old JSON cache is imported, not re-embedded, and written as binary.
func TestCacheLegacy(t *testing.T) {
	dir := t.TempDir()
	es := entries()
	vecs, _ := (&fake{}).Embed(context.Background(), Document, []string{Text(es[0]), Text(es[2])})
	k0, k2 := hash(Text(es[0])), hash(Text(es[2]))
	data, _ := json.Marshal(legacyJSON{Model: "m", Vectors: map[string][]float32{
		hex.EncodeToString(k0[:]): vecs[0], hex.EncodeToString(k2[:]): vecs[1]}})
	if err := os.WriteFile(filepath.Join(dir, "emb.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "emb.bin")
	f := &fake{}
	if _, st, err := Build(context.Background(), f, "m", es, path); err != nil || st.Cached != 2 || st.Embedded != 4 {
		t.Fatalf("legacy import: %+v %v", st, err)
	}
	if n := records(t, path); n != 6 {
		t.Errorf("%d records, want 6", n)
	}
}

func TestQuantize(t *testing.T) {
	v := normalize([]float32{0.5, -1, 0.25, 0})
	codes, scale := quantize(v)
	if !slices.Equal(codes, []int8{64, -127, 32, 0}) {
		t.Errorf("codes = %v", codes)
	}
	if got := dotInt8(v, codes, scale); got < 0.99 || got > 1.01 { // int8 rounding: about 1/254 per number
		t.Errorf("self score %v, want 1", got)
	}
	if c, s := quantize(make([]float32, 3)); s != 0 || !slices.Equal(c, []int8{0, 0, 0}) {
		t.Errorf("zero vector: %v %v", c, s)
	}
}

// random embeds every text as a random 64-dimensional vector, the same
// for the same text.
type random struct{}

func (random) Embed(_ context.Context, _ string, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		h := hash(t)
		rng := rand.New(rand.NewPCG(uint64(h[0])<<8|uint64(h[1]), uint64(h[2])))
		v := make([]float32, 64)
		for d := range v {
			v[d] = float32(rng.NormFloat64())
		}
		out[i] = v
	}
	return out, nil
}

// On an index too big to rank exactly, the sign-bit first pass still
// finds each entry as its own nearest neighbour, and its top 10 mostly
// agree with the exact ranking.
func TestFirstPass(t *testing.T) {
	var es []*lexicon.Entry
	for i := range 3000 {
		es = append(es, &lexicon.Entry{ID: string(rune('a'+i%26)) + string(rune('a'+i/26%26)) + string(rune('a'+i/676)), Gloss: "x", POS: "noun"})
	}
	idx, _, err := Build(context.Background(), random{}, "m", es, "")
	if err != nil {
		t.Fatal(err)
	}
	exact := *idx
	exact.Exact = true
	agree := 0
	for _, e := range es[:50] {
		v, _ := random{}.Embed(context.Background(), Query, []string{Text(e)})
		q := normalize(v[0])
		got, want := idx.search(q, 10, nil), exact.search(q, 10, nil)
		if got[0].Entry != want[0].Entry {
			t.Errorf("%s: first pass top %s, exact %s", e.ID, got[0].Entry.ID, want[0].Entry.ID)
		}
		for _, h := range got {
			if slices.ContainsFunc(want, func(w Hit) bool { return w.Entry == h.Entry }) {
				agree++
			}
		}
	}
	if agree < 50*10*9/10 {
		t.Errorf("first pass agrees on %d of 500 neighbours, want ≥ 90%%", agree)
	}
}

func TestDims(t *testing.T) {
	idx, _, err := BuildWith(context.Background(), random{}, "m", entries(), "", Options{Dims: 16})
	if err != nil {
		t.Fatal(err)
	}
	if idx.Dims() != 16 || len(idx.codes) != 16*len(entries()) {
		t.Errorf("dims %d, %d codes", idx.Dims(), len(idx.codes))
	}
	full, _, _ := Build(context.Background(), random{}, "m", entries(), "")
	if full.Dims() != 64 {
		t.Errorf("full dims %d", full.Dims())
	}
}
