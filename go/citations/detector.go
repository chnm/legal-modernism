package citations

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/lmullen/legal-modernism/go/sources"
)

// Detector contains a reporter and its abbreviation, and implements a detector.
type Detector struct {
	Reporter     string
	Abbreviation string
	initial      *regexp.Regexp // A subset of the regex that finds the starting place
	regex        *regexp.Regexp
}

// NewDetector creates a new citation detector and initializes its regular expression.
func NewDetector(reporter string, abbreviation string) *Detector {
	detector := &Detector{
		Reporter:     reporter,
		Abbreviation: abbreviation,
		initial:      regexp.MustCompile(`\d{1,3}\s`),
		regex:        regexp.MustCompile(`\d{1,3}\s+` + abbreviation + `\s+\d{1,4}`),
	}
	return detector
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
	detector := &Detector{
		Reporter:     reporter,
		Abbreviation: abbreviation,
		initial:      nil,
		// \b keeps the abbreviation from matching mid-word: without it "Bur"
		// matches inside "McBur".
		// \w* allows alternate long forms (e.g. Tothill matching Toth). It
		// deliberately over-captures -- "Al" also matches "Alienation" and
		// "Ala." -- because the abbreviation recorded below is the word that
		// actually matched, which the whitelist then routes to the right
		// reporter or rejects outright.
		// [.,]* allows optional period/comma separators before the page number.
		regex: regexp.MustCompile(`\b` + flexAbbr + `\w*[.,]*\s+\d{1,4}`),
	}
	return detector
}

// Detect finds all the examples matching the reporter's abbreviation.
func (d *Detector) Detect(doc sources.Document) []*Citation {
	// Hold the matches that we have detected that we have detected.
	var matches []string

	// Some kinds of detectors need to be able to find overlapping strings. If so
	// We need to use a different strategy.
	if d.initial != nil {
		// If the initial detector is present, then we need to find the start of potential
		// matches, get a substring, check if there is a match, and if so, add it to
		// the list of matches.
		starts := d.initial.FindAllStringIndex(doc.Text(), -1)

		for _, start := range starts {
			i := start[0]
			substr := getSubstr(doc.Text(), i, 25)
			m := d.regex.FindString(substr) // Only look for one match
			if m != "" {
				// If we have a match, append it to the slice
				matches = append(matches, m)
			}
		}
	} else {
		// If there is not an initial detector, then just use FindAll
		matches = d.regex.FindAllString(doc.Text(), -1)

	}

	// Now turn the matches into citations
	var citations []*Citation
	for _, m := range matches {
		// Filter out the citations which are in these formats
		// 	6 Ex parte Wray, 30
		// 	5 Rex v. Osborn, 7
		// Simply bail early if the matching string contains these substrings.
		if strings.Contains(m, " v. ") || strings.Contains(m, "Ex parte") {
			continue
		}

		c := &Citation{}
		c.ID = uuid.New()
		// Get the raw string
		c.Raw = m

		// Normalize all whitespace down to a single space
		m = reSpace.ReplaceAllString(m, " ")

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
		// A digit surrounded by letters is an OCR misreading of a letter, not a
		// spelling the whitelist should have to carry a row for, so drop it.
		// This is the same kind of repair as legalhist.ocr_corrections, which
		// can only fix spellings someone has enumerated by hand. Raw keeps the
		// uncorrected form.
		c.ReporterAbbr = stripInteriorDigits(abbr)

		// Save the source
		c.Source = doc

		citations = append(citations, c)
	}
	return citations
}

// stripInteriorDigits removes every digit that has a letter on both sides,
// which is how the OCR renders a misread letter inside an abbreviation
// ("F1ed." for "Fed."). It repeats until the string stops changing because
// reInteriorDigit consumes the flanking letters, so a single pass would miss
// the second corruption in a run like "M1a2ss." Each pass shortens the string,
// so the loop terminates.
func stripInteriorDigits(s string) string {
	for {
		stripped := reInteriorDigit.ReplaceAllString(s, "$1$2")
		if stripped == s {
			return s
		}
		s = stripped
	}
}

// Given a string s, start at i and get a substring of length l, but don't
// run beyond the end of the string.
func getSubstr(s string, i int, l int) string {
	end := min(i+l, len(s))
	return s[i:end]
}

// Return the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
