package citations

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/lmullen/legal-modernism/go/sources"
)

// Finder is what cite-detector-moml runs over a page: anything that finds
// citations in a document. Detector and YearDetector both implement it.
type Finder interface {
	Detect(doc sources.Document) []*Citation
}

// YearDetector finds citations to a reporter that is cited by year. The Law
// Reports after 1890 and the Irish Reports after 1893 restart their volume
// numbers every year, so "[1905] 2 K.B. 1" is page 1 of the second King's
// Bench volume for 1905, and without the year "2 K.B. 1" names a different
// case for every year of the series -- about fifty of them, collapsed into one
// string. The generic detectors begin at the volume and never see the year.
//
// One YearDetector is built per whitelisted spelling of every reporter flagged
// legalhist.reporters.cited_by_year, and it records the year as well as the
// volume, reporter and page. Its match begins at the year, so the generic
// detector's year-less reading of the same citation lies inside its span, and
// RemoveShadows drops that reading: one citation, one row, the one that carries
// the year.
//
// The year is matched as the OCR renders it. In a sample of the corpus a
// bracketed year came through as "[1905]", "(1915)", "t19041", "[1915J",
// "19081" and "[19061", and sometimes bare, "1901 2 K.B., 817". So the pattern
// asks for four digits beginning 18 or 19, then at most one character standing
// in for the closing bracket, an optional comma, an optional volume, the
// spelling, and the page; the opening bracket, if there is one, is taken into
// Raw afterwards. A year that is the tail of a longer number is refused.
type YearDetector struct {
	Reporter     string
	Abbreviation string
	starts       *regexp.Regexp // every place a year could begin
	anchored     *regexp.Regexp // the whole pattern, pinned to a starting place
	required     string         // a literal every match must contain; "" means no gate
}

// yearCloser is what the OCR makes of the bracket that closes a year: itself,
// or nothing, or one of the characters it is misread as ("19081", "[1915J").
// It is bounded to one character so that a digit it absorbs can only ever be
// bracket debris and never the volume: the volume is separated from the year
// by whitespace, and the closer is not.
const yearCloser = `[\]\)\}J1lI|]?`

// NewYearDetector builds the detector for one spelling of a reporter cited by
// year. The spelling is matched the way NewSingleVolDetector matches one, with
// each space standing for any run of whitespace and periods, so "K. B." finds
// "K.B." and "K. B." alike. The recorded ReporterAbbr is the spelling as it
// appeared, which is what the whitelist normalizes at link time.
func NewYearDetector(reporter, abbreviation string) *YearDetector {
	flexAbbr := strings.ReplaceAll(regexp.QuoteMeta(abbreviation), " ", `[\s.]*`)
	// Groups: 1 the year, 2 the volume (optional), 3 the spelling, 4 the page.
	pattern := `\A(1[89]\d\d)` + yearCloser + `\s*,?\s*(?:(\d{1,3})\s+)?(` + flexAbbr + `)[.,]*\s+(\d{1,4})`
	return &YearDetector{
		Reporter:     reporter,
		Abbreviation: abbreviation,
		starts:       regexp.MustCompile(`1[89]\d\d`),
		anchored:     regexp.MustCompile(pattern),
		required:     requiredLiteral(abbreviation),
	}
}

// Detect finds every citation to the reporter that carries a year, recording
// where in the text each was found so RemoveShadows can compare it with what
// the other detectors found on the same page.
func (d *YearDetector) Detect(doc sources.Document) []*Citation {
	text := doc.Text()
	if d.required != "" && !strings.Contains(text, d.required) {
		return nil
	}

	var cites []*Citation
	for _, start := range d.starts.FindAllStringIndex(text, -1) {
		i := start[0]
		// A year is a number of its own, not the tail of a longer one.
		if i > 0 && isASCIIDigit(text[i-1]) {
			continue
		}
		m := d.anchored.FindStringSubmatchIndex(text[i:])
		if m == nil {
			continue
		}
		group := func(n int) string {
			if m[2*n] < 0 {
				return ""
			}
			return text[i+m[2*n] : i+m[2*n+1]]
		}

		c := &Citation{ID: uuid.New(), Source: doc}
		year, _ := strconv.Atoi(group(1))
		c.Year = &year
		if vol := group(2); vol != "" {
			v, _ := strconv.Atoi(vol)
			c.Volume = &v
		}
		// The spelling as it appeared, normalized the way Detect normalizes an
		// abbreviation: one space for any run of whitespace, and no trailing
		// comma, which is a separator the whitelist never carries.
		abbr := reSpace.ReplaceAllString(group(3), " ")
		c.ReporterAbbr = strings.TrimRight(strings.TrimSpace(abbr), " ,")
		c.Page, _ = strconv.Atoi(group(4))

		// Take the opening bracket into the span when the year has one, so Raw
		// reads as the citation was printed.
		c.Start = i
		if i > 0 && isYearOpener(text[i-1]) {
			c.Start = i - 1
		}
		c.End = i + m[1]
		c.Raw = text[c.Start:c.End]
		cites = append(cites, c)
	}
	return cites
}

func isASCIIDigit(b byte) bool { return b >= '0' && b <= '9' }

// isYearOpener reports whether b is a bracket that opens a bracketed year.
func isYearOpener(b byte) bool { return b == '[' || b == '(' || b == '{' }
