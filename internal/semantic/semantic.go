// Package semantic searches the lexicon by meaning, with embeddings.
//
// An embedding model turns a text into a vector (768 numbers for
// nomic-embed-text) such that texts with similar meanings get vectors that
// point in similar directions. "sea" and "river (noun): River." end up
// close; "sea" and "goat (noun): A goat." don't, though no letters match.
// Closeness is the cosine of the angle between two vectors: the dot
// product divided by both lengths. The vectors are scaled to length 1
// once, at build time, so the cosine is just the dot product.
//
// The vectors are stored int8-quantized, with sign bits for a fast first
// pass (quant.go): at 300k entries a search reads the 29 MB of sign bits,
// keeps the few hundred closest by Hamming distance, and ranks those with
// the int8 codes. Still a scan, not an approximate index like HNSW: a
// popcount over 300k entries takes a few milliseconds, and an index pays
// off at millions.
//
// Embedding every entry takes seconds (hours for 300k on a laptop), so
// vectors are cached in a file (cache.go), keyed by a hash of the exact
// text embedded: editing one entry re-embeds only that entry, and an
// interrupted build resumes.
package semantic

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/olehmushka/lingua-nova/internal/lexicon"
)

// Kinds of text, for models trained with different task prefixes for the
// two sides of a search (nomic-embed-text: "search_query: ",
// "search_document: "). The service adds the prefix.
const (
	Query    = "query"
	Document = "document"
)

// Embedder turns texts into vectors, one per text, in order.
type Embedder interface {
	Embed(ctx context.Context, kind string, texts []string) ([][]float32, error)
}

// Text is what gets embedded for an entry: "lantern (noun): A lamp in a
// case." Gloss dots are spaces; the definition is left out when empty.
func Text(e *lexicon.Entry) string {
	return text(e.Gloss, e.POS, e.Definition)
}

// QueryText is Text's format for a meaning that has no entry yet, so a
// concept and the entries are embedded alike.
func QueryText(english, pos, definition string) string {
	return text(english, pos, definition)
}

func text(gloss, pos, definition string) string {
	s := strings.Join(strings.FieldsFunc(gloss, func(r rune) bool { return r == '.' || r == ' ' }), " ")
	if pos != "" {
		s += " (" + pos + ")"
	}
	if d := strings.TrimSpace(definition); d != "" {
		s += ": " + d
	}
	return s
}

// Index holds one quantized unit vector per entry.
type Index struct {
	emb     Embedder
	model   string
	dims    int // as searched: the model's, or fewer (Options.Dims)
	entries []*lexicon.Entry
	codes   []int8    // entry i: codes[i*dims : (i+1)*dims]
	scales  []float32 // entry i's vector ≈ its codes × scales[i]
	signs   []uint64  // entry i: signs[i*words(dims) : (i+1)*words(dims)]

	// Exact ranks every entry with the int8 codes, without the sign-bit
	// first pass: slower, for measuring what the first pass costs.
	Exact bool
}

// Hit is one search result. Score is the cosine similarity, from -1 to 1;
// what counts as high depends on the model (nomic's unrelated pairs often
// score 0.3–0.4).
type Hit struct {
	Entry *lexicon.Entry
	Score float64
}

// Stats says how a Build went.
type Stats struct {
	Cached, Embedded int
}

// Options tune a Build.
type Options struct {
	// Dims keeps the vectors' first Dims numbers (Matryoshka truncation):
	// less memory, worse neighbours. 0: all. The cache keeps all, so
	// changing it re-embeds nothing.
	Dims int
	// Batch is the number of texts per embedding call, and per cache
	// checkpoint; 0: DefaultBatch.
	Batch int
	// Progress, if set, is called after each batch with the texts
	// embedded so far and the number to embed.
	Progress func(done, total int)
}

// DefaultBatch texts go to the embedding model per call: big enough to
// keep a GPU busy, small enough that an interrupted build loses little.
const DefaultBatch = 256

// Build is BuildWith with the default options.
func Build(ctx context.Context, emb Embedder, model string, entries []*lexicon.Entry, cachePath string) (*Index, Stats, error) {
	return BuildWith(ctx, emb, model, entries, cachePath, Options{})
}

// BuildWith embeds every entry's Text, reusing the vectors in cachePath
// that were made by the same model from the same text (or, the first
// time, in the old JSON cache beside it). New vectors are appended to the
// cache batch by batch, so an error or a cancel keeps the batches done.
// At the end a cache with records no entry needs is rewritten with only
// the needed ones. cachePath "" means no cache.
func BuildWith(ctx context.Context, emb Embedder, model string, entries []*lexicon.Entry, cachePath string, opt Options) (*Index, Stats, error) {
	var st Stats
	keys := make([]key, len(entries))
	want := map[key][]int{} // text hash → the entries with that text
	for i, e := range entries {
		keys[i] = hash(Text(e))
		want[keys[i]] = append(want[keys[i]], i)
	}
	full := make([]*record, len(entries)) // full-dims vectors
	known := func(k key, r record) {
		for _, i := range want[k] {
			full[i] = &r
		}
	}

	// 1. What the cache (or the old JSON cache) has.
	inFile, haveFile := 0, false // records in a usable cache file
	if cachePath != "" {
		n, err := readCacheCount(cachePath, model, known)
		if err == nil {
			inFile, haveFile = n, true
		} else if errors.Is(err, os.ErrNotExist) {
			readLegacy(cachePath, model, known)
		}
	}
	for _, r := range full {
		if r != nil {
			st.Cached++
		}
	}

	// 2. Embed the rest, a batch at a time, appending each to the cache.
	var missing []key
	seen := map[key]bool{}
	for _, k := range keys {
		if full[want[k][0]] == nil && !seen[k] {
			seen[k] = true
			missing = append(missing, k)
		}
	}
	var app *appender
	defer func() {
		if app != nil {
			app.close()
		}
	}()
	open := func(dims int) error { // the cache, with every known vector in it
		if app != nil || cachePath == "" {
			return nil
		}
		var err error
		if app, err = openAppend(cachePath, cacheHeader{model, dims}, !haveFile); err != nil || haveFile {
			return err
		}
		haveFile = true
		var ks []key
		var rs []record
		for k, is := range want {
			if r := full[is[0]]; r != nil {
				ks, rs = append(ks, k), append(rs, *r)
			}
		}
		inFile += len(ks)
		return app.add(ks, rs)
	}
	batch := cmp.Or(opt.Batch, DefaultBatch)
	for b := 0; b < len(missing); b += batch {
		part := missing[b:min(b+batch, len(missing))]
		texts := make([]string, len(part))
		for j, k := range part {
			texts[j] = Text(entries[want[k][0]])
		}
		vs, err := emb.Embed(ctx, Document, texts)
		if err != nil {
			return nil, st, err
		}
		if len(vs) != len(texts) {
			return nil, st, fmt.Errorf("semantic: %d vectors for %d texts", len(vs), len(texts))
		}
		// Open (or start) the cache before this batch is known, so a new
		// file gets the earlier vectors and then this batch, once.
		if err := open(len(vs[0])); err != nil {
			return nil, st, fmt.Errorf("semantic: writing cache: %w", err)
		}
		recs := make([]record, len(part))
		for j, k := range part {
			codes, scale := quantize(normalize(vs[j]))
			recs[j] = record{scale, codes}
			known(k, recs[j])
			st.Embedded += len(want[k])
		}
		if app == nil {
			continue
		}
		if err := app.add(part, recs); err != nil {
			return nil, st, fmt.Errorf("semantic: writing cache: %w", err)
		}
		inFile += len(part)
		if opt.Progress != nil {
			opt.Progress(b+len(part), len(missing))
		}
	}

	// 3. A cache that is new (imported from JSON) or holds dead records.
	idx := newIndex(emb, model, entries, full, opt.Dims)
	if cachePath != "" && len(full) > 0 {
		var err error
		if !haveFile { // imported from the old JSON: write it out
			err = open(len(full[0].codes))
		}
		if app != nil {
			app.close()
			app = nil
		}
		if err == nil && inFile > len(want) {
			err = compact(cachePath, model, want)
		}
		if err != nil {
			return idx, st, fmt.Errorf("semantic: writing cache: %w", err)
		}
	}
	return idx, st, nil
}

// newIndex lays the entries' vectors out at dims (0: full).
func newIndex(emb Embedder, model string, entries []*lexicon.Entry, full []*record, dims int) *Index {
	fullDims := 0
	for _, r := range full {
		if r != nil {
			fullDims = len(r.codes)
			break
		}
	}
	if dims <= 0 || dims > fullDims {
		dims = fullDims
	}
	n, w := len(entries), words(dims)
	idx := &Index{emb: emb, model: model, dims: dims, entries: entries,
		codes: make([]int8, n*dims), scales: make([]float32, n), signs: make([]uint64, n*w)}
	for i, r := range full {
		codes, scale := r.codes, r.scale
		if dims < len(codes) {
			codes, scale = quantize(truncate(dequantize(codes, scale), dims))
		}
		copy(idx.codes[i*dims:], codes)
		idx.scales[i] = scale
		signsOf(codes, idx.signs[i*w:(i+1)*w])
	}
	return idx
}

// Model is the embedding model the index was built with.
func (idx *Index) Model() string { return idx.model }

// Dims is the number of dimensions searched.
func (idx *Index) Dims() int { return idx.dims }

// Bytes is the memory the vectors take.
func (idx *Index) Bytes() int { return len(idx.codes) + 4*len(idx.scales) + 8*len(idx.signs) }

// Search returns the k entries closest in meaning to the query (k ≤ 0:
// all), best first, among those keep accepts (nil: all).
func (idx *Index) Search(ctx context.Context, query string, k int, keep func(*lexicon.Entry) bool) ([]Hit, error) {
	if strings.TrimSpace(query) == "" {
		return nil, errors.New("semantic: empty query")
	}
	hs, err := idx.searchMany(ctx, []string{query}, k, keep)
	if err != nil {
		return nil, err
	}
	return hs[0], nil
}

// SearchMany is Search for several queries in one embedding call, as the
// retriever asks for a sentence's words (retrieve.Dense).
func (idx *Index) SearchMany(ctx context.Context, queries []string, k int) ([][]*lexicon.Entry, error) {
	hs, err := idx.searchMany(ctx, queries, k, nil)
	if err != nil {
		return nil, err
	}
	out := make([][]*lexicon.Entry, len(hs))
	for i, h := range hs {
		for _, x := range h {
			out[i] = append(out[i], x.Entry)
		}
	}
	return out, nil
}

func (idx *Index) searchMany(ctx context.Context, queries []string, k int, keep func(*lexicon.Entry) bool) ([][]Hit, error) {
	vs, err := idx.emb.Embed(ctx, Query, queries)
	if err != nil {
		return nil, err
	}
	if len(vs) != len(queries) {
		return nil, fmt.Errorf("semantic: %d vectors for %d queries", len(vs), len(queries))
	}
	out := make([][]Hit, len(vs))
	for i, v := range vs {
		out[i] = idx.search(truncate(v, idx.dims), k, keep)
	}
	return out, nil
}

// rescoreMin is the fewest candidates the sign-bit pass keeps, and
// rescoreFactor how many per result asked for: on the real vectors 200
// candidates kept 99% of the exact top 10.
const (
	rescoreMin    = 400
	rescoreFactor = 20
)

// search ranks the entries for a unit query vector.
func (idx *Index) search(q []float32, k int, keep func(*lexicon.Entry) bool) []Hit {
	d := idx.dims
	if len(q) > d {
		q = q[:d]
	}
	cands := idx.candidates(q, k, keep)
	hits := make([]Hit, 0, len(cands))
	for _, i := range cands {
		hits = append(hits, Hit{idx.entries[i], dotInt8(q, idx.codes[i*d:(i+1)*d], idx.scales[i])})
	}
	slices.SortStableFunc(hits, func(a, b Hit) int { return cmp.Compare(b.Score, a.Score) })
	if k > 0 && len(hits) > k {
		hits = hits[:k]
	}
	return hits
}

// candidates are the entries to rank with int8: all of those keep accepts
// (Exact, or k ≤ 0, or a small index), or the closest by sign bits. The
// cut is the smallest Hamming distance with enough entries at or under it,
// found with a histogram of the distances (one pass, no sort).
func (idx *Index) candidates(q []float32, k int, keep func(*lexicon.Entry) bool) []int {
	var all []int
	for i, e := range idx.entries {
		if keep == nil || keep(e) {
			all = append(all, i)
		}
	}
	want := max(rescoreMin, rescoreFactor*k)
	if idx.Exact || k <= 0 || len(all) <= want {
		return all
	}
	w := words(idx.dims)
	qs := make([]uint64, w)
	signsOf(q, qs)
	dist := make([]uint16, len(all))
	hist := make([]int, idx.dims+1)
	for j, i := range all {
		dist[j] = uint16(hamming(qs, idx.signs[i*w:(i+1)*w]))
		hist[dist[j]]++
	}
	cut, n := 0, 0
	for cut = range hist {
		if n += hist[cut]; n >= want {
			break
		}
	}
	out := make([]int, 0, n)
	for j, i := range all {
		if int(dist[j]) <= cut {
			out = append(out, i)
		}
	}
	return out
}

// normalize scales v to length 1. A zero vector stays zero: it is close to
// nothing, rather than NaN.
func normalize(v []float32) []float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	out := make([]float32, len(v))
	if sum == 0 {
		return out
	}
	n := math.Sqrt(sum)
	for i, x := range v {
		out[i] = float32(float64(x) / n)
	}
	return out
}

// CachePath is the default cache file for a model: .cache/embeddings-<model>.bin
// next to the language directory (the repository root), outside
// language/, which the tools never write.
func CachePath(langDir, model string) string {
	name := strings.NewReplacer(":", "-", "/", "-").Replace(model)
	return filepath.Join(filepath.Dir(filepath.Clean(langDir)), ".cache", "embeddings-"+name+".bin")
}
