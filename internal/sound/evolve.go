package sound

import "github.com/olehmushka/lingua-nova/internal/phon"

// Step is one rule that changed the word during a derivation.
type Step struct {
	Rule          Rule
	Before, After []phon.Segment
}

// Evolve runs the rules on word in order, each one on the output of the
// one before, and returns the final form and the derivation: the steps that
// changed something. Rules that didn't match leave no step.
//
// Order matters because rules interact. A rule feeds a later one when it
// creates that rule's input, and bleeds it when it removes it. In
// sound-changes.txt, degemination (tt > t) comes after voicing (t > d / V_V),
// so *patta keeps its t: when voicing runs, neither t is between vowels.
// Swap them and degemination feeds voicing: *patta → pata → pada.
func Evolve(inv *phon.Inventory, rules []Rule, word []phon.Segment) ([]phon.Segment, []Step) {
	var steps []Step
	for _, r := range rules {
		out, changed := r.Apply(inv, word)
		if changed {
			steps = append(steps, Step{Rule: r, Before: word, After: out})
		}
		word = out
	}
	return word, steps
}
