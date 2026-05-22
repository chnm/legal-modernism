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

// TestSingleVolDetector_NormalizesReporterAbbr verifies that when the canonical
// reporter_standard differs from the abbreviation that actually matched in the
// OCR, the resulting Citation carries the canonical form in ReporterAbbr while
// Raw preserves the literal OCR'd substring. This is the contract that the
// detector-creation loop in cite-detector-moml relies on for downstream
// linking against legalhist.reporters.
func TestSingleVolDetector_NormalizesReporterAbbr(t *testing.T) {
	tests := []struct {
		name         string
		canonical    string
		abbreviation string
		text         string
		expectedRaw  string
		expectedPage int
	}{
		{
			name:         "alt missing internal periods",
			canonical:    "Bail. Eq.",
			abbreviation: "Bail Eq",
			text:         "Compare Bail Eq 17 with the earlier ruling.",
			expectedRaw:  "Bail Eq 17",
			expectedPage: 17,
		},
		{
			name:         "alt matches canonical exactly",
			canonical:    "Bail. Eq.",
			abbreviation: "Bail. Eq.",
			text:         "See Bail. Eq. 42 for the rule.",
			expectedRaw:  "Bail. Eq. 42",
			expectedPage: 42,
		},
		{
			name:         "alt missing trailing period",
			canonical:    "Baldw.",
			abbreviation: "Baldw",
			text:         "The federal view in Baldw 125 was different.",
			expectedRaw:  "Baldw 125",
			expectedPage: 125,
		},
		{
			name:         "alt is longer form than canonical",
			canonical:    "Hob.",
			abbreviation: "Hobart",
			text:         "The rule in Hobart 423 was the older precedent.",
			expectedRaw:  "Hobart 423",
			expectedPage: 423,
		},
		{
			name:         `alt is much longer form (exercises \w*)`,
			canonical:    "Toth",
			abbreviation: "Tothill",
			text:         "See Tothill 876 for the early statement.",
			expectedRaw:  "Tothill 876",
			expectedPage: 876,
		},
		{
			name:         "alt with parenthesized jurisdiction (SC)",
			canonical:    "Bail. Eq.",
			abbreviation: "Bail Eq (SC)",
			text:         "See Bail Eq (SC) 42 for the ruling.",
			expectedRaw:  "Bail Eq (SC) 42",
			expectedPage: 42,
		},
		{
			name:         "alt with parenthesized jurisdiction (Eng)",
			canonical:    "Al",
			abbreviation: "Al (Eng)",
			text:         "Reference Al (Eng) 17 in the early reports.",
			expectedRaw:  "Al (Eng) 17",
			expectedPage: 17,
		},
		{
			name:         "alt with parenthesized jurisdiction (US)",
			canonical:    "Baldw.",
			abbreviation: "Baldw (US)",
			text:         "The federal view in Baldw (US) 125 was different.",
			expectedRaw:  "Baldw (US) 125",
			expectedPage: 125,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := sources.NewDoc("test-normalize", tt.text)
			d := NewSingleVolDetector(tt.canonical, tt.abbreviation)
			cites := d.Detect(doc)
			require.Len(t, cites, 1)
			assert.Equal(t, tt.canonical, cites[0].ReporterAbbr,
				"ReporterAbbr should be the canonical form, not the matched alt")
			assert.Equal(t, tt.expectedRaw, cites[0].Raw,
				"Raw should preserve the OCR'd alt spelling")
			assert.Equal(t, tt.expectedPage, cites[0].Page)
			assert.Nil(t, cites[0].Volume, "single-vol detector should produce nil Volume")
		})
	}
}

// TestSingleVolDetector_RawPreservesOCR is a focused unit test mirroring
// TestSingleVolDetector_VolumeIsNil: it documents the Raw-preservation
// invariant in isolation.
func TestSingleVolDetector_RawPreservesOCR(t *testing.T) {
	text := `See Bail Eq 42 for the ruling.`
	doc := sources.NewDoc("test", text)
	d := NewSingleVolDetector("Bail. Eq.", "Bail Eq")
	cites := d.Detect(doc)
	require.Len(t, cites, 1)
	assert.Equal(t, "Bail. Eq.", cites[0].ReporterAbbr, "ReporterAbbr should be the canonical")
	assert.Equal(t, "Bail Eq 42", cites[0].Raw, "Raw should preserve the OCR'd alt spelling")
}

// TestDetector_SpacingVariants documents the generic detector's behavior on
// multi-token abbreviations like "Ga. App." where the OCR may drop the
// whitespace between tokens. The detector matches both spellings but saves
// ReporterAbbr exactly as it appeared in the text — there is no whitespace
// normalization at detect time. The whitelist must therefore carry one row
// per spelling (or normalize whitespace before the whitelist lookup at link
// time).
func TestDetector_SpacingVariants(t *testing.T) {
	tests := []struct {
		name             string
		text             string
		expectedReporter string
		expectedVolume   int
		expectedPage     int
	}{
		{
			name:             "canonical spacing",
			text:             "See 5 Ga. App. 100 for the rule.",
			expectedReporter: "Ga. App.",
			expectedVolume:   5,
			expectedPage:     100,
		},
		{
			name:             "no space between tokens",
			text:             "See 5 Ga.App. 100 for the rule.",
			expectedReporter: "Ga.App.",
			expectedVolume:   5,
			expectedPage:     100,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := sources.NewDoc("test-spacing", tt.text)
			cites := GenericDetector.Detect(doc)
			require.Len(t, cites, 1)
			assert.Equal(t, tt.expectedReporter, cites[0].ReporterAbbr)
			require.NotNil(t, cites[0].Volume)
			assert.Equal(t, tt.expectedVolume, *cites[0].Volume)
			assert.Equal(t, tt.expectedPage, cites[0].Page)
		})
	}
}

// TestSingleVolDetector_SpacingVariants documents that the single-volume
// detector matches OCR text that omits the whitespace between abbreviation
// tokens. NewSingleVolDetector substitutes [\s.]* for every literal space in
// the abbreviation, so "Ga. App." compiles to `Ga\.[\s.]*App\.` and matches
// both "Ga. App." and "Ga.App." Saved ReporterAbbr is normalized to the
// canonical form regardless of which spelling appeared in the OCR.
func TestSingleVolDetector_SpacingVariants(t *testing.T) {
	tests := []struct {
		name         string
		text         string
		expectedRaw  string
		expectedPage int
	}{
		{
			name:         "canonical spacing",
			text:         "See Ga. App. 42 for the rule.",
			expectedRaw:  "Ga. App. 42",
			expectedPage: 42,
		},
		{
			name:         "no space between tokens",
			text:         "See Ga.App. 42 for the rule.",
			expectedRaw:  "Ga.App. 42",
			expectedPage: 42,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := sources.NewDoc("test-spacing-single", tt.text)
			d := NewSingleVolDetector("Ga. App.", "Ga. App.")
			cites := d.Detect(doc)
			require.Len(t, cites, 1)
			assert.Equal(t, "Ga. App.", cites[0].ReporterAbbr,
				"ReporterAbbr should be canonical regardless of OCR spacing")
			assert.Equal(t, tt.expectedRaw, cites[0].Raw,
				"Raw should preserve the OCR's spacing")
			assert.Equal(t, tt.expectedPage, cites[0].Page)
			assert.Nil(t, cites[0].Volume,
				"single-vol detector should produce nil Volume")
		})
	}
}
