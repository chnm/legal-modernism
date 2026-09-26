package linker

import (
	"fmt"

	"github.com/lmullen/legal-modernism/go/citations"
)

// linkCitation processes a single citation through the linking pipeline.
// All lookups are in-memory map accesses — no database queries.
func linkCitation(c *citations.UnlinkedCitation, t *Tables) *citations.LinkResult {
	result := &citations.LinkResult{CitationID: c.ID}

	// Step 1: whitelist check. None of the skips records a tier: the status is
	// already the whole explanation, and there was no cascade to reach a tier in.
	entry, ok := t.whitelist[c.ReporterAbbr]
	if !ok {
		result.Status = citations.StatusSkippedNotWhitelisted
		return result
	}
	if entry.Junk {
		result.Status = citations.StatusSkippedJunk
		return result
	}
	// A regnal-year statute ("13 Eliz. c. 5") is a real citation but not to a
	// case, so no source could hold it; skipping it here keeps it out of
	// no_match, where it would read as a coverage gap (issue #246). Checked
	// before routing because the statute rows carry a jurisdiction too.
	if entry.Statute {
		result.Status = citations.StatusSkippedStatute
		return result
	}

	// Past this point entry.ReporterStandard is never nil: non-junk whitelist
	// rows always have a standard reporter, enforced by the
	// chk_whitelist_nonjunk_has_standard constraint. A violation panics at the
	// derefs below rather than silently producing no_match.

	// Step 2: route by UK flag
	if entry.UK {
		return linkEnglishReports(c, entry, t, result)
	}
	return linkCAPThenCode(c, entry, t, result)
}

// linkCAPThenCode tries CAP first, then the FreeLaw parallel-citation crosswalk
// (which also resolves to a CAP case), then both again under alternate reporter
// spellings, then the Code Reporter, all using in-memory maps. The alternate spellings probe per map, not per alt: CAP is
// exhausted across every alternate before FreeLaw is consulted, because the
// source ranking is meaningful (CAP's own citation index over the FreeLaw
// cluster crosswalk, matching the direct-probe order) while the position of an
// alt in its list is not.
func linkCAPThenCode(
	c *citations.UnlinkedCitation,
	entry *citations.WhitelistEntry,
	t *Tables,
	result *citations.LinkResult,
) *citations.LinkResult {

	citeCleaned := buildStandardCite(c, entry)
	citeNormalized := buildCAPCite(c, entry, t.diffvols)
	result.CiteCleaned = &citeCleaned
	result.CiteNormalized = &citeNormalized

	// The cite strings built from this citation's own reporter, in the order they
	// are probed, so a no_match can be attributed to the tier the cascade
	// actually reached rather than to a second, drifting reimplementation of
	// which forms get tried.
	//
	// Alternate spellings are deliberately NOT collected here, though they are
	// probed against the exact maps below (issue #290). buildAltCites bypasses
	// buildCAPCite's reporter_cap and diffvols handling, because an alternate is
	// the other source's own spelling and remapping its volume would be wrong.
	// That makes an alternate's volume number untranslated, which is harmless
	// for an exact probe -- a miss costs nothing -- but not for containment, for
	// exactly the reason diffvolsMissing already suppresses range matching: the
	// wrong volume of the right reporter is densely populated, so containment
	// would confidently return a case from it.
	//
	// The same split once guarded against a second failure. Until issue #289
	// the loader also delivered alternates that were themselves another
	// reporter's reporter_standard ("A.D." under "Am. Dec."; 75 such rows), so
	// a probe under one asked the index about a different reporter. Feeding
	// those to the range index turned them into links: CAP holds no official or
	// nominative cite under "Am. Dec.", "P." or "Paine", so the span index had
	// no key for them at all, and every one of their 281,132 cap_page_interior
	// links came from an alternate; the same probes reaching citeIndex made the
	// failure tiers claim a reporter and volume that belong to some other
	// reporter. LoadReporterAltAbbrs now leaves those rows out, so every
	// alternate that reaches this function is a spelling of this citation's own
	// reporter. The range index and the tier ladder still see only the standard
	// forms, because the volume argument stands on its own.
	//
	// Keeping the alternates out also makes a page-interior link auditable
	// after the fact, which it was not: CiteLinked is nil on those rows, but the
	// link can now only have come from cite_cleaned or cite_normalized, and both
	// are recorded.
	probes := make([]string, 0, 4)

	// The standard-form strings alone, for the stub registry, which is keyed on
	// cite_cleaned: a CAP-spelled or volume-translated form is a different
	// reporter's string as far as the registry is concerned.
	standard := make([]string, 0, 2)

	// Every hit, exact or by page range, must also pass the year test: a case
	// decided after the citing document, a treatise volume or an opinion's
	// case, cannot be the one it cites (issue #319). A refused hit is passed over rather than returned on, so
	// the rest of the cascade still runs and can reach a case of the right date.
	gate := newYearGate(c)

	// Run the whole cascade for the form we detected before trying the volume
	// variant, so an existing link can never be rewired: the variant only ever
	// turns a no_match into a link.
	for _, f := range volumeForms(c, entry) {
		cleaned := buildStandardCite(f, entry)
		normalized := buildCAPCite(f, entry, t.diffvols)
		probes = append(probes, normalized, cleaned)
		standard = append(standard, cleaned)

		// Try CAP with the normalized cite
		if caseID, ok := t.capCites[normalized]; ok && admits(gate, t.years.CAP, caseID) {
			result.Status = citations.StatusLinkedCAP
			result.MatchTier = citations.TierCAPDirect
			result.CAPCaseID = &caseID
			result.CiteLinked = &normalized
			return result
		}

		// Fall back to the FreeLaw crosswalk: if any parallel form of this decision
		// is in our CAP data, this reaches the CAP case from the form we detected.
		// The result is still a CAP link (status linked_cap), distinguished from a
		// direct hit only by the tier.
		if caseID, ok := t.freelawCites[normalized]; ok && admits(gate, t.years.CAP, caseID) {
			result.Status = citations.StatusLinkedCAP
			result.MatchTier = citations.TierCAPFreelaw
			result.CAPCaseID = &caseID
			result.CiteLinked = &normalized
			return result
		}

		// Fall back to alternate reporter spellings: the same decision may be in
		// CAP or the FreeLaw crosswalk under a spelling that differs from our
		// reporter_standard/reporter_cap. Probe each known alternate spelling for
		// this reporter (keyed by the canonical reporter_standard, like diffvols)
		// against CAP first, then all of them against FreeLaw. A hit links to the
		// CAP case (status linked_cap).
		altCites := buildAltCites(f, t.altAbbrs[*entry.ReporterStandard])
		for i := range altCites {
			if caseID, ok := t.capCites[altCites[i]]; ok && admits(gate, t.years.CAP, caseID) {
				result.Status = citations.StatusLinkedCAP
				result.MatchTier = citations.TierCAPAltSpelling
				result.CAPCaseID = &caseID
				result.CiteLinked = &altCites[i]
				return result
			}
		}
		for i := range altCites {
			if caseID, ok := t.freelawCites[altCites[i]]; ok && admits(gate, t.years.CAP, caseID) {
				result.Status = citations.StatusLinkedCAP
				result.MatchTier = citations.TierCAPFreelawAltSpelling
				result.CAPCaseID = &caseID
				result.CiteLinked = &altCites[i]
				return result
			}
		}

		// Try Code Reporter with the cleaned cite. There is deliberately no
		// alternate-spelling probe here: it never produced a link in the whole
		// history of the table, and legalhist.code_reporter holds 633 rows of
		// one New York series, so an alternate reporter spelling has nothing to
		// reach (issue #292).
		if codeID, ok := t.codeCites[cleaned]; ok && admits(gate, t.years.Code, codeID) {
			result.Status = citations.StatusLinkedCodeReporter
			result.MatchTier = citations.TierCodeDirect
			result.CodeReporterID = &codeID
			result.CiteLinked = &cleaned
			return result
		}
	}

	// Every exact form missed. Before giving up, try page-range matching: the
	// citation may be a pin cite to an interior page of a case, which no
	// first-page cite string can ever equal. This runs last so it can only turn a
	// no_match into a link, never rewire one the exact cascade already made.
	//
	// Skipped when diffvols is missing, for the same reason usTier reports that
	// tier ahead of the volume and page ones: the reporter renumbers in CAP and no
	// reporters_diffvols row covers this volume, so every probe carries a volume
	// number known to be untranslated. An exact miss on such a probe is harmless,
	// but a range hit is not — the wrong volume of the right reporter is densely
	// populated, so containment would confidently return a case from it. Measured
	// over the current no_match pool this suppresses 45,077 otherwise-plausible
	// links that would all have been fabricated.
	missingDiffvols := diffvolsMissing(c, entry, t.diffvols)
	span := rangeMiss
	if t.capRanges != nil && !missingDiffvols {
		caseID, outcome := t.capRanges.probe(probes)
		if outcome == rangeHit && admits(gate, t.years.CAP, caseID) {
			result.Status = citations.StatusLinkedCAP
			result.MatchTier = citations.TierCAPPageInterior
			result.CAPCaseID = &caseID
			// No cite string matched, so there is nothing to record as the cite
			// that linked; CiteLinked stays nil and the tier is what identifies
			// how this row was made.
			return result
		}
		// A refusal is not a link but is still a finding, so it is carried to
		// usTier to sharpen the page step rather than dropped.
		span = outcome
	}

	// A case was found but refused as anachronistic. That outranks every tier
	// below, each of which says no case was there to be found, and it rules
	// out a stub, which stands in only for a reporter no source holds.
	if gate.refused {
		result.Status = citations.StatusNoMatch
		result.MatchTier = citations.TierUSAnachronistic
		return result
	}

	// Last of all, the stub registry (issue #248), and only when the failure
	// would be reporter_absent: no probed spelling of this reporter is in any US
	// source, so there is no case this could have been and nothing a stub can
	// outrank. A deeper tier means a source does hold the reporter, and its
	// misses are coverage gaps or pin cites, which the registry is built to
	// exclude -- checking the tier here rather than trusting the registry keeps
	// that true even when the registry is stale.
	tier := usTier(probes, t.us, missingDiffvols, span)
	if tier == citations.TierUSReporterAbsent {
		if cite, ok := t.stubs.lookup(standard); ok {
			return linkStub(result, cite)
		}
	}

	result.Status = citations.StatusNoMatch
	result.MatchTier = tier
	return result
}

// diffvolsMissing reports whether this citation's reporter renumbers its volumes
// in CAP but no legalhist.reporters_diffvols row covers the cited volume — the
// case where buildCAPCite has to fall back to the untranslated volume number, so
// every probe built from it is a guess. A volume-less citation is not counted:
// there is no volume to translate, and buildCAPCite does not consult diffvols for
// one either.
func diffvolsMissing(c *citations.UnlinkedCitation, entry *citations.WhitelistEntry, diffvols map[string]map[int]*citations.DiffVolEntry) bool {
	if !entry.CAPDifferent || c.Volume == nil {
		return false
	}
	_, ok := diffvols[*entry.ReporterStandard][*c.Volume]
	return !ok
}

// linkEnglishReports tries to link a UK citation to the English Reports
// using an in-memory map.
func linkEnglishReports(
	c *citations.UnlinkedCitation,
	entry *citations.WhitelistEntry,
	t *Tables,
	result *citations.LinkResult,
) *citations.LinkResult {
	citeCleaned := buildStandardCite(c, entry)
	result.CiteCleaned = &citeCleaned
	result.CiteNormalized = &citeCleaned

	probes := make([]string, 0, 2)

	// Set when a probe matches a cite string that several English Reports cases
	// share. Recorded rather than returned on, because it is not a reason to stop
	// probing: on a single-volume reporter one volume form can collide while the
	// other resolves cleanly, and a real link is better evidence than a refusal.
	// Only after every form has missed does it decide the tier.
	ambiguous := false

	// The year test, as on the US route: a refused hit is passed over, and
	// decides the tier only if nothing else links.
	gate := newYearGate(c)

	// The English Reports are inconsistent about the redundant volume on
	// single-volume nominate reporters: most are stored bare ("Cro Eliz 1") but
	// some carry it ("1 Vern 1"), so try both forms.
	for _, f := range volumeForms(c, entry) {
		cite := buildStandardCite(f, entry)
		probes = append(probes, cite)
		er, ok := t.erCites[cite]
		if !ok {
			continue
		}
		if er.Ambiguous {
			ambiguous = true
			continue
		}
		if !admits(gate, t.years.ER, er.ID) {
			continue
		}
		result.Status = citations.StatusLinkedEnglishReports
		result.MatchTier = citations.TierERDirect
		result.ERCaseID = &er.ID
		result.CiteLinked = &cite
		return result
	}

	// As on the US route, fall back to page-range matching for pin cites. The
	// English Reports record no page ranges of their own, so spans here are
	// bounded only by the next cite and maxSpanPages.
	span := rangeMiss
	if t.erRanges != nil {
		erID, outcome := t.erRanges.probe(probes)
		if outcome == rangeHit && admits(gate, t.years.ER, erID) {
			result.Status = citations.StatusLinkedEnglishReports
			result.MatchTier = citations.TierERPageInterior
			result.ERCaseID = &erID
			return result
		}
		span = outcome
	}

	// A refused case outranks the failure ladder and the stub registry, as on
	// the US route. It also outranks an ambiguous cite: one form found a single
	// case, and only its date ruled it out.
	if gate.refused {
		result.Status = citations.StatusNoMatch
		result.MatchTier = citations.TierUKAnachronistic
		return result
	}

	// The stub registry, under the same gate as the US route: the probes here
	// are already the standard forms the registry is keyed on.
	tier := ukTier(probes, t.uk, ambiguous, span)
	if tier == citations.TierUKReporterAbsent {
		if cite, ok := t.stubs.lookup(probes); ok {
			return linkStub(result, cite)
		}
	}

	result.Status = citations.StatusNoMatch
	result.MatchTier = tier
	return result
}

// volumeForms returns the citation forms to probe, most-faithful first. For a
// single-volume reporter "Toth 123" and "1 Toth 123" are the same citation --
// such reports were often cited with a redundant volume 1 even though there is
// only one volume to cite -- so both are tried. Everything else yields just the
// citation as detected.
//
// The variant is a copy of the citation with the volume flipped rather than a
// rewritten string, so buildStandardCite and buildCAPCite handle it with their
// existing reporter_cap and diffvols logic. That also reaches diffvols entries
// for volume 1, which buildCAPCite skips when the volume is nil.
func volumeForms(c *citations.UnlinkedCitation, entry *citations.WhitelistEntry) []*citations.UnlinkedCitation {
	forms := []*citations.UnlinkedCitation{c}
	if !entry.SingleVol {
		return forms
	}

	variant := *c
	switch {
	case c.Volume == nil:
		one := 1
		variant.Volume = &one
	case *c.Volume == 1:
		variant.Volume = nil
	default:
		// Volume 2 or higher is a real volume number, not a redundant 1, so
		// there is nothing equivalent to try.
		return forms
	}
	return append(forms, &variant)
}

// buildAltCites constructs the alternate-spelling cite strings for a citation
// form, one per alternate abbreviation, in order; nil when there are none.
// Volume-nil is handled the same way as buildStandardCite. The alternates
// deliberately bypass buildCAPCite's reporter_cap/diffvols handling: they are
// the other source's own spellings, so remapping their volumes would be wrong.
func buildAltCites(c *citations.UnlinkedCitation, alts []string) []string {
	if len(alts) == 0 {
		return nil
	}
	altCites := make([]string, len(alts))
	for i, alt := range alts {
		if c.Volume == nil {
			altCites[i] = fmt.Sprintf("%s %d", alt, c.Page)
		} else {
			altCites[i] = fmt.Sprintf("%d %s %d", *c.Volume, alt, c.Page)
		}
	}
	return altCites
}

// buildStandardCite constructs "{volume} {reporter_standard} {page}", led by
// the year for a reporter cited by year: "[1905] 2 K.B. 1". See yearPrefix.
func buildStandardCite(c *citations.UnlinkedCitation, entry *citations.WhitelistEntry) string {
	if c.Volume == nil {
		return yearPrefix(c, entry) + fmt.Sprintf("%s %d", *entry.ReporterStandard, c.Page)
	}
	return yearPrefix(c, entry) + fmt.Sprintf("%d %s %d", *c.Volume, *entry.ReporterStandard, c.Page)
}

// yearPrefix is the "[1905] " that leads the cite string of a citation to a
// reporter cited by year, when the citation carries a year from the reporter's
// cited_by_year_from on, and "" otherwise. From that year the volume restarts
// every year, so without the year the string names a different case for every
// year of the series (issue #312). The year goes into the string rather than
// into a separate probe because the string is what every target is keyed on
// -- the stub registry above all, which is where these citations end up, no
// source holding the reporter -- and what cite_cleaned records for the row.
//
// A year before cited_by_year_from, or on a reporter that is never cited by
// year, is decoration on a volume-cited citation -- "(1889) 14 App. Cas. 337",
// "(1845) 7 Q. B. 100" -- and stays out of the string, which would otherwise
// split one case's citations in two. That is what lets one reporter row hold a
// volume-cited series and its year-cited continuation: the Appeal Cases and
// A.C., the Queen's Bench of 1841-1852 and the Q.B. of 1891 on (issue #314).
func yearPrefix(c *citations.UnlinkedCitation, entry *citations.WhitelistEntry) string {
	if entry.CitedByYearFrom == 0 || c.Year == nil || *c.Year < entry.CitedByYearFrom {
		return ""
	}
	return fmt.Sprintf("[%d] ", *c.Year)
}

// buildCAPCite constructs the citation string appropriate for CAP lookup,
// handling reporters with different volume numbering.
func buildCAPCite(c *citations.UnlinkedCitation, entry *citations.WhitelistEntry, diffvols map[string]map[int]*citations.DiffVolEntry) string {
	// If this reporter uses different volume numbers in CAP, try the diffvols mapping
	if entry.CAPDifferent && c.Volume != nil {
		if vols, ok := diffvols[*entry.ReporterStandard]; ok {
			if dv, ok := vols[*c.Volume]; ok {
				return yearPrefix(c, entry) + fmt.Sprintf("%d %s %d", dv.CAPVol, dv.CAPReporter, c.Page)
			}
		}
	}

	// Use reporter_cap if available, otherwise fall back to reporter_standard
	reporter := *entry.ReporterStandard
	if entry.ReporterCAP != nil {
		reporter = *entry.ReporterCAP
	}

	if c.Volume == nil {
		return yearPrefix(c, entry) + fmt.Sprintf("%s %d", reporter, c.Page)
	}
	return yearPrefix(c, entry) + fmt.Sprintf("%d %s %d", *c.Volume, reporter, c.Page)
}
