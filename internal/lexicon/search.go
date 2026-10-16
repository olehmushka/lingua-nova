package lexicon

import (
	"cmp"
	"slices"
	"strings"
	"unicode"
)

// Match is how well a query matched a field. Lower is better.
type Match int

const (
	MatchExact     Match = iota // the whole field: water = water
	MatchWord                   // one word of it: water in spring.water
	MatchPrefix                 // the start of the field or of a word: lov → love.eros
	MatchSubstring              // anywhere in a Rifatian form: ura → huro
)

func (m Match) String() string {
	return [...]string{"exact", "word", "prefix", "substring"}[m]
}

// Hit is one search result: an entry and the field it matched best.
type Hit struct {
	Entry *Entry
	Field string // modern, word, gloss, proto or definition
	Match Match
}

// field is a searchable field with its tier: on equal Match, a lower tier
// sorts first. A query is usually a Rifatian word or an English gloss, so
// those come before proto forms and definitions.
type field struct {
	name  string
	tier  int
	value func(*Entry) string
	words bool // match single words, not only the whole field
	inner bool // allow substring matches
}

var fields = []field{
	{"modern", 0, func(e *Entry) string { return e.Modern }, false, true},
	{"word", 0, func(e *Entry) string { return e.Word }, false, true},
	{"gloss", 1, func(e *Entry) string { return e.Gloss }, true, false},
	protoField,
	{"definition", 3, func(e *Entry) string { return e.Definition }, true, false},
}

var protoField = field{"proto", 2, func(e *Entry) string { return strings.TrimPrefix(e.Proto, "*") }, false, true}

// Search finds entries for a query in either direction: Rifatian → English
// (modern, word and proto forms) and English → Rifatian (gloss and
// definition). A query starting with * searches proto forms only.
//
// Each entry appears once, with its best match. Results are ordered by
// Match, then by field tier, then by modern form, so the order is stable.
//
// English fields match by word, not by substring: "ate" should not find
// "water". Gloss dots count as spaces, so "accident" finds kill.by.accident
// and "lie down" finds lie.down. Rifatian forms also match substrings,
// because a learner may only remember part of a word.
//
// It's a linear scan: fine for hundreds of entries. A much bigger lexicon
// would want an inverted index (word → entries), built once at load time.
func (lex *Lexicon) Search(q string) []Hit {
	q = normalize(q)
	fs := fields
	if p, ok := strings.CutPrefix(q, "*"); ok {
		q, fs = p, []field{protoField}
	}
	if q == "" {
		return nil
	}
	type ranked struct {
		Hit
		tier int
	}
	var hits []ranked
	for _, e := range lex.Entries {
		var best *ranked
		for _, f := range fs {
			m, ok := match(q, normalize(f.value(e)), f)
			if !ok {
				continue
			}
			if best == nil || m < best.Match || m == best.Match && f.tier < best.tier {
				best = &ranked{Hit{e, f.name, m}, f.tier}
			}
		}
		if best != nil {
			hits = append(hits, *best)
		}
	}
	slices.SortStableFunc(hits, func(a, b ranked) int {
		return cmp.Or(
			cmp.Compare(a.Match, b.Match),
			cmp.Compare(a.tier, b.tier),
			cmp.Compare(a.Entry.Modern, b.Entry.Modern),
			cmp.Compare(a.Entry.ID, b.Entry.ID),
		)
	})
	out := make([]Hit, len(hits))
	for i, h := range hits {
		out[i] = h.Hit
	}
	return out
}

// match compares a normalized query with a normalized field value.
func match(q, v string, f field) (Match, bool) {
	switch {
	case v == "":
		return 0, false
	case v == q:
		return MatchExact, true
	}
	if f.words {
		// Multi-word queries ("lie down") match a run of words, so compare
		// on word boundaries: " lie down " in " to lie down ".
		pv, pq := " "+v+" ", " "+q
		if strings.Contains(pv, pq+" ") {
			return MatchWord, true
		}
		if strings.Contains(pv, pq) {
			return MatchPrefix, true
		}
		return 0, false
	}
	if strings.HasPrefix(v, q) {
		return MatchPrefix, true
	}
	if f.inner && strings.Contains(v, q) {
		return MatchSubstring, true
	}
	return 0, false
}

// normalize lower-cases s and turns everything that isn't a letter, a digit
// or a leading * into single spaces: "Water; any liquid." → "water any
// liquid", "kill.by.accident" → "kill by accident".
func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	star := strings.HasPrefix(s, "*")
	words := strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	out := strings.Join(words, " ")
	if star {
		out = "*" + out
	}
	return out
}
