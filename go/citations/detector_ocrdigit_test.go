package citations

import (
	"testing"

	"github.com/lmullen/legal-modernism/go/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// originalGenericPattern is the generic abbreviation pattern as it stood before
// GenericOCRDigitDetector was added. GenericDetector must keep behaving exactly
// like it, which is the whole reason the corrupted abbreviations are handled by
// a separate detector.
const originalGenericPattern = `[\p{L}\s\.,&\(\)]{3,16}((1st|2nd|3rd|\dth)\sed.)??`

// TestStripInteriorDigits covers the normalization in isolation: a digit
// between two letters is OCR noise and comes out, a digit anywhere else is a
// series or edition designator and stays.
func TestStripInteriorDigits(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no digits", "Fed.", "Fed."},
		{"interior digit", "F1ed.", "Fed."},
		{"interior digit mid-word", "Ma5ss.", "Mass."},
		{"interior digit in Ill.", "I1l.", "Il."},
		{"two interior digits", "M1a2ss.", "Mass."},
		{"adjacent interior digits", "Mi5s5s.", "Miss."},
		{"digit after space is a series designator", "Wn. (2d)", "Wn. (2d)"},
		{"digit after period is a series designator", "A.S.R.3d", "A.S.R.3d"},
		{"edition number survives", "Leach, 4th ed.", "Leach, 4th ed."},
		{"trailing digit survives", "Fed2", "Fed2"},
		{"leading digit survives", "2Fed", "2Fed"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, stripInteriorDigits(tt.in))
		})
	}
}

// TestGenericDetector_Unchanged is the guard on the whole approach. Handling
// corrupted abbreviations in a separate detector is only worth the extra scan
// if it leaves GenericDetector untouched, so assert that directly rather than
// trusting the refactor.
func TestGenericDetector_Unchanged(t *testing.T) {
	original := NewDetector("Generic", originalGenericPattern)
	assert.Equal(t, original.regex.String(), GenericDetector.regex.String(),
		"GenericDetector must still compile to the original expression")
}

// TestOCRDigitDetector_MotivatingCase is the case that started this. Both
// citations sit in the same sentence on MOML page 20004281605/00730. The second
// is invisible to GenericDetector because its character class holds no digits
// at all, so the OCR digit detector has to find it.
func TestOCRDigitDetector_MotivatingCase(t *testing.T) {
	text := `hIiclardson r. New (rleans )Debenture, e tc.. (o., 102 Fed. 780 (19(00); ` +
		`Rcti(ariloln r. New Orleauns (offee Co., 102 F1ed. 785 (1900)).`
	doc := sources.NewDoc("test-interior-digit", text)

	// The clean citation is GenericDetector's, and it still finds it.
	generic := GenericDetector.Detect(doc)
	require.Len(t, generic, 1)
	assert.Equal(t, "102 Fed. 780", generic[0].Raw)
	assert.Equal(t, "Fed.", generic[0].ReporterAbbr)

	// The corrupted one is the OCR digit detector's, and only its own.
	cites := GenericOCRDigitDetector.Detect(doc)
	require.Len(t, cites, 1)
	// Raw preserves what the OCR actually produced; ReporterAbbr is normalized
	// so the whitelist can route it to the Federal Reporter.
	assert.Equal(t, "102 F1ed. 785", cites[0].Raw)
	assert.Equal(t, "Fed.", cites[0].ReporterAbbr)
	require.NotNil(t, cites[0].Volume)
	assert.Equal(t, 102, *cites[0].Volume)
	assert.Equal(t, 785, cites[0].Page)
}

// TestOCRDigitDetector_Variants runs the shapes of corruption actually observed
// in the OCR through the detector.
func TestOCRDigitDetector_Variants(t *testing.T) {
	tests := []struct {
		text     string
		wantRaw  string
		wantAbbr string
		wantVol  int
		wantPage int
	}{
		{"see 102 I1l. 315 for the rule", "102 I1l. 315", "Il.", 102, 315},
		{"see 123 Ma5ss. 286 for the rule", "123 Ma5ss. 286", "Mass.", 123, 286},
		{"see 14 M3ich. 103 for the rule", "14 M3ich. 103", "Mich.", 14, 103},
		{"see 181 F1ed. 3 for the rule", "181 F1ed. 3", "Fed.", 181, 3},
		{"see 1 M1arsh. 525 for the rule", "1 M1arsh. 525", "Marsh.", 1, 525},
		{"see 12 M1od. 331 for the rule", "12 M1od. 331", "Mod.", 12, 331},
		{"see 38 M3d. 459 for the rule", "38 M3d. 459", "Md.", 38, 459},
		{"see 7 B1ing. 349 for the rule", "7 B1ing. 349", "Bing.", 7, 349},
	}
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			doc := sources.NewDoc("test-variants", tt.text)
			cites := GenericOCRDigitDetector.Detect(doc)
			require.Len(t, cites, 1)
			assert.Equal(t, tt.wantRaw, cites[0].Raw)
			assert.Equal(t, tt.wantAbbr, cites[0].ReporterAbbr)
			require.NotNil(t, cites[0].Volume)
			assert.Equal(t, tt.wantVol, *cites[0].Volume)
			assert.Equal(t, tt.wantPage, cites[0].Page)
		})
	}
}

// TestOCRDigitDetector_SilentOnCleanText verifies that the second scan costs
// nothing on text with no corruption: every citation there belongs to
// GenericDetector, and the OCR digit detector must not duplicate or re-cut it.
func TestOCRDigitDetector_SilentOnCleanText(t *testing.T) {
	text := `
	This is a doc with 6 N. Y. Sup. Ct. 69 citations.
	This is a doc with citations (2 Kans. 416).
	This is a doc 71 N. C. 297 with citations.
	This doc has 6 Watts & S. 314 as a citation.
	This doc has parentheses 1 C. R. (N. S.) 413 as a citation.
	This has an edition 1 Leach, 4th ed. 484 associated with it.
	Citing 1 How. Sp. T. Rep. 114 is an interesting case.
	Compare 102 Fed. 780 with 53 Fed. 19 and 81 Fed. 51 in sequence.
	`
	doc := sources.NewDoc("test-clean", text)
	assert.Empty(t, GenericOCRDigitDetector.Detect(doc),
		"the OCR digit detector should find nothing without a letter-flanked digit")
}

// TestOCRDigitDetector_DoesNotSwallowNumbers is the guard that makes the
// pattern safe. The abbreviation may contain a digit only with a letter on both
// sides, so a match can never extend across the whitespace separating the
// abbreviation from the volume and page numbers. Putting a bare \d in the
// character class instead would let one match run across adjacent citations.
//
// The distinct set is what matters here. Detect scans a 25-character window
// from every volume-looking number, so adjacent citations fall inside more than
// one window and the same citation is emitted more than once -- long-standing
// behavior for every detector, absorbed by SaveCitation's ON CONFLICT DO
// NOTHING on (treatise, page, volume, reporter_abbr, page).
func TestOCRDigitDetector_DoesNotSwallowNumbers(t *testing.T) {
	text := `Compare 102 F1ed. 780 with 53 F1ed. 19 and 81 F1ed. 51 in sequence.`
	doc := sources.NewDoc("test-no-swallow", text)

	seen := map[string]bool{}
	var raw []string
	for _, c := range GenericOCRDigitDetector.Detect(doc) {
		if !seen[c.Raw] {
			seen[c.Raw] = true
			raw = append(raw, c.Raw)
		}
	}
	assert.Equal(t, []string{"102 F1ed. 780", "53 F1ed. 19", "81 F1ed. 51"}, raw)
}

// TestGenericDetector_SeriesDesignatorUnchanged verifies that the ReporterAbbr
// normalization, which runs for every detector, does not damage the digits that
// legitimately belong in an abbreviation.
func TestGenericDetector_SeriesDesignatorUnchanged(t *testing.T) {
	text := `This has an edition 1 Leach, 4th ed. 484 associated with it.`
	doc := sources.NewDoc("test-series", text)
	cites := GenericDetector.Detect(doc)
	require.Len(t, cites, 1)
	assert.Equal(t, "Leach, 4th ed.", cites[0].ReporterAbbr)
}
