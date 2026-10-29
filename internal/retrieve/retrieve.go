package retrieve

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/olehmushka/lingua-nova/internal/english"
	"github.com/olehmushka/lingua-nova/internal/lexicon"
)

// Defaults of a Retriever.
const (
	DefaultK = 3 // entries per English word: room for a sense or two besides the best
	// DefaultBudget is a safety net for long sentences: about 80 entries,
	// where a typical sentence needs 15.
	DefaultBudget = 2048
	// rrfK damps the fusion: 1/(rrfK + rank). 60 is the value of the
	// original paper (Cormack et al. 2009) and the usual default; it keeps a
	// single first place in one list from outweighing good places in all.
	rrfK = 60
	// depth is how far down each channel's list fusion looks.
	depth = 20
)

// Dense is search by meaning (package semantic): per query, the entries
// closest in meaning, best first.
type Dense interface {
	SearchMany(ctx context.Context, queries []string, k int) ([][]*lexicon.Entry, error)
}

// Retriever picks the entries for a sentence.
type Retriever struct {
	Tagger *english.Tagger
	BM25   *BM25
	Dense  Dense // nil: lexical channels only
	// Always says which entries every context has anyway (the closed
	// classes); the channels skip them, so they don't take a word's slots.
	Always func(*lexicon.Entry) bool
	K      int // entries per word; 0: DefaultK
	Budget int // tokens for all words' entries together; 0: no limit
}

// Pick is a retrieved entry.
type Pick struct {
	Entry *lexicon.Entry
	Word  string  // the English word it was retrieved for
	Rank  int     // its place among that word's picks, 0 = best
	Score float64 // the fused score
}

// query is one English token's search: its curated readings, or its
// lemma for the search channels.
type query struct {
	word    string
	lemma   string
	tagged  []ranked // the tagger's readings' entries, ranked in tiers
	curated bool     // readings from words.yaml: complete, no search
}

// ranked is an entry at a rank in one channel's list. Equal ranks are
// ties: the channel can't tell those entries apart.
type ranked struct {
	e     *lexicon.Entry
	rank  int
	score float64 // the channel's own score, where it has one (BM25)
}

// query ranks the tagger's entries in tiers by what it knows: an exact
// gloss (water for "water") before a sense (water.cold), and a reading
// in the category the statistical tagger chose (the noun in "drink the
// water") before the others. Within a tier the order is just lexicon
// order, which means nothing; fusing it as a ranking would let file order
// pick among hundreds of senses. So a tier is one tied rank, and the
// other channels break the tie.
func (r *Retriever) query(tok english.Token) query {
	q := query{word: tok.Text, lemma: strings.ToLower(tok.Text), curated: r.Tagger.Listed(tok.Text)}
	if len(tok.Readings) > 0 {
		q.lemma = tok.Readings[0].Lemma
	}
	chosen := slices.ContainsFunc(tok.Readings, func(rd english.Reading) bool { return rd.Tagged })
	at := map[string]int{} // entry id → index in q.tagged
	for _, rd := range tok.Readings {
		for _, e := range rd.Entries {
			tier := 0
			if !strings.EqualFold(e.Gloss, q.lemma) { // God for "god"
				tier++
			}
			if chosen && !rd.Tagged {
				tier++
			}
			if q.curated {
				tier = len(q.tagged) // complete and in order: keep it
			}
			if i, ok := at[e.ID]; ok {
				q.tagged[i].rank = min(q.tagged[i].rank, tier)
				continue
			}
			at[e.ID] = len(q.tagged)
			q.tagged = append(q.tagged, ranked{e: e, rank: tier})
		}
	}
	slices.SortStableFunc(q.tagged, func(a, b ranked) int { return cmp.Compare(a.rank, b.rank) })
	return q
}

// Sentence returns the picks for every word of the sentence: up to K per
// word, fused from the channels, then cut to the Budget. An entry picked
// for two words is kept once, at its better rank. A dense error is
// returned with the lexical picks, which are still usable.
func (r *Retriever) Sentence(ctx context.Context, sentence string) ([]Pick, error) {
	var qs []query
	for _, tok := range r.Tagger.Tag(sentence).Tokens {
		qs = append(qs, r.query(tok))
	}
	lists, err := r.fuse(ctx, qs)
	var picks []Pick
	seen := map[string]int{} // entry id → index in picks
	for _, list := range lists {
		for _, p := range list {
			if i, ok := seen[p.Entry.ID]; ok {
				if p.Rank < picks[i].Rank {
					picks[i] = p
				}
				continue
			}
			seen[p.Entry.ID] = len(picks)
			picks = append(picks, p)
		}
	}
	return r.budget(picks), err
}

// Word returns the picks for one English word (up to K), as the lookup
// tool shows them.
func (r *Retriever) Word(ctx context.Context, tok english.Token, k int) ([]Pick, error) {
	rr := *r
	rr.K = k
	lists, err := rr.fuse(ctx, []query{r.query(tok)})
	return lists[0], err
}

// fuse ranks each query's candidates by Reciprocal Rank Fusion over the
// channels and keeps the top K. A curated word (words.yaml lists its
// entries) keeps its readings as they are: they are complete.
//
// RRF scores an entry Σ 1/(rrfK + rank) over the lists it is in. It uses
// ranks, not scores, so channels with incomparable scores (BM25 against
// cosine similarity) fuse without calibration, and an entry several
// channels agree on beats one that a single channel likes.
func (r *Retriever) fuse(ctx context.Context, qs []query) ([][]Pick, error) {
	k := cmp.Or(r.K, DefaultK)
	open := func(e *lexicon.Entry) bool { return r.Always == nil || !r.Always(e) }

	// Dense: one call for every searchable word.
	var dense [][]*lexicon.Entry
	var err error
	if r.Dense != nil {
		var texts []string
		for _, q := range qs {
			if !q.curated {
				texts = append(texts, q.lemma)
			}
		}
		if len(texts) > 0 {
			dense, err = r.Dense.SearchMany(ctx, texts, depth)
			if err != nil {
				dense = nil
			}
		}
	}

	out := make([][]Pick, len(qs))
	di := 0
	for i, q := range qs {
		if q.curated {
			for _, t := range q.tagged[:min(len(q.tagged), k)] {
				out[i] = append(out[i], Pick{t.e, q.word, t.rank, 1 / float64(rrfK+t.rank)})
			}
			continue
		}
		openOnly := func(l []ranked) []ranked {
			return slices.DeleteFunc(l, func(t ranked) bool { return !open(t.e) })
		}
		lists := [][]ranked{openOnly(slices.Clone(q.tagged))}
		lists = append(lists, tied(r.BM25.Search(q.lemma, depth, open)))
		if c := others(qs, i); c != "" {
			lists = append(lists, r.lesk(q, c))
		}
		if dense != nil {
			var dl []ranked
			for j, e := range dense[di] {
				dl = append(dl, ranked{e: e, rank: j})
			}
			lists = append(lists, openOnly(dl))
			di++
		}
		out[i] = top(q.word, lists, k)
	}
	return out, err
}

// others is the sentence's other searchable words, for Lesk: "god" for
// "love" in "We will love the god."
func others(qs []query, i int) string {
	var ws []string
	for j, q := range qs {
		if j != i && !q.curated {
			ws = append(ws, q.lemma)
		}
	}
	return strings.Join(ws, " ")
}

// lesk ranks the word's own entries (gloss head = the lemma) by how well
// their gloss and definition match the word together with the rest of
// the sentence. It is the simplified Lesk algorithm (Lesk 1986), the
// classic dictionary-based word-sense disambiguation: the right sense is
// the one whose definition shares words with the context. "love" in "We
// will love the god" is love.agape, whose definition says "God's love".
// BM25 does the counting, with idf, so a shared rare word counts more
// than a shared common one.
func (r *Retriever) lesk(q query, others string) []ranked {
	own := func(e *lexicon.Entry) bool {
		head, _, _ := strings.Cut(e.Gloss, ".")
		return strings.EqualFold(head, q.lemma) && (r.Always == nil || !r.Always(e))
	}
	return tied(r.BM25.Search(q.lemma+" "+others, depth, own))
}

// tied ranks BM25 hits; equal scores share a rank ("competition
// ranking": 0, 1, 1, 3), as the tagger's tiers do.
func tied(hs []Hit) []ranked {
	var out []ranked
	for j, h := range hs {
		rank := j
		if j > 0 && h.Score == out[j-1].score {
			rank = out[j-1].rank
		}
		out = append(out, ranked{e: h.Entry, rank: rank, score: h.Score})
	}
	return out
}

// top fuses ranked lists with RRF and returns the k best. Each list
// counts down to rank depth. Tied ranks make tied sums common, so a tie
// goes to the higher channel score (BM25's: the one that has scores),
// then to the first appearance (the first list first). Entries are the
// same entry by id: channels may hold different copies of a lexicon.
func top(word string, lists [][]ranked, k int) []Pick {
	score, tie := map[string]float64{}, map[string]float64{}
	var order []*lexicon.Entry
	for _, l := range lists {
		for _, t := range l {
			if t.rank >= depth {
				break
			}
			if _, ok := score[t.e.ID]; !ok {
				order = append(order, t.e)
			}
			score[t.e.ID] += 1 / float64(rrfK+t.rank)
			tie[t.e.ID] += t.score
		}
	}
	slices.SortStableFunc(order, func(a, b *lexicon.Entry) int {
		return cmp.Or(cmp.Compare(score[b.ID], score[a.ID]), cmp.Compare(tie[b.ID], tie[a.ID]))
	})
	var out []Pick
	for rank, e := range order[:min(len(order), k)] {
		out = append(out, Pick{e, word, rank, score[e.ID]})
	}
	return out
}

// budget keeps the picks that fit in Budget tokens, best ranks first:
// every word's first pick, then every word's second, and so on, so a cut
// takes the doubtful tails, never a word's only entry. Picks keep their
// order.
func (r *Retriever) budget(picks []Pick) []Pick {
	if r.Budget <= 0 {
		return picks
	}
	idx := make([]int, len(picks))
	for i := range idx {
		idx[i] = i
	}
	slices.SortStableFunc(idx, func(a, b int) int { return cmp.Compare(picks[a].Rank, picks[b].Rank) })
	keep := make([]bool, len(picks))
	used := 0
	for _, i := range idx {
		cost := Tokens(picks[i].Entry)
		if picks[i].Rank > 0 && used+cost > r.Budget {
			continue
		}
		keep[i] = true
		used += cost
	}
	var out []Pick
	for i, p := range picks {
		if keep[i] {
			out = append(out, p)
		}
	}
	return out
}

// Tokens estimates an entry's tokens in the prompt's dictionary line,
// with llm.Context.Size's estimate: its fields, 16 bytes of separators,
// 3 bytes per token.
func Tokens(e *lexicon.Entry) int {
	return (len(e.ID) + len(cmp.Or(e.Word, e.Modern)) + len(e.POS) + len(e.Gender) + len(e.Gloss) + len(e.Definition) + 16) / 3
}
