package lexicon

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/olehmushka/lingua-nova/internal/phon"
)

// PartsOfSpeech is the closed set of pos values.
var PartsOfSpeech = []string{
	"adjective", "adverb", "article", "complementizer", "conjunction", "determiner", "noun",
	"numeral", "particle", "preposition", "pronoun", "verb",
}

// verbEndings are the infinitive endings of the three verb classes.
var verbEndings = []string{"ar", "er", "ir"}

// Validate checks every entry and returns all problems (including the ones
// found by Load), sorted by file and line.
//
// The checks are:
//   - required fields, duplicate ids, pos in PartsOfSpeech;
//   - gender m/f: required on nouns, optional on pronouns (sela keeps the
//     gender of the noun it came from), not allowed elsewhere;
//   - verbs end in -ar/-er/-ir (proto and modern);
//   - proto uses only proto segments and obeys the proto phonotactics
//     (its own codas and finals, plus geminates like *pippo);
//   - modern (and word, if set) obeys the modern phonotactics.
func Validate(lex *Lexicon, inv *phon.Inventory) []Problem {
	probs := slices.Clone(lex.Problems)
	firstID := map[string]*Entry{}
	for _, e := range lex.Entries {
		bad := func(field, format string, args ...any) {
			probs = append(probs, Problem{e.File, e.LineOf(field), e.ID, fmt.Sprintf(format, args...)})
		}

		for _, f := range []struct{ name, val string }{
			{"id", e.ID}, {"proto", e.Proto}, {"modern", e.Modern},
			{"pos", e.POS}, {"gloss", e.Gloss}, {"definition", e.Definition},
		} {
			if strings.TrimSpace(f.val) == "" {
				bad(f.name, "missing %s", f.name)
			}
		}

		if e.ID != "" {
			if first, dup := firstID[e.ID]; dup {
				bad("id", "duplicate id (first defined at %s:%d)", first.File, first.Line)
			} else {
				firstID[e.ID] = e
			}
		}

		if e.POS != "" && !slices.Contains(PartsOfSpeech, e.POS) {
			bad("pos", "unknown pos %q (want one of %s)", e.POS, strings.Join(PartsOfSpeech, ", "))
		}
		switch {
		case e.POS == "noun" && e.Gender == "":
			bad("pos", "noun without gender")
		case e.Gender == "":
		case e.POS != "noun" && e.POS != "pronoun":
			bad("gender", "gender on a %s; only nouns and pronouns have gender", e.POS)
		case e.Gender != "m" && e.Gender != "f":
			bad("gender", "gender %q: want m or f", e.Gender)
		}

		if e.POS == "verb" {
			for _, f := range []struct{ name, val string }{{"proto", e.Proto}, {"modern", e.Modern}} {
				if f.val != "" && !hasVerbEnding(f.val) {
					bad(f.name, "verb %s %q must end in -ar, -er or -ir", f.name, f.val)
				}
			}
		}

		if e.Proto != "" {
			checkProto(inv, e.Proto, func(msg string) { bad("proto", "proto *%s: %s", e.Proto, msg) })
		}
		for _, f := range []struct{ name, val string }{{"modern", e.Modern}, {"word", e.Word}} {
			if f.val != "" {
				checkModern(inv, f.val, func(msg string) { bad(f.name, "%s %q: %s", f.name, f.val, msg) })
			}
		}
	}
	slices.SortStableFunc(probs, func(a, b Problem) int {
		return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line))
	})
	return probs
}

func hasVerbEnding(w string) bool {
	return slices.ContainsFunc(verbEndings, func(end string) bool {
		return len(w) > len(end) && strings.HasSuffix(w, end)
	})
}

// checkProto reports segments that only exist in the modern language and
// everything that breaks the proto phonotactics.
func checkProto(inv *phon.Inventory, form string, report func(string)) {
	segs, err := inv.Tokenize(form)
	if err != nil {
		report(err.Error())
		return
	}
	for _, s := range segs {
		if !s.Proto {
			report(fmt.Sprintf("%q /%s/ is not a proto-language sound", s.Spell, s.IPA))
		}
	}
	for _, v := range inv.CheckProtoPhonotactics(segs) {
		report(v.Msg)
	}
}

// checkModern reports everything that breaks the modern phonotactics.
func checkModern(inv *phon.Inventory, form string, report func(string)) {
	segs, err := inv.Tokenize(form)
	if err != nil {
		report(err.Error())
		return
	}
	for _, v := range inv.CheckPhonotactics(segs) {
		report(v.Msg)
	}
}
