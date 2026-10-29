// Command lnova is the toolkit for the Rifatian language.
package main

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"math/rand/v2"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/olehmushka/lingua-nova/internal/eval"
	"github.com/olehmushka/lingua-nova/internal/gen"
	"github.com/olehmushka/lingua-nova/internal/gloss"
	"github.com/olehmushka/lingua-nova/internal/lexicon"
	"github.com/olehmushka/lingua-nova/internal/llm"
	"github.com/olehmushka/lingua-nova/internal/morph"
	"github.com/olehmushka/lingua-nova/internal/phon"
	"github.com/olehmushka/lingua-nova/internal/retrieve"
	"github.com/olehmushka/lingua-nova/internal/semantic"
	"github.com/olehmushka/lingua-nova/internal/sound"
	"github.com/olehmushka/lingua-nova/internal/translate"
)

const usage = `usage: lnova [-lang dir] <command> [args]

commands:
  ipa <word>...   show segments and IPA with syllables and stress
  validate        check the lexicon against the phonology and entry format,
                  and every inflected form against the phonotactics
  rules           parse sound-changes.txt and list the rules with line numbers
  evolve          run every proto form through the sound changes and compare
                  with modern
  evolve -trace <id|*proto>...
                  show the derivation of lexicon entries or ad-hoc proto forms
  gen [-n 20] [-seed N]
                  generate new proto roots and show their modern forms
  gen -count      count the lexicon's syllable templates (generator weights)
  inflect <id> [features...]
                  inflect a word (see PST 1SG), or show its whole paradigm
  analyze <word>...
                  find the root and features of word forms (naraxu → see PST 1SG)
  gloss <sentence>
                  interlinear gloss: each word split into morphemes over
                  their glosses, ambiguity resolved by word order and agreement
  parse <english sentence>
                  tag an English sentence and show its parse trees
  translate [-n 20] [-tree] <english sentence>
                  translate simple English into Rifatian: every translation,
                  ranked, with the gloss of the best one
  llm [-ctx retrieved|full|none] [-k 3] [-budget 2048] [-dense] [-tools] [-model M] [-attempts 3] [-temp 0 -seed S] [-compare] <english sentence>
                  translate with a local model (Ollama, via the llm/ service);
                  a retrieved context has the closed classes and up to -k entries per
                  word (tagger, BM25, Lesk and with -dense embeddings, fused), within
                  -budget tokens;
                  the glosser checks each answer and problems go back to the model;
                  -tools lets the model call lookup, inflect, analyze and check
  coin -pos noun -gender f [-def D] [-id ID] [-n 5] [-want 3] [-rounds 3] <english>
                  coin new proto roots with the local model; the phonology, sound
                  changes and validator accept or reject each one, and the
                  accepted ones are printed as lexicon YAML to paste; -related N
                  of the example roots are chosen by meaning (embeddings), those
                  within -related-margin of the best match
  similar [-k 10] [-pos P] <english> | -eval evals/similar.yaml
                  search the lexicon by meaning with an embedding model; vectors
                  are cached in .cache/; -eval reports recall@k
  eval [-setups retrieved,retrieved+tools,none+tools] [-runs 5] [-temp 0.7] [-tag T] [-out F] [-baseline F] [-list]
                  translate language/evals.yaml several times per setup and
                  report pass rates with 95% intervals; -list: no model
  eval -retrieval [-gold language] [-k 3] [-budget 2048] [-dense]
                  measure the retrieved contexts without a model: recall of the
                  entries the rule-based translation uses, entries and tokens
  fixture [-n 300000] [-senses 0.1] [-homonyms 0.01] [-seed 1] [-out .cache/fixture-300k]
                  copy the language directory with -n synthetic entries added to
                  the lexicon, for scale tests (lnova -lang <out> ...)
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("lnova", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	langDir := fs.String("lang", "language", "language data directory")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return 2
	}

	cmd, rest := fs.Arg(0), fs.Args()[1:]
	switch cmd {
	case "ipa":
		return cmdIPA(*langDir, rest, stdout, stderr)
	case "validate":
		return cmdValidate(*langDir, rest, stdout, stderr)
	case "rules":
		return cmdRules(*langDir, rest, stdout, stderr)
	case "evolve":
		return cmdEvolve(*langDir, rest, stdout, stderr)
	case "gen":
		return cmdGen(*langDir, rest, stdout, stderr)
	case "inflect":
		return cmdInflect(*langDir, rest, stdout, stderr)
	case "analyze":
		return cmdAnalyze(*langDir, rest, stdout, stderr)
	case "gloss":
		return cmdGloss(*langDir, rest, stdout, stderr)
	case "parse":
		return cmdParse(*langDir, rest, stdout, stderr)
	case "translate":
		return cmdTranslate(*langDir, rest, stdout, stderr)
	case "llm":
		return cmdLLM(*langDir, rest, stdout, stderr)
	case "coin":
		return cmdCoin(*langDir, rest, stdout, stderr)
	case "similar":
		return cmdSimilar(*langDir, rest, stdout, stderr)
	case "eval":
		return cmdEval(*langDir, rest, stdout, stderr)
	case "fixture":
		return cmdFixture(*langDir, rest, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "lnova: unknown command %q\n%s", cmd, usage)
		return 2
	}
}

// cmdIPA prints each word's segments and syllabified, stressed IPA:
//
//	folha   f·o·lh·a   /ˈfo.ʎa/
//
// A leading * (proto form) is accepted and kept in the output.
func cmdIPA(langDir string, words []string, stdout, stderr io.Writer) int {
	if len(words) == 0 {
		fmt.Fprintln(stderr, "usage: lnova ipa <word>...")
		return 2
	}
	inv, err := phon.Load(filepath.Join(langDir, "phonology.yaml"))
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 3, ' ', 0)
	status := 0
	for _, w := range words {
		segs, err := inv.Tokenize(strings.TrimPrefix(w, "*"))
		if err != nil {
			fmt.Fprintf(stderr, "lnova: %v\n", err)
			status = 1
			continue
		}
		syls, err := inv.Syllabify(segs)
		if err != nil {
			fmt.Fprintf(stderr, "lnova: %q: %v\n", w, err)
			status = 1
			continue
		}
		fmt.Fprintf(tw, "%s\t%s\t/%s/\n", w, strings.Join(phon.Spellings(segs), "·"), phon.SyllableIPA(syls, phon.Stress(syls)))
	}
	tw.Flush()
	return status
}

// cmdValidate checks every lexicon entry, and every form morphology.yaml
// builds from it, and prints problems like compiler errors, one per line,
// then a summary:
//
//	language/lexicon/verbs.yaml:42: ruccar: verb modern "ruca" must end in -ar, -er or -ir
//	104 entries, 1 problem
//
// It exits 1 if there are problems, so it can run in CI.
func cmdValidate(langDir string, args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		fmt.Fprintln(stderr, "usage: lnova validate")
		return 2
	}
	inv, err := phon.Load(filepath.Join(langDir, "phonology.yaml"))
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	lex, err := lexicon.Load(filepath.Join(langDir, "lexicon"))
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	rules, err := morph.Load(filepath.Join(langDir, "morphology.yaml"), inv)
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	probs := append(lexicon.Validate(lex, inv), morph.Check(lex, rules)...)
	if _, err := os.Stat(filepath.Join(langDir, "english")); err == nil {
		g, w, err := translate.LoadEnglish(langDir)
		if err != nil {
			fmt.Fprintf(stderr, "lnova: %v\n", err)
			return 1
		}
		probs = append(probs, translate.Check(g, w, lex)...)
	}
	slices.SortStableFunc(probs, func(a, b lexicon.Problem) int {
		return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line))
	})
	for _, p := range probs {
		fmt.Fprintln(stdout, p)
	}
	noun := "problems"
	if len(probs) == 1 {
		noun = "problem"
	}
	fmt.Fprintf(stdout, "%d entries, %d %s\n", len(lex.Entries), len(probs), noun)
	if len(probs) > 0 {
		return 1
	}
	return 0
}

// cmdRules parses the sound changes and prints each rule with its line
// number, the id used in derivation traces:
//
//	26   li > lh / _V
//
// Parse errors are printed like compiler errors, and the exit code is 1.
func cmdRules(langDir string, args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		fmt.Fprintln(stderr, "usage: lnova rules")
		return 2
	}
	inv, err := phon.Load(filepath.Join(langDir, "phonology.yaml"))
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	rules, err := sound.Load(filepath.Join(langDir, "sound-changes.txt"), inv)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 3, ' ', 0)
	for _, r := range rules {
		fmt.Fprintf(tw, "%d\t%s\n", r.Line, r)
	}
	tw.Flush()
	return 0
}

// cmdEvolve runs every lexicon proto form through the sound changes and
// prints the entries whose result differs from modern, then a summary:
//
//	language/lexicon/nature.yaml:39: water: *esia → esa, want exa
//	104 entries, 1 mismatch
//
// It exits 1 on any mismatch. With -trace it prints derivations instead.
func cmdEvolve(langDir string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("evolve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	traceMode := fs.Bool("trace", false, "print the derivation of each argument")
	fs.Usage = func() { fmt.Fprintln(stderr, "usage: lnova evolve [-trace <id|*proto>...]") }
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *traceMode != (fs.NArg() > 0) {
		fs.Usage()
		return 2
	}

	inv, err := phon.Load(filepath.Join(langDir, "phonology.yaml"))
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	rules, err := sound.Load(filepath.Join(langDir, "sound-changes.txt"), inv)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	lex, err := lexicon.Load(filepath.Join(langDir, "lexicon"))
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	if *traceMode {
		return traceWords(inv, rules, lex, fs.Args(), stdout, stderr)
	}

	bad := 0
	for _, e := range lex.Entries {
		segs, err := inv.Tokenize(strings.TrimPrefix(e.Proto, "*"))
		if err != nil {
			fmt.Fprintf(stdout, "%s:%d: %s: proto *%s: %v\n", e.File, e.LineOf("proto"), e.ID, e.Proto, err)
			bad++
			continue
		}
		out, _ := sound.Evolve(inv, rules, segs)
		if got := phon.Spell(out); got != e.Modern {
			fmt.Fprintf(stdout, "%s:%d: %s: *%s → %s, want %s\n", e.File, e.LineOf("modern"), e.ID, e.Proto, got, e.Modern)
			bad++
		}
	}
	noun := "mismatches"
	if bad == 1 {
		noun = "mismatch"
	}
	fmt.Fprintf(stdout, "%d entries, %d %s\n", len(lex.Entries), bad, noun)
	if bad > 0 {
		return 1
	}
	return 0
}

// traceWords prints a derivation for each argument, a lexicon id or a proto
// form written with a leading *. One line per rule that changed the word:
//
//	water: *esia → exa
//	  28   si > x / _V   esia → exa
//
// A lexicon entry whose result isn't its modern form gets a "want" line,
// and the exit code is 1.
func traceWords(inv *phon.Inventory, rules []sound.Rule, lex *lexicon.Lexicon, args []string, stdout, stderr io.Writer) int {
	byID := make(map[string]*lexicon.Entry, len(lex.Entries))
	for _, e := range lex.Entries {
		byID[e.ID] = e
	}
	status := 0
	for i, arg := range args {
		var entry *lexicon.Entry
		proto, label := strings.TrimPrefix(arg, "*"), ""
		if !strings.HasPrefix(arg, "*") {
			entry = byID[arg]
			if entry == nil {
				fmt.Fprintf(stderr, "lnova: no lexicon entry %q (write *%s for a proto form)\n", arg, arg)
				status = 1
				continue
			}
			proto, label = strings.TrimPrefix(entry.Proto, "*"), entry.ID+": "
		}
		segs, err := inv.Tokenize(proto)
		if err != nil {
			fmt.Fprintf(stderr, "lnova: %v\n", err)
			status = 1
			continue
		}
		out, steps := sound.Evolve(inv, rules, segs)
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		fmt.Fprintf(stdout, "%s*%s → %s\n", label, proto, phon.Spell(out))
		tw := tabwriter.NewWriter(stdout, 0, 0, 3, ' ', 0)
		for _, s := range steps {
			fmt.Fprintf(tw, "  %d\t%s\t%s → %s\n", s.Rule.Line, s.Rule, phon.Spell(s.Before), phon.Spell(s.After))
		}
		if len(steps) == 0 {
			fmt.Fprintln(tw, "  (no rule applies)")
		}
		tw.Flush()
		if entry != nil && phon.Spell(out) != entry.Modern {
			fmt.Fprintf(stdout, "  want %s\n", entry.Modern)
			status = 1
		}
	}
	return status
}

// cmdGen prints n new proto roots with their evolved modern form and IPA,
// then a summary. Candidates come from internal/gen and are kept only if
// they pass the proto phonotactics, aren't already in the lexicon (or the
// batch), and evolve into a legal modern word. A modern form that equals an
// existing word's is kept but marked:
//
//	*gaurdo   gaurdo   /ˈgau̯ɾ.do/
//	*esio     exo      /ˈe.ʃo/       = egg
//	20 words, seed 1234; rejected 41: 35 proto phonotactics, 4 existing, 2 modern phonotactics
//
// The seed is random unless given, and always printed, so a batch can be
// reproduced.
func cmdGen(langDir string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gen", flag.ContinueOnError)
	fs.SetOutput(stderr)
	n := fs.Int("n", 20, "number of words")
	seed := fs.Uint64("seed", 0, "random seed (default: random)")
	countMode := fs.Bool("count", false, "print the lexicon's template counts as a generator: block")
	fs.Usage = func() { fmt.Fprintln(stderr, "usage: lnova gen [-n 20] [-seed N] | -count") }
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 || *n < 1 {
		fs.Usage()
		return 2
	}
	seeded := false
	fs.Visit(func(f *flag.Flag) { seeded = seeded || f.Name == "seed" })
	if !seeded {
		*seed = rand.Uint64()
	}

	inv, err := phon.Load(filepath.Join(langDir, "phonology.yaml"))
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	rules, err := sound.Load(filepath.Join(langDir, "sound-changes.txt"), inv)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	lex, err := lexicon.Load(filepath.Join(langDir, "lexicon"))
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	var corpus [][]phon.Segment
	seen := map[string]bool{}        // proto forms: lexicon + this batch
	homophone := map[string]string{} // modern form → lexicon id
	for _, e := range lex.Entries {
		proto := strings.TrimPrefix(e.Proto, "*")
		segs, err := inv.Tokenize(proto)
		if err != nil {
			fmt.Fprintf(stderr, "%s:%d: %s: proto *%s: %v\n", e.File, e.LineOf("proto"), e.ID, e.Proto, err)
			return 1
		}
		corpus = append(corpus, segs)
		seen[proto] = true
		if _, ok := homophone[e.Modern]; !ok {
			homophone[e.Modern] = e.ID
		}
	}
	if *countMode {
		return printCounts(inv, corpus, stdout, stderr)
	}
	g, err := gen.New(inv, corpus, rand.New(rand.NewPCG(*seed, 0)))
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}

	var table bytes.Buffer
	tw := tabwriter.NewWriter(&table, 0, 0, 3, ' ', 0)
	words, badProto, existing, badModern := 0, 0, 0, 0
	for attempt := 0; words < *n && attempt < *n*1000; attempt++ {
		segs := g.Word()
		proto := phon.Spell(segs)
		switch {
		case len(inv.CheckProtoPhonotactics(segs)) > 0:
			badProto++
			continue
		case seen[proto]:
			existing++
			continue
		}
		out, _ := sound.Evolve(inv, rules, segs)
		if len(inv.CheckPhonotactics(out)) > 0 {
			badModern++
			continue
		}
		seen[proto] = true
		words++
		modern := phon.Spell(out)
		syls, _ := inv.Syllabify(out) // out has a vowel: the rules never delete one from a legal proto
		note := ""
		if id, ok := homophone[modern]; ok {
			note = "= " + id
		}
		fmt.Fprintf(tw, "*%s\t%s\t/%s/\t%s\n", proto, modern, phon.SyllableIPA(syls, phon.Stress(syls)), note)
	}
	tw.Flush()
	for line := range strings.Lines(table.String()) {
		// The note column is usually empty; don't leave its padding behind.
		fmt.Fprintln(stdout, strings.TrimRight(line, " \n"))
	}
	fmt.Fprintf(stdout, "%d words, seed %d; rejected %d: %d proto phonotactics, %d existing, %d modern phonotactics\n",
		words, *seed, badProto+existing+badModern, badProto, existing, badModern)
	if words < *n {
		fmt.Fprintf(stderr, "lnova: only %d of %d words after %d attempts\n", words, *n, *n*1000)
		return 1
	}
	return 0
}

// printCounts prints the syllable counts and templates of the lexicon's
// proto forms in the shape of phonology.yaml's proto: generator: block,
// biggest count first, ready to compare or paste:
//
//	syllables: {1: 10, 2: 72, 3: 19, 4: 3}
//	mono:    {CV: 5, CVV: 3, CVC: 2}
func printCounts(inv *phon.Inventory, corpus [][]phon.Segment, stdout, stderr io.Writer) int {
	c, err := gen.Count(inv, corpus)
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	var syl []string
	for _, n := range slices.Sorted(maps.Keys(c.Syllables)) {
		syl = append(syl, fmt.Sprintf("%d: %d", n, c.Syllables[n]))
	}
	fmt.Fprintf(stdout, "# counts in the %d roots\n", len(corpus))
	fmt.Fprintf(stdout, "syllables: {%s}\n", strings.Join(syl, ", "))
	for _, row := range []struct {
		name string
		m    map[string]int
	}{{"mono", c.Mono}, {"initial", c.Initial}, {"medial", c.Medial}, {"final", c.Final}} {
		ts := slices.SortedFunc(maps.Keys(row.m), func(a, b string) int {
			return cmp.Or(cmp.Compare(row.m[b], row.m[a]), cmp.Compare(a, b))
		})
		parts := make([]string, len(ts))
		for i, t := range ts {
			parts[i] = fmt.Sprintf("%s: %d", t, row.m[t])
		}
		fmt.Fprintf(stdout, "%-8s {%s}\n", row.name+":", strings.Join(parts, ", "))
	}
	return 0
}

// cmdInflect prints one form of a lexicon entry, or with no features every
// form in its paradigm, one per line:
//
//	PST.1SG   naraxu   nar-a-x-u   see-TH-PST-1SG   /naˈɾa.ʃu/
//
// A form the language lacks (hunu has no plural) is printed as "—".
func cmdInflect(langDir string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: lnova inflect <id> [features...]")
		return 2
	}
	inv, err := phon.Load(filepath.Join(langDir, "phonology.yaml"))
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	rules, err := morph.Load(filepath.Join(langDir, "morphology.yaml"), inv)
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	lex, err := lexicon.Load(filepath.Join(langDir, "lexicon"))
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	i := slices.IndexFunc(lex.Entries, func(e *lexicon.Entry) bool { return e.ID == args[0] })
	if i < 0 {
		fmt.Fprintf(stderr, "lnova: no lexicon entry %q\n", args[0])
		return 1
	}
	e := lex.Entries[i]

	var forms []*morph.Form
	var labels []string // for missing forms, which have no Label
	if len(args) > 1 {
		f, err := rules.Inflect(e, args[1:]...)
		if err != nil {
			fmt.Fprintf(stderr, "lnova: %v\n", err)
			return 1
		}
		forms, labels = []*morph.Form{&f}, []string{""}
	} else {
		t, err := rules.Paradigm(e)
		if err != nil {
			fmt.Fprintf(stderr, "lnova: %v\n", err)
			return 1
		}
		if t == nil {
			fmt.Fprintf(stderr, "lnova: %s (%s) doesn't inflect\n", e.ID, e.POS)
			return 1
		}
		for j, col := range t.Cols { // column by column: all of PRS, then PST…
			for _, row := range t.Rows {
				forms = append(forms, row.Cells[j])
				labels = append(labels, strings.Trim(row.Label+"."+col, "."))
			}
		}
	}

	tw := tabwriter.NewWriter(stdout, 0, 0, 3, ' ', 0)
	for j, f := range forms {
		if f == nil {
			fmt.Fprintf(tw, "%s\t—\n", labels[j])
			continue
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t/%s/\n", f.Label(), f.Word(), f.Segmented(), f.Gloss(), ipa(inv, f.Word()))
	}
	tw.Flush()
	return 0
}

// ipa returns a word's syllabified, stressed IPA, or "?" if it can't be
// syllabified (validate reports those).
func ipa(inv *phon.Inventory, word string) string {
	segs, err := inv.Tokenize(word)
	if err != nil {
		return "?"
	}
	syls, err := inv.Syllabify(segs)
	if err != nil {
		return "?"
	}
	return phon.SyllableIPA(syls, phon.Stress(syls))
}

// cmdAnalyze prints every analysis of each word, best first:
//
//	naraxu   see   verb I   PST.1SG   nar-a-x-u   see-TH-PST-1SG
//	pumaxu   ?pumar   verb I   PST.1SG   pum-a-x-u   ?-TH-PST-1SG   (guess)
//
// A word with no analysis prints —. Exits 1 if any word has none.
func cmdAnalyze(langDir string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: lnova analyze <word>...")
		return 2
	}
	inv, err := phon.Load(filepath.Join(langDir, "phonology.yaml"))
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	rules, err := morph.Load(filepath.Join(langDir, "morphology.yaml"), inv)
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	lex, err := lexicon.Load(filepath.Join(langDir, "lexicon"))
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	a := morph.NewAnalyzer(rules, lex)

	code := 0
	tw := tabwriter.NewWriter(stdout, 0, 0, 3, ' ', 0)
	for _, w := range args {
		as, err := a.Analyze(w)
		if err != nil {
			tw.Flush()
			fmt.Fprintf(stderr, "lnova: %v\n", err)
			code = 1
			continue
		}
		if len(as) == 0 {
			fmt.Fprintf(tw, "%s\t—\n", w)
			code = 1
		}
		for _, an := range as {
			lemma, guess := an.Lemma, ""
			if an.Guess() {
				lemma, guess = "?"+lemma, "\t(guess)"
			} else {
				lemma = an.Entry.ID
			}
			pos := strings.TrimSpace(an.POS + " " + an.Class)
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s%s\n", w, lemma, pos, cmp.Or(an.Form.Label(), "-"), an.Form.Segmented(), an.Form.Gloss(), guess)
		}
	}
	tw.Flush()
	return code
}

// cmdGloss prints a sentence's interlinear gloss, then remarks: readings
// the rules removed, guesses, grammar notes.
//
//	Lu   mena   nar-a       lu   trubo.
//	the  woman  see-TH.3SG  the  river
//
// The words of all arguments form one sentence, so quotes are optional.
// Exits 1 if any word has no analysis.
func cmdGloss(langDir string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: lnova gloss <sentence>")
		return 2
	}
	inv, err := phon.Load(filepath.Join(langDir, "phonology.yaml"))
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	rules, err := morph.Load(filepath.Join(langDir, "morphology.yaml"), inv)
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	lex, err := lexicon.Load(filepath.Join(langDir, "lexicon"))
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	res := gloss.New(morph.NewAnalyzer(rules, lex)).Gloss(strings.Join(args, " "))
	fmt.Fprint(stdout, res)
	for _, w := range res.Words {
		if w.Top() == nil {
			return 1
		}
	}
	return 0
}

// cmdParse shows how the translator reads an English sentence: each
// token with the tagger's Penn Treebank tag and the categories it can
// have, then every parse tree, best first. A tree's score is how many of
// its leaves have the category the tagger chose.
//
//	Didst/NNP:Aux  thou/NN:Pron  see/VB:V  ...
//	1 tree
//	7/7  (S (YN (Aux Didst) (NP (Pron thou)) (VP (V see) ...)))
func cmdParse(langDir string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: lnova parse <english sentence>")
		return 2
	}
	tr, err := translate.Load(langDir)
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	s, parses := tr.Parse(strings.Join(args, " "))
	toks := make([]string, len(s.Tokens))
	for i, t := range s.Tokens {
		toks[i] = t.Text + "/" + t.Tag + ":" + strings.Join(t.Cats(), "|")
	}
	fmt.Fprintln(stdout, strings.Join(toks, "  "))
	noun := "trees"
	if len(parses) == 1 {
		noun = "tree"
	}
	fmt.Fprintf(stdout, "%d %s\n", len(parses), noun)
	for _, p := range parses {
		fmt.Fprintf(stdout, "%d/%d  %s\n", p.Score, len(s.Tokens), p.Tree.Bracketed(s.Words()))
	}
	if len(parses) == 0 {
		return 1
	}
	return 0
}

// cmdTranslate prints the ranked translations, then the gloss of the best
// one:
//
//	1  Chu naraxis lu menas huro?
//	1 translation
//
//	Chu  nar-a-x-is      lu   mena-s      huro?
//	2SG  see-TH-PST-2SG  the  woman-POSS  dog
//
// Exits 1 if the sentence doesn't parse or has words not in the lexicon.
func cmdTranslate(langDir string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("translate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	n := fs.Int("n", 20, "show at most `N` translations (0 = all)")
	tree := fs.Bool("tree", false, "show each translation's English parse tree")
	usage := func() { fmt.Fprintln(stderr, "usage: lnova translate [-n 20] [-tree] <english sentence>") }
	fs.Usage = usage
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 || *n < 0 {
		usage()
		return 2
	}
	tr, err := translate.Load(langDir)
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	res, err := tr.Translate(strings.Join(fs.Args(), " "), *n)
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	code := 0
	for i, x := range res.Translations {
		fmt.Fprintf(stdout, "%d  %s\n", i+1, x.Text)
		if *tree {
			fmt.Fprintf(stdout, "   %s\n", x.Tree.Bracketed(res.Input.Words()))
		}
		for _, note := range x.Notes {
			fmt.Fprintf(stdout, "   ! %s\n", note)
		}
		if len(x.Unknown) > 0 {
			code = 1
		}
	}
	switch shown := len(res.Translations); {
	case res.Total == 1:
		fmt.Fprintln(stdout, "1 translation")
	case shown < res.Total:
		fmt.Fprintf(stdout, "%d of %d translations (-n 0 shows all)\n", shown, res.Total)
	default:
		fmt.Fprintf(stdout, "%d translations\n", res.Total)
	}
	if g := res.Translations[0].Gloss; g != nil {
		fmt.Fprintf(stdout, "\n%s", g)
	}
	return code
}

// cmdLLM translates with the local model and shows every attempt:
//
//	retrieved context: 41 entries, num_ctx 12288, qwen2.5:7b-instruct-q4_K_M
//	attempt 1  Lu mena nara lu blomp.  ✗  2.1s, 3080+60 tokens
//	   ! blomp: not a form of any dictionary word
//	attempt 2  Lu mena nara lu trubo.  ✓  0.8s, 3210+58 tokens
//
// With -tools, each attempt's tool calls come first, with their results:
//
//	→ inflect(eat, [PRS 3SG]) = bauge  (eat-TH.3SG)
//
// then the gloss of the last answer and, with -compare, the rule-based
// translator's verdict.
func cmdLLM(langDir string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("llm", flag.ContinueOnError)
	fs.SetOutput(stderr)
	mode := fs.String("ctx", llm.Retrieved, "context: `retrieved` (entries the sentence needs), full (every entry) or none (no entries)")
	tune := retrievalFlags(fs)
	withDense := denseFlags(fs)
	tools := fs.Bool("tools", false, "let the model call lookup, inflect, analyze and check")
	rounds := fs.Int("tool-rounds", 8, "with -tools, ask for the answer after `N` rounds of tool calls")
	model := fs.String("model", llm.DefaultModel, "Ollama `model`")
	attempts := fs.Int("attempts", 3, "give up after `N` attempts")
	url := fs.String("url", llm.DefaultURL, "`URL` of the llm/ service")
	compare := fs.Bool("compare", false, "compare with the rule-based translator")
	temp := fs.Float64("temp", 0, "sampling `temperature`; 0 = greedy (always the most likely token)")
	seed := fs.Int("seed", -1, "with -temp > 0, the sampler's `seed` (an eval run's seed replays it); -1 = random")
	usage := func() {
		fmt.Fprintln(stderr, "usage: lnova llm [-ctx retrieved|full|none] [-k 3] [-budget 2048] [-dense [-embed-model M] [-cache F] [-dims N]] [-tools] [-tool-rounds 8] [-model M] [-attempts 3] [-temp 0] [-seed S] [-url U] [-compare] <english sentence>")
	}
	fs.Usage = usage
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 || *attempts < 1 || *rounds < 1 || *temp < 0 || !slices.Contains([]string{llm.Retrieved, llm.Full, llm.None}, *mode) {
		usage()
		return 2
	}
	sentence := strings.Join(fs.Args(), " ")

	env, err := loadLLM(langDir, *url)
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	tune(env.builder.Retriever)
	withDense(context.Background(), langDir, *url, env, stderr)
	tr, lt := env.tr, env.translator

	ctx := context.Background()
	c, err := env.builder.Build(ctx, *mode, sentence)
	if err != nil {
		fmt.Fprintf(stderr, "lnova: warning: %v; the context has the lexical matches only\n", err)
	}
	opt := llm.Options{Model: *model, MaxAttempts: *attempts, MaxToolRounds: *rounds, Temperature: *temp}
	if *seed >= 0 {
		opt.Seed = seed
	}
	with := ""
	if *tools {
		opt.Tools = env.tools
		with = ", tools"
	}
	fmt.Fprintf(stdout, "%s context: %d entries%s, %s\n", *mode, len(c.Entries), with, *model)
	res, err := lt.Translate(ctx, sentence, c, opt)
	fmt.Fprintf(stdout, "meaning check: %s\n", res.Expect.Kind())
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	for i, a := range res.Attempts {
		mark := "✗"
		if a.Check.OK() {
			mark = "✓"
		}
		fmt.Fprintf(tw, "attempt %d\t%s\t%s\t%.1fs, %d+%d tokens\n", i+1, a.Answer.Translation, mark,
			a.Duration.Seconds(), a.PromptTokens, a.EvalTokens)
		for _, st := range a.Steps {
			for j, c := range st.Calls {
				fmt.Fprintf(tw, "   → %s = %s\n", c, toolSummary(st.Results[j].Content))
			}
		}
		for _, p := range a.Check.Problems {
			fmt.Fprintf(tw, "   ! %s\n", p)
		}
		for _, r := range a.Check.Remarks {
			fmt.Fprintf(tw, "   ~ %s\n", r)
		}
		if n := strings.TrimSpace(a.Answer.Notes); n != "" {
			fmt.Fprintf(tw, "   model: %s\n", n)
		}
	}
	tw.Flush()
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	last := res.Last()
	if last.Check.Gloss != nil {
		fmt.Fprintf(stdout, "\n%s", strings.Join(strings.SplitN(last.Check.Gloss.String(), "\n", 3)[:2], "\n")+"\n")
	}
	if *compare {
		fmt.Fprintf(stdout, "\nrule-based: %s\n", compareRuleBased(tr, sentence, last.Answer.Translation))
	}
	if !res.OK {
		fmt.Fprintf(stdout, "no valid translation in %d attempts\n", len(res.Attempts))
		return 1
	}
	return 0
}

// cmdCoin asks the model for proto roots for a concept the lexicon lacks
// and checks each one. The rounds (every candidate, ✓ or ✗ and why) and
// the totals go to stderr; the accepted roots go to stdout as lexicon
// YAML, the first ready to paste and the others commented out:
//
//   - id: lantern
//     proto: pattuca
//     modern: patuga
//     ...
//     # - id: lantern
//     #   proto: ...
//
// The lexicon is never written: you choose a root and paste it.
func cmdCoin(langDir string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("coin", flag.ContinueOnError)
	fs.SetOutput(stderr)
	pos := fs.String("pos", "", "part of speech (`noun`, verb, adjective, ...)")
	gender := fs.String("gender", "", "a noun's gender: `m` or f")
	def := fs.String("def", "", "the `definition` (default: the model writes one)")
	id := fs.String("id", "", "lexicon `id` (default: the English with _ for spaces); needed for a second sense of a word the lexicon has")
	n := fs.Int("n", 5, "candidates per round")
	want := fs.Int("want", 3, "stop after `N` accepted roots")
	rounds := fs.Int("rounds", 3, "give up after `N` rounds")
	model := fs.String("model", llm.DefaultModel, "Ollama `model`")
	temp := fs.Float64("temp", 0.7, "sampling `temperature`")
	seed := fs.Int("seed", -1, "the sampler's `seed`; -1 = random")
	url := fs.String("url", llm.DefaultURL, "`URL` of the llm/ service")
	related := fs.Int("related", 10, "choose `N` of the 30 example roots by meaning (0 = none)")
	margin := fs.Float64("related-margin", 0.05, "keep related roots only within `D` of the best score (0 = keep all N)")
	embedModel := fs.String("embed-model", llm.DefaultEmbedModel, "Ollama embedding `model` for -related")
	cache := fs.String("cache", "", "embedding cache `file`, as in lnova similar")
	usage := func() {
		fmt.Fprintln(stderr, "usage: lnova coin -pos P [-gender m|f] [-def D] [-id ID] [-n 5] [-want 3] [-rounds 3] [-related 10] [-related-margin 0.05] [-model M] [-temp 0.7] [-seed S] [-url U] <english>")
	}
	fs.Usage = usage
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 || *n < 1 || *want < 1 || *rounds < 1 || *temp < 0 || *related < 0 || *margin < 0 || !slices.Contains(lexicon.PartsOfSpeech, *pos) {
		usage()
		return 2
	}
	switch {
	case *pos == "noun" && *gender != "m" && *gender != "f":
		fmt.Fprintln(stderr, "lnova: a noun needs -gender m or f")
		return 2
	case *pos != "noun" && *gender != "":
		fmt.Fprintf(stderr, "lnova: -gender is for nouns, not a %s\n", *pos)
		return 2
	}
	concept := llm.Concept{English: strings.Join(fs.Args(), " "), POS: *pos, Gender: *gender, Definition: *def}

	inv, err := phon.Load(filepath.Join(langDir, "phonology.yaml"))
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	rules, err := sound.Load(filepath.Join(langDir, "sound-changes.txt"), inv)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	lex, err := lexicon.Load(filepath.Join(langDir, "lexicon"))
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	c := llm.NewCoiner(llm.NewClient(*url), inv, rules, lex)
	entryID := cmp.Or(*id, concept.ID())
	if have := c.Existing(concept); len(have) > 0 && *id == "" {
		for _, e := range have {
			fmt.Fprintf(stderr, "lnova: the lexicon has %q: %s (%s) at %s:%d\n", concept.English, e.ID, cmp.Or(e.Word, e.Modern), e.File, e.Line)
		}
		fmt.Fprintln(stderr, "for a second sense, give the new entry its own id: -id "+concept.ID()+"_2 (or a clearer name)")
		return 1
	}
	for _, e := range lex.Entries {
		if e.ID == entryID {
			fmt.Fprintf(stderr, "lnova: id %s is taken: %s:%d (%q); choose another with -id\n", entryID, e.File, e.Line, e.Gloss)
			return 1
		}
	}

	ctx := context.Background()
	if *related > 0 {
		// Without the embedding model coin still works, on examples
		// chosen without regard to meaning.
		idx, err := buildIndex(ctx, lex, *url, *embedModel, cachePath(langDir, *embedModel, *cache), 0, stderr)
		if err != nil {
			fmt.Fprintf(stderr, "lnova: no related examples: %v\n", err)
		}
		c.Index = idx
	}

	opt := llm.CoinOptions{Model: *model, N: *n, Want: *want, MaxRounds: *rounds, Temperature: *temp,
		Related: *related, RelatedMargin: *margin}
	if *seed >= 0 {
		opt.Seed = seed
	}
	res, err := c.Coin(ctx, concept, opt)
	if res.RelatedErr != nil {
		fmt.Fprintf(stderr, "lnova: no related examples: %v\n", res.RelatedErr)
	}
	if res.Related > 0 {
		var rel []string
		for _, r := range res.Examples[:res.Related] {
			rel = append(rel, fmt.Sprintf("%s (%s)", r.Modern, r.Gloss))
		}
		fmt.Fprintf(stderr, "related examples: %s\n", strings.Join(rel, ", "))
	}
	tw := tabwriter.NewWriter(stderr, 0, 0, 2, ' ', 0)
	total, first := 0, 0
	for i, rd := range res.Rounds {
		fmt.Fprintf(tw, "round %d\t%.1fs, %d+%d tokens\n", i+1, rd.Duration.Seconds(), rd.PromptTokens, rd.EvalTokens)
		for _, k := range rd.Checked {
			total++
			if k.OK() {
				if i == 0 {
					first++
				}
				fmt.Fprintf(tw, "  ✓ *%s → %s\t%s\n", k.Proto, k.Modern, k.Reason)
			} else {
				var msgs []string
				for _, r := range k.Rejections {
					msgs = append(msgs, r.Msg)
				}
				fmt.Fprintf(tw, "  ✗ *%s\t%s: %s\n", k.Proto, k.Rejections[0].Kind, strings.Join(msgs, "; "))
			}
			for _, r := range k.Remarks {
				fmt.Fprintf(tw, "    ~ %s\t\n", r)
			}
		}
	}
	tw.Flush()
	if total > 0 {
		fmt.Fprintf(stderr, "accepted %d of %d candidates (%d%%)", len(res.Accepted), total, len(res.Accepted)*100/total)
		if len(res.Rounds) > 0 {
			fmt.Fprintf(stderr, "; round 1: %d of %d", first, len(res.Rounds[0].Checked))
		}
		fmt.Fprintln(stderr)
		if kinds := res.RejectedKinds(); len(kinds) > 0 {
			counts := res.Rejected()
			var parts []string
			for _, k := range kinds {
				parts = append(parts, fmt.Sprintf("%s %d", k, counts[k]))
			}
			fmt.Fprintf(stderr, "rejected: %s\n", strings.Join(parts, ", "))
		}
	}
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	if len(res.Accepted) == 0 {
		fmt.Fprintf(stderr, "lnova: no root accepted in %d rounds\n", len(res.Rounds))
		return 1
	}

	def0 := cmp.Or(res.Definition(), "TODO")
	g := ""
	if concept.Gender != "" {
		g = ", " + concept.Gender
	}
	fmt.Fprintf(stdout, "# %q (%s%s), coined by %s: keep one, paste it into language/lexicon/, run lnova validate\n", concept.English, concept.POS, g, *model)
	for i, k := range res.Accepted {
		y := llm.EntryYAML(c.Entry(k, concept, entryID, def0, *model))
		if i > 0 {
			y = "# " + strings.ReplaceAll(strings.TrimSuffix(y, "\n"), "\n", "\n# ") + "\n"
		}
		fmt.Fprint(stdout, y)
	}
	return 0
}

// llmEnv is what an LLM translation needs, loaded from the language
// directory once: the rule-based translator (glosser, tagger, references),
// the lexicon, the context builder, the checker and the toolbox.
type llmEnv struct {
	tr         *translate.Translator
	lex        *lexicon.Lexicon
	builder    *llm.Builder
	translator *llm.Translator
	tools      *llm.Toolbox
}

func loadLLM(langDir, url string) (*llmEnv, error) {
	// Through a snapshot in .cache/: parsing a big lexicon takes seconds.
	lex, err := lexicon.LoadCached(filepath.Join(langDir, "lexicon"), filepath.Join(filepath.Dir(filepath.Clean(langDir)), ".cache"))
	if err != nil {
		return nil, err
	}
	tr, err := translate.LoadWith(langDir, lex)
	if err != nil {
		return nil, err
	}
	b, err := llm.NewBuilder(langDir, lex, tr.Tagger)
	if err != nil {
		return nil, err
	}
	ids := map[string]bool{}
	for _, e := range lex.Entries {
		ids[e.ID] = true
	}
	k := &llm.Checker{Glosser: tr.Glosser, Known: func(id string) bool { return ids[id] }, Ref: tr, Lex: lex}
	return &llmEnv{
		tr:         tr,
		lex:        lex,
		builder:    b,
		translator: &llm.Translator{Client: llm.NewClient(url), Checker: k},
		tools:      llm.NewToolbox(lex, tr.Rules, tr.Tagger, k, b.Retriever),
	}, nil
}

// retrievalFlags adds -k and -budget, the retrieved context's limits, to
// a command; the returned func sets them on the retriever.
func retrievalFlags(fs *flag.FlagSet) func(*retrieve.Retriever) {
	k := fs.Int("k", retrieve.DefaultK, "retrieved context: entries per English `word`")
	budget := fs.Int("budget", retrieve.DefaultBudget, "retrieved context: `tokens` for the words' entries together (0: no limit; a word's best entry always stays)")
	return func(r *retrieve.Retriever) { r.K, r.Budget = max(*k, 1), max(*budget, 0) }
}

// retrievalSettings names what a retrieved context depends on, for an
// eval's results: runs with other settings aren't comparable.
func retrievalSettings(r *retrieve.Retriever) string {
	dense := "-"
	if idx, ok := r.Dense.(*semantic.Index); ok {
		dense = fmt.Sprintf("%s/%d", idx.Model(), idx.Dims())
	}
	return fmt.Sprintf("k=%d budget=%d dense=%s", cmp.Or(r.K, retrieve.DefaultK), r.Budget, dense)
}

// denseFlags adds -dense, search by meaning as a retrieved context's
// third channel, with its embedding model, cache and dimensions. The
// returned func builds the index and adds it to the retriever; without
// the embedding model it says so and the context stays lexical.
func denseFlags(fs *flag.FlagSet) func(ctx context.Context, langDir, url string, env *llmEnv, stderr io.Writer) {
	dense := fs.Bool("dense", false, "retrieved context: also search by meaning (embeddings; the first run embeds the lexicon)")
	model := fs.String("embed-model", llm.DefaultEmbedModel, "with -dense: Ollama embedding `model`")
	cache := fs.String("cache", "", "with -dense: embedding cache `file`, as in lnova similar")
	dims := fs.Int("dims", 0, "with -dense: keep the vectors' first `N` numbers; 0: all")
	return func(ctx context.Context, langDir, url string, env *llmEnv, stderr io.Writer) {
		if !*dense {
			return
		}
		idx, err := buildIndex(ctx, env.lex, url, *model, cachePath(langDir, *model, *cache), *dims, stderr)
		if err != nil {
			fmt.Fprintf(stderr, "lnova: warning: no search by meaning: %v\n", err)
			return
		}
		env.builder.Retriever.Dense = idx
	}
}

// cmdEval runs the eval set (language/evals.yaml) through the model in
// each setup, several times, and prints passes per sentence and each
// setup's pass rate with a 95% interval:
//
//	sentence                   retrieved  retrieved+tools
//	The woman sees the river.  5/5        5/5
//	pass rate [95% CI]         52% [40–64]  80% [69–88]
//
// -out saves every record as JSON, after each translation; -resume
// continues an unfinished -out file. -baseline compares with a saved run.
// -list shows the sentences and their meaning check without the model.
// Ctrl-C (or kill, or a closed terminal) stops early and still reports
// and saves the runs done so far.
func cmdEval(langDir string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("eval", flag.ContinueOnError)
	fs.SetOutput(stderr)
	setupList := fs.String("setups", "retrieved,retrieved+tools,none+tools", "comma-separated `setups`: retrieved, full or none, optionally +tools")
	runs := fs.Int("runs", 5, "translate each sentence `N` times per setup (run i uses seed i)")
	temp := fs.Float64("temp", 0.7, "sampling `temperature`; 0 = greedy, and runs barely differ")
	model := fs.String("model", llm.DefaultModel, "Ollama `model`")
	attempts := fs.Int("attempts", 3, "attempts per translation")
	rounds := fs.Int("tool-rounds", 8, "tool rounds per attempt")
	timeout := fs.Duration("timeout", 5*time.Minute, "give up on one translation after `d` (counted as an error)")
	tag := fs.String("tag", "", "only the sentences with this `tag`")
	url := fs.String("url", llm.DefaultURL, "`URL` of the llm/ service")
	out := fs.String("out", "", "save the results as JSON to `file`")
	baseline := fs.String("baseline", "", "compare with saved results in `file`")
	show := fs.String("show", "", "report saved results from `file` (with -baseline: compare); don't run the model")
	list := fs.Bool("list", false, "list the sentences and their meaning check; don't run the model")
	tune := retrievalFlags(fs)
	withDense := denseFlags(fs)
	retrieval := fs.Bool("retrieval", false, "measure the retrieved contexts (recall of the entries a translation needs, size); don't run the model")
	goldDir := fs.String("gold", "", "with -retrieval: the language `dir` whose rule-based translations give the needed entries (default: -lang; for a fixture: language)")
	resume := fs.Bool("resume", false, "continue the unfinished run saved in -out: skip the translations it has")
	usage := func() {
		fmt.Fprintln(stderr, "usage: lnova eval [-setups retrieved,retrieved+tools,none+tools] [-runs 5] [-temp 0.7] [-model M] [-attempts 3] [-tool-rounds 8] [-timeout 5m] [-tag T] [-url U] [-out F [-resume]] [-baseline F] [-list] [-show F] [-k 3] [-budget 2048] [-dense] | -retrieval [-gold DIR]")
	}
	fs.Usage = usage
	if err := fs.Parse(args); err != nil {
		return 2
	}
	setups, err := eval.ParseSetups(*setupList)
	if fs.NArg() != 0 || err != nil || *runs < 1 || *attempts < 1 || *rounds < 1 || *temp < 0 {
		if err != nil {
			fmt.Fprintf(stderr, "lnova: %v\n", err)
		}
		usage()
		return 2
	}
	if *show != "" {
		return evalShow(*show, *baseline, stdout, stderr)
	}
	all, err := eval.Load(filepath.Join(langDir, "evals.yaml"))
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	cases := eval.WithTag(all, *tag)
	if len(cases) == 0 {
		fmt.Fprintf(stderr, "lnova: no sentence has the tag %q\n", *tag)
		return 1
	}
	env, err := loadLLM(langDir, *url)
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	tune(env.builder.Retriever)
	withDense(context.Background(), langDir, *url, env, stderr)
	if *list {
		return evalList(env, cases, stdout)
	}
	if *retrieval {
		return evalRetrieval(env, *goldDir, *url, cases, stdout, stderr)
	}

	var names []string
	for _, su := range setups {
		names = append(names, su.String())
	}
	meta := eval.Meta{
		Model: *model, Temperature: *temp, Runs: *runs, MaxAttempts: *attempts, MaxToolRounds: *rounds,
		Setups: names, Tag: *tag, Commit: gitDescribe(), Entries: len(env.lex.Entries), EvalSet: eval.Hash(cases),
		Retrieval: retrievalSettings(env.builder.Retriever),
	}
	opt := llm.Options{Model: *model, MaxAttempts: *attempts, MaxToolRounds: *rounds, Temperature: *temp}
	r := &eval.Runner{Translator: env.translator, Builder: env.builder, Tools: env.tools, Options: opt, Timeout: *timeout, Progress: stderr}
	if *resume {
		if *out == "" {
			fmt.Fprintln(stderr, "lnova: -resume needs -out, the file to continue")
			return 2
		}
		if old, err := eval.LoadResults(*out); err == nil {
			if why := eval.Mismatch(old.Meta, meta); why != "" {
				fmt.Fprintf(stderr, "lnova: can't resume %s: %s\n", *out, why)
				return 1
			}
			r.Done = old.Records
			fmt.Fprintf(stderr, "resuming %s: %d translations done\n", *out, len(old.Records))
		} else if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(stderr, "lnova: %v\n", err)
			return 1
		}
	}
	save := func(recs []eval.Record, interrupted bool) error {
		m := meta
		m.Date, m.Interrupted = time.Now().UTC().Truncate(time.Second), interrupted
		return eval.Save(*out, &eval.Results{Meta: m, Records: recs})
	}
	if *out != "" {
		// Saved after every translation, marked interrupted until the end:
		// a run that dies loses one translation, and -resume continues it.
		r.Saved = func(recs []eval.Record) {
			if err := save(recs, true); err != nil {
				fmt.Fprintf(stderr, "lnova: %v\n", err)
			}
		}
	}
	// Ctrl-C, kill and a closed terminal all stop the run cleanly.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	fmt.Fprintf(stderr, "%d sentences × %d setups × %d runs, %s, temperature %g\n", len(cases), len(setups), *runs, *model, *temp)
	recs, err := r.Run(ctx, cases, setups, *runs)
	interrupted := err != nil
	if interrupted {
		fmt.Fprintf(stderr, "lnova: stopped (%v) after %d translations\n", err, len(recs))
	}
	if len(recs) == 0 {
		return 1
	}
	eval.Report(stdout, recs)

	res := &eval.Results{Meta: meta, Records: recs}
	res.Meta.Date, res.Meta.Interrupted = time.Now().UTC().Truncate(time.Second), interrupted
	status := 0
	if *out != "" {
		if err := save(recs, interrupted); err != nil {
			fmt.Fprintf(stderr, "lnova: %v\n", err)
			status = 1
		} else {
			fmt.Fprintf(stderr, "saved %s\n", *out)
		}
	}
	if *baseline != "" {
		old, err := eval.LoadResults(*baseline)
		if err != nil {
			fmt.Fprintf(stderr, "lnova: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "\ncompared with %s (%s):\n", *baseline, old.Meta.Date.Format(time.DateOnly))
		eval.Diff(stdout, old, res)
	}
	if interrupted {
		return 1
	}
	return status
}

// evalShow reports saved results, and compares them with a baseline.
func evalShow(path, baseline string, stdout, stderr io.Writer) int {
	res, err := eval.LoadResults(path)
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	m := res.Meta
	state := ""
	if m.Interrupted {
		state = ", interrupted"
	}
	fmt.Fprintf(stdout, "%s: %s, %s, temperature %g, %d runs planned%s\n\n", path, m.Date.Format(time.DateOnly), m.Model, m.Temperature, m.Runs, state)
	eval.Report(stdout, res.Records)
	if baseline != "" {
		old, err := eval.LoadResults(baseline)
		if err != nil {
			fmt.Fprintf(stderr, "lnova: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "\ncompared with %s (%s):\n", baseline, old.Meta.Date.Format(time.DateOnly))
		eval.Diff(stdout, old, res)
	}
	return 0
}

// evalList prints each eval sentence with its tags and the meaning check
// it will get: frame (the rule-based translator's references) or lexical
// (content words only), and the content words that have no entry and so
// can't be checked at all.
func evalList(env *llmEnv, cases []eval.Case, stdout io.Writer) int {
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	unknown := 0
	for _, c := range cases {
		x := env.translator.Checker.Expect(c.English)
		fmt.Fprintf(tw, "%s\t%s\t%s\n", c.English, strings.Join(c.Tags, " "), x.Kind())
		if len(x.Refs) > 0 {
			fmt.Fprintf(tw, "   = %s\t\t\n", x.Refs[0].Text)
		}
		for _, w := range x.Words {
			fmt.Fprintf(tw, "   %s → %s\t\t\n", w.Word, strings.Join(w.IDs, ", "))
		}
		for _, w := range x.Unknown {
			fmt.Fprintf(tw, "   ! %s: not in the lexicon, not checked\t\t\n", w)
			unknown++
		}
	}
	tw.Flush()
	fmt.Fprintf(stdout, "%d sentences, %d unknown words\n", len(cases), unknown)
	if unknown > 0 {
		return 1
	}
	return 0
}

// evalRetrieval runs the retrieval eval (eval.Retrieval) on the
// retrieved contexts. The gold comes from goldDir's rule-based translator
// when given: on a fixture, the real lexicon's.
func evalRetrieval(env *llmEnv, goldDir, url string, cases []eval.Case, stdout, stderr io.Writer) int {
	ref := env
	if goldDir != "" {
		var err error
		if ref, err = loadLLM(goldDir, url); err != nil {
			fmt.Fprintf(stderr, "lnova: -gold: %v\n", err)
			return 1
		}
	}
	gold := func(s string) eval.Gold { return eval.GoldOf(ref.translator.Checker.Expect(s)) }
	build := func(s string) llm.Context {
		c, err := env.builder.Build(context.Background(), llm.Retrieved, s)
		if err != nil {
			fmt.Fprintf(stderr, "lnova: warning: %q: %v\n", s, err)
		}
		return c
	}
	eval.ReportRetrieval(stdout, eval.Retrieval(cases, gold, build))
	return 0
}

// gitDescribe names the code an eval ran with ("2cf8e80-dirty"), or "".
func gitDescribe() string {
	out, err := exec.Command("git", "describe", "--always", "--dirty").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// toolSummary shortens a tool result to one line: a check's problems, or
// the first line and how many more there are.
func toolSummary(s string) string {
	if strings.HasPrefix(s, "repeated call:") {
		return "↻ repeated call, the earlier result again"
	}
	if _, probs, ok := strings.Cut(s, "Problems:\n"); ok {
		return "✗ " + strings.ReplaceAll(strings.TrimPrefix(probs, "- "), "\n- ", "; ")
	}
	first, rest, _ := strings.Cut(s, "\n")
	if n := strings.Count(rest, "\n") + 1; rest != "" {
		return fmt.Sprintf("%s  (+%d more)", first, n)
	}
	return first
}

// compareRuleBased says how the rule-based translator's output relates to
// the model's answer: same as its best, among its translations, different,
// or no parse.
func compareRuleBased(tr *translate.Translator, sentence, answer string) string {
	res, err := tr.Translate(sentence, 0)
	if err != nil {
		return "no translation (" + err.Error() + ")"
	}
	norm := func(s string) string { return strings.ToLower(strings.TrimRight(strings.TrimSpace(s), ".?!")) }
	for i, x := range res.Translations {
		if norm(x.Text) == norm(answer) {
			if i == 0 {
				return x.Text + "  = same"
			}
			return fmt.Sprintf("%s  ≠ best, but the model's answer is its #%d of %d", res.Translations[0].Text, i+1, res.Total)
		}
	}
	return res.Translations[0].Text + "  ≠ differs"
}
