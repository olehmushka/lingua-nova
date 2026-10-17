package gloss

import (
	"regexp"
	"strings"
)

// Example is an interlinear example from a Markdown document such as
// grammar.md: a code block paragraph of segmented line, gloss line and
// "translation".
type Example struct {
	Segmented   string // Lu mena nar-a lu trubo.
	Gloss       string // the woman see-TH.3SG the river
	Translation string // The woman sees the river. (without the quotes)
}

// Sentence is the example's Rifatian sentence: the segmented line
// without morpheme breaks.
func (e Example) Sentence() string { return strings.ReplaceAll(e.Segmented, "-", "") }

var codeBlock = regexp.MustCompile("(?s)```\n(.*?)```")

// Examples finds every interlinear example in a Markdown document. Spacing
// inside the lines is normalized to single spaces.
func Examples(md string) []Example {
	var out []Example
	for _, block := range codeBlock.FindAllStringSubmatch(md, -1) {
		for _, para := range strings.Split(strings.TrimSpace(block[1]), "\n\n") {
			l := strings.Split(para, "\n")
			if len(l) != 3 || !strings.HasPrefix(l[2], `"`) {
				continue
			}
			out = append(out, Example{
				Segmented:   strings.Join(strings.Fields(l[0]), " "),
				Gloss:       strings.Join(strings.Fields(l[1]), " "),
				Translation: strings.Trim(strings.TrimSpace(l[2]), `"`),
			})
		}
	}
	return out
}
