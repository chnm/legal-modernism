package main

import (
	"testing"

	"github.com/lmullen/legal-modernism/go/citations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// span17Mass is volume 17 of Mass. as CAP actually holds it, abridged to the
// stretch that makes the point. It is a reprint whose internal pagination runs
// roughly 110 pages behind the pagination the citation uses, which is why every
// Length here is far smaller than the difference between the cite pages would
// suggest if first_page were read as an absolute position.
//
//	official cite   first_page  last_page   length
//	17 Mass. 470       382         388         7
//	17 Mass. 478       388         416        29
//	17 Mass. 514       417         445        29
//	17 Mass. 585       473         478         6
//	17 Mass. 591       478         490        13
var span17Mass = []citations.CaseSpan[int64]{
	{Cite: "17 Mass. 470", ID: 470, Length: 7},
	{Cite: "17 Mass. 478", ID: 478, Length: 29},
	{Cite: "17 Mass. 514", ID: 514, Length: 29},
	{Cite: "17 Mass. 585", ID: 585, Length: 6},
	{Cite: "17 Mass. 591", ID: 591, Length: 13},
}

func TestRangeIndexLookup(t *testing.T) {
	ix := newRangeIndex(span17Mass)

	tests := []struct {
		name        string
		page        int
		wantID      int64
		wantOutcome rangeOutcome
	}{
		{
			// The case for issue #242. Containment on first_page/last_page puts
			// page 479 inside [478, 490] and links it to the case cited
			// 17 Mass. 591, which is a different case entirely. In citation space
			// the answer is unambiguous.
			name: "pin cite in a drifted volume resolves to the citation-space owner",
			page: 479, wantID: 478, wantOutcome: rangeHit,
		},
		{
			// Reaching here means the exact cascade already probed this string and
			// missed, i.e. cap.citations dropped it as ambiguous. Re-resolving it
			// would undo that policy on less evidence.
			name: "an exact first-page cite is left to the exact cascade",
			page: 478, wantOutcome: rangeMiss,
		},
		{"last page a case owns", 506, 478, rangeHit},
		{
			// Cites jump 478 -> 514, a 36-page hole, but CAP records the case at
			// 478 as 29 pages long. Pages 507-513 are in the hole and must not be
			// attached to it.
			name: "page in a coverage hole is a gap, not an interior page",
			page: 507, wantOutcome: rangeGap,
		},
		{"page just before the next case is still a gap", 513, 0, rangeGap},
		{"the next case's own first page is an exact cite", 514, 0, rangeMiss},
		{"first interior page of the next case", 515, 514, rangeHit},
		{"page before the volume's first indexed cite", 469, 0, rangeMiss},
		{
			// The volume-final case has no successor, so only its own length
			// bounds it: 591 + 13 - 1 = 603.
			name: "volume-final case is bounded by its own length",
			page: 603, wantID: 591, wantOutcome: rangeHit,
		},
		{"past the volume-final case's length", 604, 0, rangeGap},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, outcome := ix.lookup("17", "Mass.", tt.page)
			assert.Equal(t, tt.wantOutcome, outcome)
			if tt.wantOutcome == rangeHit {
				assert.Equal(t, tt.wantID, id)
			}
		})
	}
}

// TestRangeIndexSharedBoundaryPage covers the convention CAP uses 72.8% of the
// time: a case's last_page is the page the next case begins on, so its length
// overshoots the distance to the next cite by exactly one. The distance has to
// win, or every case would claim its successor's first page.
func TestRangeIndexSharedBoundaryPage(t *testing.T) {
	ix := newRangeIndex([]citations.CaseSpan[int64]{
		{Cite: "1 Test 10", ID: 10, Length: 6}, // first_page 10, last_page 15
		{Cite: "1 Test 15", ID: 15, Length: 5}, // first_page 15, last_page 19
	})

	id, outcome := ix.lookup("1", "Test", 14)
	assert.Equal(t, rangeHit, outcome)
	assert.Equal(t, int64(10), id, "page 14 belongs to the case starting at 10")

	// The clamping itself is what checkSelfConsistency enforces: unclamped, the
	// case at 10 would run 10-15 and overlap the case that starts at 15.
	assert.NoError(t, ix.checkSelfConsistency(),
		"length must be clamped to the distance to the next case")

	_, outcome = ix.lookup("1", "Test", 15)
	assert.Equal(t, rangeMiss, outcome, "the boundary page is an exact cite for the next case")
}

// TestRangeIndexCleanTiling covers the other 25.7%: last_page is one before the
// next case's first_page, so length and distance agree exactly and no page is
// lost at the boundary.
func TestRangeIndexCleanTiling(t *testing.T) {
	ix := newRangeIndex([]citations.CaseSpan[int64]{
		{Cite: "1 Test 10", ID: 10, Length: 5}, // first_page 10, last_page 14
		{Cite: "1 Test 15", ID: 15, Length: 5},
	})

	id, outcome := ix.lookup("1", "Test", 14)
	assert.Equal(t, rangeHit, outcome)
	assert.Equal(t, int64(10), id, "the last page of a cleanly tiled case is not lost")
}

func TestRangeIndexAmbiguousStartPage(t *testing.T) {
	ix := newRangeIndex([]citations.CaseSpan[int64]{
		{Cite: "1 Test 10", ID: 101, Length: 5},
		{Cite: "1 Test 10", ID: 102, Length: 5}, // two decisions begin on page 10
		{Cite: "1 Test 20", ID: 200, Length: 5},
	})

	// The shared page itself is an exact cite, so it stays with the exact
	// cascade like any other; only the interior pages it owns are reported
	// ambiguous.
	_, outcome := ix.lookup("1", "Test", 10)
	assert.Equal(t, rangeMiss, outcome, "the shared start page is an exact cite")

	for _, page := range []int{12, 14} {
		_, outcome := ix.lookup("1", "Test", page)
		assert.Equal(t, rangeAmbiguous, outcome, "page %d", page)
	}

	id, outcome := ix.lookup("1", "Test", 21)
	assert.Equal(t, rangeHit, outcome, "the next unshared case still resolves")
	assert.Equal(t, int64(200), id)
}

// TestRangeIndexNoLengthFallsBackToMaxSpan is the English Reports case, and the
// 0.05% of CAP cases with a NULL first or last page: with no recorded length the
// span can only be bounded by the next cite, and by maxSpanPages when there is
// no next cite either.
func TestRangeIndexNoLengthFallsBackToMaxSpan(t *testing.T) {
	ix := newRangeIndex([]citations.CaseSpan[string]{
		{Cite: "1 E.R. 100", ID: "a"},
		{Cite: "1 E.R. 300", ID: "b"}, // a 200-page hole, far beyond any real case
	})

	id, outcome := ix.lookup("1", "E.R.", 100+maxSpanPages-1)
	assert.Equal(t, rangeHit, outcome)
	assert.Equal(t, "a", id)

	_, outcome = ix.lookup("1", "E.R.", 100+maxSpanPages)
	assert.Equal(t, rangeGap, outcome, "an unbounded span must not swallow the hole")

	id, outcome = ix.lookup("1", "E.R.", 300+maxSpanPages-1)
	assert.Equal(t, rangeHit, outcome, "the volume-final case gets maxSpanPages")
	assert.Equal(t, "b", id)
}

func TestRangeIndexUnknownVolume(t *testing.T) {
	ix := newRangeIndex(span17Mass)

	_, outcome := ix.lookup("18", "Mass.", 479)
	assert.Equal(t, rangeMiss, outcome, "another volume of the same reporter")

	_, outcome = ix.lookup("17", "Cal.", 479)
	assert.Equal(t, rangeMiss, outcome, "another reporter at the same volume")

	_, outcome = ix.lookup("", "Mass.", 479)
	assert.Equal(t, rangeMiss, outcome, "the volume-less form is never a CAP key")
}

// TestRangeIndexSkipsUnparsableCites mirrors TestCiteIndexSkipsUnparsableKeys:
// a cite string that is not a cite must be dropped rather than indexed under a
// junk key.
func TestRangeIndexSkipsUnparsableCites(t *testing.T) {
	ix := newRangeIndex([]citations.CaseSpan[int64]{
		{Cite: "Cox, Manual Trade-Mark Cas.", ID: 1},
		{Cite: "2013-NMCA-039", ID: 2},
		{Cite: "1 Test 10", ID: 3, Length: 5},
	})

	volumes, spans := ix.size()
	assert.Equal(t, 1, volumes)
	assert.Equal(t, 1, spans)
}

func TestRangeIndexProbePrefersHitOverAmbiguity(t *testing.T) {
	ix := newRangeIndex([]citations.CaseSpan[int64]{
		// The alternate spelling's volume is crowded at this page...
		{Cite: "1 Alt 10", ID: 901, Length: 20},
		{Cite: "1 Alt 10", ID: 902, Length: 20},
		// ...while the canonical spelling resolves cleanly.
		{Cite: "1 Test 10", ID: 100, Length: 20},
	})

	// Probe order deliberately puts the ambiguous spelling first.
	id, outcome := ix.probe([]string{"1 Alt 12", "1 Test 12"})
	assert.Equal(t, rangeHit, outcome)
	assert.Equal(t, int64(100), id)
}

func TestRangeIndexProbeKeepsMostInformativeMiss(t *testing.T) {
	ix := newRangeIndex([]citations.CaseSpan[int64]{
		{Cite: "1 Test 10", ID: 100, Length: 5},
		{Cite: "1 Test 60", ID: 600, Length: 5},
	})

	// "9 Test 12" is an unknown volume (miss); "1 Test 20" is in the hole between
	// 10 and 60 (gap). The gap is the more informative answer.
	_, outcome := ix.probe([]string{"9 Test 12", "1 Test 20"})
	assert.Equal(t, rangeGap, outcome)

	_, outcome = ix.probe([]string{"9 Test 12", "8 Test 20"})
	assert.Equal(t, rangeMiss, outcome, "nothing probed reached the index")

	_, outcome = ix.probe([]string{"Cox, Manual Trade-Mark Cas."})
	assert.Equal(t, rangeMiss, outcome, "an unparsable probe is ignored, not a miss to report")
}

func TestRangeIndexSelfConsistency(t *testing.T) {
	ix := newRangeIndex(span17Mass)
	require.NoError(t, ix.checkSelfConsistency())

	// Every case must own the page immediately after its own cite page. The cite
	// page itself is deliberately left to the exact cascade, so this is the
	// nearest page that exercises the span the case was given — and it is the
	// invariant that would break if the sort, the collapse, or the bound
	// arithmetic were wrong.
	for _, s := range span17Mass {
		_, _, page, ok := splitCite(s.Cite)
		require.True(t, ok)
		id, outcome := ix.lookup("17", "Mass.", page+1)
		assert.Equal(t, rangeHit, outcome, "cite %q", s.Cite)
		assert.Equal(t, s.ID, id, "the page after %q belongs to that case", s.Cite)
	}
}

// TestRangeIndexEmpty checks that an index built from nothing answers rangeMiss
// rather than panicking, which is what makes the nil-spans path in the tests and
// any future partial load safe.
func TestRangeIndexEmpty(t *testing.T) {
	ix := newRangeIndex[int64](nil)
	_, outcome := ix.lookup("17", "Mass.", 479)
	assert.Equal(t, rangeMiss, outcome)
	assert.NoError(t, ix.checkSelfConsistency())

	volumes, spans := ix.size()
	assert.Zero(t, volumes)
	assert.Zero(t, spans)
}
