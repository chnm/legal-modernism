package sources

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Every document type satisfies the interface, checked at compile time.
var (
	_ Document = (*CAPOpinion)(nil)
	_ Document = (*TreatisePage)(nil)
	_ Document = (*Doc)(nil)
)

func TestCAPOpinion(t *testing.T) {
	o := NewCAPOpinion(7, 3, "majority", "See 5 U.S. 137.")

	// The ids are carried as strings for the Document interface, in a form the
	// store can cast back to bigint, and the opinion belongs to its case.
	assert.Equal(t, "7", o.ID())
	assert.Equal(t, "3", o.ParentID())
	assert.True(t, o.HasParent())
	assert.Equal(t, "majority", o.Type)
	assert.Equal(t, "See 5 U.S. 137.", o.Text())
	assert.Equal(t, "Case <3>, opinion <7>", o.String())
	assert.Equal(t, []any{"cap_case", int64(3), "cap_opinion", int64(7)}, o.LogID())

	// Corrections and rewrites change the text in place, as on a page.
	o.CorrectOCR(NewOCRReplacer([]*OCRSubstitution{{Mistake: "U.S.", Correction: "US"}}))
	assert.Equal(t, "See 5 US 137.", o.Text())
	o.Rewrite(strings.ToUpper)
	assert.Equal(t, "SEE 5 US 137.", o.Text())

	// A nil replacer leaves the text alone.
	o.CorrectOCR(nil)
	assert.Equal(t, "SEE 5 US 137.", o.Text())
}

func TestTreatisePageLogID(t *testing.T) {
	p := NewTreatisePage("p12", "psmid1", "")
	assert.Equal(t, []any{"treatise_id", "psmid1", "page_id", "p12"}, p.LogID())
}
