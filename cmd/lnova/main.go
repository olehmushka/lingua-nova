// Command lnova is the toolkit for the Rifatian language.
package main

import (
	"bytes"
	"cmp"
	"context"
	"flag"
	"fmt"
	"io"
	"maps"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/olehmushka/lingua-nova/internal/gen"
	"github.com/olehmushka/lingua-nova/internal/gloss"
	"github.com/olehmushka/lingua-nova/internal/lexicon"
	"github.com/olehmushka/lingua-nova/internal/llm"
	"github.com/olehmushka/lingua-nova/internal/morph"
	"github.com/olehmushka/lingua-nova/internal/phon"
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
  llm [-ctx retrieved|full|none] [-tools] [-model M] [-attempts 3] [-compare] <english sentence>
                  translate with a local model (Ollama, via the llm/ service);
                  the glosser checks each answer and problems go back to the model;
                  -tools lets the model call lookup, inflect, analyze and check
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
	tools := fs.Bool("tools", false, "let the model call lookup, inflect, analyze and check")
	rounds := fs.Int("tool-rounds", 8, "with -tools, ask for the answer after `N` rounds of tool calls")
	model := fs.String("model", llm.DefaultModel, "Ollama `model`")
	attempts := fs.Int("attempts", 3, "give up after `N` attempts")
	url := fs.String("url", llm.DefaultURL, "`URL` of the llm/ service")
	compare := fs.Bool("compare", false, "compare with the rule-based translator")
	usage := func() {
		fmt.Fprintln(stderr, "usage: lnova llm [-ctx retrieved|full|none] [-tools] [-tool-rounds 8] [-model M] [-attempts 3] [-url U] [-compare] <english sentence>")
	}
	fs.Usage = usage
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 || *attempts < 1 || *rounds < 1 || !slices.Contains([]string{llm.Retrieved, llm.Full, llm.None}, *mode) {
		usage()
		return 2
	}
	sentence := strings.Join(fs.Args(), " ")

	tr, err := translate.Load(langDir)
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	lex, err := lexicon.Load(filepath.Join(langDir, "lexicon"))
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	b, err := llm.NewBuilder(langDir, lex, tr.Tagger)
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	ids := map[string]bool{}
	for _, e := range lex.Entries {
		ids[e.ID] = true
	}
	lt := &llm.Translator{
		Client:  llm.NewClient(*url),
		Checker: &llm.Checker{Glosser: tr.Glosser, Known: func(id string) bool { return ids[id] }, Ref: tr, Lex: lex},
	}

	c := b.Build(*mode, sentence)
	opt := llm.Options{Model: *model, MaxAttempts: *attempts, MaxToolRounds: *rounds}
	with := ""
	if *tools {
		opt.Tools = llm.NewToolbox(lex, tr.Rules, tr.Tagger, lt.Checker)
		with = ", tools"
	}
	fmt.Fprintf(stdout, "%s context: %d entries%s, %s\n", *mode, len(c.Entries), with, *model)
	res, err := lt.Translate(context.Background(), sentence, c, opt)
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

// toolSummary shortens a tool result to one line: a check's problems, or
// the first line and how many more there are.
func toolSummary(s string) string {
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
