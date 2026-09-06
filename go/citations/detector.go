package citations

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/lmullen/legal-modernism/go/sources"
)

// Detector contains a reporter and its abbreviation, and implements a detector.
type Detector struct {
	Reporter     string
	Abbreviation string
	initial      *regexp.Regexp // A subset of the regex that finds the starting place
	regex        *regexp.Regexp
	anchored     *regexp.Regexp // regex pinned to the start of the text it is given
	required     string         // a literal every match must contain; "" means no gate
}

// NewDetector creates a new citation detector and initializes its regular expression.
func NewDetector(reporter string, abbreviation string) *Detector {
	pattern := `\d{1,3}\s+` + abbreviation + `\s+\d{1,4}`
	detector := &Detector{
		Reporter:     reporter,
		Abbreviation: abbreviation,
		initial:      regexp.MustCompile(`\d{1,3}\s`),
		regex:        regexp.MustCompile(pattern),
		// \A pins the match to the first byte of whatever string Detect hands
		// it, so the match may run as far as the pattern needs without a window
		// to cut it short. Anchoring also makes the search cheaper than the
		// window scan it replaces: RE2 tries exactly one start position instead
		// of every position in the window.
		anchored: regexp.MustCompile(`\A(?:` + pattern + `)`),
	}
	return detector
}

// stemMinAbbrLen is the shortest abbreviation, counted without its whitespace
// and periods, that may carry the \w* stem. The stem lets an abbreviation reach
// a longer form of the same word, but a short abbreviation is a prefix of far
// too many ordinary words to do that safely: "Jac" reaches "Jackson", "Al"
// reaches "Ala." and "Alexander", "Coop" reaches "Cooper".
//
// Measured over the corpus, the stem produced 1,466,414 rows whose spelling no
// alternate list knows and exactly 130 links, and both of those links came from
// three-character abbreviations. Requiring five characters drops 1,312,136 of
// those rows (89%) and costs the same 130 links as removing the stem outright,
// while leaving the long abbreviations it was written for untouched. The long
// forms of short abbreviations are reached instead by registering them in
// legalhist.reporters_abbreviations, which is what
// db/queries/single-vol-uncovered-spellings.sql finds (issue #283).
const stemMinAbbrLen = 5

// abbrCoreLen counts the characters of an abbreviation that are neither
// whitespace nor a period, so that "Ch. Cas." and "ChCas" are the same length.
// Those are exactly the characters flexAbbr makes optional, so they say nothing
// about how distinctive the abbreviation is.
func abbrCoreLen(abbreviation string) int {
	n := 0
	for _, r := range abbreviation {
		if r == '.' || unicode.IsSpace(r) {
			continue
		}
		n++
	}
	return n
}

// requiredLiteral returns the longest run of non-whitespace characters in the
// abbreviation, which is a literal string every match of the single-volume
// pattern must contain, or "" when the abbreviation has no such run.
//
// The pattern NewSingleVolDetector builds is \b + flexAbbr + stem + [.,]*\s+\d{1,4},
// where flexAbbr is regexp.QuoteMeta(abbreviation) with its spaces replaced by
// [\s.]*. QuoteMeta changes only how a character is spelled in the pattern, not
// what it matches, and the replacement touches nothing but the spaces -- so
// every maximal run of non-whitespace characters in the abbreviation survives
// into the pattern as a contiguous sequence of literal characters that a match
// has to contain. Neither the stem (\w*, which may match nothing) nor the
// separators can supply those bytes.
//
// The gate is therefore conservative in the only direction that matters. If the
// abbreviation holds whitespace this function does not know about -- a tab, a
// non-breaking space -- strings.Fields still splits there while flexAbbr does
// not, so the literal is a substring of what the pattern requires, and the gate
// admits pages it could have skipped. It never rejects one the pattern would
// have matched.
func requiredLiteral(abbreviation string) string {
	longest := ""
	for _, segment := range strings.Fields(abbreviation) {
		if len(segment) > len(longest) {
			longest = segment
		}
	}
	return longest
}

// NewSingleVolDetector creates a new citation detector and initializes its
// regular expression. The detector will not look for a volume number
func NewSingleVolDetector(reporter string, abbreviation string) *Detector {
	// Escape regex metacharacters first so abbreviations containing parens,
	// periods, etc. (e.g. "Bail Eq (SC)") match literally rather than being
	// interpreted as capture groups or wildcards. Then replace literal spaces
	// (which QuoteMeta leaves alone) with [\s.]* to match variant forms:
	// e.g. "M & M" matches "M&M.", "M. & M.", etc.
	flexAbbr := strings.ReplaceAll(regexp.QuoteMeta(abbreviation), " ", `[\s.]*`)
	// The stem is applied only to abbreviations long enough to carry it; see
	// stemMinAbbrLen.
	stem := `\w*`
	if abbrCoreLen(abbreviation) < stemMinAbbrLen {
		stem = ``
	}
	detector := &Detector{
		Reporter:     reporter,
		Abbreviation: abbreviation,
		initial:      nil,
		// \b keeps the abbreviation from matching mid-word: without it "Bur"
		// matches inside "McBur".
		// The stem allows alternate long forms (e.g. Tothill matching Toth),
		// recording the word that actually matched so the whitelist routes it
		// to the right reporter or rejects it outright. It over-captures by
		// design, which is why it is carried only by abbreviations of
		// stemMinAbbrLen characters or more; on a short one it reaches far more
		// ordinary words than reporters.
		// [.,]* allows optional period/comma separators before the page number.
		//
		// Nothing here looks at what precedes the abbreviation, so on its own
		// this detector also matches the tail of a multi-volume citation:
		// "Cal. 185" inside "123 Cal. 185", or "Raym. 45" inside "5 Ld. Raym.
		// 45". RE2 has no lookbehind to rule that out, and a leading optional
		// volume would not see the longer-abbreviation case. RemoveShadows
		// drops those matches afterwards by comparing spans across detectors
		// (issue #267).
		regex: regexp.MustCompile(`\b` + flexAbbr + stem + `[.,]*\s+\d{1,4}`),
		// The literal that lets Detect skip the scan on a page this
		// abbreviation cannot appear on. See requiredLiteral, and the note on
		// Detect for why it is worth having.
		required: requiredLiteral(abbreviation),
	}
	return detector
}

// Detect finds all the examples matching the reporter's abbreviation. Each
// citation records where in the text it was found (Start, End), which is what
// lets RemoveShadows compare the output of several detectors on one page.
func (d *Detector) Detect(doc sources.Document) []*Citation {
	text := doc.Text()

	// A page that does not contain the abbreviation's mandatory literal cannot
	// match this detector's pattern, so skip the scan rather than run it to a
	// foregone conclusion. The corpus is scanned by one detector per
	// (single-volume reporter, spelling) pair -- 1,054 of them as of 2026-09-06
	// -- and each one costs a full pass over the page whether or not it can
	// possibly match. Measured over a random sample of MOML pages, the scans
	// were 99.75% of the detector's per-page CPU and 1,004 of the 1,054 found
	// nothing at all; gating them here cut that stage from 28.0 ms/page to
	// 0.31 ms/page, with identical detections on every one of 4,000 pages.
	//
	// Only the single-volume detectors carry a literal. The generic detectors
	// match an abbreviation by character class, so there is nothing mandatory to
	// test for, and required is empty for them.
	if d.required != "" && !strings.Contains(text, d.required) {
		return nil
	}

	// Hold the spans of the matches that we have detected, as [start, end)
	// byte offsets into text.
	var matches [][]int

	// Some kinds of detectors need to be able to find overlapping strings. If so
	// We need to use a different strategy.
	if d.initial != nil {
		// If the initial detector is present, then we find every place a
		// citation could begin -- a volume number followed by whitespace -- and
		// try to match the whole pattern there. One match per starting place.
		//
		// The match is anchored at the starting place rather than sought inside
		// a fixed-width window. A window bounded how long the match could be as
		// well as where it could start, and the pattern's trailing \d{1,4} has
		// no right-hand boundary, so a citation that ran past the window edge
		// had its page number cut mid-number: "13 Gray, 209" was recorded as
		// page 2, and the truncated row went on to link to a different case
		// than the real one (issue #281).
		//
		// Nothing is lost by requiring the match to begin at the starting place.
		// Every match begins with \d{1,3}\s+, so its own first byte is itself a
		// starting place unless it falls inside an earlier one -- and because
		// the starting places do not overlap, that can only happen inside their
		// digit run, where the alternative is a suffix of the same number
		// ("3 Gray, 209" for "13 Gray, 209"). Those readings are worse than the
		// one already found, not additional citations.
		starts := d.initial.FindAllStringIndex(text, -1)

		for _, start := range starts {
			i := start[0]
			loc := d.anchored.FindStringIndex(text[i:])
			if loc != nil {
				// The location is relative to text[i:], and loc[0] is always 0
				// because the pattern is anchored, so shift the end back into
				// the coordinates of the whole text.
				matches = append(matches, []int{i, i + loc[1]})
			}
		}
	} else {
		// If there is not an initial detector, then just use FindAll
		matches = d.regex.FindAllStringIndex(text, -1)

	}

	// Now turn the matches into citations
	var citations []*Citation
	for _, span := range matches {
		raw := text[span[0]:span[1]]

		// Normalize all whitespace down to a single space. This happens before
		// anything is decided about the match, because the OCR text separates
		// lines with "\n\t\t\t" and every test below assumes a single space:
		// filtering on the unnormalized string let 271,407 case names split
		// across a line break through (issue #283).
		m := reSpace.ReplaceAllString(raw, " ")

		// Filter out the citations which are in these formats
		// 	6 Ex parte Wray, 30
		// 	5 Rex v. Osborn, 7
		// Simply bail early if the matching string contains these substrings.
		if strings.Contains(m, " v. ") || strings.Contains(m, "Ex parte") {
			continue
		}

		c := &Citation{}
		c.ID = uuid.New()
		// Get the raw string and where it was found. Raw is the text as it was
		// scanned, not the normalized form the rest of this loop works on.
		c.Raw = raw
		c.Start = span[0]
		c.End = span[1]

		// Get the volume
		vol := reVolume.FindString(m)
		if vol != "" {
			v, _ := strconv.Atoi(vol)
			c.Volume = &v
		}

		// Get the page
		pp := rePage.FindString(m)
		c.Page, _ = strconv.Atoi(pp)

		// Derive the abbreviation as it actually appeared in the text by
		// trimming the volume and page numbers off the match. Both reVolume and
		// rePage are anchored, so trimming the prefix and suffix leaves any
		// digits inside the abbreviation itself alone. A trailing comma is a
		// separator admitted by the single-volume regex rather than part of the
		// abbreviation, so drop it; trailing periods are kept because they are
		// usually part of the abbreviation ("Ala.", "Hob.").
		//
		// Recording what was detected rather than the reporter the detector was
		// built from is what lets the whitelist normalize single-volume
		// citations at link time, the same way it does for every other citation.
		abbr := m
		abbr = strings.TrimPrefix(abbr, vol)
		abbr = strings.TrimSuffix(abbr, pp)
		abbr = strings.TrimRight(strings.TrimSpace(abbr), " ,")

		// An abbreviation with no letter in it is not a reporter. abbrChar
		// admits whitespace, periods, commas, ampersands and parentheses, so
		// GenericDetector otherwise matches the leader dots between two numbers
		// in a table of contents and records the leader as the reporter: 1.1M
		// such rows, every one of them junk at link time (issue #283).
		if !reHasLetter.MatchString(abbr) {
			continue
		}
		c.ReporterAbbr = abbr

		// Save the source
		c.Source = doc

		citations = append(citations, c)
	}
	return citations
}
