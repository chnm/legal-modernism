package main

import (
	"strings"

	"github.com/lmullen/legal-modernism/go/citations"
)

// citeIndex answers two questions about the cite strings in one or more lookup
// maps: does this reporter spelling appear at all, and does this reporter appear
// with this volume. That is what lets a failed cascade report how close it got —
// nothing to match against, wrong volume, or right volume and wrong page — which
// is otherwise recoverable only by re-deriving the diagnosis with a query per
// reporter over the whole citation corpus (issue #255).
//
// The index is derived from the same maps the cascade probes, by parsing their
// keys with the same splitCite the probe strings go through, so the two can never
// disagree about what a reporter or a volume is.
type citeIndex struct {
	reporters map[string]struct{}
	volumes   map[string]struct{}
}

// newCiteIndex builds an index over the keys of every given map. Several maps
// share one index when they are probed as a unit: the US cascade tries CAP, the
// FreeLaw crosswalk, and the code reporter before giving up, so its tier is the
// closest approach across all three.
func newCiteIndex[V any](maps ...map[string]V) *citeIndex {
	ix := &citeIndex{
		reporters: make(map[string]struct{}),
		volumes:   make(map[string]struct{}),
	}
	for _, m := range maps {
		for cite := range m {
			vol, reporter, ok := splitCite(cite)
			if !ok {
				continue
			}
			ix.reporters[reporter] = struct{}{}
			ix.volumes[volumeKey(vol, reporter)] = struct{}{}
		}
	}
	return ix
}

// reached reports whether any of the cite strings probed named a reporter the
// index knows, and whether any named a reporter-and-volume it knows. A probe
// string the index cannot parse is ignored rather than counted as a miss.
func (ix *citeIndex) reached(probes []string) (reporter, volume bool) {
	for _, p := range probes {
		vol, rep, ok := splitCite(p)
		if !ok {
			continue
		}
		if _, found := ix.reporters[rep]; found {
			reporter = true
		}
		if _, found := ix.volumes[volumeKey(vol, rep)]; found {
			volume = true
			// The volume implies the reporter, and nothing deeper is recorded, so
			// there is no reason to keep looking.
			return true, true
		}
	}
	return reporter, volume
}

// volumeKey joins a volume and reporter into a map key. The NUL separator cannot
// occur in either part, so no volume/reporter pair can collide with another.
func volumeKey(vol, reporter string) string {
	return vol + "\x00" + reporter
}

// splitCite splits a cite string of the form "{volume} {reporter} {page}" into
// its volume and reporter parts, discarding the page; vol is empty for the
// volume-less form "{reporter} {page}" that single-volume reporters use. ok is
// false when the string is not a cite at all, which is not hypothetical: the
// code-reporter map deliberately holds keys like "Cox, Manual Trade-Mark Cas."
// that no probe will ever match.
//
// Volume and page stay strings because they are only ever compared with other
// cite strings built the same way; parsing them to int would add failure modes
// for no gain.
func splitCite(cite string) (vol, reporter string, ok bool) {
	sp := strings.LastIndexByte(cite, ' ')
	if sp <= 0 || !allDigits(cite[sp+1:]) {
		return "", "", false // no page, so not a cite
	}
	head := cite[:sp]
	if v := strings.IndexByte(head, ' '); v > 0 && allDigits(head[:v]) {
		return head[:v], head[v+1:], true
	}
	return "", head, true
}

// allDigits reports whether s is a non-empty run of ASCII digits.
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// carriesVolume reports whether any of the probes named a volume at all. When
// none did, the citation was detected without one and the cascade never had a
// volume to look up, which is a different failure from a volume no source holds
// (issue #261). A probe string that cannot be parsed is ignored, as in reached.
//
// This is read off the probes rather than off the citation so that the
// single-volume variant counts: volumeForms adds "1 Toth 123" beside a
// volume-less "Toth 123", and for such a reporter the bare form is a real cite,
// so its failures are volume_absent or page_absent, never volume_missing.
func carriesVolume(probes []string) bool {
	for _, p := range probes {
		if vol, _, ok := splitCite(p); ok && vol != "" {
			return true
		}
	}
	return false
}

// usTier reports which tier a US cascade that failed to link reached, given
// every cite string it probed. diffvolsMissing takes precedence over the volume
// and page tiers: when a reporter renumbers in CAP and no reporters_diffvols row
// covers the cited volume, every probe was built on a volume number we know to be
// untranslated, so how far those probes got says nothing about the citation.
//
// A miss at the volume step is volume_missing when no probe carried a volume
// and volume_absent otherwise. The reporter must be known first: a volume-less
// citation to a reporter no source holds is still reporter_absent.
func usTier(probes []string, ix *citeIndex, diffvolsMissing bool) string {
	reporter, volume := ix.reached(probes)
	switch {
	case !reporter:
		return citations.TierUSReporterAbsent
	case diffvolsMissing:
		return citations.TierUSDiffVolsMissing
	case !volume && !carriesVolume(probes):
		return citations.TierUSVolumeMissing
	case !volume:
		return citations.TierUSVolumeAbsent
	default:
		return citations.TierUSPageAbsent
	}
}

// ukTier reports which tier an English Reports cascade that failed to link
// reached. There is no diffvols equivalent on this route.
//
// ambiguous reports that some probe matched a cite string several cases share.
// It outranks page_absent because it says strictly more: the cite was found, and
// only the choice among the cases sharing it was impossible. It cannot in
// practice conflict with the reporter and volume tiers — the flag is only set by
// a probe that matched a key exactly, and every such key is in the index — but
// the ladder is written in full so it stays correct if the index is ever built
// from something other than the map the cascade probes.
func ukTier(probes []string, ix *citeIndex, ambiguous bool) string {
	reporter, volume := ix.reached(probes)
	switch {
	case !reporter:
		return citations.TierUKReporterAbsent
	case !volume && !carriesVolume(probes):
		return citations.TierUKVolumeMissing
	case !volume:
		return citations.TierUKVolumeAbsent
	case ambiguous:
		return citations.TierUKPageAmbiguous
	default:
		return citations.TierUKPageAbsent
	}
}
