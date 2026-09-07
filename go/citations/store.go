package citations

import "context"

// SingleVolReporter pairs a canonical reporter_standard with one of its
// recognized abbreviations (which may itself be the reporter_standard).
type SingleVolReporter struct {
	Standard string
	Abbr     string
}

// YearCitedReporter pairs a reporter that is cited by year
// (legalhist.reporters.cited_by_year) with one of its whitelisted spellings.
// Each pair becomes one YearDetector.
type YearCitedReporter struct {
	Standard string
	Abbr     string
}

// Store is an interface describing a data store for objects relating to citations.
type Store interface {
	SaveCitation(ctx context.Context, c *Citation) error
	// SaveCitations inserts a page's worth of citations in one statement,
	// collapsing duplicates on the key of the citations_unlinked_uq unique
	// index. cite-detector-moml uses this rather than SaveCitation: one insert
	// per citation is 59.4M round trips where 10.5M will do.
	SaveCitations(ctx context.Context, cites []*Citation) error
	GetSingleVolReporterAbbrs(ctx context.Context) ([]SingleVolReporter, error)
	// GetYearCitedReporterAbbrs returns one row per (reporter_standard,
	// spelling) pair for every reporter flagged cited_by_year, drawn from the
	// non-junk whitelist so that the OCR variants the corpus actually uses are
	// covered.
	GetYearCitedReporterAbbrs(ctx context.Context) ([]YearCitedReporter, error)
}
