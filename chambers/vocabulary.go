package main

import (
	"strings"

	"github.com/lmullen/legal-modernism/go/citations"
)

// The linking vocabulary the pages speak: what each status and match tier
// means, in what order they are shown, and what colour they take. The keys are
// the constants the linker writes (go/citations/linker.go), so a tier added
// there without a gloss here fails the vocabulary test rather than reaching a
// page unlabelled.

// TierInfo describes one match tier, or one of the statuses that carry no
// tier, for the tables and charts. JSON tags are the field names the linking
// pages' scripts read.
type TierInfo struct {
	Key    string `json:"key"`
	Status string `json:"status"`
	Color  string `json:"color"`
	Gloss  string `json:"gloss"`
}

// tierVocabulary lists every tier in display order: the linked tiers by
// source, the failure tiers by route in cascade order, then the statuses that
// carry no tier. Each status family shares a hue, and within a family the
// colour darkens with the directness of the match, or how early in the cascade
// the failure came. The anachronistic tiers stand outside the failure ladder --
// a case was found and refused on its date (#319) -- so each takes an off-ramp
// shade of its route's hue.
var tierVocabulary = []TierInfo{
	{citations.TierCAPDirect, citations.StatusLinkedCAP, "#0f5132", "matched a first-page cite in cap.citations"},
	{citations.TierCAPFreelaw, citations.StatusLinkedCAP, "#198754", "matched through the FreeLaw crosswalk"},
	{citations.TierCAPAltSpelling, citations.StatusLinkedCAP, "#3fa76f", "matched cap.citations under an alternate spelling"},
	{citations.TierCAPFreelawAltSpelling, citations.StatusLinkedCAP, "#75c79a", "matched the FreeLaw crosswalk under an alternate spelling"},
	{citations.TierCAPPageInterior, citations.StatusLinkedCAP, "#a8ddc0", "matched inside a case's page range: a pin cite"},
	{citations.TierERDirect, citations.StatusLinkedEnglishReports, "#0a58ca", "matched an English Reports case"},
	{citations.TierERPageInterior, citations.StatusLinkedEnglishReports, "#6ea8fe", "matched inside an English Reports case's page range: a pin cite"},
	{citations.TierCodeDirect, citations.StatusLinkedCodeReporter, "#0dcaf0", "matched a code reporter entry"},
	{citations.TierStubDirect, citations.StatusLinkedStub, "#6f42c1", "matched a stub case: a cite string no source holds, cited often enough to treat as a case (#248)"},
	{citations.TierUSReporterAbsent, citations.StatusNoMatch, "#5c0a14", "no probed spelling of the reporter appears in any US source"},
	{citations.TierUSDiffVolsMissing, citations.StatusNoMatch, "#8a1421", "the reporter renumbers in CAP, but no diffvols row covers this volume"},
	{citations.TierUSVolumeAbsent, citations.StatusNoMatch, "#b02333", "reporter present, this volume never appears"},
	{citations.TierUSVolumeMissing, citations.StatusNoMatch, "#dc3545", "reporter present, but the citation carries no volume to look up"},
	{citations.TierUSPageAbsent, citations.StatusNoMatch, "#e5626f", "reporter and volume present, no case begins on or spans this page"},
	{citations.TierUSPageAmbiguous, citations.StatusNoMatch, "#ee929b", "the page falls in a span, but several cases begin on that span's first page"},
	{citations.TierUSPageGap, citations.StatusNoMatch, "#f5bfc4", "the page falls past the end of the preceding case, in a hole in CAP's coverage"},
	{citations.TierUSAnachronistic, citations.StatusNoMatch, "#c2185b", "a case was found, but decided after the treatise was published (#319)"},
	{citations.TierUKReporterAbsent, citations.StatusNoMatch, "#7a2e0e", "the reporter does not appear in the English Reports index"},
	{citations.TierUKVolumeAbsent, citations.StatusNoMatch, "#a0420f", "reporter present, this volume never appears"},
	{citations.TierUKVolumeMissing, citations.StatusNoMatch, "#c45a12", "reporter present, but the citation carries no volume to look up"},
	{citations.TierUKPageAbsent, citations.StatusNoMatch, "#e2761a", "reporter and volume present, no case begins on or spans this page"},
	{citations.TierUKPageAmbiguous, citations.StatusNoMatch, "#f09a4f", "the cite, or the span covering it, belongs to more than one case"},
	{citations.TierUKPageGap, citations.StatusNoMatch, "#f7c08d", "the page falls past the end of the preceding case, in a hole in the corpus"},
	{citations.TierUKAnachronistic, citations.StatusNoMatch, "#8d5524", "a case was found, but decided after the treatise was published (#319)"},
	{citations.StatusSkippedStatute, citations.StatusSkippedStatute, "#8fa4bd", "no tier: a regnal-year statute citation, never probed"},
	{citations.StatusSkippedJunk, citations.StatusSkippedJunk, "#c9ced3", "no tier: the spelling is junk in the whitelist, never probed"},
	{citations.StatusSkippedNotWhitelisted, citations.StatusSkippedNotWhitelisted, "#e0c36a", "no tier: the spelling is not in the whitelist, never probed"},
	{statusUnprocessed, statusUnprocessed, "#adb5bd", "no tier: not yet linked"},
}

// statusUnprocessed is not a stored status: a citation with no citation_links
// row has not been linked yet.
const statusUnprocessed = "unprocessed"

// statusLabels are the human names of the statuses.
var statusLabels = map[string]string{
	citations.StatusLinkedCAP:             "Linked (CAP)",
	citations.StatusLinkedEnglishReports:  "Linked (English Reports)",
	citations.StatusLinkedCodeReporter:    "Linked (Code Reporter)",
	citations.StatusLinkedStub:            "Linked (stub case)",
	citations.StatusNoMatch:               "No match",
	citations.StatusSkippedStatute:        "Skipped as statute",
	citations.StatusSkippedJunk:           "Skipped as junk",
	citations.StatusSkippedNotWhitelisted: "Not whitelisted",
	statusUnprocessed:                     "Unprocessed",
}

// statusOrder is the order the statuses are listed in, linked first.
var statusOrder = []string{
	citations.StatusLinkedCAP,
	citations.StatusLinkedEnglishReports,
	citations.StatusLinkedCodeReporter,
	citations.StatusLinkedStub,
	citations.StatusNoMatch,
	citations.StatusSkippedStatute,
	citations.StatusSkippedJunk,
	citations.StatusSkippedNotWhitelisted,
	statusUnprocessed,
}

var tierByKey = func() map[string]TierInfo {
	m := make(map[string]TierInfo, len(tierVocabulary))
	for _, t := range tierVocabulary {
		m[t.Key] = t
	}
	return m
}()

// tierGloss returns the one-line meaning of a tier, or "" for an unknown one.
func tierGloss(tier string) string { return tierByKey[tier].Gloss }

// statusLabel returns the human name of a status, or the status itself.
func statusLabel(status string) string {
	if l, ok := statusLabels[status]; ok {
		return l
	}
	return status
}

// Chip is the status and tier of one citation's link, as the pages show it: a
// coloured label with the tier as a tooltip.
type Chip struct {
	Status string // "" when the citation has no link row yet
	Tier   string
}

// chipFor builds a Chip from the nullable columns of citation_links.
func chipFor(status, tier *string) Chip {
	c := Chip{}
	if status != nil {
		c.Status = *status
	}
	if tier != nil {
		c.Tier = *tier
	}
	return c
}

// Linked reports whether the citation resolved to a case, a stub included.
func (c Chip) Linked() bool { return strings.HasPrefix(c.Status, "linked_") }

// Class is the colour class: green for linked, red for no match, gray for the
// junk and statute skips (not case citations at all), amber for the citations
// nothing was attempted on (not whitelisted, or unprocessed).
func (c Chip) Class() string {
	switch {
	case c.Linked():
		return "chip-linked"
	case c.Status == citations.StatusNoMatch:
		return "chip-nomatch"
	case c.Status == citations.StatusSkippedJunk, c.Status == citations.StatusSkippedStatute:
		return "chip-junk"
	default:
		return "chip-skip"
	}
}

// Label is the status in words.
func (c Chip) Label() string {
	if c.Status == "" {
		return statusLabel(statusUnprocessed)
	}
	return statusLabel(c.Status)
}

// Gloss is the tier's meaning, for a tooltip; a tier-less status explains
// itself.
func (c Chip) Gloss() string {
	if c.Tier != "" {
		return c.Tier + ": " + tierGloss(c.Tier)
	}
	return tierGloss(c.Key())
}

// Key is the tier, or the status for the tier-less statuses: the key of the
// vocabulary entry.
func (c Chip) Key() string {
	if c.Tier != "" {
		return c.Tier
	}
	if c.Status == "" {
		return statusUnprocessed
	}
	return c.Status
}
