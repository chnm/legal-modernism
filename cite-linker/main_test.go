package main

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lmullen/legal-modernism/go/citations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr[T any](v T) *T { return &v }

func TestLinkCitation(t *testing.T) {
	usStd := "U.S."
	qbStd := "Q.B."
	statStd := "Stat."
	tothStd := "Toth"
	croStd := "Cro Eliz"
	vernStd := "Vern"
	alaStd := "Ala."
	aleynStd := "Al"

	tests := []struct {
		name         string
		cite         citations.UnlinkedCitation
		whitelist    map[string]*citations.WhitelistEntry
		diffvols     map[string]map[int]*citations.DiffVolEntry
		capCites     map[string]int64
		freelawCites map[string]int64
		altAbbrs     map[string][]string
		codeCites    map[string]int64
		erCites      map[string]string
		wantStatus   string
		wantTier     string
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
			wantTier:     citations.TierCAPDirect,
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
			wantTier:     citations.TierCAPFreelaw,
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
			wantTier:     citations.TierCodeDirect,
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
			wantTier:     citations.TierUSReporterAbsent,
			wantLinked:   nil,
		},
		{
			name:         "FreeLaw is not consulted for UK reporters",
			cite:         citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(1), ReporterAbbr: "Q.B.", Page: 20},
			whitelist:    map[string]*citations.WhitelistEntry{"Q.B.": {ReporterStandard: &qbStd, UK: true}},
			freelawCites: map[string]int64{"1 Q.B. 20": 222},
			erCites:      map[string]string{"1 Q.B. 20": "er-1"},
			wantStatus:   citations.StatusLinkedEnglishReports,
			wantTier:     citations.TierERDirect,
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
			wantTier:     citations.TierCAPFreelawAltSpelling,
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
			wantTier:     citations.TierCAPDirect,
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
			wantTier:     citations.TierCAPFreelaw,
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
			wantTier:     citations.TierCodeDirect,
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
			wantTier:     citations.TierCAPFreelawAltSpelling,
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
			wantTier:     citations.TierCAPFreelawAltSpelling,
			wantCAPID:    ptr(int64(444)),
			wantLinked:   ptr("Stat 30"),
		},
		{
			name:         "alt_abbr CAP hit recovers a no_match",
			cite:         citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(5), ReporterAbbr: "U.S.", Page: 10},
			whitelist:    map[string]*citations.WhitelistEntry{"U.S.": {ReporterStandard: &usStd}},
			capCites:     map[string]int64{"5 US 10": 333}, // CAP holds the no-period spelling
			freelawCites: map[string]int64{},
			altAbbrs:     map[string][]string{"U.S.": {"US"}},
			wantStatus:   citations.StatusLinkedCAP,
			wantTier:     citations.TierCAPAltSpelling,
			wantCAPID:    ptr(int64(333)),
			wantLinked:   ptr("5 US 10"),
		},
		{
			// The alternates probe per map, not per alt: CAP is exhausted across
			// every alternate before FreeLaw is consulted, so a later alt's CAP
			// hit beats an earlier alt's FreeLaw hit.
			name:         "alt CAP hit wins over an earlier-alt FreeLaw hit",
			cite:         citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(5), ReporterAbbr: "U.S.", Page: 10},
			whitelist:    map[string]*citations.WhitelistEntry{"U.S.": {ReporterStandard: &usStd}},
			capCites:     map[string]int64{"5 U. S. 10": 333}, // second alt
			freelawCites: map[string]int64{"5 US 10": 444},    // first alt
			altAbbrs:     map[string][]string{"U.S.": {"US", "U. S."}},
			wantStatus:   citations.StatusLinkedCAP,
			wantTier:     citations.TierCAPAltSpelling,
			wantCAPID:    ptr(int64(333)),
			wantLinked:   ptr("5 U. S. 10"),
		},
		{
			// The alt probes sit after both direct probes: a normalized-form
			// FreeLaw hit still beats any alternate-spelling CAP hit.
			name:         "direct FreeLaw-normalized hit wins over alt CAP",
			cite:         citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(5), ReporterAbbr: "U.S.", Page: 10},
			whitelist:    map[string]*citations.WhitelistEntry{"U.S.": {ReporterStandard: &usStd}},
			capCites:     map[string]int64{"5 US 10": 333},
			freelawCites: map[string]int64{"5 U.S. 10": 222},
			altAbbrs:     map[string][]string{"U.S.": {"US"}},
			wantStatus:   citations.StatusLinkedCAP,
			wantTier:     citations.TierCAPFreelaw,
			wantCAPID:    ptr(int64(222)),
			wantLinked:   ptr("5 U.S. 10"),
		},
		{
			name:         "alt_abbr code-reporter hit recovers a no_match",
			cite:         citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(2), ReporterAbbr: "Stat.", Page: 30},
			whitelist:    map[string]*citations.WhitelistEntry{"Stat.": {ReporterStandard: &statStd}},
			capCites:     map[string]int64{},
			freelawCites: map[string]int64{},
			altAbbrs:     map[string][]string{"Stat.": {"Statx"}},
			codeCites:    map[string]int64{"2 Statx 30": 999},
			wantStatus:   citations.StatusLinkedCodeReporter,
			wantTier:     citations.TierCodeAltSpelling,
			wantCodeID:   ptr(int64(999)),
			wantLinked:   ptr("2 Statx 30"),
		},
		{
			name:         "official code cite wins over alt code cite",
			cite:         citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(2), ReporterAbbr: "Stat.", Page: 30},
			whitelist:    map[string]*citations.WhitelistEntry{"Stat.": {ReporterStandard: &statStd}},
			capCites:     map[string]int64{},
			freelawCites: map[string]int64{},
			altAbbrs:     map[string][]string{"Stat.": {"Statx"}},
			codeCites:    map[string]int64{"2 Stat. 30": 111, "2 Statx 30": 222},
			wantStatus:   citations.StatusLinkedCodeReporter,
			wantTier:     citations.TierCodeDirect,
			wantCodeID:   ptr(int64(111)),
			wantLinked:   ptr("2 Stat. 30"),
		},
		{
			// The new code-reporter alt probe runs inside the volumeForms loop:
			// the detected form is exhausted across every source, alternates
			// included, before the volume variant is tried.
			name:       "alt code hit on the detected form wins over CAP hit on the variant",
			cite:       citations.UnlinkedCitation{ID: uuid.New(), Volume: nil, ReporterAbbr: "Toth", Page: 123},
			whitelist:  map[string]*citations.WhitelistEntry{"Toth": {ReporterStandard: &tothStd, SingleVol: true}},
			capCites:   map[string]int64{"1 Toth 123": 777}, // variant form only
			altAbbrs:   map[string][]string{"Toth": {"Tothill"}},
			codeCites:  map[string]int64{"Tothill 123": 999}, // detected form, alt spelling
			wantStatus: citations.StatusLinkedCodeReporter,
			wantTier:   citations.TierCodeAltSpelling,
			wantCodeID: ptr(int64(999)),
			wantLinked: ptr("Tothill 123"),
		},
		{
			name:       "nil-volume alt code hit",
			cite:       citations.UnlinkedCitation{ID: uuid.New(), Volume: nil, ReporterAbbr: "Stat.", Page: 30},
			whitelist:  map[string]*citations.WhitelistEntry{"Stat.": {ReporterStandard: &statStd}},
			capCites:   map[string]int64{},
			altAbbrs:   map[string][]string{"Stat.": {"Stat"}},
			codeCites:  map[string]int64{"Stat 30": 555},
			wantStatus: citations.StatusLinkedCodeReporter,
			wantTier:   citations.TierCodeAltSpelling,
			wantCodeID: ptr(int64(555)),
			wantLinked: ptr("Stat 30"),
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
			wantTier:   citations.TierCAPDirect,
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
			wantTier:   citations.TierUSReporterAbsent,
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
			wantTier:     citations.TierUKReporterAbsent,
			wantLinked:   nil,
		},
		{
			// A single-volume reporter is the same citation with or without a
			// leading volume 1. CAP normalizes everything to a volume, so a
			// volume-less detection needs the variant to reach it.
			name:       "single-volume nil volume links under an explicit volume 1",
			cite:       citations.UnlinkedCitation{ID: uuid.New(), Volume: nil, ReporterAbbr: "Toth", Page: 123},
			whitelist:  map[string]*citations.WhitelistEntry{"Toth": {ReporterStandard: &tothStd, SingleVol: true}},
			capCites:   map[string]int64{"1 Toth 123": 777},
			wantStatus: citations.StatusLinkedCAP,
			wantTier:   citations.TierCAPDirect,
			wantCAPID:  ptr(int64(777)),
			wantLinked: ptr("1 Toth 123"),
		},
		{
			// The other direction: the English Reports store most nominate
			// reporters bare, so a citation written "1 Cro Eliz 5" has to drop
			// the redundant volume to match.
			name:       "single-volume explicit volume 1 links under the bare form",
			cite:       citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(1), ReporterAbbr: "Cro Eliz", Page: 5},
			whitelist:  map[string]*citations.WhitelistEntry{"Cro Eliz": {ReporterStandard: &croStd, UK: true, SingleVol: true}},
			erCites:    map[string]string{"Cro Eliz 5": "er-cro-5"},
			wantStatus: citations.StatusLinkedEnglishReports,
			wantTier:   citations.TierERDirect,
			wantERID:   ptr("er-cro-5"),
			wantLinked: ptr("Cro Eliz 5"),
		},
		{
			// ...but some English Reports rows do carry the redundant volume
			// ("1 Vern 1"), which is why the variant is needed in both
			// directions rather than just one.
			name:       "single-volume nil volume links to an English Reports cite stored with the redundant 1",
			cite:       citations.UnlinkedCitation{ID: uuid.New(), Volume: nil, ReporterAbbr: "Vern", Page: 12},
			whitelist:  map[string]*citations.WhitelistEntry{"Vern": {ReporterStandard: &vernStd, UK: true, SingleVol: true}},
			erCites:    map[string]string{"1 Vern 12": "er-vern-12"},
			wantStatus: citations.StatusLinkedEnglishReports,
			wantTier:   citations.TierERDirect,
			wantERID:   ptr("er-vern-12"),
			wantLinked: ptr("1 Vern 12"),
		},
		{
			// The alt-spelling probe has to use the variant's volume, not the
			// detected one, or it builds the wrong string.
			name:         "alt spelling is probed under the variant volume too",
			cite:         citations.UnlinkedCitation{ID: uuid.New(), Volume: nil, ReporterAbbr: "Toth", Page: 123},
			whitelist:    map[string]*citations.WhitelistEntry{"Toth": {ReporterStandard: &tothStd, SingleVol: true}},
			freelawCites: map[string]int64{"1 Tothill 123": 555},
			altAbbrs:     map[string][]string{"Toth": {"Tothill"}},
			wantStatus:   citations.StatusLinkedCAP,
			wantTier:     citations.TierCAPFreelawAltSpelling,
			wantCAPID:    ptr(int64(555)),
			wantLinked:   ptr("1 Tothill 123"),
		},
		{
			// The detected form is exhausted across every source before the
			// variant is tried, so a link that exists today can never be
			// rewired to a different source.
			name:       "detected form wins over the volume variant",
			cite:       citations.UnlinkedCitation{ID: uuid.New(), Volume: nil, ReporterAbbr: "Toth", Page: 123},
			whitelist:  map[string]*citations.WhitelistEntry{"Toth": {ReporterStandard: &tothStd, SingleVol: true}},
			capCites:   map[string]int64{"1 Toth 123": 888},
			codeCites:  map[string]int64{"Toth 123": 999},
			wantStatus: citations.StatusLinkedCodeReporter,
			wantTier:   citations.TierCodeDirect,
			wantCodeID: ptr(int64(999)),
			wantLinked: ptr("Toth 123"),
		},
		{
			// Volume 2 is a real volume, not a redundant 1, so there is nothing
			// equivalent to try even on a single-volume reporter.
			name:       "single-volume reporter with volume 2 has no variant",
			cite:       citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(2), ReporterAbbr: "Toth", Page: 123},
			whitelist:  map[string]*citations.WhitelistEntry{"Toth": {ReporterStandard: &tothStd, SingleVol: true}},
			capCites:   map[string]int64{"Toth 123": 777, "1 Toth 123": 888},
			wantStatus: citations.StatusNoMatch,
			wantTier:   citations.TierUSVolumeAbsent,
			wantLinked: nil,
		},
		{
			// For a multi-volume reporter "1 U.S. 10" is a real volume 1 and
			// means something different from "U.S. 10".
			name:       "multi-volume reporter does not gain a volume 1",
			cite:       citations.UnlinkedCitation{ID: uuid.New(), Volume: nil, ReporterAbbr: "U.S.", Page: 10},
			whitelist:  map[string]*citations.WhitelistEntry{"U.S.": {ReporterStandard: &usStd}},
			capCites:   map[string]int64{"1 U.S. 10": 111},
			wantStatus: citations.StatusNoMatch,
			wantTier:   citations.TierUSVolumeAbsent,
			wantLinked: nil,
		},
		{
			name:       "multi-volume reporter does not drop its volume 1",
			cite:       citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(1), ReporterAbbr: "U.S.", Page: 10},
			whitelist:  map[string]*citations.WhitelistEntry{"U.S.": {ReporterStandard: &usStd}},
			capCites:   map[string]int64{"U.S. 10": 111},
			wantStatus: citations.StatusNoMatch,
			wantTier:   citations.TierUSVolumeAbsent,
			wantLinked: nil,
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
			// The volume is in CAP and only the page missed, which is the
			// pin-cite pool #242 is about — the tier is what makes it countable
			// without a per-reporter query.
			name:       "reporter and volume present, page missed, is us_page_absent",
			cite:       citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(5), ReporterAbbr: "U.S.", Page: 10},
			whitelist:  map[string]*citations.WhitelistEntry{"U.S.": {ReporterStandard: &usStd}},
			capCites:   map[string]int64{"5 U.S. 11": 111}, // same volume, different page
			wantStatus: citations.StatusNoMatch,
			wantTier:   citations.TierUSPageAbsent,
		},
		{
			// A reporter that renumbers in CAP with no diffvols row for the cited
			// volume: every probe was built on an untranslated volume number, so
			// this outranks the volume and page tiers even though the probed
			// volume happens to exist in CAP.
			name:       "unmapped diffvols volume is us_diffvols_missing",
			cite:       citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(7), ReporterAbbr: "Stat.", Page: 30},
			whitelist:  map[string]*citations.WhitelistEntry{"Stat.": {ReporterStandard: &statStd, CAPDifferent: true}},
			diffvols:   map[string]map[int]*citations.DiffVolEntry{"Stat.": {2: {CAPVol: 81, CAPReporter: "Stat."}}},
			capCites:   map[string]int64{"7 Stat. 31": 111},
			wantStatus: citations.StatusNoMatch,
			wantTier:   citations.TierUSDiffVolsMissing,
		},
		{
			name:       "UK reporter at another volume is uk_volume_absent",
			cite:       citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(1), ReporterAbbr: "Q.B.", Page: 20},
			whitelist:  map[string]*citations.WhitelistEntry{"Q.B.": {ReporterStandard: &qbStd, UK: true}},
			erCites:    map[string]string{"9 Q.B. 20": "er-9"},
			wantStatus: citations.StatusNoMatch,
			wantTier:   citations.TierUKVolumeAbsent,
		},
		{
			name:       "UK volume present, page missed, is uk_page_absent",
			cite:       citations.UnlinkedCitation{ID: uuid.New(), Volume: ptr(1), ReporterAbbr: "Q.B.", Page: 20},
			whitelist:  map[string]*citations.WhitelistEntry{"Q.B.": {ReporterStandard: &qbStd, UK: true}},
			erCites:    map[string]string{"1 Q.B. 21": "er-1"},
			wantStatus: citations.StatusNoMatch,
			wantTier:   citations.TierUKPageAbsent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tables := newLinkTables(tt.whitelist, tt.diffvols, tt.capCites, tt.freelawCites, tt.altAbbrs, tt.codeCites, tt.erCites)
			got := linkCitation(&tt.cite, tables)

			assert.Equal(t, tt.wantStatus, got.Status)
			assert.Equal(t, tt.wantTier, got.MatchTier)
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

func TestVolumeForms(t *testing.T) {
	std := "Toth"
	single := &citations.WhitelistEntry{ReporterStandard: &std, SingleVol: true}
	multi := &citations.WhitelistEntry{ReporterStandard: &std}

	tests := []struct {
		name    string
		volume  *int
		entry   *citations.WhitelistEntry
		wantLen int
		wantVar *int // volume of the variant, when there is one
	}{
		{"single-vol nil volume gains a 1", nil, single, 2, ptr(1)},
		{"single-vol volume 1 loses it", ptr(1), single, 2, nil},
		{"single-vol volume 2 has no variant", ptr(2), single, 1, nil},
		{"multi-vol nil volume has no variant", nil, multi, 1, nil},
		{"multi-vol volume 1 has no variant", ptr(1), multi, 1, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &citations.UnlinkedCitation{ID: uuid.New(), Volume: tt.volume, ReporterAbbr: "Toth", Page: 123}
			forms := volumeForms(c, tt.entry)

			require.Len(t, forms, tt.wantLen)
			assert.Same(t, c, forms[0], "the detected citation must be probed first")

			if tt.wantLen == 1 {
				return
			}
			if tt.wantVar == nil {
				assert.Nil(t, forms[1].Volume)
			} else if assert.NotNil(t, forms[1].Volume) {
				assert.Equal(t, *tt.wantVar, *forms[1].Volume)
			}
			// The variant is a copy: the original must not be mutated.
			assert.Equal(t, tt.volume, c.Volume)
		})
	}
}

// TestVolumeVariantRecordsDetectedForm checks that when the variant is what
// matched, cite_linked reports the string that actually hit while cite_cleaned
// and cite_normalized still describe the citation as it was detected.
func TestVolumeVariantRecordsDetectedForm(t *testing.T) {
	std := "Toth"
	c := &citations.UnlinkedCitation{ID: uuid.New(), Volume: nil, ReporterAbbr: "Toth", Page: 123}
	whitelist := map[string]*citations.WhitelistEntry{"Toth": {ReporterStandard: &std, SingleVol: true}}

	tables := newLinkTables(whitelist, map[string]map[int]*citations.DiffVolEntry{},
		map[string]int64{"1 Toth 123": 777}, nil, nil, nil, nil)
	got := linkCitation(c, tables)

	assert.Equal(t, citations.StatusLinkedCAP, got.Status)
	if assert.NotNil(t, got.CiteLinked) {
		assert.Equal(t, "1 Toth 123", *got.CiteLinked, "cite_linked is the form that matched")
	}
	if assert.NotNil(t, got.CiteCleaned) {
		assert.Equal(t, "Toth 123", *got.CiteCleaned, "cite_cleaned stays the detected form")
	}
	if assert.NotNil(t, got.CiteNormalized) {
		assert.Equal(t, "Toth 123", *got.CiteNormalized, "cite_normalized stays the detected form")
	}
}

func TestStartProgressHeartbeat(t *testing.T) {
	var processed atomic.Int64
	processed.Store(42)

	var mu sync.Mutex
	var counts []int64
	var elapseds []time.Duration

	stop := startProgressHeartbeat(5*time.Millisecond, &processed,
		func(n int64, elapsed time.Duration) {
			mu.Lock()
			defer mu.Unlock()
			counts = append(counts, n)
			elapseds = append(elapseds, elapsed)
		})

	// It reports repeatedly on the timer, not just once.
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(counts) >= 3
	}, 2*time.Second, time.Millisecond, "heartbeat did not report repeatedly")

	stop()

	mu.Lock()
	atStop := len(counts)
	firstCount := counts[0]
	firstElapsed := elapseds[0]
	mu.Unlock()

	// It reports the live count and a positive elapsed time.
	assert.Equal(t, int64(42), firstCount, "heartbeat should report the current processed count")
	assert.Positive(t, firstElapsed, "heartbeat should report elapsed time since it started")

	// stop() waits for the goroutine to exit, so nothing is reported afterward.
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, atStop, len(counts), "heartbeat kept reporting after stop returned")
}
