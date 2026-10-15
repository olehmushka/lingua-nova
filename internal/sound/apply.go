package sound

import "github.com/olehmushka/lingua-nova/internal/phon"

// Apply runs the rule on one word and reports whether anything changed.
//
// Application is simultaneous: every match is found in the input word, as it
// was before the rule ran, and then all of them are rewritten. A change
// can't create or destroy a context for another match of the same rule.
// Matches don't overlap; the scan goes left to right and skips past each one.
// So a > e / e_ turns eaaa into eeaa. Iterative application, matching against
// the word as it is being rewritten, would spread the change: eeee.
func (r Rule) Apply(inv *phon.Inventory, word []phon.Segment) ([]phon.Segment, bool) {
	var out []phon.Segment
	changed := false
	for i := 0; i < len(word); {
		end := i + len(r.Target)
		if matchForward(inv, r.Target, word, i) &&
			matchBackward(inv, r.Left, word, i) &&
			matchForward(inv, r.Right, word, end) {
			out = append(out, r.Replacement...)
			changed = true
			i = end
			continue
		}
		out = append(out, word[i])
		i++
	}
	return out, changed
}

// matchForward reports whether items match word starting at start. A
// Boundary matches the end of the word.
func matchForward(inv *phon.Inventory, items []Item, word []phon.Segment, start int) bool {
	for j, it := range items {
		pos := start + j
		if it.Kind == Boundary {
			if pos != len(word) {
				return false
			}
			continue
		}
		if pos >= len(word) || !inv.Matches(it.Name, word[pos].Spell) {
			return false
		}
	}
	return true
}

// matchBackward reports whether items match word ending just before end. A
// Boundary matches the start of the word.
func matchBackward(inv *phon.Inventory, items []Item, word []phon.Segment, end int) bool {
	for j := range items {
		it := items[len(items)-1-j]
		pos := end - 1 - j
		if it.Kind == Boundary {
			if pos != -1 {
				return false
			}
			continue
		}
		if pos < 0 || !inv.Matches(it.Name, word[pos].Spell) {
			return false
		}
	}
	return true
}
