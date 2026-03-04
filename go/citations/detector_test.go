package citations

import (
	"fmt"
	"testing"

	"github.com/lmullen/legal-modernism/go/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetector_Detect(t *testing.T) {
	text := `
	This is a doc with 6 N. Y. Sup. Ct. 69 citations.
	This is a doc with citations (2 Kans. 416).
	This is a doc 71 N. C. 297 with citations.
	This is a doc with 71 N.C. 297 citations
	This doc has 39 N. Y. 436, 438 two page numbers.
	This doc has 39 N. Y. 436-438 a page range.
	This doc has 6 Watts & S. 314 as a citation.
	This doc has a two character reporter 43 Md. 295 as a citation.
	This doc has parentheses 1 C. R. (N. S.) 413 as a citation.
	This doc has something that looks like a citation 6 Ex parte Wray, 30 but isn't.
	This doc has something that looks like a citation 6 Rex v. Osborn, 30 but isn't.
	This has a citation 30 Missis. 673 that is pretty clear.
	6 Ex parte Wray, 30 Missis. 673; Street v. Tle State, 43 Missis. 1.
	This has an edition 1 Leach, 4th ed. 484 associated with it.
	This has an edition 25 Biznes, 3rd ed. 484 associated with it.
	Citing 1 How. Sp. T. Rep. 114 is an interesting case.
	`
	expected := []string{
		"6 N. Y. Sup. Ct. 69",
		"2 Kans. 416",
		"71 N. C. 297",
		"71 N.C. 297",
		"39 N. Y. 436",
		"39 N. Y. 436",
		"6 Watts & S. 314",
		"43 Md. 295",
		"1 C. R. (N. S.) 413",
		"30 Missis. 673",
		"30 Missis. 673",
		"43 Missis. 1",
		"1 Leach, 4th ed. 484",
		"25 Biznes, 3rd ed. 484",
		"1 How. Sp. T. Rep. 114",
	}

	doc := sources.NewDoc("test", text)
	citations := GenericDetector.Detect(doc)

	require.Equal(t, len(expected), len(citations))

	for i := range expected {
		assert.Equal(t, expected[i], citations[i].CleanCite(), fmt.Sprintf("Citation %v", i))
	}
}

func TestSingleVolDetector_Detect(t *testing.T) {
	text := `
	Lorem ipsum dolor sit amet, consectetur adipiscing elit. The court's ruling in
	Busb. Eq. Rep. 234 established the principles of equity jurisdiction. Nam vel
	justo sed felis aliquam malesuada. See also Freem Chy 876, which extended 
	those principles to questions of performance. Nulla ut finibus dui. Hob. 423 
	remains the leading authority on common law pleading. Aliquam purus tellus. 
	Compare with Baldw. 125 for the federal perspective. Ut pharetra augue nulla. 
	The state court first addressed this question in Cheves Eq. 12 before the 
	federal rule was established. Praesent ornare massa quis augue egestas; the 
	same point was reinforced in Busb. Eq. Rep. 234.
	
	Lorem ipsum dolor sit amet, consectetur adipiscing elit. The court's ruling 
	in Toth., 234 established the principles of equity jurisdiction. Nam vel justo
	sed felis aliquam malesuada. See also Tothill, 876, which extended those 
	principles to questions of contract performance. Nulla ut finibus dui. This 
	remains the leading authority on common law pleading. Aliquam purus 
	tellus. Compare with Toth. 125 for the federal perspective (Toth 462). Ut 
	pharetra augue nulla. The state court first addressed this question in M&M. 12
	before the federal rule was established (M & M 123). Praesent ornare massa
	quis augue egestas; the same point was reinforced in M. & M. 234.
	`

	doc := sources.NewDoc("test-single-vol", text)

	tests := []struct {
		name         string
		abbreviation string
		expected     []string
	}{
		{
			name:         "Busb. Eq. Rep.",
			abbreviation: `Busb. Eq. Rep.`,
			expected:     []string{"Busb. Eq. Rep. 234", "Busb. Eq. Rep. 234"},
		},
		{
			name:         "Freem Chy",
			abbreviation: `Freem Chy`,
			expected:     []string{"Freem Chy 876"},
		},
		{
			name:         "Hob.",
			abbreviation: `Hob.`,
			expected:     []string{"Hob. 423"},
		},
		{
			name:         "Baldw.",
			abbreviation: `Baldw.`,
			expected:     []string{"Baldw. 125"},
		},
		{
			name:         "Cheves Eq.",
			abbreviation: `Cheves Eq.`,
			expected:     []string{"Cheves Eq. 12"},
		},
		{
			name:         "Toth",
			abbreviation: `Toth`,
			expected:     []string{"Toth 234", "Toth 876", "Toth 125", "Toth 462"},
		},
		{
			name:         "M & M",
			abbreviation: `M & M`,
			expected:     []string{"M & M 12", "M & M 123", "M & M 234"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewSingleVolDetector(tt.name, tt.abbreviation)
			cites := d.Detect(doc)
			require.Equal(t, len(tt.expected), len(cites))
			for i, exp := range tt.expected {
				assert.Equal(t, exp, cites[i].CleanCite(), fmt.Sprintf("Citation %v", i))
			}
		})
	}
}

func TestCleanCite_NilVolume(t *testing.T) {
	c := &Citation{
		ReporterAbbr: "U.S.",
		Page:         100,
	}
	assert.Equal(t, "U.S. 100", c.CleanCite())
}

func TestCleanCite_NonNilVolume(t *testing.T) {
	vol := 5
	c := &Citation{
		Volume:       &vol,
		ReporterAbbr: "U.S.",
		Page:         100,
	}
	assert.Equal(t, "5 U.S. 100", c.CleanCite())
}

func TestDetector_VolumeIsNonNil(t *testing.T) {
	text := `This has 30 Missis. 673 as a citation.`
	doc := sources.NewDoc("test", text)
	cites := GenericDetector.Detect(doc)
	require.Len(t, cites, 1)
	require.NotNil(t, cites[0].Volume, "standard detector should produce non-nil Volume")
	assert.Equal(t, 30, *cites[0].Volume)
}

func TestSingleVolDetector_VolumeIsNil(t *testing.T) {
	text := `See Hob. 423 for the ruling.`
	doc := sources.NewDoc("test", text)
	d := NewSingleVolDetector("Hob.", "Hob.")
	cites := d.Detect(doc)
	require.Len(t, cites, 1)
	assert.Nil(t, cites[0].Volume, "single-vol detector should produce nil Volume")
}
