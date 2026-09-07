package citations

import (
	"testing"

	"github.com/lmullen/legal-modernism/go/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestYearDetector_Detect(t *testing.T) {
	kb := NewYearDetector("K.B.", "K. B.")
	kbNoSpace := NewYearDetector("K.B.", "K.B.")
	kbComma := NewYearDetector("K.B.", "K. B.,")
	ir := NewYearDetector("L.R.Ir.", "I. R.")
	ac := NewYearDetector("L.R.A.C.", "A. C.")

	tests := []struct {
		name     string
		text     string
		detector *YearDetector
		wantRaw  string
		wantYear int
		wantVol  *int // nil when the citation carries no volume
		wantAbbr string
		wantPage int
	}{
		{
			name: "bracketed year", text: "Hinton v. Doe [1906] 2 K. B. 171).",
			detector: kb, wantRaw: "[1906] 2 K. B. 171", wantYear: 1906, wantVol: ptr(2), wantAbbr: "K. B.", wantPage: 171,
		},
		{
			// Some treatises put the year of a year-cited reporter in parentheses.
			name: "parenthesized year", text: "Poulton v. Moore, (1915) 1 K. B. 400; 84 L. J. K. B. 1.",
			detector: kb, wantRaw: "(1915) 1 K. B. 400", wantYear: 1915, wantVol: ptr(1), wantAbbr: "K. B.", wantPage: 400,
		},
		{
			// The closing bracket read as a digit, and the opening one lost.
			name: "closing bracket read as 1", text: "Injury to passenger 19081 2 I. R. 393, 402; 42 I. L. T. 1.",
			detector: ir, wantRaw: "19081 2 I. R. 393", wantYear: 1908, wantVol: ptr(2), wantAbbr: "I. R.", wantPage: 393,
		},
		{
			name: "closing bracket read as J", text: "See [1915J 2 I. R. 210 for the point.",
			detector: ir, wantRaw: "[1915J 2 I. R. 210", wantYear: 1915, wantVol: ptr(2), wantAbbr: "I. R.", wantPage: 210,
		},
		{
			name: "opening bracket read as a letter", text: "Compare t19041 2 I. R. 417 with the rule.",
			detector: ir, wantRaw: "19041 2 I. R. 417", wantYear: 1904, wantVol: ptr(2), wantAbbr: "I. R.", wantPage: 417,
		},
		{
			name: "both brackets mangled", text: "75 L. J. K. B. 501. [19061 2 K. B. 293.",
			detector: kb, wantRaw: "[19061 2 K. B. 293", wantYear: 1906, wantVol: ptr(2), wantAbbr: "K. B.", wantPage: 293,
		},
		{
			// A bare year, and the spelling with its trailing comma, which is a
			// separator rather than part of the abbreviation.
			name: "bare year and trailing comma", text: "Elliot v. Pilcher, 1901 2 K.B., 817; 70 L.J., K.B. 1.",
			detector: kbNoSpace, wantRaw: "1901 2 K.B., 817", wantYear: 1901, wantVol: ptr(2), wantAbbr: "K.B.", wantPage: 817,
		},
		{
			name: "spelling registered with its comma", text: "See [1910] 2 K. B., 859.",
			detector: kbComma, wantRaw: "[1910] 2 K. B., 859", wantYear: 1910, wantVol: ptr(2), wantAbbr: "K. B.", wantPage: 859,
		},
		{
			// The Appeal Cases after 1890 carry no volume at all.
			name: "year-cited reporter without a volume", text: "Derry v. Peek is not [1893] A. C. 22.",
			detector: ac, wantRaw: "[1893] A. C. 22", wantYear: 1893, wantVol: nil, wantAbbr: "A. C.", wantPage: 22,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.detector.Detect(sources.NewDoc("test", tt.text))
			require.Len(t, got, 1)
			c := got[0]
			assert.Equal(t, tt.wantRaw, c.Raw)
			assert.Equal(t, c.Raw, tt.text[c.Start:c.End], "span must locate Raw")
			if assert.NotNil(t, c.Year) {
				assert.Equal(t, tt.wantYear, *c.Year)
			}
			if tt.wantVol == nil {
				assert.Nil(t, c.Volume)
			} else if assert.NotNil(t, c.Volume) {
				assert.Equal(t, *tt.wantVol, *c.Volume)
			}
			assert.Equal(t, tt.wantAbbr, c.ReporterAbbr)
			assert.Equal(t, tt.wantPage, c.Page)
		})
	}
}

func TestYearDetector_Detect_NoMatch(t *testing.T) {
	kb := NewYearDetector("K.B.", "K. B.")
	for name, text := range map[string]string{
		"no year":                      "See 75 L. J. K. B. 501 and 2 K. B. 293 for the point.",
		"year destroyed by the OCR":    "Ex parte Myers, (l!10S) 1 K. B. 911: 77 L. J. K. B. 1.",
		"year is the tail of a number": "See 21905 2 K. B. 1.",
		"year but another reporter":    "(1911) 28 T. L. R. 93 settles it.",
		"year without a citation":      "In 1905 the King's Bench Division sat.",
		"another reporter's spelling":  "[1906] 2 Q. B. 171",
	} {
		t.Run(name, func(t *testing.T) {
			assert.Empty(t, kb.Detect(sources.NewDoc("test", text)))
		})
	}
}

// TestYearDetector_ManyOnAPage exercises the starting-place scan: several
// year-cited citations on one page, some of them to other reporters, each found
// once.
// TestYearDetector_SpellingIsFlexible: as with the single-volume detectors,
// each space in the registered spelling stands for any run of whitespace and
// periods, so the detector built from "K. B." reaches "K.B." as well. Two
// detectors built from those two spellings then find the same span, which the
// unique index collapses to one row, exactly as for prefix pairs like "Toth"
// and "Tothill".
func TestYearDetector_SpellingIsFlexible(t *testing.T) {
	doc := sources.NewDoc("test", "[1906] 2 K.B. 171")
	spaced := NewYearDetector("K.B.", "K. B.").Detect(doc)
	tight := NewYearDetector("K.B.", "K.B.").Detect(doc)
	require.Len(t, spaced, 1)
	require.Len(t, tight, 1)
	assert.Equal(t, spaced[0].Raw, tight[0].Raw)
	assert.Equal(t, "K.B.", spaced[0].ReporterAbbr, "the spelling as it appeared, not the one registered")
	assert.Equal(t, spaced[0].Start, tight[0].Start)
	assert.Equal(t, spaced[0].End, tight[0].End)
}

func TestYearDetector_ManyOnAPage(t *testing.T) {
	text := `Tramways, Ltd., [1907] 2 K. B. 991; 71 J. P. 50; (1911) 28 T. L. R. 93;
	(1912) 1 K. B. 158; 76 J. P. 12; Vickers, Ltd., [1916] 1 K. B. 180.`
	got := NewYearDetector("K.B.", "K. B.").Detect(sources.NewDoc("test", text))
	require.Len(t, got, 3)
	assert.Equal(t, "[1907] 2 K. B. 991", got[0].Raw)
	assert.Equal(t, "(1912) 1 K. B. 158", got[1].Raw)
	assert.Equal(t, "[1916] 1 K. B. 180", got[2].Raw)
}

func ptr[T any](v T) *T { return &v }
