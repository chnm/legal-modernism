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
		wantOK       bool
	}{
		{"volume, reporter, page", "17 Mass. 210", "17", "Mass.", true},
		{"multi-word reporter", "1 Ves Sen 1", "1", "Ves Sen", true},
		{"no volume", "Cro Eliz 1", "", "Cro Eliz", true},
		{"no volume, single-word reporter", "Stat 30", "", "Stat", true},
		{"multi-digit volume and page", "123 U.S. 4567", "123", "U.S.", true},
		{
			// A reporter whose own name starts with something numeric-looking
			// must not have it read as a volume.
			name: "leading token that is not all digits is part of the reporter",
			cite: "2d Cir. 5", wantVol: "", wantReporter: "2d Cir.", wantOK: true,
		},
		{
			// The code-reporter map holds keys like this on purpose; they must not
			// pollute the reporter set.
			name: "no page is not a cite",
			cite: "Cox, Manual Trade-Mark Cas.", wantOK: false,
		},
		{"empty string", "", "", "", false},
		{"page only", "210", "", "", false},
		{"leading space", " 210", "", "", false},
		{"trailing space", "17 Mass. ", "", "", false},
		{"non-numeric page", "17 Mass. cciii", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vol, reporter, ok := splitCite(tt.cite)
			assert.Equal(t, tt.wantOK, ok)
			if !tt.wantOK {
				return
			}
			assert.Equal(t, tt.wantVol, vol, "volume")
			assert.Equal(t, tt.wantReporter, reporter, "reporter")
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
	ix := newCiteIndex(map[string]int64{"17 Mass. 210": 1})

	tests := []struct {
		name            string
		probes          []string
		diffvolsMissing bool
		want            string
	}{
		{"unknown reporter", []string{"1 Nonesuch 5"}, false, citations.TierUSReporterAbsent},
		{"unknown volume", []string{"22 Mass. 5"}, false, citations.TierUSVolumeAbsent},
		{"known volume, unknown page", []string{"17 Mass. 5"}, false, citations.TierUSPageAbsent},
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, usTier(tt.probes, ix, tt.diffvolsMissing))
		})
	}
}

func TestUKTier(t *testing.T) {
	ix := newCiteIndex(map[string]string{"1 Ves Sen 1": "er-1"})

	assert.Equal(t, citations.TierUKReporterAbsent, ukTier([]string{"1 Nonesuch 5"}, ix))
	assert.Equal(t, citations.TierUKVolumeAbsent, ukTier([]string{"4 Ves Sen 100"}, ix),
		"Vesey Junior volumes cited as Vesey Senior are a volume-range miss")
	assert.Equal(t, citations.TierUKPageAbsent, ukTier([]string{"1 Ves Sen 100"}, ix))
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
