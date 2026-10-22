package main

import (
	"cmp"
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/olehmushka/lingua-nova/internal/lexicon"
	"github.com/olehmushka/lingua-nova/internal/llm"
	"github.com/olehmushka/lingua-nova/internal/semantic"
)

// buildIndex embeds the lexicon (or reads the vectors from the cache) and
// says on stderr when it had to embed anything, since that takes seconds.
func buildIndex(ctx context.Context, lex *lexicon.Lexicon, url, model, cache string, stderr io.Writer) (*semantic.Index, error) {
	idx, st, err := semantic.Build(ctx, llm.NewClient(url).Embedder(model), model, lex.Entries, cache)
	if err != nil && idx == nil {
		return nil, err
	}
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err) // the cache write failed; the index works
	}
	if st.Embedded > 0 {
		fmt.Fprintf(stderr, "embedded %d entries with %s (%d from the cache)\n", st.Embedded, model, st.Cached)
	}
	return idx, nil
}

// cachePath resolves a -cache flag: "" is the default file, "none" no cache.
func cachePath(langDir, model, flag string) string {
	switch flag {
	case "":
		return semantic.CachePath(langDir, model)
	case "none":
		return ""
	}
	return flag
}

// cmdSimilar searches the lexicon by meaning:
//
//	$ lnova similar the sea
//	0.71  maro  noun m  sea
//	0.66  exa   noun f  water
//
// -eval runs a query file instead and reports recall@k.
func cmdSimilar(langDir string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("similar", flag.ContinueOnError)
	fs.SetOutput(stderr)
	k := fs.Int("k", 10, "show the `N` closest entries")
	pos := fs.String("pos", "", "only this part of speech")
	model := fs.String("model", llm.DefaultEmbedModel, "Ollama embedding `model`")
	url := fs.String("url", llm.DefaultURL, "`URL` of the llm/ service")
	cache := fs.String("cache", "", "embedding cache `file` (default .cache/embeddings-<model>.json next to the language dir; \"none\" = no cache)")
	evalFile := fs.String("eval", "", "run the queries in `file` (evals/similar.yaml) and report recall@k")
	usage := func() {
		fmt.Fprintln(stderr, "usage: lnova similar [-k 10] [-pos P] [-model M] [-url U] [-cache F] <english> | -eval evals/similar.yaml")
	}
	fs.Usage = usage
	if err := fs.Parse(args); err != nil {
		return 2
	}
	query := strings.Join(fs.Args(), " ")
	if (query == "") == (*evalFile == "") || *k < 1 || *pos != "" && !slices.Contains(lexicon.PartsOfSpeech, *pos) {
		usage()
		return 2
	}
	*cache = cachePath(langDir, *model, *cache)

	lex, err := lexicon.Load(filepath.Join(langDir, "lexicon"))
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	var queries []semantic.EvalQuery
	if *evalFile != "" {
		if queries, err = semantic.LoadEval(*evalFile, lex); err != nil {
			fmt.Fprintf(stderr, "lnova: %v\n", err)
			return 1
		}
	}
	ctx := context.Background()
	idx, err := buildIndex(ctx, lex, *url, *model, *cache, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "lnova: %v\n", err)
		return 1
	}
	var keep func(*lexicon.Entry) bool
	if *pos != "" {
		keep = func(e *lexicon.Entry) bool { return e.POS == *pos }
	}

	if queries == nil {
		hits, err := idx.Search(ctx, query, *k, keep)
		if err != nil {
			fmt.Fprintf(stderr, "lnova: %v\n", err)
			return 1
		}
		tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
		for _, h := range hits {
			e := h.Entry
			fmt.Fprintf(tw, "%.2f\t%s\t%s\t%s\n", h.Score, cmp.Or(e.Word, e.Modern), strings.TrimSpace(e.POS+" "+e.Gender), e.Gloss)
		}
		tw.Flush()
		return 0
	}

	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "query\trecall@%d\tmissing\n", *k)
	var sum float64
	for _, q := range queries {
		hits, err := idx.Search(ctx, q.Query, *k, keep)
		if err != nil {
			tw.Flush()
			fmt.Fprintf(stderr, "lnova: %v\n", err)
			return 1
		}
		r, missing := semantic.Recall(hits, q.Expect)
		sum += r
		fmt.Fprintf(tw, "%s\t%d/%d\t%s\n", q.Query, len(q.Expect)-len(missing), len(q.Expect), strings.Join(missing, " "))
	}
	fmt.Fprintf(tw, "mean\t%.0f%%\t\n", sum*100/float64(len(queries)))
	tw.Flush()
	return 0
}
