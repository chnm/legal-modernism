package citations

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/lmullen/legal-modernism/go/sources"
)

// Citation represents a citation from a Document to a particular case.
type Citation struct {
	ID           uuid.UUID
	Source       sources.Document
	Raw          string
	Volume       *int
	ReporterAbbr string
	Page         int

	// Start and End are the byte offsets of Raw in Source.Text(), so that
	// Source.Text()[Start:End] == Raw. They are not persisted: SaveCitation
	// does not read them. They exist so that detections from different
	// detectors can be compared by position, which is how RemoveShadows tells
	// that "Cal. 185" was found inside "123 Cal. 185".
	Start int
	End   int
}

func (c Citation) String() string {
	return fmt.Sprintf("[%s] cites [%s]", c.Source.ID(), c.CleanCite())
}

// CleanCite returns a clean citation without spaces.
func (c *Citation) CleanCite() string {
	if c.Volume == nil {
		return fmt.Sprintf("%s %v", c.CleanReporter(), c.Page)
	}
	return fmt.Sprintf("%v %s %v", *c.Volume, c.CleanReporter(), c.Page)
}

// CleanReporter returns a normalized string for the reporter abbreviation.
func (c *Citation) CleanReporter() string {
	return normalizeReporter(c.ReporterAbbr)
}

// Helper function to do the dirty work in normalizing the reporter
func normalizeReporter(r string) string {
	r = reSpace.ReplaceAllString(r, " ")
	r = reMultiplePeriodsSpace.ReplaceAllString(r, ". ")
	r = reMultiplePeriodsNoSpace.ReplaceAllString(r, ".")
	return r
}
