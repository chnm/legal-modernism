package main

import (
	"context"
	"fmt"

	"github.com/lmullen/legal-modernism/go/citations"
)

// caseYears holds the years that decide whether a link is anachronistic (issue
// #319): a treatise cannot cite a case decided after it was published, so a hit
// on such a case is a wrong link, however exactly the cite string matched. A nil
// map holds no years, so every lookup in it misses and every link is kept.
type caseYears struct {
	treatise map[string]int // moml.volumes.year, by psmid
	cap      map[int64]int  // cap.cases.decision_year
	code     map[int64]int  // legalhist.code_reporter.decision_year
	er       map[string]int // english_reports.cases.murrell_year, else er_year
}

// loadCaseYears loads every table caseYears holds.
func loadCaseYears(ctx context.Context, store citations.LinkerStore) (caseYears, error) {
	var y caseYears
	var err error
	if y.treatise, err = store.LoadTreatiseYears(ctx); err != nil {
		return y, fmt.Errorf("treatise years: %w", err)
	}
	if y.cap, err = store.LoadCAPCaseYears(ctx); err != nil {
		return y, fmt.Errorf("CAP case years: %w", err)
	}
	if y.code, err = store.LoadCodeReporterYears(ctx); err != nil {
		return y, fmt.Errorf("code reporter years: %w", err)
	}
	if y.er, err = store.LoadERCaseYears(ctx); err != nil {
		return y, fmt.Errorf("English Reports case years: %w", err)
	}
	return y, nil
}

// yearGate tests one citation's hits against the year its treatise volume was
// published, and remembers whether it refused any. The cascade needs the memory
// because a refusal is not a reason to stop probing: a later volume form or
// target can still reach a case of the right date. Only once every probe has
// failed does the refusal decide the tier, and it outranks the failure ladder,
// whose tiers all claim that no case was there to be found.
type yearGate struct {
	published int  // the treatise volume's year
	known     bool // false when moml.volumes has no year for the volume
	refused   bool // set once any hit has been refused
}

func newYearGate(c *citations.UnlinkedCitation, years caseYears) *yearGate {
	published, known := years.treatise[c.MomlTreatise]
	return &yearGate{published: published, known: known}
}

// admits reports whether a hit on case id may link. It refuses only a case
// decided in a later year than the treatise was published: a treatise and a
// case of the same year are not anachronistic, since the book could have gone
// to press after the decision. An unknown year on either side is no evidence
// against the link, so it is admitted.
func admits[K comparable](g *yearGate, years map[K]int, id K) bool {
	if !g.known {
		return true
	}
	decided, ok := years[id]
	if ok && g.published < decided {
		g.refused = true
		return false
	}
	return true
}
