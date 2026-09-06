package citations

import "context"

// SingleVolReporter pairs a canonical reporter_standard with one of its
// recognized abbreviations (which may itself be the reporter_standard).
type SingleVolReporter struct {
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
}
