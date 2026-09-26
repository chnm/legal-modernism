package linker

import (
	"context"
	"fmt"

	"github.com/lmullen/legal-modernism/go/citations"
)

// Years holds the years of the cases a citation can link to, which decide
// whether a link is anachronistic (issue #319): a document cannot cite a case
// decided after it was published, so a hit on such a case is a wrong link,
// however exactly the cite string matched. The citing document's own year
// arrives on the citation as SourceYear. A nil map holds no years, so every
// lookup in it misses and every link is kept.
type Years struct {
	CAP  map[int64]int  // cap.cases.decision_year
	Code map[int64]int  // legalhist.code_reporter.decision_year
	ER   map[string]int // english_reports.cases.murrell_year, else er_year
}

// LoadYears loads every table Years holds.
func LoadYears(ctx context.Context, store citations.LinkerStore) (Years, error) {
	var y Years
	var err error
	if y.CAP, err = store.LoadCAPCaseYears(ctx); err != nil {
		return y, fmt.Errorf("CAP case years: %w", err)
	}
	if y.Code, err = store.LoadCodeReporterYears(ctx); err != nil {
		return y, fmt.Errorf("code reporter years: %w", err)
	}
	if y.ER, err = store.LoadERCaseYears(ctx); err != nil {
		return y, fmt.Errorf("English Reports case years: %w", err)
	}
	return y, nil
}

// yearGate tests one citation's hits against the year of the document it was
// found in, and remembers whether it refused any. The cascade needs the memory
// because a refusal is not a reason to stop probing: a later volume form or
// target can still reach a case of the right date. Only once every probe has
// failed does the refusal decide the tier, and it outranks the failure ladder,
// whose tiers all claim that no case was there to be found.
type yearGate struct {
	published int  // the citing document's year: the treatise volume's, or the citing case's
	known     bool // false when the citation carries no SourceYear
	refused   bool // set once any hit has been refused
}

func newYearGate(c *citations.UnlinkedCitation) *yearGate {
	if c.SourceYear == nil {
		return &yearGate{}
	}
	return &yearGate{published: *c.SourceYear, known: true}
}

// admits reports whether a hit on case id may link. It refuses only a case
// decided in a later year than the citing document: a treatise and a case of
// the same year are not anachronistic, since the book could have gone to press
// after the decision, and neither is an opinion and a case of the same year,
// which is also what admits an opinion's citation of its own case. An unknown
// year on either side is no evidence against the link, so it is admitted.
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
