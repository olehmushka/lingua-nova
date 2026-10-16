// Command lnova is the toolkit for the Rifatian language.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/olehmushka/lingua-nova/internal/gen"
	"github.com/olehmushka/lingua-nova/internal/lexicon"
	"github.com/olehmushka/lingua-nova/internal/phon"
	"github.com/olehmushka/lingua-nova/internal/sound"
)

const usage = `usage: lnova [-lang dir] <command> [args]

commands:
  ipa <word>...   show segments and IPA with syllables and stress
  validate        check the lexicon against the phonology and entry format
  rules           parse sound-changes.txt and list the rules with line numbers
  evolve          run every proto form through the sound changes and compare
                  with modern
  evolve -trace <id|*proto>...
                  show the derivation of lexicon entries or ad-hoc proto forms
  gen [-n 20] [-seed N]
                  generate new proto roots and show their modern forms
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

// cmdValidate checks every lexicon entry and prints problems like compiler
// errors, one per line, then a summary:
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
	probs := lexicon.Validate(lex, inv)
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
		if got := spell(out); got != e.Modern {
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
		fmt.Fprintf(stdout, "%s*%s → %s\n", label, proto, spell(out))
		tw := tabwriter.NewWriter(stdout, 0, 0, 3, ' ', 0)
		for _, s := range steps {
			fmt.Fprintf(tw, "  %d\t%s\t%s → %s\n", s.Rule.Line, s.Rule, spell(s.Before), spell(s.After))
		}
		if len(steps) == 0 {
			fmt.Fprintln(tw, "  (no rule applies)")
		}
		tw.Flush()
		if entry != nil && spell(out) != entry.Modern {
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
	fs.Usage = func() { fmt.Fprintln(stderr, "usage: lnova gen [-n 20] [-seed N]") }
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
		proto := spell(segs)
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
		modern := spell(out)
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

func spell(segs []phon.Segment) string { return strings.Join(phon.Spellings(segs), "") }
