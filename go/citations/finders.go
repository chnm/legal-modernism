package citations

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/lmullen/legal-modernism/go/sources"
)

// FinderSource is where LoadFinders reads the reporters it builds detectors
// for: the single-volume reporters with their spellings, and the reporters
// cited by year. *DBStore is one.
type FinderSource interface {
	GetSingleVolReporterAbbrs(ctx context.Context) ([]SingleVolReporter, error)
	GetYearCitedReporterAbbrs(ctx context.Context) ([]YearCitedReporter, error)
}

// LoadFinders builds the detectors a detector program runs over every
// document, in the order they run: the two general-purpose detectors, one
// single-volume detector per (reporter, spelling) pair, and one year detector
// per whitelisted spelling of every reporter cited by year. cite-detector-moml
// and cite-detector-cap share it so that the two corpora are detected under
// the same semantics (issue #74).
//
// It logs each stage at INFO, as the detector always has. Both loads are fatal
// to the caller: continuing without the single-volume or year detectors would
// detect the whole corpus under different semantics than every previous run.
func LoadFinders(ctx context.Context, store FinderSource) ([]Finder, error) {
	var detectors []Finder

	// Load the general-purpose detectors. The second finds citations whose
	// abbreviation the OCR corrupted by reading a letter as a digit ("F1ed."
	// for "Fed."), which the first cannot match at all. It scans separately so
	// that it can only add citations, never displace one.
	detectors = append(detectors, GenericDetector, GenericOCRDigitDetector)
	slog.Info("prepared general-purpose detectors", "num_detectors", len(detectors))

	// Create and load the single volume detectors. Each row is a
	// (reporter_standard, abbreviation) pair. The saved reporter_abbr is the
	// spelling that actually appeared in the OCR, not the reporter_standard the
	// detector was built from; the linker normalizes it through
	// legalhist.whitelist, so a spelling that belongs to a different reporter is
	// linked to that reporter instead of to this single volume.
	//
	// These detectors do not check what precedes the abbreviation, so they also
	// match inside longer citations ("Cal. 185" in "123 Cal. 185"). Those
	// shadows are dropped per document, once every detector has run, by
	// RemoveShadows in DetectDocument.
	singleVolReporters, err := store.GetSingleVolReporterAbbrs(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not get single volume reporters from database: %w", err)
	}
	for _, sv := range singleVolReporters {
		detectors = append(detectors, NewSingleVolDetector(sv.Standard, sv.Abbr))
	}
	slog.Info("prepared single volume detectors", "num_detectors", len(detectors))

	// The year-cited detectors, one per whitelisted spelling of every reporter
	// with cited_by_year_from set (issue #312). A reporter cited by year restarts its
	// volume numbers every year, so "2 K. B. 1" without the year names a
	// different case for every year of the series; these record the year, and
	// their match covers the generic detector's year-less reading of the same
	// citation, which RemoveShadows then drops.
	yearCitedReporters, err := store.GetYearCitedReporterAbbrs(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not get year-cited reporters from database: %w", err)
	}
	for _, yc := range yearCitedReporters {
		detectors = append(detectors, NewYearDetector(yc.Standard, yc.Abbr))
	}
	slog.Info("prepared year-cited detectors", "spellings", len(yearCitedReporters), "num_detectors", len(detectors))

	return detectors, nil
}

// DetectDocument finds the citations in one document: it applies the OCR
// corrections and the series-prefix rewrite to the text, runs every finder
// over it, and drops the shadows. It returns the citations kept and how many
// were dropped as shadows, for the caller to log with its own keys. The
// document's text is rewritten in place, as the detector programs have always
// done.
func DetectDocument(doc sources.Document, finders []Finder, ocr *sources.OCRReplacer) (kept []*Citation, dropped int) {
	doc.CorrectOCR(ocr)
	// Then the one normalization that is not a literal substitution:
	// the Law Reports' series prefix, "L. R. 5 Ch. 100", is moved
	// behind the volume so the series spelling survives detection
	// (issue #314).
	doc.Rewrite(NormalizeSeriesPrefix)

	// Run every detector over the document before deciding anything, so
	// that a single-volume match found inside a longer citation can be
	// recognized as a shadow of it and dropped.
	var found []*Citation
	for _, f := range finders {
		found = append(found, f.Detect(doc)...)
	}
	kept = RemoveShadows(found)
	return kept, len(found) - len(kept)
}
