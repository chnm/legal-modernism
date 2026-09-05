package main

import (
	"testing"

	"github.com/lmullen/legal-modernism/go/citations"
	"github.com/stretchr/testify/assert"
)

func TestSplitCite(t *testing.T) {
	tests := []struct {
		name         string
		cite         string
		wantVol      string
		wantReporter string
		wantPage     int
		wantOK       bool
	}{
		{"volume, reporter, page", "17 Mass. 210", "17", "Mass.", 210, true},
		{"multi-word reporter", "1 Ves Sen 1", "1", "Ves Sen", 1, true},
		{"no volume", "Cro Eliz 1", "", "Cro Eliz", 1, true},
		{"no volume, single-word reporter", "Stat 30", "", "Stat", 30, true},
		{"multi-digit volume and page", "123 U.S. 4567", "123", "U.S.", 4567, true},
		{
			// A reporter whose own name starts with something numeric-looking
			// must not have it read as a volume.
			name: "leading token that is not all digits is part of the reporter",
			cite: "2d Cir. 5", wantVol: "", wantReporter: "2d Cir.", wantPage: 5, wantOK: true,
		},
		{
			// The code-reporter map holds keys like this on purpose; they must not
			// pollute the reporter set.
			name: "no page is not a cite",
			cite: "Cox, Manual Trade-Mark Cas.", wantOK: false,
		},
		{
			// allDigits accepts it, but it overflows an int, so Atoi has to be what
			// decides. A wrapped page would silently land in the wrong span.
			name: "page too large for an int is not a cite",
			cite: "1 Mass. 99999999999999999999", wantOK: false,
		},
		{"empty string", "", "", "", 0, false},
		{"page only", "210", "", "", 0, false},
		{"leading space", " 210", "", "", 0, false},
		{"trailing space", "17 Mass. ", "", "", 0, false},
		{"non-numeric page", "17 Mass. cciii", "", "", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vol, reporter, page, ok := splitCite(tt.cite)
			assert.Equal(t, tt.wantOK, ok)
			if !tt.wantOK {
				return
			}
			assert.Equal(t, tt.wantVol, vol, "volume")
			assert.Equal(t, tt.wantReporter, reporter, "reporter")
			assert.Equal(t, tt.wantPage, page, "page")
		})
	}
}

func TestCiteIndexReached(t *testing.T) {
	// One index over two maps, the way the US cascade indexes CAP, FreeLaw, and
	// the code reporter together.
	ix := newCiteIndex(
		map[string]int64{"17 Mass. 210": 1, "5 U.S. 10": 2, "Cro Eliz 1": 3},
		map[string]int64{"9 Q.B. 20": 4, "Cox, Manual Trade-Mark Cas.": 5},
	)

	tests := []struct {
		name         string
		probes       []string
		wantReporter bool
		wantVolume   bool
	}{
		{"exact hit", []string{"17 Mass. 210"}, true, true},
		{"same volume, other page", []string{"17 Mass. 999"}, true, true},
		{"known reporter, unknown volume", []string{"22 Mass. 210"}, true, false},
		{"unknown reporter", []string{"1 Nonesuch 5"}, false, false},
		{"volume-less form is its own volume bucket", []string{"Mass. 210"}, true, false},
		{"volume-less reporter matches only bare", []string{"Cro Eliz 9"}, true, true},
		{"volume-less reporter under a volume 1", []string{"1 Cro Eliz 9"}, true, false},
		{"a later probe supplies the volume", []string{"22 Mass. 1", "17 Mass. 1"}, true, true},
		{"the reporter is found even when no probe has its volume", []string{"22 Mass. 1", "23 Mass. 1"}, true, false},
		{"second map is indexed too", []string{"9 Q.B. 21"}, true, true},
		{"unparsable probes are ignored", []string{"Cox, Manual Trade-Mark Cas."}, false, false},
		{"no probes", nil, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reporter, volume := ix.reached(tt.probes)
			assert.Equal(t, tt.wantReporter, reporter, "reporter found")
			assert.Equal(t, tt.wantVolume, volume, "volume found")
		})
	}
}

// TestCiteIndexSkipsUnparsableKeys checks that a map key that is not a cite
// contributes nothing to either set, so it can never make a reporter look
// present.
func TestCiteIndexSkipsUnparsableKeys(t *testing.T) {
	ix := newCiteIndex(map[string]int64{"Cox, Manual Trade-Mark Cas.": 1, "210": 2, "": 3})
	assert.Empty(t, ix.reporters)
	assert.Empty(t, ix.volumes)
}

func TestUSTier(t *testing.T) {
	ix := newCiteIndex(map[string]int64{"17 Mass. 210": 1, "Cro Eliz 1": 2})

	tests := []struct {
		name            string
		probes          []string
		diffvolsMissing bool
		span            rangeOutcome
		want            string
	}{
		{"unknown reporter", []string{"1 Nonesuch 5"}, false, rangeMiss, citations.TierUSReporterAbsent},
		{"unknown volume", []string{"22 Mass. 5"}, false, rangeMiss, citations.TierUSVolumeAbsent},
		{"known volume, unknown page", []string{"17 Mass. 5"}, false, rangeMiss, citations.TierUSPageAbsent},
		{
			// #261: the citation carried no volume, so the cascade never had
			// one to look up. That is not evidence about coverage.
			name:   "no probe carries a volume",
			probes: []string{"Mass. 5"},
			want:   citations.TierUSVolumeMissing,
		},
		{
			// ...but the reporter has to be known first.
			name:   "a volume-less citation to an unknown reporter is reporter_absent",
			probes: []string{"Nonesuch 5"},
			want:   citations.TierUSReporterAbsent,
		},
		{
			// A single-volume reporter's variant supplies a volume, so a miss
			// on both forms is a real volume_absent, not volume_missing.
			name:   "the volume-1 variant counts as carrying a volume",
			probes: []string{"Mass. 5", "1 Mass. 5"},
			want:   citations.TierUSVolumeAbsent,
		},
		{
			// A bare form the index holds is a real volume bucket, so the
			// cascade got past the volume step.
			name:   "a bare form the index holds reaches page_absent",
			probes: []string{"Cro Eliz 9"},
			want:   citations.TierUSPageAbsent,
		},
		{
			// The diffvols gap outranks both the volume and page tiers: the probes
			// were built on a volume number we know to be untranslated, so how
			// far they got is not evidence about the citation.
			name:   "diffvols gap outranks the page tier",
			probes: []string{"17 Mass. 5"}, diffvolsMissing: true,
			want: citations.TierUSDiffVolsMissing,
		},
		{
			// ...but an absent reporter is more fundamental and still wins.
			name:   "an absent reporter outranks the diffvols gap",
			probes: []string{"1 Nonesuch 5"}, diffvolsMissing: true,
			want: citations.TierUSReporterAbsent,
		},
		{
			// #242: page-range matching found the page inside a case span that
			// two cases begin on, which says strictly more than page_absent.
			name:   "an ambiguous span refines the page tier",
			probes: []string{"17 Mass. 5"}, span: rangeAmbiguous,
			want: citations.TierUSPageAmbiguous,
		},
		{
			// #242: the page is past the end of the case before it, in a hole in
			// CAP's coverage rather than inside a case — the #99 pool.
			name:   "a gap span refines the page tier",
			probes: []string{"17 Mass. 5"}, span: rangeGap,
			want: citations.TierUSPageGap,
		},
		{
			// A span outcome is itself proof the reporter and volume were
			// reached, since the span index is keyed on both. The two indexes
			// are built from different loads and can disagree — a volume whose
			// CAP cites are all ambiguous is dropped from the exact maps but
			// kept in the span index — and where they do, the one that actually
			// found the volume is the one to believe.
			name:   "a span outcome proves the reporter and volume were reached",
			probes: []string{"1 Nonesuch 5"}, span: rangeGap,
			want: citations.TierUSPageGap,
		},
		{
			// ...but the diffvols gap still outranks it. In the cascade that
			// combination cannot arise, because range matching is skipped
			// outright when diffvols is missing; the ladder says so anyway.
			name:   "the diffvols gap outranks a span outcome",
			probes: []string{"17 Mass. 5"}, diffvolsMissing: true, span: rangeGap,
			want: citations.TierUSDiffVolsMissing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, usTier(tt.probes, ix, tt.diffvolsMissing, tt.span))
		})
	}
}

func TestUKTier(t *testing.T) {
	ix := newCiteIndex(map[string]citations.ERCase{
		"1 Ves Sen 1": {ID: "er-1", Cases: 1},
		"1 Ves Sen 4": {Ambiguous: true, Cases: 3},
	})

	assert.Equal(t, citations.TierUKReporterAbsent, ukTier([]string{"1 Nonesuch 5"}, ix, false, rangeMiss))
	assert.Equal(t, citations.TierUKVolumeAbsent, ukTier([]string{"4 Ves Sen 100"}, ix, false, rangeMiss),
		"Vesey Junior volumes cited as Vesey Senior are a volume-range miss")
	assert.Equal(t, citations.TierUKPageAbsent, ukTier([]string{"1 Ves Sen 100"}, ix, false, rangeMiss))

	// #261: a citation detected without a volume never had one to look up.
	assert.Equal(t, citations.TierUKVolumeMissing, ukTier([]string{"Ves Sen 100"}, ix, false, rangeMiss))
	assert.Equal(t, citations.TierUKReporterAbsent, ukTier([]string{"Nonesuch 5"}, ix, false, rangeMiss),
		"the reporter must be known before the volume step is judged")
	assert.Equal(t, citations.TierUKVolumeAbsent, ukTier([]string{"Ves Sen 100", "4 Ves Sen 100"}, ix, false, rangeMiss),
		"the single-volume variant carries a volume, so the miss is volume_absent")

	// #256: a cite the corpus holds but cannot resolve is a different failure
	// from a page it has never seen, and outranks it.
	assert.Equal(t, citations.TierUKPageAmbiguous, ukTier([]string{"1 Ves Sen 4"}, ix, true, rangeMiss))

	// ...but only once the reporter and the volume are actually known. An
	// ambiguous flag must never promote a citation past a tier it did not reach.
	assert.Equal(t, citations.TierUKReporterAbsent, ukTier([]string{"1 Nonesuch 5"}, ix, true, rangeMiss))
	assert.Equal(t, citations.TierUKVolumeAbsent, ukTier([]string{"4 Ves Sen 100"}, ix, true, rangeMiss))

	// #243: the page-range outcomes refine the page step the same way, and are
	// held to the same rule about not promoting past the volume step.
	assert.Equal(t, citations.TierUKPageAmbiguous, ukTier([]string{"1 Ves Sen 100"}, ix, false, rangeAmbiguous))
	assert.Equal(t, citations.TierUKPageGap, ukTier([]string{"1 Ves Sen 100"}, ix, false, rangeGap))
	assert.Equal(t, citations.TierUKPageGap, ukTier([]string{"4 Ves Sen 100"}, ix, false, rangeGap),
		"a span outcome proves the volume was reached even where the cite index missed it")

	// An exact cite the corpus holds outranks a gap reached under some other
	// volume form: a known cite string says more than a page inside a hole.
	assert.Equal(t, citations.TierUKPageAmbiguous, ukTier([]string{"1 Ves Sen 4"}, ix, true, rangeGap))
}

// TestCiteIndexKeepsAmbiguousKeys guards the property that makes
// uk_page_ambiguous reachable at all: LoadEnglishReportsCitations keeps
// ambiguous cites in the map, so the index still learns their reporter and
// volume. Dropping them, as the CAP loader does, would report a citation that
// reached a real volume as uk_volume_absent.
func TestCiteIndexKeepsAmbiguousKeys(t *testing.T) {
	ix := newCiteIndex(map[string]citations.ERCase{"3 Keb 408": {Ambiguous: true, Cases: 2}})

	reporter, volume := ix.reached([]string{"3 Keb 999"})
	assert.True(t, reporter, "an ambiguous cite still proves the reporter exists")
	assert.True(t, volume, "an ambiguous cite still proves the volume exists")
}

// TestDiffvolsMissing covers the three ways the diffvols check can decline to
// fire, since a false positive here would mislabel every no_match for the
// reporter.
func TestDiffvolsMissing(t *testing.T) {
	std := "Smith Pa."
	diffvols := map[string]map[int]*citations.DiffVolEntry{
		std: {31: {CAPVol: 81, CAPReporter: "Pa."}},
	}
	renumbering := &citations.WhitelistEntry{ReporterStandard: &std, CAPDifferent: true}
	plain := &citations.WhitelistEntry{ReporterStandard: &std}

	cite := func(vol *int) *citations.UnlinkedCitation {
		return &citations.UnlinkedCitation{ReporterAbbr: "Sm.", Volume: vol, Page: 635}
	}

	assert.True(t, diffvolsMissing(cite(ptr(7)), renumbering, diffvols),
		"a renumbering reporter with no row for the cited volume")
	assert.False(t, diffvolsMissing(cite(ptr(31)), renumbering, diffvols),
		"the cited volume has a row")
	assert.False(t, diffvolsMissing(cite(ptr(7)), plain, diffvols),
		"the reporter does not renumber in CAP")
	assert.False(t, diffvolsMissing(cite(nil), renumbering, diffvols),
		"no volume to translate, and buildCAPCite does not consult diffvols either")
}
