package citations

import "github.com/google/uuid"

// WhitelistEntry holds the mapping from a found reporter abbreviation to its
// standardized form and metadata about the reporter type.
type WhitelistEntry struct {
	ReporterStandard *string
	ReporterCAP      *string
	Statute          bool
	UK               bool
	Junk             bool
	CAPDifferent     bool
}

// DiffVolEntry maps an original volume number to the corresponding CAP volume
// and reporter abbreviation for reporters with different numbering schemes.
type DiffVolEntry struct {
	CAPVol      int
	CAPReporter string
}

// UnlinkedCitation is a raw citation fetched from the database for linking.
type UnlinkedCitation struct {
	ID           uuid.UUID
	MomlTreatise string
	MomlPage     string
	Raw          string
	Volume       *int
	ReporterAbbr string
	Page         int
}

// LinkResult records the outcome of attempting to link a single citation.
type LinkResult struct {
	CitationID     uuid.UUID
	Status         string
	CAPCaseID      *int64
	CodeReporterID *int64
	ERCaseID       *string
	CiteCleaned    *string // reporter abbreviation standardized via whitelist; nil for skipped
	CiteNormalized *string // after diffvols transformation (equals CiteCleaned if no transformation); nil for skipped
	CiteLinked     *string // the cite string that matched, nil if no match
}

// Status constants for link results.
const (
	StatusLinkedCAP            = "linked_cap"
	StatusLinkedCodeReporter   = "linked_code_reporter"
	StatusLinkedEnglishReports = "linked_english_reports"
	StatusSkippedNotWhitelisted = "skipped_not_whitelisted"
	StatusSkippedStatute       = "skipped_statute"
	StatusSkippedJunk          = "skipped_junk"
	StatusNoMatch              = "no_match"
)
