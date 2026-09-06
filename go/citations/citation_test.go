package citations

import (
	"fmt"
	"testing"

	"github.com/lmullen/legal-modernism/go/sources"
	"github.com/stretchr/testify/assert"
)

func Test_normalizeReporter(t *testing.T) {
	var tests = []struct {
		description string
		input       string
		output      string
	}{

		{"removes multiple periods", "S.C..L.", "S.C.L."},
		{"doesn't screw up a good citation", "S.C.L.", "S.C.L."},
		{"fixes extra weird spacing", "S.   C. L.", "S. C. L."},
	}

	for i, tt := range tests {
		output := normalizeReporter(tt.input)
		assert.Equal(t, tt.output, output, fmt.Sprint("test", i, ": ", tt.description))
	}
}

// TestCitationKey pins the in-batch deduplication key to the columns of the
// citations_unlinked_uq unique index, including its COALESCE(volume, -1): a
// volume-less citation and one to volume 1 of the same reporter and page are
// different citations, and must not collapse into each other.
func TestCitationKey(t *testing.T) {
	page := sources.NewTreatisePage("p1", "t1", "")
	one, two := 1, 2

	cite := func(vol *int, abbr string, page2 int) *Citation {
		return &Citation{Source: page, Volume: vol, ReporterAbbr: abbr, Page: page2}
	}

	same := []struct {
		name string
		a, b *Citation
	}{
		{"identical", cite(nil, "Hob.", 423), cite(nil, "Hob.", 423)},
		{"identical with volume", cite(&one, "Toth", 123), cite(&one, "Toth", 123)},
	}
	for _, tt := range same {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, citationKey(tt.a), citationKey(tt.b))
		})
	}

	different := []struct {
		name string
		a, b *Citation
	}{
		{"volume-less vs volume 1", cite(nil, "Toth", 123), cite(&one, "Toth", 123)},
		{"different volume", cite(&one, "Toth", 123), cite(&two, "Toth", 123)},
		{"different reporter", cite(nil, "Hob.", 423), cite(nil, "Hobart", 423)},
		{"different page", cite(nil, "Hob.", 423), cite(nil, "Hob.", 424)},
		{"different source page", cite(nil, "Hob.", 423),
			&Citation{Source: sources.NewTreatisePage("p2", "t1", ""), ReporterAbbr: "Hob.", Page: 423}},
		{"different treatise", cite(nil, "Hob.", 423),
			&Citation{Source: sources.NewTreatisePage("p1", "t2", ""), ReporterAbbr: "Hob.", Page: 423}},
	}
	for _, tt := range different {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEqual(t, citationKey(tt.a), citationKey(tt.b))
		})
	}
}
