package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
	"time"
)

// Rate is k passes out of n runs, with a 95% confidence interval.
type Rate struct {
	K, N   int
	Lo, Hi float64
}

// z for a two-sided 95% interval.
const z = 1.959964

// Wilson is the Wilson score interval for k successes in n trials.
//
// The textbook interval p ± z·√(p(1−p)/n) (the normal approximation)
// breaks down for small n or p near 0 or 1, which is exactly an eval's
// situation: 5/5 gives p = 1 and a width of 0, as if 5 runs proved a
// sentence always passes. Wilson's interval instead asks which true rates
// p would make k/n unsurprising, and solves that for p. It stays inside
// [0, 1] and is never zero-width: 5/5 → [57%, 100%].
func Wilson(k, n int) Rate {
	r := Rate{K: k, N: n}
	if n == 0 {
		r.Hi = 1
		return r
	}
	p, nf, z2 := float64(k)/float64(n), float64(n), z*z
	center := (p + z2/(2*nf)) / (1 + z2/nf)
	half := z / (1 + z2/nf) * math.Sqrt(p*(1-p)/nf+z2/(4*nf*nf))
	r.Lo, r.Hi = max(0, center-half), min(1, center+half)
	return r
}

func (r Rate) P() float64 {
	if r.N == 0 {
		return 0
	}
	return float64(r.K) / float64(r.N)
}

func (r Rate) String() string {
	return fmt.Sprintf("%.0f%% [%.0f–%.0f]", 100*r.P(), 100*r.Lo, 100*r.Hi)
}

// Overlaps says whether two intervals overlap. If they do, the difference
// may be noise. (Overlap doesn't prove "no difference": it's a cautious
// rule of thumb, not a test.)
func (r Rate) Overlaps(o Rate) bool { return r.Lo <= o.Hi && o.Lo <= r.Hi }

// Summary is one setup's numbers.
type Summary struct {
	Setup     string
	Pass      Rate // passed within the attempts
	First     Rate // passed on the first attempt, before any feedback
	Errors    int  // timeouts and service failures (counted as not passed)
	MedianMS  int64
	MedianTok int // prompt tokens per translation
	ToolCalls float64
	Repeats   int
}

// Summarize groups records by setup, in the order the setups first appear.
func Summarize(recs []Record) []Summary {
	var out []Summary
	for _, name := range setupsOf(recs) {
		s := Summary{Setup: name}
		var ms []int64
		var tok []int
		calls := 0
		for _, r := range recs {
			if r.Setup != name {
				continue
			}
			s.Pass.N++
			if r.OK {
				s.Pass.K++
			}
			if r.FirstOK {
				s.First.K++
			}
			if r.Error != "" {
				s.Errors++
			}
			ms = append(ms, r.DurationMS)
			tok = append(tok, r.PromptTokens)
			calls += r.ToolCalls
			s.Repeats += r.Repeats
		}
		s.Pass = Wilson(s.Pass.K, s.Pass.N)
		s.First = Wilson(s.First.K, s.Pass.N)
		s.MedianMS, s.MedianTok = median(ms), median(tok)
		s.ToolCalls = float64(calls) / float64(s.Pass.N)
		out = append(out, s)
	}
	return out
}

func median[T int | int64](xs []T) T {
	if len(xs) == 0 {
		return 0
	}
	xs = slices.Clone(xs)
	slices.Sort(xs)
	return xs[len(xs)/2]
}

func setupsOf(recs []Record) []string {
	var out []string
	for _, r := range recs {
		if !slices.Contains(out, r.Setup) {
			out = append(out, r.Setup)
		}
	}
	return out
}

func sentencesOf(recs []Record) []string {
	var out []string
	for _, r := range recs {
		if !slices.Contains(out, r.English) {
			out = append(out, r.English)
		}
	}
	return out
}

// cell counts the passes of one sentence in one setup.
func cell(recs []Record, english, setup string) (k, n int) {
	for _, r := range recs {
		if r.English == english && r.Setup == setup {
			n++
			if r.OK {
				k++
			}
		}
	}
	return k, n
}

// Report prints a sentence × setup table of passes, then each setup's
// summary.
func Report(w io.Writer, recs []Record) {
	setups := setupsOf(recs)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprint(tw, "sentence")
	for _, s := range setups {
		fmt.Fprintf(tw, "\t%s", s)
	}
	fmt.Fprintln(tw)
	for _, en := range sentencesOf(recs) {
		fmt.Fprint(tw, en)
		for _, s := range setups {
			k, n := cell(recs, en, s)
			fmt.Fprintf(tw, "\t%d/%d", k, n)
		}
		fmt.Fprintln(tw)
	}
	sums := Summarize(recs)
	row := func(label string, f func(Summary) string) {
		fmt.Fprint(tw, label)
		for _, s := range sums {
			fmt.Fprintf(tw, "\t%s", f(s))
		}
		fmt.Fprintln(tw)
	}
	fmt.Fprintln(tw)
	row("pass rate [95% CI]", func(s Summary) string { return s.Pass.String() })
	row("first attempt", func(s Summary) string { return s.First.String() })
	row("errors (timeouts…)", func(s Summary) string { return fmt.Sprint(s.Errors) })
	row("median time", func(s Summary) string { return fmt.Sprintf("%.1fs", float64(s.MedianMS)/1000) })
	row("median prompt tokens", func(s Summary) string { return fmt.Sprintf("%.1fK", float64(s.MedianTok)/1000) })
	row("tool calls / translation", func(s Summary) string {
		return fmt.Sprintf("%.1f (%d repeated)", s.ToolCalls, s.Repeats)
	})
	tw.Flush()
}

// Results is what an eval saves: how it was run, and every record.
type Results struct {
	Meta    Meta     `json:"meta"`
	Records []Record `json:"records"`
}

// Meta is what a later run must match to be comparable.
type Meta struct {
	Date          time.Time `json:"date"`
	Model         string    `json:"model"`
	Temperature   float64   `json:"temperature"`
	Runs          int       `json:"runs"`
	MaxAttempts   int       `json:"max_attempts"`
	MaxToolRounds int       `json:"max_tool_rounds"`
	Setups        []string  `json:"setups"`
	Tag           string    `json:"tag,omitempty"`
	Commit        string    `json:"commit,omitempty"` // git describe of the code
	Entries       int       `json:"entries"`          // lexicon size
	EvalSet       string    `json:"eval_set"`         // hash of the sentences
	Interrupted   bool      `json:"interrupted,omitempty"`
}

// Mismatch says why results run as old can't be continued as new, or "".
// Everything that changes what a translation is must match; the commit,
// lexicon size, run count and setups may differ (more runs, a setup
// added).
func Mismatch(old, new Meta) string {
	var out []string
	for _, d := range []struct {
		what     string
		old, new any
	}{
		{"model", old.Model, new.Model},
		{"temperature", old.Temperature, new.Temperature},
		{"attempts", old.MaxAttempts, new.MaxAttempts},
		{"tool rounds", old.MaxToolRounds, new.MaxToolRounds},
		{"eval set", old.EvalSet, new.EvalSet},
	} {
		if d.old != d.new {
			out = append(out, fmt.Sprintf("%s %v ≠ %v", d.what, d.old, d.new))
		}
	}
	return strings.Join(out, ", ")
}

// Hash identifies an eval set: the same sentences in the same order give
// the same hash, whatever the tags and comments.
func Hash(cases []Case) string {
	h := sha256.New()
	for _, c := range cases {
		fmt.Fprintln(h, c.English)
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

func Save(path string, r *Results) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func LoadResults(path string) (*Results, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r Results
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &r, nil
}

// Diff prints how new compares with old: what differs in how they were
// run, each setup's pass rate before and after, and the sentences whose
// pass count changed.
func Diff(w io.Writer, old, new *Results) {
	om, nm := old.Meta, new.Meta
	for _, d := range []struct {
		what     string
		old, new any
	}{
		{"model", om.Model, nm.Model},
		{"temperature", om.Temperature, nm.Temperature},
		{"runs", om.Runs, nm.Runs},
		{"attempts", om.MaxAttempts, nm.MaxAttempts},
		{"tool rounds", om.MaxToolRounds, nm.MaxToolRounds},
		{"lexicon entries", om.Entries, nm.Entries},
		{"eval set", om.EvalSet, nm.EvalSet},
		{"code", om.Commit, nm.Commit},
	} {
		if fmt.Sprint(d.old) != fmt.Sprint(d.new) {
			fmt.Fprintf(w, "differs: %s %v → %v\n", d.what, d.old, d.new)
		}
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "\nsetup\tbefore\tafter\t")
	olds := Summarize(old.Records)
	for _, ns := range Summarize(new.Records) {
		i := slices.IndexFunc(olds, func(s Summary) bool { return s.Setup == ns.Setup })
		if i < 0 {
			fmt.Fprintf(tw, "%s\t–\t%s\tnew setup\n", ns.Setup, ns.Pass)
			continue
		}
		was := olds[i]
		verdict := "intervals overlap: could be noise"
		if !was.Pass.Overlaps(ns.Pass) {
			verdict = "better"
			if ns.Pass.P() < was.Pass.P() {
				verdict = "worse"
			}
			verdict += ": intervals don't overlap"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", ns.Setup, was.Pass, ns.Pass, verdict)
	}
	tw.Flush()

	var changed [][2]string
	for _, s := range setupsOf(new.Records) {
		for _, en := range sentencesOf(new.Records) {
			ok, on := cell(old.Records, en, s)
			nk, nn := cell(new.Records, en, s)
			if on == 0 || nn == 0 || ok*nn == nk*on {
				continue
			}
			arrow := "↑"
			if nk*on < ok*nn {
				arrow = "↓"
			}
			changed = append(changed, [2]string{fmt.Sprintf("%s %s", arrow, s), fmt.Sprintf("%s  %d/%d → %d/%d", en, ok, on, nk, nn)})
		}
	}
	if len(changed) == 0 {
		fmt.Fprintln(w, "\nno sentence changed its pass count")
		return
	}
	fmt.Fprintln(w, "\nchanged:")
	tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, c := range changed {
		fmt.Fprintf(tw, "  %s\t%s\n", c[0], c[1])
	}
	tw.Flush()
}
