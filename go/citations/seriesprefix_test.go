package citations

import (
	"testing"

	"github.com/lmullen/legal-modernism/go/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeSeriesPrefix(t *testing.T) {
	tests := map[string]struct{ in, want string }{
		"chancery appeals":          {"See L. R. 5 Ch. 100 for the point.", "See 5 L. R. Ch. 100 for the point."},
		"no space in the prefix":    {"L.R. 10 Q. B. 100", "10 L. R. Q. B. 100"},
		"comma after the prefix":    {"L. R., 7 H. L. 653", "7 L. R. H. L. 653"},
		"line break":                {"L. R.\n\t\t\t9 Eq. 100", "9 L. R. Eq. 100"},
		"two in a row":              {"L. R. 1 Sc. & Div. 100; L. R. 3 A. & E. 12", "1 L. R. Sc. & Div. 100; 3 L. R. A. & E. 12"},
		"start of text":             {"L. R. 2 P. C. 5", "2 L. R. P. C. 5"},
		"volume-first is unchanged": {"5 L. R. Ch. 100", "5 L. R. Ch. 100"},
		"a year is not a volume":    {"L. R. 1876 was busy", "L. R. 1876 was busy"},
		"prefix inside a word":      {"XL. R. 5 Ch. 100", "XL. R. 5 Ch. 100"},
		"nothing to do":             {"The rule in 123 Cal. 185 was different.", "The rule in 123 Cal. 185 was different."},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tt.want, NormalizeSeriesPrefix(tt.in))
		})
	}
}

// TestNormalizeSeriesPrefix_Detected follows the rewrite through detection: the
// generic detector records the series spelling, not the bare abbreviation, and
// the neighbouring citation is untouched. (The detector also records the run
// "100 with 2" between them, as it does for any two numbers a short word apart;
// the whitelist rejects that, and it is not this test's concern.)
func TestNormalizeSeriesPrefix_Detected(t *testing.T) {
	doc := sources.NewDoc("test", "Compare L. R. 5 Ch. 100 with 2 Ch. Cas. 45.")
	doc.Rewrite(NormalizeSeriesPrefix)
	byAbbr := map[string]*Citation{}
	for _, c := range GenericDetector.Detect(doc) {
		byAbbr[c.ReporterAbbr] = c
	}
	require.Contains(t, byAbbr, "L. R. Ch.")
	assert.Equal(t, 5, *byAbbr["L. R. Ch."].Volume)
	assert.Equal(t, 100, byAbbr["L. R. Ch."].Page)
	assert.Equal(t, "5 L. R. Ch. 100", byAbbr["L. R. Ch."].Raw)
	require.Contains(t, byAbbr, "Ch. Cas.")
	assert.Equal(t, 2, *byAbbr["Ch. Cas."].Volume)
	assert.NotContains(t, byAbbr, "Ch.", "the bare abbreviation must not survive the rewrite")
}
