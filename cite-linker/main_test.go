package main

import (
	"testing"

	"github.com/google/uuid"
	"github.com/lmullen/legal-modernism/go/citations"
	"github.com/stretchr/testify/assert"
)

func ptr[T any](v T) *T { return &v }

func TestLinkCitation(t *testing.T) {
	usStd := "U.S."
	qbStd := "Q.B."
	statStd := "Stat."
	alaStd := "Ala."
	aleynStd := "Al"

	tests := []struct {
		name         string
		cite         citations.UnlinkedCitation
		whitelist    map[string]*citations.WhitelistEntry
		capCites     map[string]int64
		freelawCites map[string]int64
		altAbbrs     map[string][]string
		codeCites    map[string]int64
		erCites      map[string]string
		wantStatus   string
		wantCAPID    *int64
		wantCodeID   *int64
		wantERID     *string
		wantLinked   *string
	}{
		{
			name:         "exact CAP hit wins over FreeLaw",
			cite:         citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(5), ReporterAbbr: "U.S.", Page: 10},
			whitelist:    map[string]*citations.WhitelistEntry{"U.S.": {ReporterStandard: &usStd}},
			capCites:     map[string]int64{"5 U.S. 10": 111},
			freelawCites: map[string]int64{"5 U.S. 10": 222},
			wantStatus:   citations.StatusLinkedCAP,
			wantCAPID:    ptr(int64(111)),
			wantLinked:   ptr("5 U.S. 10"),
		},
		{
			name:         "CAP miss, FreeLaw hit links to CAP case",
			cite:         citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(5), ReporterAbbr: "U.S.", Page: 10},
			whitelist:    map[string]*citations.WhitelistEntry{"U.S.": {ReporterStandard: &usStd}},
			capCites:     map[string]int64{},
			freelawCites: map[string]int64{"5 U.S. 10": 222},
			wantStatus:   citations.StatusLinkedCAP,
			wantCAPID:    ptr(int64(222)),
			wantLinked:   ptr("5 U.S. 10"),
		},
		{
			name:         "CAP miss, FreeLaw miss falls through to code reporter",
			cite:         citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(2), ReporterAbbr: "Stat.", Page: 30},
			whitelist:    map[string]*citations.WhitelistEntry{"Stat.": {ReporterStandard: &statStd}},
			capCites:     map[string]int64{},
			freelawCites: map[string]int64{},
			codeCites:    map[string]int64{"2 Stat. 30": 999},
			wantStatus:   citations.StatusLinkedCodeReporter,
			wantCodeID:   ptr(int64(999)),
			wantLinked:   ptr("2 Stat. 30"),
		},
		{
			name:         "CAP miss, FreeLaw miss, code miss is no_match",
			cite:         citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(5), ReporterAbbr: "U.S.", Page: 10},
			whitelist:    map[string]*citations.WhitelistEntry{"U.S.": {ReporterStandard: &usStd}},
			capCites:     map[string]int64{},
			freelawCites: map[string]int64{},
			codeCites:    map[string]int64{},
			wantStatus:   citations.StatusNoMatch,
			wantLinked:   nil,
		},
		{
			name:         "FreeLaw is not consulted for UK reporters",
			cite:         citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(1), ReporterAbbr: "Q.B.", Page: 20},
			whitelist:    map[string]*citations.WhitelistEntry{"Q.B.": {ReporterStandard: &qbStd, UK: true}},
			freelawCites: map[string]int64{"1 Q.B. 20": 222},
			erCites:      map[string]string{"1 Q.B. 20": "er-1"},
			wantStatus:   citations.StatusLinkedEnglishReports,
			wantERID:     ptr("er-1"),
			wantLinked:   ptr("1 Q.B. 20"),
		},
		{
			name:         "alt_abbr FreeLaw hit recovers a no_match",
			cite:         citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(5), ReporterAbbr: "U.S.", Page: 10},
			whitelist:    map[string]*citations.WhitelistEntry{"U.S.": {ReporterStandard: &usStd}},
			capCites:     map[string]int64{},
			freelawCites: map[string]int64{"5 US 10": 333}, // CourtListener spelling, no periods
			altAbbrs:     map[string][]string{"U.S.": {"US"}},
			wantStatus:   citations.StatusLinkedCAP,
			wantCAPID:    ptr(int64(333)),
			wantLinked:   ptr("5 US 10"),
		},
		{
			name:         "direct CAP hit wins over alt_abbr",
			cite:         citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(5), ReporterAbbr: "U.S.", Page: 10},
			whitelist:    map[string]*citations.WhitelistEntry{"U.S.": {ReporterStandard: &usStd}},
			capCites:     map[string]int64{"5 U.S. 10": 111},
			freelawCites: map[string]int64{"5 US 10": 333},
			altAbbrs:     map[string][]string{"U.S.": {"US"}},
			wantStatus:   citations.StatusLinkedCAP,
			wantCAPID:    ptr(int64(111)),
			wantLinked:   ptr("5 U.S. 10"),
		},
		{
			name:         "direct FreeLaw-normalized hit wins over alt_abbr",
			cite:         citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(5), ReporterAbbr: "U.S.", Page: 10},
			whitelist:    map[string]*citations.WhitelistEntry{"U.S.": {ReporterStandard: &usStd}},
			capCites:     map[string]int64{},
			freelawCites: map[string]int64{"5 U.S. 10": 222, "5 US 10": 333},
			altAbbrs:     map[string][]string{"U.S.": {"US"}},
			wantStatus:   citations.StatusLinkedCAP,
			wantCAPID:    ptr(int64(222)),
			wantLinked:   ptr("5 U.S. 10"),
		},
		{
			name:         "alt_abbr miss falls through to code reporter",
			cite:         citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(2), ReporterAbbr: "Stat.", Page: 30},
			whitelist:    map[string]*citations.WhitelistEntry{"Stat.": {ReporterStandard: &statStd}},
			capCites:     map[string]int64{},
			freelawCites: map[string]int64{},
			altAbbrs:     map[string][]string{"Stat.": {"Statx"}}, // present but never matches FreeLaw
			codeCites:    map[string]int64{"2 Stat. 30": 999},
			wantStatus:   citations.StatusLinkedCodeReporter,
			wantCodeID:   ptr(int64(999)),
			wantLinked:   ptr("2 Stat. 30"),
		},
		{
			name:         "multiple alt_abbrs, later entry matches",
			cite:         citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(5), ReporterAbbr: "U.S.", Page: 10},
			whitelist:    map[string]*citations.WhitelistEntry{"U.S.": {ReporterStandard: &usStd}},
			capCites:     map[string]int64{},
			freelawCites: map[string]int64{"5 US 10": 333},
			altAbbrs:     map[string][]string{"U.S.": {"USA", "US"}}, // first misses, second hits
			wantStatus:   citations.StatusLinkedCAP,
			wantCAPID:    ptr(int64(333)),
			wantLinked:   ptr("5 US 10"),
		},
		{
			name:         "nil-volume alt_abbr hit",
			cite:         citations.UnlinkedCitation{ID: uuid.New(), Volume: nil, ReporterAbbr: "Stat.", Page: 30},
			whitelist:    map[string]*citations.WhitelistEntry{"Stat.": {ReporterStandard: &statStd}},
			capCites:     map[string]int64{},
			freelawCites: map[string]int64{"Stat 30": 444},
			altAbbrs:     map[string][]string{"Stat.": {"Stat"}},
			wantStatus:   citations.StatusLinkedCAP,
			wantCAPID:    ptr(int64(444)),
			wantLinked:   ptr("Stat 30"),
		},
		{
			// Regression test for #218/#226. The single-volume detector for
			// Aleyn's King's Bench ("Al") also fires on "Ala." in OCR'd text.
			// Now that the detector records the spelling it actually found, the
			// whitelist routes the citation to the Alabama reporter. Previously
			// ReporterAbbr arrived as "Al", so the linker built candidate cites
			// against Aleyn's single volume instead.
			name: "detected spelling routes to its own reporter",
			cite: citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(12), ReporterAbbr: "Ala.", Page: 672},
			whitelist: map[string]*citations.WhitelistEntry{
				"Ala.": {ReporterStandard: &alaStd},
				"Al":   {ReporterStandard: &aleynStd, UK: true},
			},
			capCites:   map[string]int64{"12 Ala. 672": 555},
			erCites:    map[string]string{"Al 672": "aleyn-false-positive"},
			wantStatus: citations.StatusLinkedCAP,
			wantCAPID:  ptr(int64(555)),
			wantLinked: ptr("12 Ala. 672"),
		},
		{
			// The same routing, but with nothing to link to. The English
			// Reports entry keyed on "Al 672" is exactly the false positive
			// this issue removes: it is reachable only if the citation claims
			// to be a citation to Aleyn.
			name: "detected spelling is not attributed to the detecting single volume",
			cite: citations.UnlinkedCitation{ID: uuid.New(), Volume: nil, ReporterAbbr: "Ala.", Page: 672},
			whitelist: map[string]*citations.WhitelistEntry{
				"Ala.": {ReporterStandard: &alaStd},
				"Al":   {ReporterStandard: &aleynStd, UK: true},
			},
			erCites:    map[string]string{"Al 672": "aleyn-false-positive"},
			wantStatus: citations.StatusNoMatch,
			wantLinked: nil,
		},
		{
			name:         "alt_abbr path is not consulted for UK reporters",
			cite:         citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(1), ReporterAbbr: "Q.B.", Page: 20},
			whitelist:    map[string]*citations.WhitelistEntry{"Q.B.": {ReporterStandard: &qbStd, UK: true}},
			freelawCites: map[string]int64{"1 QB 20": 222}, // would match if alt path ran
			altAbbrs:     map[string][]string{"Q.B.": {"QB"}},
			erCites:      map[string]string{}, // no English Reports match
			wantStatus:   citations.StatusNoMatch,
			wantLinked:   nil,
		},
		{
			name:       "not whitelisted is skipped",
			cite:       citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(5), ReporterAbbr: "Bogus", Page: 10},
			whitelist:  map[string]*citations.WhitelistEntry{},
			wantStatus: citations.StatusSkippedNotWhitelisted,
		},
		{
			name:       "junk reporter is skipped",
			cite:       citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(5), ReporterAbbr: "U.S.", Page: 10},
			whitelist:  map[string]*citations.WhitelistEntry{"U.S.": {Junk: true}},
			wantStatus: citations.StatusSkippedJunk,
		},
		{
			name:       "whitelisted but no standard reporter is no_match",
			cite:       citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(5), ReporterAbbr: "U.S.", Page: 10},
			whitelist:  map[string]*citations.WhitelistEntry{"U.S.": {ReporterStandard: nil}},
			wantStatus: citations.StatusNoMatch,
		},
	}

	diffvols := map[string]map[int]*citations.DiffVolEntry{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := linkCitation(&tt.cite, tt.whitelist, diffvols, tt.capCites, tt.freelawCites, tt.altAbbrs, tt.codeCites, tt.erCites)

			assert.Equal(t, tt.wantStatus, got.Status)
			assert.Equal(t, tt.cite.ID, got.CitationID)

			if tt.wantCAPID == nil {
				assert.Nil(t, got.CAPCaseID)
			} else {
				if assert.NotNil(t, got.CAPCaseID) {
					assert.Equal(t, *tt.wantCAPID, *got.CAPCaseID)
				}
			}

			if tt.wantCodeID == nil {
				assert.Nil(t, got.CodeReporterID)
			} else {
				if assert.NotNil(t, got.CodeReporterID) {
					assert.Equal(t, *tt.wantCodeID, *got.CodeReporterID)
				}
			}

			if tt.wantERID == nil {
				assert.Nil(t, got.ERCaseID)
			} else {
				if assert.NotNil(t, got.ERCaseID) {
					assert.Equal(t, *tt.wantERID, *got.ERCaseID)
				}
			}

			if tt.wantLinked == nil {
				assert.Nil(t, got.CiteLinked)
			} else {
				if assert.NotNil(t, got.CiteLinked) {
					assert.Equal(t, *tt.wantLinked, *got.CiteLinked)
				}
			}
		})
	}
}
