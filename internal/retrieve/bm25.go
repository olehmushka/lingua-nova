// Package retrieve picks the dictionary entries a sentence's prompt
// needs, out of a lexicon of any size.
//
// It is the R of RAG (retrieval-augmented generation): the model can't
// read 300k entries, so each prompt gets the few it needs. Retrieval
// runs several retrievers ("channels"), each good at different things,
// and fuses their rankings:
//
//   - the English tagger's readings: the entries whose gloss is the
//     word's lemma, exact and precise, but blind to synonyms;
//   - BM25, a lexical search over gloss, id and definition, which also
//     finds a word inside a definition ("big" in "Large; big.");
//   - optionally dense vectors (package semantic), which find meaning
//     without shared letters (sea → ocean).
//
// Reciprocal Rank Fusion merges their lists, and a per-word limit and a
// token budget keep the prompt the same size however big the lexicon
// grows.
package retrieve

import (
	"cmp"
	"math"
	"slices"
	"strings"
	"unicode"

	"github.com/olehmushka/lingua-nova/internal/lexicon"
)

// BM25 is an inverted index over the lexicon, ranked with BM25F.
//
// An inverted index maps each term to the entries that contain it (its
// postings), so a query reads only the postings of its own terms, not
// every entry: at 300k entries a word like "goat" touches a few dozen.
//
// BM25 scores an entry for a query term by three things:
//
//   - how often the term occurs in it (tf), with diminishing returns,
//     saturating at k1: a gloss saying "water" twice isn't twice as
//     relevant;
//   - how rare the term is in the whole lexicon (idf): "goat" says more
//     than "animal", which hundreds of definitions use;
//   - how long the field is (bNorm): "water" in a one-word gloss counts for
//     more than in a 20-word definition.
//
// BM25F, the multi-field variant, weights each field's term frequency
// before saturating: a gloss match is worth GlossWeight definition
// matches.
type BM25 struct {
	entries  []*lexicon.Entry
	postings map[string][]posting
	lens     [nFields][]uint16 // per field, per entry: terms
	avg      [nFields]float64
}

type posting struct {
	doc int32
	tf  [nFields]uint16
}

// Fields, their weights and the BM25 parameters: the usual defaults
// (k1 1.2, bNorm 0.75); the gloss weight makes a gloss match win over any
// number of definition matches of a common word.
const (
	fGloss = iota
	fID
	fDef
	nFields

	k1          = 1.2
	bNorm       = 0.75
	GlossWeight = 3
)

var weights = [nFields]float64{fGloss: GlossWeight, fID: 1, fDef: 1}

// Hit is a search result: higher scores are better; the scale depends on
// the query.
type Hit struct {
	Entry *lexicon.Entry
	Score float64
}

// NewBM25 indexes the entries.
func NewBM25(entries []*lexicon.Entry) *BM25 {
	x := &BM25{entries: entries, postings: map[string][]posting{}}
	var sum [nFields]float64
	for f := range nFields {
		x.lens[f] = make([]uint16, len(entries))
	}
	for i, e := range entries {
		tf := map[string]*posting{}
		for f, text := range [nFields]string{fGloss: e.Gloss, fID: e.ID, fDef: e.Definition} {
			terms := Terms(text)
			x.lens[f][i] = uint16(min(len(terms), math.MaxUint16))
			sum[f] += float64(len(terms))
			for _, t := range terms {
				p := tf[t]
				if p == nil {
					p = &posting{doc: int32(i)}
					tf[t] = p
				}
				p.tf[f]++
			}
		}
		for t, p := range tf {
			x.postings[t] = append(x.postings[t], *p)
		}
	}
	for f := range nFields {
		x.avg[f] = max(sum[f]/float64(max(len(entries), 1)), 1)
	}
	return x
}

// Search returns the k best entries for the query (k ≤ 0: all that match
// a term), best first, among those keep accepts (nil: all). Ties go in
// lexicon order.
func (x *BM25) Search(query string, k int, keep func(*lexicon.Entry) bool) []Hit {
	scores := map[int32]float64{}
	n := float64(len(x.entries))
	for _, t := range slices.Compact(slices.Sorted(slices.Values(Terms(query)))) {
		ps := x.postings[t]
		if len(ps) == 0 {
			continue
		}
		df := float64(len(ps))
		idf := math.Log(1 + (n-df+0.5)/(df+0.5))
		for _, p := range ps {
			var tf float64
			for f := range nFields {
				if p.tf[f] > 0 {
					norm := 1 - bNorm + bNorm*float64(x.lens[f][p.doc])/x.avg[f]
					tf += weights[f] * float64(p.tf[f]) / norm
				}
			}
			scores[p.doc] += idf * tf / (k1 + tf)
		}
	}
	hits := make([]Hit, 0, len(scores))
	docs := make([]int32, 0, len(scores))
	for d := range scores {
		docs = append(docs, d)
	}
	slices.SortFunc(docs, func(a, b int32) int { return cmp.Or(cmp.Compare(scores[b], scores[a]), cmp.Compare(a, b)) })
	for _, d := range docs {
		if keep != nil && !keep(x.entries[d]) {
			continue
		}
		hits = append(hits, Hit{x.entries[d], scores[d]})
		if k > 0 && len(hits) == k {
			break
		}
	}
	return hits
}

// stopwords carry no meaning to search by.
var stopwords = map[string]bool{
	"a": true, "an": true, "the": true, "of": true, "to": true, "in": true, "on": true,
	"and": true, "or": true, "is": true, "be": true, "as": true, "at": true, "by": true,
	"for": true, "with": true, "from": true, "it": true, "its": true, "that": true,
	"this": true, "any": true, "one": true, "who": true, "which": true, "what": true,
}

// Terms splits a text into search terms: lowercase letter runs, without
// stopwords, each through Stem. Gloss dots and id underscores split words.
func Terms(text string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) }) {
		if !stopwords[w] {
			out = append(out, Stem(w))
		}
	}
	return out
}

// Stem is the S-stemmer (Harman 1991): it only undoes English plurals,
// which is all a query of lemmas against definitions needs ("eyes" in a
// definition matches the query "eye"), and it never changes a word
// beyond recognition, as heavier stemmers do (Porter: "news" → "new").
func Stem(w string) string {
	switch {
	case len(w) > 3 && strings.HasSuffix(w, "ies") && !strings.HasSuffix(w, "eies") && !strings.HasSuffix(w, "aies"):
		return w[:len(w)-3] + "y"
	case len(w) > 3 && strings.HasSuffix(w, "es") && !strings.HasSuffix(w, "aes") && !strings.HasSuffix(w, "ees") && !strings.HasSuffix(w, "oes"):
		return w[:len(w)-1]
	case len(w) > 3 && strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "us") && !strings.HasSuffix(w, "ss"):
		return w[:len(w)-1]
	}
	return w
}
