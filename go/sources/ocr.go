package sources

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strings"
)

// OCRSubstitution represents an OCR correction that should be made to an input
// document via simple string substitution.
type OCRSubstitution struct {
	Mistake    string
	Correction string
}

// OCRReplacer applies a set of OCR corrections to a document's text in a single
// pass.
//
// The single pass is the point, not an optimization. The corrections used to be
// applied as one strings.ReplaceAll per rule over the whole text, so every rule
// re-scanned the output of every rule before it, and a shorter rule could
// consume the prefix of a longer one before the longer one was reached. Because
// the rows arrived in whatever order the table gave them, which rule won was
// unspecified, and the damage is in the corpus:
//
//	"Cusl" -> "Cush"     turns "Cuslr" into "Cushr", never "Cush"    (7 rows)
//	"N. ." -> "N. Y."    turns "N. .. I." into "N. Y.. I.", not "N. J. L."  (23 rows)
//	"Wvis" -> "Wis"      turns "Wvisc" into "Wisc", never "Wis"      (1,942 rows)
//	"Johns. Cl" -> "Johns. Ch"  turns "Johns. Cll" into "Johns. Chl" (5 rows)
//
// A strings.Replacer matches each position of the input once, never re-scans its
// own output, and compares the rules in argument order -- so with the rules
// sorted longest-mistake-first, the longest applicable correction wins at every
// position, and the result no longer depends on the order the rows arrived in
// (issue #285).
type OCRReplacer struct {
	r *strings.Replacer
}

// NewOCRReplacer builds a replacer from the corrections, longest mistake first
// so that no rule is shadowed by a shorter rule it begins with. Ties are broken
// on the mistake itself, so two runs over the same table always produce the same
// replacer.
func NewOCRReplacer(subs []*OCRSubstitution) *OCRReplacer {
	ordered := make([]*OCRSubstitution, 0, len(subs))
	for _, s := range subs {
		// A Replacer given an empty pattern would match at every position.
		if s == nil || s.Mistake == "" {
			continue
		}
		ordered = append(ordered, s)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if len(ordered[i].Mistake) != len(ordered[j].Mistake) {
			return len(ordered[i].Mistake) > len(ordered[j].Mistake)
		}
		return ordered[i].Mistake < ordered[j].Mistake
	})

	pairs := make([]string, 0, 2*len(ordered))
	for _, s := range ordered {
		pairs = append(pairs, s.Mistake, s.Correction)
	}
	return &OCRReplacer{r: strings.NewReplacer(pairs...)}
}

// Replace applies the corrections to s. A nil OCRReplacer leaves the text alone,
// which is what an empty corrections table amounts to.
func (o *OCRReplacer) Replace(s string) string {
	if o == nil || o.r == nil {
		return s
	}
	return o.r.Replace(s)
}

// OCRSubstitutionsFromCSV reads OCR substitutions from a CSV file.
func OCRSubstitutionsFromCSV(path string) ([]*OCRSubstitution, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("Error opening CSV: %w", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = 2
	inputs, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("Error reading CSV: %w", err)
	}

	output := make([]*OCRSubstitution, len(inputs))

	for i, input := range inputs {
		output[i] = &OCRSubstitution{Mistake: input[0], Correction: input[1]}
	}

	return output, nil
}
