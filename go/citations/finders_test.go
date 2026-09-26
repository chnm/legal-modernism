package citations

import (
	"context"
	"errors"
	"testing"

	"github.com/lmullen/legal-modernism/go/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeFinderSource struct {
	single []SingleVolReporter
	year   []YearCitedReporter
	err    error
}

func (f *fakeFinderSource) GetSingleVolReporterAbbrs(context.Context) ([]SingleVolReporter, error) {
	return f.single, f.err
}
func (f *fakeFinderSource) GetYearCitedReporterAbbrs(context.Context) ([]YearCitedReporter, error) {
	return f.year, f.err
}

func TestLoadFinders(t *testing.T) {
	src := &fakeFinderSource{
		single: []SingleVolReporter{{Standard: "Toth", Abbr: "Toth"}, {Standard: "Toth", Abbr: "Tothill"}},
		year:   []YearCitedReporter{{Standard: "K.B.", Abbr: "K. B."}},
	}
	finders, err := LoadFinders(context.Background(), src)
	require.NoError(t, err)
	// The two generic detectors, then one per single-volume spelling, then one
	// per year-cited spelling, in that order.
	require.Len(t, finders, 5)
	assert.Same(t, GenericDetector, finders[0])
	assert.Same(t, GenericOCRDigitDetector, finders[1])
	assert.IsType(t, &Detector{}, finders[2])
	assert.IsType(t, &Detector{}, finders[3])
	assert.IsType(t, &YearDetector{}, finders[4])

	_, err = LoadFinders(context.Background(), &fakeFinderSource{err: errors.New("db down")})
	assert.Error(t, err)
}

// TestDetectDocument runs the whole per-document pipeline on one text: the
// OCR correction repairs a spelling, the series prefix is moved behind the
// volume, and the single-volume detector's reading of the tail of a longer
// citation is dropped as a shadow.
func TestDetectDocument(t *testing.T) {
	finders := []Finder{GenericDetector, NewSingleVolDetector("Cal.", "Cal.")}
	ocr := sources.NewOCRReplacer([]*sources.OCRSubstitution{{Mistake: "Ca1.", Correction: "Cal."}})
	// Sentences rather than a list: the generic detector reads a run like
	// "185 and 5" as a citation too, which is real behaviour (the whitelist
	// rejects it at link time) but not what this test is about.
	doc := sources.NewDoc("d1", "See 123 Ca1. 185. The case in L. R. 5 Ch. 100 was followed. Also Cal. 12 applies.")

	kept, dropped := DetectDocument(doc, finders, ocr)

	assert.Equal(t, "See 123 Cal. 185. The case in 5 L. R. Ch. 100 was followed. Also Cal. 12 applies.", doc.Text(), "the text is corrected and normalized in place")
	assert.Equal(t, 1, dropped, "the single-volume reading of 'Cal. 185' inside '123 Cal. 185' is a shadow")

	var cites []string
	for _, c := range kept {
		cites = append(cites, c.CleanCite())
	}
	assert.ElementsMatch(t, []string{"123 Cal. 185", "5 L. R. Ch. 100", "Cal. 12"}, cites)
	for _, c := range kept {
		assert.Same(t, doc, c.Source)
	}
}
