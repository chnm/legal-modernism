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
			// \w* lets the "Toth" abbreviation reach the longer form
			// "Tothill", which is recorded as "Tothill" rather than "Toth" so
			// that the whitelist decides where it belongs.
			name:         "Toth",
			abbreviation: `Toth`,
			expected:     []string{"Toth. 234", "Tothill 876", "Toth. 125", "Toth 462"},
		},
		{
			name:         "Tothill",
			abbreviation: `Tothill`,
			expected:     []string{"Tothill 876"},
		},
		{
			name:         "M & M",
			abbreviation: `M & M`,
			expected:     []string{"M&M. 12", "M & M 123", "M. & M. 234"},
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

// TestSingleVolDetector_RecordsDetectedAbbr verifies that the Citation records
// the abbreviation that actually matched in the OCR, not the reporter the
// detector was built from. cite-linker normalizes that detected form through
// legalhist.whitelist, exactly as it does for the generic detector. Recording
// the reporter instead would assert that every match belongs to this single
// volume, which is how "Alienation, 118" came to be linked to Aleyn's King's
// Bench.
func TestSingleVolDetector_RecordsDetectedAbbr(t *testing.T) {
	tests := []struct {
		name         string
		reporter     string
		abbreviation string
		text         string
		expectedAbbr string
		expectedRaw  string
		expectedPage int
	}{
		{
			name:         "alt missing internal periods",
			reporter:     "Bail. Eq.",
			abbreviation: "Bail Eq",
			text:         "Compare Bail Eq 17 with the earlier ruling.",
			expectedAbbr: "Bail Eq",
			expectedRaw:  "Bail Eq 17",
			expectedPage: 17,
		},
		{
			name:         "alt matches reporter exactly",
			reporter:     "Bail. Eq.",
			abbreviation: "Bail. Eq.",
			text:         "See Bail. Eq. 42 for the rule.",
			expectedAbbr: "Bail. Eq.",
			expectedRaw:  "Bail. Eq. 42",
			expectedPage: 42,
		},
		{
			name:         "alt missing trailing period",
			reporter:     "Baldw.",
			abbreviation: "Baldw",
			text:         "The federal view in Baldw 125 was different.",
			expectedAbbr: "Baldw",
			expectedRaw:  "Baldw 125",
			expectedPage: 125,
		},
		{
			name:         "trailing comma is a separator, not part of the abbr",
			reporter:     "Baldw.",
			abbreviation: "Baldw",
			text:         "The federal view in Baldw, 125 was different.",
			expectedAbbr: "Baldw",
			expectedRaw:  "Baldw, 125",
			expectedPage: 125,
		},
		{
			name:         "alt is longer form than reporter",
			reporter:     "Hob.",
			abbreviation: "Hobart",
			text:         "The rule in Hobart 423 was the older precedent.",
			expectedAbbr: "Hobart",
			expectedRaw:  "Hobart 423",
			expectedPage: 423,
		},
		{
			name:         "alt is much longer form than reporter",
			reporter:     "Toth",
			abbreviation: "Tothill",
			text:         "See Tothill 876 for the early statement.",
			expectedAbbr: "Tothill",
			expectedRaw:  "Tothill 876",
			expectedPage: 876,
		},
		{
			name:         "alt with parenthesized jurisdiction (SC)",
			reporter:     "Bail. Eq.",
			abbreviation: "Bail Eq (SC)",
			text:         "See Bail Eq (SC) 42 for the ruling.",
			expectedAbbr: "Bail Eq (SC)",
			expectedRaw:  "Bail Eq (SC) 42",
			expectedPage: 42,
		},
		{
			name:         "alt with parenthesized jurisdiction (Eng)",
			reporter:     "Al",
			abbreviation: "Al (Eng)",
			text:         "Reference Al (Eng) 17 in the early reports.",
			expectedAbbr: "Al (Eng)",
			expectedRaw:  "Al (Eng) 17",
			expectedPage: 17,
		},
		{
			name:         "alt with parenthesized jurisdiction (US)",
			reporter:     "Baldw.",
			abbreviation: "Baldw (US)",
			text:         "The federal view in Baldw (US) 125 was different.",
			expectedAbbr: "Baldw (US)",
			expectedRaw:  "Baldw (US) 125",
			expectedPage: 125,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := sources.NewDoc("test-detected-abbr", tt.text)
			d := NewSingleVolDetector(tt.reporter, tt.abbreviation)
			cites := d.Detect(doc)
			require.Len(t, cites, 1)
			assert.Equal(t, tt.expectedAbbr, cites[0].ReporterAbbr,
				"ReporterAbbr should be the spelling that matched, not the reporter")
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
	assert.Equal(t, "Bail Eq", cites[0].ReporterAbbr, "ReporterAbbr should be the detected spelling")
	assert.Equal(t, "Bail Eq 42", cites[0].Raw, "Raw should preserve the OCR'd alt spelling")
}

// TestSingleVolDetector_StemRecordsMatchedWord pins down what the \w* stem
// costs and why that is now tolerable. The stem makes the abbreviation "Al"
// reach any word beginning with it, so Aleyn's King's Bench detector fires on
// "Alienation", "Alimony" and "Ala." alike. Under the old behavior every one of
// those was recorded as a citation to Aleyn and linked on the page number
// alone, which issue #218 identifies as the largest source of false positives
// in the data.
//
// Recording the word that actually matched moves the decision to the
// whitelist: "Ala." routes to the Alabama reporter, "Alienation" and "Alimony"
// are not whitelisted and are skipped. The over-capture becomes a volume
// problem rather than an accuracy problem.
func TestSingleVolDetector_StemRecordsMatchedWord(t *testing.T) {
	tests := []struct {
		text         string
		expectedAbbr string
		expectedPage int
	}{
		// Stemmed matches: recorded as the longer word, not as "Al".
		{"the law of Alienation, 118 is settled", "Alienation", 118},
		{"see Ala., 672 for the rule", "Ala.", 672},
		{"compare Allen, 2 with the later cases", "Allen", 2},
		{"questions of Alimony 122 aside", "Alimony", 122},
		{"discussed in Alexander, 3 at length", "Alexander", 3},
		// Exact matches still record the abbreviation itself.
		{"see Al. 17 for the rule", "Al.", 17},
		{"see Al 17 for the rule", "Al", 17},
		{"see Al, 17 for the rule", "Al", 17},
	}

	d := NewSingleVolDetector("Al", "Al")
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			doc := sources.NewDoc("test-stem", tt.text)
			cites := d.Detect(doc)
			require.Len(t, cites, 1)
			assert.Equal(t, tt.expectedAbbr, cites[0].ReporterAbbr,
				"ReporterAbbr should be the word that matched, not the reporter")
			assert.Equal(t, tt.expectedPage, cites[0].Page)
		})
	}
}

// TestSingleVolDetector_WordBoundary verifies that an abbreviation does not
// match in the middle of a word, which is what the leading \b in the regex
// guards against.
func TestSingleVolDetector_WordBoundary(t *testing.T) {
	d := NewSingleVolDetector("Bur.", "Bur")

	doc := sources.NewDoc("test-boundary", "the estate of McBur 12 was disputed")
	assert.Empty(t, d.Detect(doc), "abbreviation should not match mid-word")

	doc = sources.NewDoc("test-boundary", "the estate in Bur. 12 was disputed")
	cites := d.Detect(doc)
	require.Len(t, cites, 1)
	assert.Equal(t, "Bur.", cites[0].ReporterAbbr)
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
// both "Ga. App." and "Ga.App." As with the generic detector, ReporterAbbr is
// saved exactly as it appeared, so the whitelist must carry one row per
// spelling. See TestDetector_SpacingVariants for the same contract.
func TestSingleVolDetector_SpacingVariants(t *testing.T) {
	tests := []struct {
		name         string
		text         string
		expectedAbbr string
		expectedRaw  string
		expectedPage int
	}{
		{
			name:         "canonical spacing",
			text:         "See Ga. App. 42 for the rule.",
			expectedAbbr: "Ga. App.",
			expectedRaw:  "Ga. App. 42",
			expectedPage: 42,
		},
		{
			name:         "no space between tokens",
			text:         "See Ga.App. 42 for the rule.",
			expectedAbbr: "Ga.App.",
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
			assert.Equal(t, tt.expectedAbbr, cites[0].ReporterAbbr,
				"ReporterAbbr should preserve the OCR's spacing")
			assert.Equal(t, tt.expectedRaw, cites[0].Raw,
				"Raw should preserve the OCR's spacing")
			assert.Equal(t, tt.expectedPage, cites[0].Page)
			assert.Nil(t, cites[0].Volume,
				"single-vol detector should produce nil Volume")
		})
	}
}
