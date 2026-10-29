package llm

import (
	"cmp"
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/olehmushka/lingua-nova/internal/lexicon"
	"github.com/olehmushka/lingua-nova/internal/translate"
)

// The scale benchmarks run on the language directory in LNOVA_LANG
// (default: the real one), so the same numbers can be taken on a big
// fixture (lnova fixture):
//
//	LNOVA_LANG=$PWD/.cache/fixture-300k go test ./internal/llm -run '^$' -bench Scale

var scaleSentences = []string{
	"The woman sees the river.",
	"The men will drink the water.",
	"Didst thou see the woman's dog?",
	"The woman gave bread to me.",
}

var (
	scaleOnce sync.Once
	scaleB    *Builder
	scaleK    *Checker
	scaleErr  error
)

func scaleSetup(b *testing.B) (*Builder, *Checker) {
	b.Helper()
	scaleOnce.Do(func() {
		dir := cmp.Or(os.Getenv("LNOVA_LANG"), langDir)
		tr, err := translate.Load(dir)
		if err != nil {
			scaleErr = err
			return
		}
		lex, err := lexicon.Load(filepath.Join(dir, "lexicon"))
		if err != nil {
			scaleErr = err
			return
		}
		scaleB, scaleErr = NewBuilder(dir, lex, tr.Tagger)
		scaleK = &Checker{Glosser: tr.Glosser, Ref: tr, Lex: lex}
		b.Logf("%s: %d entries", dir, len(lex.Entries))
	})
	if scaleErr != nil {
		b.Fatal(scaleErr)
	}
	return scaleB, scaleK
}

func BenchmarkScaleLoad(b *testing.B) {
	dir := filepath.Join(cmp.Or(os.Getenv("LNOVA_LANG"), langDir), "lexicon")
	for b.Loop() {
		if _, err := lexicon.Load(dir); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkScaleRetrieved(b *testing.B) {
	bl, _ := scaleSetup(b)
	entries, size := 0, 0
	for b.Loop() {
		entries, size = 0, 0
		for _, s := range scaleSentences {
			c, err := bl.Retrieved(context.Background(), s)
			if err != nil {
				b.Fatal(err)
			}
			entries += len(c.Entries)
			size += c.Size()
		}
	}
	n := float64(len(scaleSentences))
	b.ReportMetric(float64(entries)/n, "entries/sentence")
	b.ReportMetric(float64(size)/3/n, "tokens/sentence")
}

func BenchmarkScaleExpect(b *testing.B) {
	_, k := scaleSetup(b)
	for b.Loop() {
		for _, s := range scaleSentences {
			k.Expect(s)
		}
	}
}
