package citations

// RemoveShadows drops every citation that is a second reading of a citation
// another detector found on the same page, and returns the rest in their
// original order. Two kinds of reading are shadows:
//
// A volume-less citation whose span lies strictly inside another citation's
// span. The single-volume detectors match an abbreviation with no regard for
// what precedes it, so they re-detect the tail of any longer citation that ends
// in that abbreviation: "Cal. 185" inside the generic detector's "123 Cal. 185",
// "Raym. 45" inside "5 Ld. Raym. 45", or "Cas. 45" inside another single-volume
// detector's "Ch. Cas. 45". Each such shadow was a separate row in
// citations_unlinked, where it either failed to link (a volume-less cite to a
// multi-volume reporter can never match) or, worse, linked to the single-volume
// reporter that a citation to California or Lord Raymond never meant (issue
// #267). Only volume-less citations are candidates here, because only the
// single-volume detectors produce them; the containing citation may come from
// any detector.
//
// Any citation whose span lies strictly inside a citation that carries a year.
// A YearDetector match begins at the year, so the generic detector's reading of
// the same citation without it -- "2 K. B. 1" inside "[1905] 2 K. B. 1" -- is
// inside its span. For a reporter cited by year that reading names no case in
// particular, and keeping both would write the same citation twice, once
// usefully and once not.
//
// Containment must be strict: two single-volume detectors whose abbreviations
// are prefixes of one another ("Toth" and "Tothill") produce identical spans for
// the same text, and those are the same citation, not a shadow -- the unique
// index on citations_unlinked collapses them at save time.
//
// The comparison is quadratic in the number of citations on the page, which is
// at most a few hundred.
func RemoveShadows(cites []*Citation) []*Citation {
	kept := make([]*Citation, 0, len(cites))
	for _, c := range cites {
		if shadowed(c, cites) {
			continue
		}
		kept = append(kept, c)
	}
	return kept
}

// shadowed reports whether c's span lies strictly inside the span of another
// citation in cites that it is a lesser reading of: any other citation when c
// carries no volume, or a citation that carries a year.
func shadowed(c *Citation, cites []*Citation) bool {
	for _, o := range cites {
		if o == c {
			continue
		}
		contains := o.Start <= c.Start && c.End <= o.End
		strict := o.Start < c.Start || c.End < o.End
		if !(contains && strict) {
			continue
		}
		if c.Volume == nil || o.Year != nil {
			return true
		}
	}
	return false
}
