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

	fmt.Println(citations)

	require.Equal(t, len(expected), len(citations))

	for i := range expected {
		assert.Equal(t, expected[i], citations[i].CleanCite(), fmt.Sprintf("Citation %v", i))
	}
}

func TestSingleVolDetector_Detect(t *testing.T) {
	text := `Lorem ipsum dolor sit amet, consectetur adipiscing elit. The court's ruling in Busb. Eq. Rep. 234 established the principles of equity jurisdiction. Nam vel justo sed felis aliquam malesuada. See also Freem Chy 876, which extended those principles to questions of contract performance. Nulla ut finibus dui. Hob. 423 remains the leading authority on common law pleading. Aliquam purus tellus. Compare with Baldw. 125 for the federal perspective. Ut pharetra augue nulla. The state court first addressed this question in Cheves Eq. 12 before the federal rule was established. Praesent ornare massa quis augue egestas; the same point was reinforced in Busb. Eq. Rep. 234.`

	doc := sources.NewDoc("test-single-vol", text)

	tests := []struct {
		name         string
		abbreviation string
		expected     []string
	}{
		{
			name:         "Busb. Eq. Rep.",
			abbreviation: `Busb. Eq. Rep.`,
			expected:     []string{"0 Busb. Eq. Rep. 234", "0 Busb. Eq. Rep. 234"},
		},
		{
			name:         "Freem Chy",
			abbreviation: `Freem Chy`,
			expected:     []string{"0 Freem Chy 876"},
		},
		{
			name:         "Hob.",
			abbreviation: `Hob.`,
			expected:     []string{"0 Hob. 423"},
		},
		{
			name:         "Baldw.",
			abbreviation: `Baldw.`,
			expected:     []string{"0 Baldw. 125"},
		},
		{
			name:         "Cheves Eq.",
			abbreviation: `Cheves Eq.`,
			expected:     []string{"0 Cheves Eq. 12"},
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
