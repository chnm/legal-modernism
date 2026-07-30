package citations

import "github.com/google/uuid"

// WhitelistEntry holds the mapping from a found reporter abbreviation to its
// standardized form and metadata about the reporter type.
type WhitelistEntry struct {
	ReporterStandard *string
	ReporterCAP      *string
	UK               bool
	Junk             bool
	CAPDifferent     bool
	// SingleVol reports whether the reporter has only one volume, in which case
	// a citation to it means the same thing with or without a leading volume 1.
	// legalhist.reporters.single_vol is nullable, and an unclassified reporter
	// is loaded as false so the volume variant stays off it.
	SingleVol bool
}

// DiffVolEntry maps an original volume number to the corresponding CAP volume
// and reporter abbreviation for reporters with different numbering schemes.
type DiffVolEntry struct {
	CAPVol      int
	CAPReporter string
}

// CaseSpan is one first-page cite string, the case it names, and how many pages
// that case occupies. It is the input to the page-range index that resolves pin
// cites — citations to an interior page of a case, which can never match a
// first-page cite string exactly.
//
// Length is the source's own page count for the case (last_page - first_page +
// 1), or 0 when the source records no page range at all, as english_reports.cases
// does. It is deliberately a length rather than an end page: CAP's first_page and
// last_page are the pagination of the scanned printing, which for ~2,770 volumes
// is offset from the pagination the citation uses, so an absolute end page cannot
// be trusted while a difference between two pages in the same system can.
type CaseSpan[ID comparable] struct {
	Cite   string
	ID     ID
	Length int
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
	MatchTier      string // how far the cascade got, see the Tier constants; "" (SQL NULL) for skipped
	CAPCaseID      *int64
	CodeReporterID *int64
	ERCaseID       *string
	CiteCleaned    *string // reporter abbreviation standardized via whitelist; nil for skipped
	CiteNormalized *string // after diffvols transformation (equals CiteCleaned if no transformation); nil for skipped
	CiteLinked     *string // the cite string that matched, nil if no match
}

// Status constants for link results.
const (
	StatusLinkedCAP             = "linked_cap"
	StatusLinkedCodeReporter    = "linked_code_reporter"
	StatusLinkedEnglishReports  = "linked_english_reports"
	StatusSkippedNotWhitelisted = "skipped_not_whitelisted"
	StatusSkippedJunk           = "skipped_junk"
	StatusNoMatch               = "no_match"
)

// Tier constants for LinkResult.MatchTier: how far the linking cascade got with
// a citation. Status says whether a citation linked; the tier says why it did
// not, or which probe succeeded when it did. Without it the 20M no_match rows
// are undifferentiated and the reason has to be re-derived by query.
//
// The failure tiers carry a route prefix (us_/uk_) rather than a target prefix
// because a failure exhausts the whole cascade: a US no_match missed CAP and the
// FreeLaw crosswalk and the code reporter, so naming any one target would be a
// half-truth. They report the closest approach across every target probed,
// ordered reporter_absent (nothing to match against) through page_absent (right
// reporter, right volume, wrong page — the pin-cite pool). The success tiers name
// the target that produced the link, since exactly one did.
//
// The values are constrained in SQL by chk_citation_links_match_tier; adding one
// here needs a migration to widen that constraint.
const (
	// no_match, US route (CAP -> FreeLaw -> alternate spellings -> code reporter).
	TierUSReporterAbsent  = "us_reporter_absent"  // no probed reporter spelling appears in any US source
	TierUSDiffVolsMissing = "us_diffvols_missing" // reporter renumbers in CAP, but no reporters_diffvols row covers this volume
	TierUSVolumeAbsent    = "us_volume_absent"    // reporter present, this volume never appears
	TierUSPageAbsent      = "us_page_absent"      // reporter and volume present, page is not a first-page cite and no case's range covers it
	TierUSPageAmbiguous   = "us_page_ambiguous"   // the page falls in a range, but more than one case starts at that range's first page
	TierUSPageGap         = "us_page_gap"         // the page falls past the end of the preceding case, in a hole in CAP's coverage

	// no_match, UK route (English Reports).
	TierUKReporterAbsent = "uk_reporter_absent"
	TierUKVolumeAbsent   = "uk_volume_absent"
	TierUKPageAbsent     = "uk_page_absent"
	TierUKPageAmbiguous  = "uk_page_ambiguous"
	TierUKPageGap        = "uk_page_gap"

	// linked_*: which probe produced the link.
	TierCAPDirect             = "cap_direct"               // cap.citations, under the normalized cite
	TierCAPFreelaw            = "cap_freelaw"              // freelaw.cite_to_cap, under the normalized cite
	TierCAPAltSpelling        = "cap_alt_spelling"         // cap.citations, under a reporters_abbreviations alternate
	TierCAPFreelawAltSpelling = "cap_freelaw_alt_spelling" // freelaw.cite_to_cap, under an alternate
	TierCAPPageInterior       = "cap_page_interior"        // cap.citations page range, pin cite to an interior page
	TierCodeDirect            = "code_direct"              // legalhist.code_reporter, under the cleaned cite
	TierCodeAltSpelling       = "code_alt_spelling"        // legalhist.code_reporter, under an alternate
	TierERDirect              = "er_direct"                // english_reports.cases
	TierERPageInterior        = "er_page_interior"         // english_reports.cases page range, pin cite to an interior page
)
