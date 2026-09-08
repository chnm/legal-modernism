package citations

import "regexp"

// reSeriesPrefix finds a Law Reports citation in its native form, "L. R. 5 Ch.
// 100": the series prefix "L. R." ahead of the volume. The first character of
// the match is whatever precedes "L", captured so the prefix is only matched as
// a word ("Vol." must not become "Vo5 L. R. l."); RE2 has no lookbehind.
var reSeriesPrefix = regexp.MustCompile(`(^|[^\pL])L\.\s?R\.,?\s+(\d{1,3})\s+`)

// NormalizeSeriesPrefix rewrites every "L. R. {volume} " in the text as
// "{volume} L. R. ", so that a citation to the Law Reports of 1865-1875 reaches
// the detectors in the volume-first form every other citation takes:
// "L. R. 5 Ch. 100" becomes "5 L. R. Ch. 100" and is detected with the spelling
// "L. R. Ch.", which the whitelist can send to the Chancery Appeal Cases. Without
// this the generic detector, which begins at the volume, records "5 Ch. 100",
// and the whitelist sends that to whatever older reporter "Ch." names -- Cases
// in Chancery, 1660-1698 -- where it either links to a case two centuries off
// or fails. Measured on a random sample of the corpus, an "L. R." prefix stands
// ahead of 60% of the citations detected as "C. P." and "H. L.", 44% of "Eq."
// and 15% of "Ch." (issue #314).
//
// The prefix is moved rather than dropped so that the series spelling stays
// distinct from the bare abbreviation: "L. R. Ch." is the 1865-1875 series,
// "Ch." is something else. Only the position of the volume changes, and the
// separator after the series prefix is normalized to one space; the series
// spelling itself is left as the OCR read it, for the whitelist to recognize.
// Applied to the page text before detection, after the OCR corrections.
func NormalizeSeriesPrefix(text string) string {
	return reSeriesPrefix.ReplaceAllString(text, "${1}${2} L. R. ")
}
