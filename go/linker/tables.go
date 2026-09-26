// Package linker links detected citations to cases: the cascade that turns a
// citation's reporter spelling, volume and page into a CAP, English Reports,
// code reporter or stub case, or into a no_match with the tier it failed at.
// It is shared by cite-linker, which links the citations detected in the MOML
// treatises, and cite-linker-cap, which links those detected in CAP opinions
// (issue #74). The cascade never reads which corpus a citation came from: the
// one corpus-specific fact it needs, the year of the citing document for the
// anachronism rule (issue #319), arrives on the citation itself.
package linker

import (
	"fmt"

	"github.com/lmullen/legal-modernism/go/citations"
)

// Tables holds every in-memory lookup table the cascade reads, together with
// the derived indexes that let a failure report which tier it reached. It is
// built once at startup and never written to afterwards, which is what makes it
// safe to share across the insert workers.
type Tables struct {
	whitelist    map[string]*citations.WhitelistEntry
	diffvols     map[string]map[int]*citations.DiffVolEntry
	capCites     map[string]int64
	freelawCites map[string]int64
	altAbbrs     map[string][]string
	codeCites    map[string]int64
	erCites      map[string]citations.ERCase

	// us indexes the three maps the US cascade probes as a unit; uk indexes the
	// English Reports. Building them walks every cite string once, which is why
	// it happens here rather than per citation.
	us *citeIndex
	uk *citeIndex

	// capRanges and erRanges resolve pin cites — citations to an interior page of
	// a case — after the exact cascade has missed. Either may be nil, in which
	// case range matching is simply skipped.
	capRanges *rangeIndex[int64]
	erRanges  *rangeIndex[string]

	// stubs is the registry of cases no source holds (legalhist.stub_cases),
	// probed last and only for a citation whose reporter no source knows. It
	// is deliberately kept out of the us/uk cite indexes: a stub is evidence
	// that a case exists, not a source that holds it, and counting it as a
	// reached reporter would make reporter_absent -- the very condition a stub
	// depends on -- impossible to report. May be nil.
	stubs stubIndex

	// years refuses a hit on a case decided after the citing document, a
	// treatise volume or an opinion's case (issue #319). Its zero value holds
	// no years and refuses nothing.
	years Years
}

// NewTables assembles the linking tables from the loaded lookups, building the
// cite and page-range indexes as it goes. Load is the usual caller; tests
// build them directly, passing nil for what they do not need.
func NewTables(
	whitelist map[string]*citations.WhitelistEntry,
	diffvols map[string]map[int]*citations.DiffVolEntry,
	capCites map[string]int64,
	freelawCites map[string]int64,
	altAbbrs map[string][]string,
	codeCites map[string]int64,
	erCites map[string]citations.ERCase,
	capSpans []citations.CaseSpan[int64],
	erSpans []citations.CaseSpan[string],
	stubs map[string]struct{},
	years Years,
) *Tables {
	return &Tables{
		whitelist:    whitelist,
		diffvols:     diffvols,
		capCites:     capCites,
		freelawCites: freelawCites,
		altAbbrs:     altAbbrs,
		codeCites:    codeCites,
		erCites:      erCites,
		us:           newCiteIndex(capCites, freelawCites, codeCites),
		uk:           newCiteIndex(erCites),
		capRanges:    newRangeIndex(capSpans),
		erRanges:     newRangeIndex(erSpans),
		stubs:        stubIndex(stubs),
		years:        years,
	}
}

// Link runs the cascade over one citation. All lookups are in-memory map
// accesses on the tables; no database is touched.
func (t *Tables) Link(c *citations.UnlinkedCitation) *citations.LinkResult {
	return linkCitation(c, t)
}

// Stats reports the sizes of the indexes the tables hold, for the startup log:
// how many reporters and reporter-volumes the US and UK cite indexes know, and
// how many reporter-volumes and spans the two page-range indexes hold.
type Stats struct {
	USReporters, USVolumes int
	UKReporters, UKVolumes int
	CAPVolumes, CAPSpans   int
	ERVolumes, ERSpans     int
}

// Stats returns the sizes of the indexes.
func (t *Tables) Stats() Stats {
	s := Stats{
		USReporters: len(t.us.reporters), USVolumes: len(t.us.volumes),
		UKReporters: len(t.uk.reporters), UKVolumes: len(t.uk.volumes),
	}
	if t.capRanges != nil {
		s.CAPVolumes, s.CAPSpans = t.capRanges.size()
	}
	if t.erRanges != nil {
		s.ERVolumes, s.ERSpans = t.erRanges.size()
	}
	return s
}

// Check verifies the invariants the page-range indexes must satisfy by
// construction. A malformed span index mislinks silently and at scale, so a
// driver calls this before linking anything.
func (t *Tables) Check() error {
	if t.capRanges != nil {
		if err := t.capRanges.checkSelfConsistency(); err != nil {
			return fmt.Errorf("cap page span index is inconsistent: %w", err)
		}
	}
	if t.erRanges != nil {
		if err := t.erRanges.checkSelfConsistency(); err != nil {
			return fmt.Errorf("er page span index is inconsistent: %w", err)
		}
	}
	return nil
}
