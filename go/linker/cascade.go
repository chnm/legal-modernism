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

	// A spelling with two readings is read first as the one of the citing
	// document's jurisdiction (issue #401).
	first, second := readings(c, entry)

	// Step 2: route by UK flag
	result = route(c, first, t, result)

	// Step 3: a spelling that can mean another reporter tries that one, but
	// only after a miss that says this was the wrong reporter (issue #358).
	if second != nil && result.Status == citations.StatusNoMatch && fallsBack(result.MatchTier) {
		fb := route(c, second, t, &citations.LinkResult{CitationID: c.ID})
		if linkedToSource(fb.Status) {
			return fb
		}
	}
	return result
}

// readings returns the order in which a spelling's readings are tried: its
// own reporter, then its fallback (or nil). When the citing document is of
// the fallback's jurisdiction and not of the spelling's own, the two trade
// places (issue #401). "2 Hill 93" in a New York opinion is Hill's New York
// Reports, not Hill's three volumes of South Carolina Law Reports that bare
// "Hill" names elsewhere, and a South Carolina reading of a New York cite
// that happens to land on a page some S.C. case begins on is a wrong link
// the fallback could never undo, since the fallback runs only after a miss.
// The citing jurisdiction is known only for a CAP opinion; a treatise's is
// "", which never reorders.
func readings(c *citations.UnlinkedCitation, entry *citations.WhitelistEntry) (first, second *citations.WhitelistEntry) {
	fb := entry.Fallback
	if fb == nil {
		return entry, nil
	}
	home := c.SourceJurisdiction
	if home != "" && fb.Jurisdiction == home && entry.Jurisdiction != home {
		return fb, entry
	}
	return entry, fb
}

// route sends a citation down the English Reports or the US cascade by its
// reporter's jurisdiction.
func route(c *citations.UnlinkedCitation, entry *citations.WhitelistEntry, t *Tables, result *citations.LinkResult) *citations.LinkResult {
	if entry.UK {
		return linkEnglishReports(c, entry, t, result)
	}
	return linkCAPThenCode(c, entry, t, result)
}

// fallsBack reports whether a no_match tier says the citation probably names
// some other reporter, so that a spelling's fallback reading is worth trying.
// The volume is not one the reporter has, or its numbering has no row for it,
// or the case found was decided after the citing document: "10 Howard" cannot
// be Howard's seven volumes of Mississippi Reports, and "1 Stew." in an 1850
// Alabama opinion cannot be Stewart's New Jersey Equity Reports of 1877. A page
// tier is no such evidence, since the reporter and volume were found, and
// probing another reporter at that volume and page would be a guess.
func fallsBack(tier string) bool {
	switch tier {
	case citations.TierUSDiffVolsMissing, citations.TierUSVolumeAbsent, citations.TierUSAnachronistic,
		citations.TierUKVolumeAbsent, citations.TierUKAnachronistic:
		return true
	}
	return false
}

// linkedToSource reports whether a status is a link to a case some source holds.
// A fallback reading's stub link does not count: the registry is built from
// the misses of each spelling's own reading, so a stub reached through the
// fallback is not evidence that the fallback is what the citation meant.
func linkedToSource(status string) bool {
	switch status {
	case citations.StatusLinkedCAP, citations.StatusLinkedEnglishReports, citations.StatusLinkedCodeReporter:
		return true
	}
	return false
}

// linkCAPThenCode tries CAP first, then the FreeLaw parallel-citation crosswalk
// (which also resolves to a CAP case), then both under the standard spelling
// when the first probe was built from another (a diffvols translation or
// reporter_cap), then both again under alternate reporter spellings, then the
// Code Reporter, all using in-memory maps. The alternate spellings probe per map, not per alt: CAP is
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
	// the other source's own spelling and remapping its volume would be wrong,
	// so a probe under one says nothing about which reporter and volume this
	// citation's own spelling reached. Until issue #289 the loader also
	// delivered alternates that were themselves another reporter's
	// reporter_standard ("A.D." under "Am. Dec."; 75 such rows), and the same
	// probes reaching citeIndex made the failure tiers claim a reporter and
	// volume that belong to some other reporter. LoadReporterAltAbbrs now
	// leaves those rows out, so every alternate that reaches this function is a
	// spelling of this citation's own reporter; the tier ladder still sees only
	// the standard forms.
	probes := make([]string, 0, 4)

	// The standard-form strings alone, for the stub registry, which is keyed on
	// cite_cleaned: a CAP-spelled or volume-translated form is a different
	// reporter's string as far as the registry is concerned.
	standard := make([]string, 0, 2)

	// Every hit must also pass the year test: a case
	// decided after the citing document, a treatise volume or an opinion's
	// case, cannot be the one it cites (issue #319). A refused hit is passed over rather than returned on, so
	// the rest of the cascade still runs and can reach a case of the right date.
	gate := newYearGate(c)

	// Set when an exact probe reached a cite several CAP cases share and more
	// than one of them could be the case cited, so the miss is reported as an
	// ambiguity rather than as a page no case begins on (issue #359).
	ambiguous := false

	// Run the whole cascade for the form we detected before trying the volume
	// variant, so an existing link can never be rewired: the variant only ever
	// turns a no_match into a link.
	for _, f := range volumeForms(c, entry) {
		cleaned := buildStandardCite(f, entry)
		normalized := buildCAPCite(f, entry, t.diffvols)
		probes = append(probes, normalized, cleaned)
		standard = append(standard, cleaned)

		// When the reporter renumbers in CAP and no diffvols row covers this
		// volume, normalized carries the untranslated volume under the CAP
		// spelling, so a hit on it is some other case of that reporter rather
		// than the one cited (issue #357): "18 South." (the Southern Reporter)
		// was probed as "18 N.J.L." and linked to an 1841 New Jersey case. This
		// made 24,708 CAP-ledger and 15,619 MOML links, and CAP's citation graph
		// agreed with none of the largest groups. The CAP and FreeLaw probes on
		// it are skipped; the alternate spellings and the Code Reporter still
		// run, since they are probed in their own spellings.
		untranslated := diffvolsMissing(f, entry, t.diffvols)

		// Try CAP with the normalized cite
		if caseID, ok := t.capCites[normalized]; ok && !untranslated && admits(gate, t.years.CAP, caseID) {
			result.Status = citations.StatusLinkedCAP
			result.MatchTier = citations.TierCAPDirect
			result.CAPCaseID = &caseID
			result.CiteLinked = &normalized
			return result
		}

		// A cite several CAP cases share, which capCites leaves out (issue #359).
		// The cases that begin on one page are nearly always a decision of some
		// length and one or more memorandum decisions of a page or less, printed
		// in the space the longer one leaves: 89 Ga. 645 is Reese v. Reese, nine
		// pages, beside Summers v. State, one. A citation to the page means the
		// decision that fills it, so the citation links to the longest of the
		// cases the year gate admits; the gate is applied here as it is to every
		// other hit, not as the tie-breaker. When the longest is not unique, as
		// in a table of one-page decisions (61 Kan. 859 holds eleven), nothing in
		// the cite tells them apart: an honest ambiguity, and the cascade goes on
		// in case another form or source resolves it. Skipped for an untranslated
		// diffvols volume, for the same reason the direct probe is: the cite
		// belongs to some other volume.
		if shared, ok := t.capShared[normalized]; ok && !untranslated {
			left := admitted(gate, t.years.CAP, shared)
			if caseID, ok := longest(left); ok {
				result.Status = citations.StatusLinkedCAP
				result.MatchTier = citations.TierCAPSharedLongest
				result.CAPCaseID = &caseID
				result.CiteLinked = &normalized
				return result
			} else if len(left) > 1 {
				ambiguous = true
			}
		}

		// Fall back to the FreeLaw crosswalk: if any parallel form of this decision
		// is in our CAP data, this reaches the CAP case from the form we detected.
		// The result is still a CAP link (status linked_cap), distinguished from a
		// direct hit only by the tier.
		if caseID, ok := t.freelawCites[normalized]; ok && !untranslated && admits(gate, t.years.CAP, caseID) {
			result.Status = citations.StatusLinkedCAP
			result.MatchTier = citations.TierCAPFreelaw
			result.CAPCaseID = &caseID
			result.CiteLinked = &normalized
			return result
		}

		// The standard spelling itself, when the probes above were built
		// from another one (a diffvols translation or reporter_cap). CAP
		// keys a case under its nominative cites as well as its official
		// one -- "1 Allen 456" beside "83 Mass. 456" -- and spells those the
		// way reporter_standard does, so this is the one spelling CAP shares
		// with us; until now it was in probes, so the tier ladder counted it
		// as tried, but no exact probe ever ran it
		// (issue #371). It comes after the translated form, so a link that
		// form makes is unchanged, and before the alternates, which are
		// weaker evidence than the standard. A hit records cite_linked equal
		// to cite_cleaned, which is what tells it from a hit under
		// cite_normalized. The untranslated reporter_cap probe that runs
		// ahead of this when no diffvols row covers the volume is issue
		// #357; once it is gone, this is the probe that stands in its place.
		if cleaned != normalized {
			if caseID, ok := t.capCites[cleaned]; ok && admits(gate, t.years.CAP, caseID) {
				result.Status = citations.StatusLinkedCAP
				result.MatchTier = citations.TierCAPDirect
				result.CAPCaseID = &caseID
				result.CiteLinked = &cleaned
				return result
			}
			if caseID, ok := t.freelawCites[cleaned]; ok && admits(gate, t.years.CAP, caseID) {
				result.Status = citations.StatusLinkedCAP
				result.MatchTier = citations.TierCAPFreelaw
				result.CAPCaseID = &caseID
				result.CiteLinked = &cleaned
				return result
			}
		}

		// Fall back to alternate reporter spellings: the same decision may be in
		// CAP or the FreeLaw crosswalk under a spelling that differs from our
		// reporter_standard/reporter_cap. Probe each known alternate spelling for
		// this reporter (keyed by the canonical reporter_standard, like diffvols)
		// against CAP first, then all of them against FreeLaw. A hit links to the
		// CAP case (status linked_cap).
		altCites := buildAltCites(f, entry, t.altAbbrs[*entry.ReporterStandard])
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

	// Every exact form missed. A citation to an interior page of a case -- a pin
	// cite -- is left a miss on purpose: page-range matching once linked those
	// (issues #242 and #243), and most of the links were to the wrong case, so
	// it was removed (issue #393).

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
	tier := usTier(probes, t.us, diffvolsMissing(c, entry, t.diffvols), ambiguous)
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
	tier := ukTier(probes, t.uk, ambiguous)
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
// Volume-nil is handled the same way as buildStandardCite, and so is the year:
// a citation to a reporter cited by year leads with it, "[1905] 2 KB 1", as
// every other probe of such a reporter does (see yearPrefix). Without the
// year the string named a different reporter as far as every index is
// concerned (issue #372). The alternates deliberately bypass buildCAPCite's
// reporter_cap/diffvols handling: they are the other source's own spellings,
// so remapping their volumes would be wrong.
func buildAltCites(c *citations.UnlinkedCitation, entry *citations.WhitelistEntry, alts []string) []string {
	if len(alts) == 0 {
		return nil
	}
	year := yearPrefix(c, entry)
	altCites := make([]string, len(alts))
	for i, alt := range alts {
		if c.Volume == nil {
			altCites[i] = year + fmt.Sprintf("%s %d", alt, c.Page)
		} else {
			altCites[i] = year + fmt.Sprintf("%d %s %d", *c.Volume, alt, c.Page)
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
