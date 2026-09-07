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
	// Year is the year a year-cited reporter is cited by ("[1905] 2 K.B. 1"),
	// recorded by YearDetector and nil for every other detector. For such a
	// reporter the volume restarts every year, so the year is part of what
	// identifies the case.
	Year *int

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

// CleanCite returns a clean citation without spaces. A year, when the
// citation carries one, leads in brackets: "[1905] 2 K.B. 1".
func (c *Citation) CleanCite() string {
	var cite string
	if c.Volume == nil {
		cite = fmt.Sprintf("%s %v", c.CleanReporter(), c.Page)
	} else {
		cite = fmt.Sprintf("%v %s %v", *c.Volume, c.CleanReporter(), c.Page)
	}
	if c.Year != nil {
		return fmt.Sprintf("[%d] %s", *c.Year, cite)
	}
	return cite
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
