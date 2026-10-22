package semantic

import (
	"fmt"
	"os"

	"go.yaml.in/yaml/v3"

	"github.com/olehmushka/lingua-nova/internal/lexicon"
)

// EvalQuery is one query of a search eval (evals/similar.yaml) and the
// ids of the entries it should find.
type EvalQuery struct {
	Query  string   `yaml:"query"`
	Expect []string `yaml:"expect"`
}

// LoadEval reads an eval file and checks that every expected id is in
// the lexicon, so a renamed entry doesn't silently lower the score.
func LoadEval(path string, lex *lexicon.Lexicon) ([]EvalQuery, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var qs []EvalQuery
	if err := yaml.Unmarshal(data, &qs); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	ids := map[string]bool{}
	for _, e := range lex.Entries {
		ids[e.ID] = true
	}
	for _, q := range qs {
		if q.Query == "" || len(q.Expect) == 0 {
			return nil, fmt.Errorf("%s: every item needs a query and expect", path)
		}
		for _, id := range q.Expect {
			if !ids[id] {
				return nil, fmt.Errorf("%s: %q: no entry with id %s", path, q.Query, id)
			}
		}
	}
	return qs, nil
}

// Recall is the share of the expected ids among the hits (recall@k for k
// hits), and the expected ids that are missing.
func Recall(hits []Hit, expect []string) (float64, []string) {
	got := map[string]bool{}
	for _, h := range hits {
		got[h.Entry.ID] = true
	}
	var missing []string
	for _, id := range expect {
		if !got[id] {
			missing = append(missing, id)
		}
	}
	if len(expect) == 0 {
		return 0, nil
	}
	return float64(len(expect)-len(missing)) / float64(len(expect)), missing
}
