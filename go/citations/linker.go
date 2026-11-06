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
	// Statute reports that the reporter row is type = 'statute': the spelling is
	// a regnal-year statute citation ("3 & 4 Will. 4, c. 74" read as volume 3,
	// reporter "Will.", page 4), which names no case in any source. The linker
	// skips it rather than probing (issue #246).
	Statute bool
	// CitedByYearFrom is the first year from which the reporter is cited by
	// year (legalhist.reporters.cited_by_year_from), or 0 when it never is.
	// From that year on its volume numbers restart every year, so "2 K.B. 1"
	// names a different case for every year of the series and the year is
	// part of the cite string the linker builds: "[1905] 2 K.B. 1" (issue
	// #312). A year before it is decoration on a volume-cited citation --
	// "(1889) 14 App. Cas. 337" -- and stays out of the string, which is what
	// lets one reporter row hold a volume-cited series and its year-cited
	// continuation (issue #314).
	CitedByYearFrom int
	// Jurisdiction is the reporter's legalhist.reporters.jurisdiction
	// ("us:sc", "uk:kb"), or "" when it has none. The linker compares it with
	// a citation's SourceJurisdiction to choose which of a spelling's two
	// readings to try first (issue #401).
	Jurisdiction string
	// Fallback is the reporter a spelling means when it does not mean its own
	// (legalhist.whitelist.fallback_standard), or nil. Some spellings name one
	// reporter in one jurisdiction or run of volumes and another elsewhere:
	// "Porter" is Porter's Indiana Reports, but in Alabama Porter's Alabama
	// Reports (issue #358). The linker tries the fallback only once the
	// spelling's own reading has failed in a way that says it was the wrong
	// reporter -- unless the citing document is of the fallback's
	// jurisdiction and not of the spelling's own, when the two trade places
	// (issue #401). Its fields describe the fallback reporter; it never has a
	// Fallback of its own.
	Fallback *WhitelistEntry
}

// DiffVolEntry maps an original volume number to the corresponding CAP volume
// and reporter abbreviation for reporters with different numbering schemes.
type DiffVolEntry struct {
	CAPVol      int
	CAPReporter string
}

// ERCase is what one English Reports cite string resolves to. A cite that
// several cases share resolves to no case at all: nominate-reporter pages often
// carry many short decisions, so 40,677 of the 185,538 distinct cite strings
// (21.9%) belong to more than one case, and resolving those to an arbitrary one
// was issue #256.
//
// Ambiguous cites stay in the map rather than being dropped, which is the one
// way this differs from the policy LoadCAPCitations enforces with its HAVING
// clause. A cite the corpus knows but cannot resolve and a cite it has never
// seen are different failures, and the linker reports them as different tiers
// (uk_page_ambiguous against uk_page_absent), so the key has to survive the load
// for the distinction to be available at link time.
type ERCase struct {
	ID        string // the case; empty when Ambiguous
	Ambiguous bool   // the cite string belongs to more than one case
	Cases     int    // how many cases share the cite string; 1 when unambiguous
}

// SharedCase is one of the CAP cases that share a first-page cite string, with
// how many pages it occupies (issue #359). The cases that begin on one page are
// nearly always a decision of some length and one or more memorandum decisions
// of a page or less, printed in the space the longer one leaves, so the pages
// are what tell them apart. Pages is CAP's last_page - first_page + 1, or 0 when
// CAP records no page range or an inverted one. It is a length, not an end
// page, because the scanned pagination can be offset from the cited one: CAP's
// first_page and last_page are the pagination of the scanned printing, which
// for ~2,770 volumes is offset from the pagination the citation uses.
type SharedCase struct {
	ID    int64
	Pages int
}

// UnlinkedCitation is a raw citation fetched from the database for linking.
// It carries nothing that says which corpus it came from: the cascade links a
// citation from a treatise page and one from a CAP opinion the same way.
type UnlinkedCitation struct {
	ID           uuid.UUID
	Raw          string
	Volume       *int
	ReporterAbbr string
	Page         int
	// Year is the year the citation was cited by, recorded by the year detector
	// for reporters flagged cited_by_year, and nil otherwise.
	Year *int
	// SourceYear is the year of the document the citation appears in:
	// moml.volumes.year for a treatise page, cap.cases.decision_year for a CAP
	// opinion, and nil when it is unknown. The linker refuses a link to a case
	// decided in a later year (issue #319); a nil year refuses nothing. The
	// corpus store fills it when it streams the citation, which is how the
	// one corpus-specific fact the cascade needs reaches it without the
	// cascade knowing the corpus.
	SourceYear *int
	// SourceJurisdiction is the jurisdiction of the document the citation
	// appears in, in legalhist.reporters.jurisdiction's codes ("us:ny"): the
	// citing case's state for a CAP opinion, through
	// legalhist.cap_jurisdiction_codes, and "" for a treatise page or a case
	// with no code. A spelling whose fallback reporter is of this
	// jurisdiction, and whose own reporter is not, is read as the fallback
	// first (issue #401).
	SourceJurisdiction string
}

// LinkResult records the outcome of attempting to link a single citation.
type LinkResult struct {
	CitationID     uuid.UUID
	Status         string
	MatchTier      string // how far the cascade got, see the Tier constants; "" (SQL NULL) for skipped
	CAPCaseID      *int64
	CodeReporterID *int64
	ERCaseID       *string
	StubCite       *string // legalhist.stub_cases key, set only for StatusLinkedStub
	CiteCleaned    *string // reporter abbreviation standardized via whitelist; nil for skipped
	CiteNormalized *string // after diffvols transformation (equals CiteCleaned if no transformation); nil for skipped
	CiteLinked     *string // the cite string that matched, nil if no match
}

// Status constants for link results.
const (
	StatusLinkedCAP            = "linked_cap"
	StatusLinkedCodeReporter   = "linked_code_reporter"
	StatusLinkedEnglishReports = "linked_english_reports"
	// StatusLinkedStub: the citation names a case no source holds, but the
	// cite string recurs across the corpus often enough that
	// legalhist.stub_cases carries a record for it (issue #248). The link
	// carries less than a link to a real case -- the record knows only the
	// cite string and how often it is cited -- which is why it has its own
	// status rather than borrowing one of the source-backed ones. It still
	// matches the linked% pattern every dashboard consumer keys on, by
	// design: a stub link is a citation found, and match_tier is what splits
	// it out.
	StatusLinkedStub            = "linked_stub"
	StatusSkippedNotWhitelisted = "skipped_not_whitelisted"
	StatusSkippedJunk           = "skipped_junk"
	StatusSkippedStatute        = "skipped_statute"
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
// reporter, right volume, wrong page: a pin cite into a case, or a case the
// source lacks; the cascade no longer tells them apart, issue #393). volume_missing sits
// beside volume_absent rather than below it: both mean the volume step failed,
// but for a different reason — the citation carried no volume for the probes to
// look up, so nothing is known about coverage (issue #261). anachronistic stands
// apart from that ladder: the cascade did find a case, and refused it because
// the case was decided after the citing document, a treatise volume or an
// opinion's case (issue #319). The
// success tiers name the target that produced the link, since exactly one did.
//
// The values are constrained in SQL by chk_citation_links_match_tier; adding one
// here needs a migration to widen that constraint.
const (
	// no_match, US route (CAP -> FreeLaw -> alternate spellings -> code reporter).
	TierUSReporterAbsent  = "us_reporter_absent"  // no probed reporter spelling appears in any US source
	TierUSDiffVolsMissing = "us_diffvols_missing" // reporter renumbers in CAP, but no reporters_diffvols row covers this volume
	TierUSVolumeAbsent    = "us_volume_absent"    // reporter present, this volume never appears
	TierUSVolumeMissing   = "us_volume_missing"   // reporter present, but the citation carries no volume to look up
	TierUSPageAbsent      = "us_page_absent"      // reporter and volume present, page is not a first-page cite
	TierUSPageAmbiguous   = "us_page_ambiguous"   // the cite is one several CAP cases share, none of them longest (#359)
	TierUSAnachronistic   = "us_anachronistic"    // every case found was decided after the citing document

	// no_match, UK route (English Reports).
	TierUKReporterAbsent = "uk_reporter_absent"
	TierUKVolumeAbsent   = "uk_volume_absent"
	TierUKVolumeMissing  = "uk_volume_missing"
	TierUKPageAbsent     = "uk_page_absent"
	// TierUKPageAmbiguous: the cite string belongs to more than one English
	// Reports case, so there is nothing to link to (#256).
	TierUKPageAmbiguous = "uk_page_ambiguous"
	// TierUKAnachronistic: every case found was decided after the citing
	// document, judged by murrell_year, or er_year where Murrell gives none.
	TierUKAnachronistic = "uk_anachronistic"

	// linked_*: which probe produced the link.
	TierCAPDirect             = "cap_direct"               // cap.citations, under the normalized cite
	TierCAPSharedLongest      = "cap_shared_longest"       // a cite several CAP cases share, linked to the longest of them (#359)
	TierCAPFreelaw            = "cap_freelaw"              // freelaw.cite_to_cap, under the normalized cite
	TierCAPAltSpelling        = "cap_alt_spelling"         // cap.citations, under a reporters_abbreviations alternate
	TierCAPFreelawAltSpelling = "cap_freelaw_alt_spelling" // freelaw.cite_to_cap, under an alternate
	TierCodeDirect            = "code_direct"              // legalhist.code_reporter, under the cleaned cite
	TierERDirect              = "er_direct"                // english_reports.cases
	TierStubDirect            = "stub_direct"              // legalhist.stub_cases, under the cleaned cite (#248)
)
