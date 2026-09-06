package sources

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var input = "This is a stringy with the same stringy error twice, plus a teh."
var want = "This is a string with the same string error twice, plus a the."

var subS = &OCRSubstitution{Mistake: "stringy", Correction: "string"}
var subT = &OCRSubstitution{Mistake: "teh", Correction: "the"}
var subs = []*OCRSubstitution{subS, subT}

func TestOCRReplacer_Replace(t *testing.T) {
	got := NewOCRReplacer(subs).Replace(input)
	assert.Equal(t, want, got, "Corrects multiple errors")
}

func TestDoc_CorrectOCR(t *testing.T) {
	doc := NewDoc("ID", input)
	doc.CorrectOCR(NewOCRReplacer(subs))
	assert.Equal(t, want, doc.Text(), "Can be applied to a document")
}

// TestOCRReplacer_LongestMistakeWins covers issue #285. Several corrections
// begin with another one, and applying them as successive ReplaceAll passes let
// the shorter rule consume the prefix of the longer one and leave a spelling no
// rule then repairs. Each case here is a real pair from
// legalhist.ocr_corrections, with the artifact it produced still in
// citations_unlinked.
func TestOCRReplacer_LongestMistakeWins(t *testing.T) {
	corrections := []*OCRSubstitution{
		{Mistake: "Cusl", Correction: "Cush"},
		{Mistake: "Cuslr", Correction: "Cush"},
		{Mistake: "N. .", Correction: "N. Y."},
		{Mistake: "N. .. I.", Correction: "N. J. L."},
		{Mistake: "Wvis", Correction: "Wis"},
		{Mistake: "Wvisc", Correction: "Wis"},
		{Mistake: "Johns. Cl", Correction: "Johns. Ch"},
		{Mistake: "Johns. Cll", Correction: "Johns. Ch"},
	}
	r := NewOCRReplacer(corrections)

	tests := []struct {
		name string
		text string
		want string
		// artifact is what the successive-ReplaceAll approach produced, and how
		// many rows carry it in citations_unlinked today.
		artifact string
	}{
		{"Cuslr", "see Cuslr. 12", "see Cush. 12", "Cushr."},
		{"N. .. I.", "see N. .. I. 12", "see N. J. L. 12", "N. Y.. I."},
		{"Wvisc", "see Wvisc. 12", "see Wis. 12", "Wisc."},
		{"Johns. Cll", "see Johns. Cll 12", "see Johns. Ch 12", "Johns. Chl"},
		// The shorter rule still applies where the longer one does not match.
		{"Cusl alone", "see Cusl. 12", "see Cush. 12", ""},
		{"N. . alone", "see N. . 12", "see N. Y. 12", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.Replace(tt.text)
			assert.Equal(t, tt.want, got)
			if tt.artifact != "" {
				assert.NotContains(t, got, tt.artifact,
					"the cascade artifact must not survive")
			}
		})
	}
}

// TestOCRReplacer_DoesNotRescanItsOwnOutput is the other half of #285: a
// correction's output must not be fed back through the rules. "Wise" -> "Wis"
// and "Vis" -> "Wis" are both real rules; under successive passes "Wise" could
// be rewritten and then matched again by a rule that fires on the result.
func TestOCRReplacer_DoesNotRescanItsOwnOutput(t *testing.T) {
	r := NewOCRReplacer([]*OCRSubstitution{
		{Mistake: "ab", Correction: "bc"},
		{Mistake: "bc", Correction: "cd"},
	})
	assert.Equal(t, "bc", r.Replace("ab"), "the output of one rule is not input to another")
}

// TestOCRReplacer_OrderIndependent checks that the replacer does not depend on
// the order the rows arrived in, which is what the database gave no guarantee
// about.
func TestOCRReplacer_OrderIndependent(t *testing.T) {
	forward := []*OCRSubstitution{
		{Mistake: "Cusl", Correction: "Cush"},
		{Mistake: "Cuslr", Correction: "Cush"},
	}
	reversed := []*OCRSubstitution{forward[1], forward[0]}
	assert.Equal(t,
		NewOCRReplacer(forward).Replace("Cuslr. 12"),
		NewOCRReplacer(reversed).Replace("Cuslr. 12"))
}

// TestOCRReplacer_Empty covers the degenerate inputs: no corrections at all, a
// nil replacer, and a row with an empty mistake, which would otherwise match at
// every position.
func TestOCRReplacer_Empty(t *testing.T) {
	assert.Equal(t, "untouched", NewOCRReplacer(nil).Replace("untouched"))

	var nilReplacer *OCRReplacer
	assert.Equal(t, "untouched", nilReplacer.Replace("untouched"))

	r := NewOCRReplacer([]*OCRSubstitution{
		{Mistake: "", Correction: "XXX"},
		{Mistake: "teh", Correction: "the"},
	})
	assert.Equal(t, "the end", r.Replace("teh end"))
}
