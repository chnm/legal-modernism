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
	// Replace spaces in the abbreviation with [\s.]* to match variant forms:
	// e.g. "M & M" matches "M&M.", "M. & M.", etc.
	flexAbbr := strings.ReplaceAll(abbreviation, " ", `[\s.]*`)
	detector := &Detector{
		Reporter:     reporter,
		Abbreviation: abbreviation,
		initial:      nil,
		// \w* allows alternate long forms (e.g. Tothill matching Toth).
		// [.,]* allows optional period/comma separators before the page number.
		regex: regexp.MustCompile(flexAbbr + `\w*[.,]*\s+\d{1,4}`),
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
		c.Volume, _ = strconv.Atoi(vol)

		// Get the page
		pp := rePage.FindString(m)
		c.Page, _ = strconv.Atoi(pp)

		// Determine the reporter abbreviation.
		if d.initial == nil {
			// Single-volume detector: use the canonical reporter name directly,
			// since the raw match may include separator characters (commas,
			// trailing periods) that are not part of the abbreviation.
			c.ReporterAbbr = d.Reporter
		} else {
			// Standard detector: derive the abbreviation by trimming the volume
			// and page numbers from the raw match.
			abbr := m
			abbr = strings.Replace(abbr, vol, "", 1)
			abbr = strings.Replace(abbr, pp, "", 1)
			abbr = strings.TrimSpace(abbr)
			c.ReporterAbbr = abbr
		}

		// Save the source
		c.Source = doc

		citations = append(citations, c)
	}
	return citations
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
