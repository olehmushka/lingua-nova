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
// Search is a brute-force scan: the query's vector against every entry's,
// 629 × 768 multiplications, well under a millisecond. Vector databases
// with approximate indexes (HNSW and friends) pay off at millions of
// documents, not hundreds.
//
// Embedding every entry takes seconds, so vectors are cached in a file,
// keyed by a hash of the exact text embedded: editing one entry re-embeds
// only that entry.
package semantic

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

// Index holds one unit vector per entry.
type Index struct {
	emb     Embedder
	model   string
	entries []*lexicon.Entry
	vecs    [][]float32
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

// cacheFile is the cache's JSON: text hash → unit vector, for one model.
type cacheFile struct {
	Model   string               `json:"model"`
	Vectors map[string][]float32 `json:"vectors"`
}

// Build embeds every entry's Text, reusing the vectors in cachePath that
// were made by the same model from the same text, and writes the cache
// back with only the current entries' vectors. cachePath "" means no
// cache. A missing or unreadable cache is rebuilt, not an error.
func Build(ctx context.Context, emb Embedder, model string, entries []*lexicon.Entry, cachePath string) (*Index, Stats, error) {
	var st Stats
	cached := map[string][]float32{}
	if cachePath != "" {
		if data, err := os.ReadFile(cachePath); err == nil {
			var c cacheFile
			if json.Unmarshal(data, &c) == nil && c.Model == model && c.Vectors != nil {
				cached = c.Vectors
			}
		}
	}

	idx := &Index{emb: emb, model: model, entries: entries, vecs: make([][]float32, len(entries))}
	keys := make([]string, len(entries))
	var missing []int
	var texts []string
	for i, e := range entries {
		t := Text(e)
		keys[i] = hash(t)
		if v, ok := cached[keys[i]]; ok {
			idx.vecs[i] = v
			st.Cached++
			continue
		}
		missing = append(missing, i)
		texts = append(texts, t)
	}
	if len(texts) > 0 {
		vs, err := emb.Embed(ctx, Document, texts)
		if err != nil {
			return nil, st, err
		}
		if len(vs) != len(texts) {
			return nil, st, fmt.Errorf("semantic: %d vectors for %d texts", len(vs), len(texts))
		}
		for j, i := range missing {
			idx.vecs[i] = normalize(vs[j])
		}
		st.Embedded = len(texts)
	}

	if cachePath != "" && (st.Embedded > 0 || len(cached) != st.Cached) {
		out := cacheFile{Model: model, Vectors: make(map[string][]float32, len(entries))}
		for i, k := range keys {
			out.Vectors[k] = idx.vecs[i]
		}
		if err := writeCache(cachePath, out); err != nil {
			return idx, st, fmt.Errorf("semantic: writing cache: %w", err)
		}
	}
	return idx, st, nil
}

// Model is the embedding model the index was built with.
func (idx *Index) Model() string { return idx.model }

// Search returns the k entries closest in meaning to the query (k ≤ 0:
// all), best first, among those keep accepts (nil: all).
func (idx *Index) Search(ctx context.Context, query string, k int, keep func(*lexicon.Entry) bool) ([]Hit, error) {
	if strings.TrimSpace(query) == "" {
		return nil, errors.New("semantic: empty query")
	}
	vs, err := idx.emb.Embed(ctx, Query, []string{query})
	if err != nil {
		return nil, err
	}
	if len(vs) != 1 {
		return nil, fmt.Errorf("semantic: %d vectors for 1 query", len(vs))
	}
	q := normalize(vs[0])
	var hits []Hit
	for i, e := range idx.entries {
		if keep == nil || keep(e) {
			hits = append(hits, Hit{e, dot(q, idx.vecs[i])})
		}
	}
	slices.SortStableFunc(hits, func(a, b Hit) int { return cmp.Compare(b.Score, a.Score) })
	if k > 0 && len(hits) > k {
		hits = hits[:k]
	}
	return hits, nil
}

func hash(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:16])
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

// dot is the dot product; vectors of different lengths (a cache from a
// different model build) score 0 beyond the shorter one.
func dot(a, b []float32) float64 {
	var s float64
	for i := range min(len(a), len(b)) {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}

// writeCache writes the file atomically, so an interrupted run never
// leaves half a cache.
func writeCache(path string, c cacheFile) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".embeddings-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// CachePath is the default cache file for a model: .cache/embeddings-<model>.json
// next to the language directory (the repository root), outside
// language/, which the tools never write.
func CachePath(langDir, model string) string {
	name := strings.NewReplacer(":", "-", "/", "-").Replace(model)
	return filepath.Join(filepath.Dir(filepath.Clean(langDir)), ".cache", "embeddings-"+name+".json")
}
