package semantic

import (
	"context"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/olehmushka/lingua-nova/internal/lexicon"
)

// wide embeds texts as random 768-dimensional vectors, nomic's size.
type wide struct{ rng *rand.Rand }

func (w wide) Embed(_ context.Context, _ string, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		v := make([]float32, 768)
		for d := range v {
			v[d] = float32(w.rng.NormFloat64())
		}
		out[i] = v
	}
	return out, nil
}

// BenchmarkSearch300k is one query against 300k entries, nomic-sized:
//
//	go test ./internal/semantic -run '^$' -bench Search300k
func BenchmarkSearch300k(b *testing.B) {
	es := make([]*lexicon.Entry, 300000)
	for i := range es {
		es[i] = &lexicon.Entry{ID: fmt.Sprint(i), Gloss: fmt.Sprint("w", i), POS: "noun"}
	}
	emb := wide{rand.New(rand.NewPCG(1, 2))}
	idx, _, err := BuildWith(context.Background(), emb, "m", es, "", Options{Batch: 10000})
	if err != nil {
		b.Fatal(err)
	}
	v, _ := emb.Embed(context.Background(), Query, []string{"q"})
	q := normalize(v[0])
	for _, exact := range []bool{false, true} {
		x := *idx
		x.Exact = exact
		b.Run(fmt.Sprintf("exact=%v", exact), func(b *testing.B) {
			for b.Loop() {
				x.search(q, 20, nil)
			}
			b.ReportMetric(float64(x.Bytes())/1e6, "MB")
		})
	}
}
