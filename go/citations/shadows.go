package citations

// RemoveShadows drops every volume-less citation whose span lies strictly
// inside another citation's span on the same page, and returns the rest in
// their original order.
//
// The single-volume detectors match an abbreviation with no regard for what
// precedes it, so they re-detect the tail of any longer citation that ends in
// that abbreviation: "Cal. 185" inside the generic detector's "123 Cal. 185",
// "Raym. 45" inside "5 Ld. Raym. 45", or "Cas. 45" inside another
// single-volume detector's "Ch. Cas. 45". Each such shadow was a separate row in
// citations_unlinked, where it either failed to link (a volume-less cite to a
// multi-volume reporter can never match) or, worse, linked to the single-volume
// reporter that a citation to California or Lord Raymond never meant (issue
// #267).
//
// Only volume-less citations are candidates, because only the single-volume
// detectors produce them; the generic detectors always capture a volume. The
// containing citation may come from any detector. Containment must be strict:
// two single-volume detectors whose abbreviations are prefixes of one another
// ("Toth" and "Tothill") produce identical spans for the same text, and those
// are the same citation, not a shadow -- the unique index on citations_unlinked
// collapses them at save time.
//
// The comparison is quadratic in the number of citations on the page, which is
// at most a few hundred.
func RemoveShadows(cites []*Citation) []*Citation {
	kept := make([]*Citation, 0, len(cites))
	for _, c := range cites {
		if c.Volume == nil && shadowed(c, cites) {
			continue
		}
		kept = append(kept, c)
	}
	return kept
}

// shadowed reports whether c's span lies strictly inside the span of any other
// citation in cites.
func shadowed(c *Citation, cites []*Citation) bool {
	for _, o := range cites {
		if o == c {
			continue
		}
		contains := o.Start <= c.Start && c.End <= o.End
		strict := o.Start < c.Start || c.End < o.End
		if contains && strict {
			return true
		}
	}
	return false
}
