package english

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/olehmushka/lingua-nova/internal/lexicon"
)

const langDir = "../../language"

var (
	once      sync.Once
	shared    *Tagger
	sharedLex *lexicon.Lexicon
	setupErr  error
)

// tagger is shared: loading the perceptron model takes a moment.
func tagger(t *testing.T) *Tagger {
	t.Helper()
	once.Do(func() {
		w, err := LoadWords(filepath.Join(langDir, "english", "words.yaml"))
		if err != nil {
			setupErr = err
			return
		}
		sharedLex, setupErr = lexicon.Load(filepath.Join(langDir, "lexicon"))
		if setupErr == nil {
			shared = New(w, sharedLex)
		}
	})
	if setupErr != nil {
		t.Fatal(setupErr)
	}
	return shared
}

// show renders a token's readings compactly: saw V:see.PST[see]
// N:… with ids of the entries, and * marking the tagger's choice.
func show(tok Token) string {
	var rs []string
	for _, r := range tok.Readings {
		s := r.Cat + ":" + r.Lemma
		for _, f := range []string{r.Num, r.Tense} {
			if f != "" {
				s += "." + f
			}
		}
		var ids []string
		for _, e := range r.Entries {
			ids = append(ids, e.ID)
		}
		s += "[" + strings.Join(ids, ",") + "]"
		if r.Tagged {
			s += "*"
		}
		rs = append(rs, s)
	}
	return tok.Text + " " + strings.Join(rs, " ")
}

func TestTag(t *testing.T) {
	tg := tagger(t)
	tests := []struct {
		in   string
		want []string
		end  string
	}{
		{"Didst thou see the woman's dog?", []string{
			// The tagger says NNP and NN for these two; the table wins.
			"Didst Aux:didst.PST[]*",
			"thou Pron:thou.SG[thou]*",
			"see V:see.PRS[see]*",
			"the Det:the[the]*",
			"woman N:woman.SG[woman]*",
			"'s Gen:'s[]*",
			"dog N:dog.SG[dog]*",
		}, "?"},
		{"The men will drink the water.", []string{
			"The Det:the[the]*",
			"men N:man.PL[man]*",
			"will Aux:will.FUT[]*",
			"drink V:drink.PRS[drink]*",
			"the Det:the[the]*",
			"water N:water.SG[water]*",
		}, "."},
		{"I won't love her", []string{
			"I Pron:i.SG[i]*",
			"wo Aux:wo.FUT[]*",
			"n't Neg:n't[not]*",
			"love V:love.PRS[love_agape,love_eros,love_philia,love_storge,love_doing]*",
			"her Pron:her.SG[she]* Poss:her.SG[she]*",
		}, ""},
		{"Thou sawest the women's dogs.", []string{
			"Thou Pron:thou.SG[thou]*",
			"sawest V:see.PST[see]*",
			"the Det:the[the]*",
			"women N:woman.PL[woman]*",
			"'s Gen:'s[]*",
			"dogs N:dog.PL[dog]*",
		}, "."},
		{"You killed them", []string{
			"You Pron:you.SG[thou,you_polite]* Pron:you.PL[thou,you_polite]*",
			"killed V:kill.PST[kill,kill_accident]*",
			"them Pron:them.PL[he,she]*",
		}, ""},
		// Unknown words keep the tagger's category and have no entries.
		// Here the unknown word also confuses the tagger (eats as a
		// plural noun, bread as a verb), but the lexicon readings keep the
		// right categories available for the parser.
		{"The dragon eats bread", []string{
			"The Det:the[the]*",
			"dragon N:dragon.SG[]*",
			"eats N:eats.PL[]* V:eat.PRS[eat]",
			"bread V:bread.PRS[]* N:bread.SG[bread]",
		}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			s := tg.Tag(tt.in)
			var got []string
			for _, tok := range s.Tokens {
				got = append(got, show(tok))
			}
			if g, w := strings.Join(got, "\n"), strings.Join(tt.want, "\n"); g != w {
				t.Errorf("got\n%s\nwant\n%s", g, w)
			}
			if s.End != tt.end {
				t.Errorf("end %q, want %q", s.End, tt.end)
			}
		})
	}
}

func TestLemmas(t *testing.T) {
	tg := tagger(t)
	tests := []struct{ cat, word, want string }{
		{"N", "dogs", "dog+"},
		{"N", "wolves", "wolf+"},
		{"N", "men", "man+"},
		{"V", "sees", "see"},
		{"V", "loved", "love+"},
		{"V", "stopped", "stop+"},
		{"V", "carried", "carry+"},
		{"V", "seest", "see"},
		{"V", "loveth", "love"},
		{"V", "wept", "weep+"},
		// be: present forms aren't past (no +), past ones are.
		{"V", "is", "be"},
		{"V", "are", "be"},
		{"V", "art", "be"},
		{"V", "was", "be+"},
		{"V", "wast", "be+"},
		{"V", "were", "be+"},
	}
	for _, tt := range tests {
		var got string
		for _, c := range tg.lemmas(tt.cat, tt.word) {
			if c.lemma == strings.TrimSuffix(tt.want, "+") {
				got = c.lemma
				if c.inflected {
					got += "+"
				}
			}
		}
		if got != tt.want {
			t.Errorf("%s %s: got %q in %v, want %q", tt.cat, tt.word, got, tg.lemmas(tt.cat, tt.word), tt.want)
		}
	}
}

func TestWordsCheck(t *testing.T) {
	tg := tagger(t)
	if ps := tg.words.Check(sharedLex, nil); len(ps) > 0 {
		t.Errorf("words.yaml: %v", ps)
	}
	w, err := ParseWords("w.yaml", []byte("words:\n  foo: [{cat: Det, ids: [nope]}]\n"))
	if err != nil {
		t.Fatal(err)
	}
	ps := w.Check(sharedLex, []string{"N"})
	got := fmt.Sprint(ps)
	for _, want := range []string{"w.yaml:2: foo: category Det isn't in the grammar", `w.yaml:2: foo: no lexicon entry "nope"`} {
		if !strings.Contains(got, want) {
			t.Errorf("got %s, want %s", got, want)
		}
	}
}

func TestParseWordsErrors(t *testing.T) {
	for _, src := range []string{
		"words:\n  foo: [{ids: [the]}]\n",
		"words:\n  foo: [{cat: Pron, num: DU}]\n",
		"words:\n  foo: [{cat: Aux, tense: AOR}]\n",
		"words:\n  foo: [{cat: Aux, mood: SBJV}]\n",
		"word: {}\n",
	} {
		if _, err := ParseWords("w.yaml", []byte(src)); err == nil {
			t.Errorf("%q: no error", src)
		}
	}
}
