// Command lnova is the toolkit for the Rifatian language.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/olehmushka/lingua-nova/internal/lexicon"
	"github.com/olehmushka/lingua-nova/internal/phon"
	"github.com/olehmushka/lingua-nova/internal/sound"
)

const usage = `usage: lnova [-lang dir] <command> [args]

commands:
  ipa <word>...   show segments and IPA with syllables and stress
  validate        check the lexicon against the phonology and entry format
  rules           parse sound-changes.txt and list the rules with line numbers
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
