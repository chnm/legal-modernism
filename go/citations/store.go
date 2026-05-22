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
	GetSingleVolReporterAbbrs(ctx context.Context) ([]SingleVolReporter, error)
}
