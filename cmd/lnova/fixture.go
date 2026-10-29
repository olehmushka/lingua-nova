package main

import (
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"

	"github.com/olehmushka/lingua-nova/internal/fixture"
	"github.com/olehmushka/lingua-nova/internal/gen"
	"github.com/olehmushka/lingua-nova/internal/lexicon"
	"github.com/olehmushka/lingua-nova/internal/llm"
	"github.com/olehmushka/lingua-nova/internal/phon"
	"github.com/olehmushka/lingua-nova/internal/sound"
)

// fixtureFileSize is the number of synthetic entries per YAML file. The
// files are named aa-… so they load before the real ones: where ranking
// falls back on lexicon order (equal glosses), the synthetic entries win,
// and recall on the fixture is the worst case, not flattered.
const fixtureFileSize = 10000

// cmdFixture writes a copy of the language directory whose lexicon has
// the real entries plus -n synthetic ones (fixture.Entries), for
// measuring retrieval at scale: lnova -lang <out> eval -retrieval.
// The roots come from the generator and the sound changes, like lnova gen,
// but aren't checked for homophones: the fixture is never validated.
func cmdFixture(langDir string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("fixture", flag.ContinueOnError)
	fs.SetOutput(stderr)
	n := fs.Int("n", 300000, "synthetic entries")
	senses := fs.Float64("senses", 0.1, "share of them that are new senses of a real word (water.cold)")
	homonyms := fs.Float64("homonyms", 0.01, "share that have exactly a real word's gloss (water), with another meaning")
	seed := fs.Uint64("seed", 1, "random seed")
	out := fs.String("out", ".cache/fixture-300k", "output language directory (must not exist)")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: lnova fixture [-n 300000] [-senses 0.1] [-homonyms 0.01] [-seed 1] [-out .cache/fixture-300k]")
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 || *n < 1 || *senses < 0 || *homonyms < 0 || *senses+*homonyms > 1 {
		fs.Usage()
		return 2
	}
	if _, err := os.Stat(*out); err == nil {
		fmt.Fprintf(stderr, "lnova: %s exists; remove it first\n", *out)
		return 1
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
	for _, e := range lex.Entries {
		segs, err := inv.Tokenize(strings.TrimPrefix(e.Proto, "*"))
		if err != nil {
			fmt.Fprintf(stderr, "%s:%d: %s: %v\n", e.File, e.LineOf("proto"), e.ID, err)
			return 1
		}
		corpus = append(corpus, segs)
	}
	rng := rand.New(rand.NewPCG(*seed, 0))
	g, err := gen.New(inv, corpus, rng)
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	endings := []string{"ar", "er", "ir"}
	form := func(pos string) (string, string) {
		for {
			segs := g.Word()
			if pos == "verb" {
				end, err := inv.Tokenize(endings[rng.IntN(len(endings))])
				if err != nil {
					panic(err) // the endings are in every inventory
				}
				segs = append(segs, end...)
			}
			if len(inv.CheckProtoPhonotactics(segs)) > 0 {
				continue
			}
			evolved, _ := sound.Evolve(inv, rules, segs)
			modern := phon.Spell(evolved)
			if len(inv.CheckPhonotactics(evolved)) > 0 || pos == "verb" && !lexicon.HasVerbEnding(modern) {
				continue
			}
			return phon.Spell(segs), modern
		}
	}
	entries := fixture.Entries(lex.Entries, form, fixture.Options{N: *n, Senses: *senses, Homonyms: *homonyms}, rng)

	if err := os.CopyFS(*out, os.DirFS(langDir)); err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	for i := 0; i < len(entries); i += fixtureFileSize {
		var b strings.Builder
		fmt.Fprintf(&b, "# Synthetic entries for scale tests (lnova fixture -n %d -senses %g -homonyms %g -seed %d).\n",
			*n, *senses, *homonyms, *seed)
		for _, e := range entries[i:min(i+fixtureFileSize, len(entries))] {
			b.WriteString("\n" + llm.EntryYAML(e))
		}
		name := filepath.Join(*out, "lexicon", fmt.Sprintf("aa-synthetic-%03d.yaml", i/fixtureFileSize))
		if err := os.WriteFile(name, []byte(b.String()), 0o644); err != nil {
			fmt.Fprintf(stderr, "lnova: %v\n", err)
			return 1
		}
	}
	fmt.Fprintf(stdout, "%s: %d real + %d synthetic entries\n", *out, len(lex.Entries), len(entries))
	return 0
}
