package english

import "strings"

// candidate is a possible lemma of a word form. inflected means the form
// carried the marked feature: plural for a noun, past for a verb.
type candidate struct {
	lemma     string
	inflected bool
}

// lemmas lists the possible lemmas of a word under a category, most
// likely first; the first is the word itself or its irregular base.
// English inflection is mostly suffixes, so this is suffix stripping, the
// same idea as the Rifatian analyzer, only without the verification step:
// the caller keeps the first candidate the lexicon knows.
func (t *Tagger) lemmas(cat, w string) []candidate {
	var out []candidate
	add := func(lemma string, infl bool) {
		if len(lemma) < 2 {
			return
		}
		for _, c := range out {
			if c.lemma == lemma {
				return
			}
		}
		out = append(out, candidate{lemma, infl})
	}
	switch cat {
	case "N":
		if base, ok := t.words.Plural[w]; ok {
			add(base, true)
		}
		add(w, false)
		if s, ok := strings.CutSuffix(w, "ies"); ok {
			add(s+"y", true)
		}
		if s, ok := strings.CutSuffix(w, "ves"); ok {
			add(s+"f", true)
			add(s+"fe", true)
		}
		if s, ok := strings.CutSuffix(w, "es"); ok {
			add(s, true)
		}
		if s, ok := strings.CutSuffix(w, "s"); ok && !strings.HasSuffix(w, "ss") {
			add(s, true)
		}
	case "V":
		// Archaic 2sg -est/-st and 3sg -eth/-th: sawest, seest, loveth.
		forms := []string{w}
		for _, suf := range []string{"est", "st", "eth", "th"} {
			if s, ok := strings.CutSuffix(w, suf); ok {
				forms = append(forms, s, s+"e")
			}
		}
		for _, f := range forms {
			if base, ok := t.words.Past[f]; ok {
				add(base, true)
			}
			if base, ok := t.words.Present[f]; ok {
				add(base, false)
			}
		}
		for _, f := range forms {
			add(f, false)
			if s, ok := strings.CutSuffix(f, "ies"); ok {
				add(s+"y", false)
			}
			if s, ok := strings.CutSuffix(f, "es"); ok {
				add(s, false)
			}
			if s, ok := strings.CutSuffix(f, "s"); ok && !strings.HasSuffix(f, "ss") {
				add(s, false)
			}
			if s, ok := strings.CutSuffix(f, "ied"); ok {
				add(s+"y", true)
			}
			if s, ok := strings.CutSuffix(f, "ed"); ok {
				add(s, true)     // walked
				add(s+"e", true) // loved
				if n := len(s); n > 2 && s[n-1] == s[n-2] {
					add(s[:n-1], true) // stopped
				}
			}
		}
	default:
		add(w, false)
	}
	if len(out) == 0 {
		out = append(out, candidate{w, false})
	}
	return out
}
