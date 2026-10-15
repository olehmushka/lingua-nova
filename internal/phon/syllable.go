package phon

import (
	"errors"
	"strings"
)

// Syllable is onset + nucleus + coda. The nucleus is one vowel or a
// diphthong; onset and coda may be empty.
type Syllable struct {
	Onset, Nucleus, Coda []Segment
}

// Segments returns the syllable's segments in order.
func (s Syllable) Segments() []Segment {
	out := make([]Segment, 0, len(s.Onset)+len(s.Nucleus)+len(s.Coda))
	out = append(out, s.Onset...)
	out = append(out, s.Nucleus...)
	return append(out, s.Coda...)
}

// ErrNoVowel is returned by Syllabify for a word with no syllable nucleus.
var ErrNoVowel = errors.New("word has no vowel")

// Syllabify splits a segmented word into syllables:
//
//  1. Nuclei: scanning left to right, a vowel joins the next vowel when the
//     pair is a diphthong; otherwise it is a nucleus by itself (di·a).
//  2. Consonants between two nuclei: the longest legal onset at the end of
//     the run goes to the next syllable, the rest is the previous coda
//     (maximal onset principle: du·bro, bil·sa).
//  3. Consonants before the first nucleus are its onset; consonants after
//     the last nucleus are its coda.
//
// Syllabify is a structural parser, not a validator: an illegal coda
// (*cuppe → cup·pe) or a forbidden V-V-V run is split anyway, and checking
// legality is left to the caller. The only error is a word with no vowel.
func (inv *Inventory) Syllabify(segs []Segment) ([]Syllable, error) {
	// Nuclei as [start, end) index ranges into segs.
	type span struct{ start, end int }
	var nuclei []span
	for i := 0; i < len(segs); i++ {
		if segs[i].Kind != Vowel {
			continue
		}
		end := i + 1
		if end < len(segs) && segs[end].Kind == Vowel &&
			inv.diphthongs[[2]string{segs[i].Spell, segs[end].Spell}] {
			end++
		}
		nuclei = append(nuclei, span{i, end})
		i = end - 1
	}
	if len(nuclei) == 0 {
		return nil, ErrNoVowel
	}

	syls := make([]Syllable, len(nuclei))
	onsetStart := 0 // where the current syllable's onset begins
	for k, n := range nuclei {
		syls[k].Onset = segs[onsetStart:n.start]
		syls[k].Nucleus = segs[n.start:n.end]
		if k == len(nuclei)-1 {
			syls[k].Coda = segs[n.end:]
			break
		}
		run := segs[n.end:nuclei[k+1].start]
		split := len(run) - inv.maxOnset(run)
		syls[k].Coda = run[:split]
		onsetStart = n.end + split
	}
	return syls, nil
}

// maxOnset returns the length of the longest suffix of the consonant run
// that is a legal onset. A single consonant is always legal, so any
// non-empty run gives at least 1.
func (inv *Inventory) maxOnset(run []Segment) int {
	for n := len(run); n >= 2; n-- {
		if inv.onsets[seqKey(Spellings(run[len(run)-n:]))] {
			return n
		}
	}
	return min(len(run), 1)
}

// Stress returns the index of the stressed syllable: always the
// penultimate, or the only syllable of a monosyllable.
func Stress(syls []Syllable) int {
	return max(len(syls)-2, 0)
}

// SyllableIPA writes syllables in IPA with dots between them and the stress
// mark ˈ in place of the dot before the stressed syllable: "naˈɾa.ʃu".
// A monosyllable gets no mark.
func SyllableIPA(syls []Syllable, stress int) string {
	var b strings.Builder
	for k, s := range syls {
		switch {
		case k == stress && len(syls) > 1:
			b.WriteString("ˈ")
		case k > 0:
			b.WriteString(".")
		}
		b.WriteString(IPA(s.Segments()))
	}
	return b.String()
}

// SyllableSpelling writes syllables in spelling with dots between them:
// "na.ra.xu".
func SyllableSpelling(syls []Syllable) string {
	parts := make([]string, len(syls))
	for k, s := range syls {
		parts[k] = strings.Join(Spellings(s.Segments()), "")
	}
	return strings.Join(parts, ".")
}
