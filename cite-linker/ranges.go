package main

import (
	"fmt"
	"sort"

	"github.com/lmullen/legal-modernism/go/citations"
)

// maxSpanPages bounds how far past its own first page a case may be assumed to
// run when the source gives no page count of its own — the English Reports, which
// record no page range at all, and the 0.05% of CAP cases with a NULL first or
// last page. Without a bound, a case sitting in front of a hole in the corpus
// silently swallows the whole hole. 40 is just above the 99th percentile of the
// distance between consecutive CAP cite pages (37), so it rejects holes without
// truncating real cases.
//
// It is a substitute for a missing length, not a ceiling on a known one: a case
// whose length the source records is bounded by that length however long it is.
// See boundFor.
const maxSpanPages = 40

// rangeOutcome is what a page-range lookup concluded.
type rangeOutcome int

const (
	// rangeMiss: nothing to say. The volume is not indexed, or the page falls
	// before its first cite. The caller falls back to the reporter/volume tiers.
	rangeMiss rangeOutcome = iota
	// rangeHit: exactly one case's span covers the page.
	rangeHit
	// rangeAmbiguous: the covering span belongs to more than one case, because
	// several decisions begin on its first page.
	rangeAmbiguous
	// rangeGap: the page is past the end of the preceding case but before the
	// next one — a hole in the corpus, not an interior page.
	rangeGap
)

// span is one case's claim on a run of pages within a reporter-volume, expressed
// entirely in the pagination the citation uses.
type span[ID comparable] struct {
	page   int  // the first page cite for this case
	bound  int  // pages the case owns, starting at page; 0 when unbounded
	id     ID   // the case, meaningful only when !shared
	shared bool // more than one case begins on this page
}

// rangeIndex answers "which case does page N of this reporter-volume belong to?"
// for pages that are not themselves first-page cites — pin cites, which the exact
// cascade can never match.
//
// Spans are derived from cite strings alone, never from cap.cases.first_page.
// Those two are different paginations: first_page describes the scanned printing,
// and for ~2,770 CAP volumes it drifts from the pagination the citation uses, so
// containment on it attaches pin cites to the wrong case with full confidence
// (measured: right 99.4% of the time in volumes where the two agree, wrong 27.5%
// of the time in volumes where they do not). Working in citation space makes the
// offset irrelevant.
//
// The case's own page count still gets used, but only as a *length*: a length is
// a difference between two pages in one system, so it survives the offset that an
// absolute end page does not.
type rangeIndex[ID comparable] struct {
	volumes map[string][]span[ID]
}

// newRangeIndex builds the index from one first-page cite per case per keying.
// Cite strings it cannot parse are skipped, exactly as newCiteIndex skips them,
// so the index and the cascade's probes can never disagree about what a cite is.
func newRangeIndex[ID comparable](spans []citations.CaseSpan[ID]) *rangeIndex[ID] {
	byVolume := make(map[string][]span[ID])
	for _, s := range spans {
		vol, reporter, page, ok := splitCite(s.Cite)
		if !ok {
			continue
		}
		key := volumeKey(vol, reporter)
		byVolume[key] = append(byVolume[key], span[ID]{
			page:  page,
			bound: s.Length,
			id:    s.ID,
		})
	}

	for key, list := range byVolume {
		sort.Slice(list, func(i, j int) bool { return list[i].page < list[j].page })

		// Collapse the cases that share a first page into one span marked shared,
		// then bound each span by whichever is tighter: the distance to the next
		// case, or the case's own length. They agree whenever the corpus is
		// complete — CAP's ranges share their boundary page with the next case
		// 72.8% of the time, which makes length overshoot by one, and the distance
		// clamps it — so the minimum bites only where a case is missing.
		out := list[:0]
		for i := 0; i < len(list); {
			j := i
			for j+1 < len(list) && list[j+1].page == list[i].page {
				j++
			}
			cur := list[i]
			cur.shared = j > i
			cur.bound = boundFor(cur.bound, nextPage(list, j+1), cur.page)
			out = append(out, cur)
			i = j + 1
		}
		byVolume[key] = out
	}
	return &rangeIndex[ID]{volumes: byVolume}
}

// nextPage returns the first page of the span at i, or 0 if i is past the end —
// the volume-final case, which has no successor to bound it.
func nextPage[ID comparable](list []span[ID], i int) int {
	if i >= len(list) {
		return 0
	}
	return list[i].page
}

// boundFor picks how many pages a case owns: the tighter of its own recorded
// length and the distance to the next case. A zero length means the source
// recorded no page range; a zero next means this is the last case in the volume.
//
// maxSpanPages substitutes only for a missing length, never for a recorded one.
// A length is the source's own measurement of the case and survives the
// pagination offset that is the whole reason spans are built in citation space,
// so there is no ground to second-guess it — and capping it would silently
// truncate the 23,546 CAP cases that really do run past 40 pages, 671 of them
// volume-final and so reliant on the length alone.
func boundFor(length, next, page int) int {
	gap := 0
	if next > page {
		gap = next - page
	}
	switch {
	case length > 0 && gap > 0:
		return min(length, gap)
	case length > 0:
		return length
	case gap > 0:
		return min(gap, maxSpanPages)
	default:
		return maxSpanPages
	}
}

// lookup finds the case whose span covers page in the given reporter-volume.
func (ix *rangeIndex[ID]) lookup(vol, reporter string, page int) (ID, rangeOutcome) {
	var zero ID
	list := ix.volumes[volumeKey(vol, reporter)]
	if len(list) == 0 {
		return zero, rangeMiss
	}
	// The last span starting at or before page is the only one that can cover it.
	i := sort.Search(len(list), func(i int) bool { return list[i].page > page }) - 1
	if i < 0 {
		return zero, rangeMiss // page precedes the volume's first case
	}
	s := list[i]
	if s.page == page {
		// An exact first-page cite is not this index's business. The cascade
		// already probed it against cap.citations and missed, which means it was
		// dropped there for belonging to more than one case — a deliberate policy
		// (#250) that an ambiguous cite is an honest miss rather than a link to an
		// arbitrary case. Re-resolving it here would quietly undo that decision on
		// strictly less evidence, since this index sees only the official and
		// nominative cites while the drop was judged across all of them. Report
		// nothing and let the reporter/volume tiers describe the miss.
		return zero, rangeMiss
	}
	if s.shared {
		return zero, rangeAmbiguous
	}
	if page >= s.page+s.bound {
		return zero, rangeGap
	}
	return s.id, rangeHit
}

// probe runs every cite string the cascade tried through the index and reports
// the best outcome any of them reached, preferring a hit, then an ambiguity, then
// a gap. Taking the probes verbatim is what keeps the index and the cascade in
// agreement: there is no second list of forms to drift out of sync.
//
// A hit wins over an ambiguity because the probes are alternate spellings of one
// citation, so one spelling resolving cleanly is real evidence, while another
// spelling colliding says only that some unrelated reporter-volume is crowded at
// that page.
func (ix *rangeIndex[ID]) probe(probes []string) (ID, rangeOutcome) {
	var (
		zero ID
		best = rangeMiss
		bid  ID
	)
	for _, p := range probes {
		vol, reporter, page, ok := splitCite(p)
		if !ok {
			continue
		}
		id, outcome := ix.lookup(vol, reporter, page)
		if outcome == rangeHit {
			return id, rangeHit
		}
		if rank(outcome) > rank(best) {
			best, bid = outcome, id
		}
	}
	if best == rangeMiss {
		return zero, rangeMiss
	}
	return bid, best
}

// rank orders outcomes by how much they say about the citation, so probe can keep
// the most informative one across alternate spellings.
func rank(o rangeOutcome) int {
	switch o {
	case rangeHit:
		return 3
	case rangeAmbiguous:
		return 2
	case rangeGap:
		return 1
	default:
		return 0
	}
}

// checkSelfConsistency verifies that every case's own first-page cite falls
// inside the span built for it. That has to hold by construction, so a violation
// means the sort, the collapse, or the bound arithmetic is wrong — and the
// failure mode of getting it wrong is silent mislinking at scale, not a crash.
// Shared pages are exempt: they are deliberately unresolvable.
func (ix *rangeIndex[ID]) checkSelfConsistency() error {
	for key, list := range ix.volumes {
		for i, s := range list {
			if s.shared {
				continue
			}
			if s.bound < 1 {
				return fmt.Errorf("volume %q span %d (page %d) has bound %d, want >= 1",
					key, i, s.page, s.bound)
			}
			if i > 0 && list[i-1].page >= s.page {
				return fmt.Errorf("volume %q spans %d and %d are not in ascending page order (%d >= %d)",
					key, i-1, i, list[i-1].page, s.page)
			}
			if i > 0 && list[i-1].page+list[i-1].bound > s.page {
				return fmt.Errorf("volume %q span %d (pages %d-%d) overlaps the next case at page %d",
					key, i-1, list[i-1].page, list[i-1].page+list[i-1].bound-1, s.page)
			}
		}
	}
	return nil
}

// size reports the number of reporter-volumes and spans held, for the startup log.
func (ix *rangeIndex[ID]) size() (volumes, spans int) {
	for _, list := range ix.volumes {
		spans += len(list)
	}
	return len(ix.volumes), spans
}
