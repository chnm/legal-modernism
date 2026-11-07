// Command field-code-network derives the data behind chambers' Field Code
// network (/viz/field-code, issue #117) from the research compendium of
// Kellen Funk and Lincoln A. Mullen, "The Spine of American Law: Digital Text
// Analysis and U.S. Legal Practice," American Historical Review 123, no. 1
// (2018), https://doi.org/10.1093/ahr/123.1.132.
//
// The compendium's best_matches.csv gives, for every section of the codes
// and statutes of civil procedure in the article's corpus, the section of an
// earlier code it was most likely borrowed from, if any. This program reduces
// those section matches to the network of the article's figure 5 the way
// scripts/network-graphs.R in the article's repository
// (https://github.com/lmullen/civil-procedure-codes) did: the matches are
// summed into borrowings from one code to another; a borrowing is kept when it
// is at least 50 sections or a fifth of the borrowing code's sections; each
// code keeps at most its two largest, and loses the second when it comes from
// the same state as the first and not from the borrower's own state. Each code
// is then placed by its distance, along the borrowings, from one of the seven
// drafts of New York's Field Code: a Field code, an original borrower (one
// step), a subsequent borrower (two or more), or a code that did not borrow
// from one at all. The three codes of Dakota Territory are labelled, as in the
// figure, by the state that later took them.
//
// Every code in the corpus is written, not only the ones with a borrowing
// left after the pruning: `in_figure` says which. With --figure, the
// hand-edited SVG of the published figure (article/figure-05-handedited-2.svg
// in the article's repository) supplies each code's position in the figure,
// as a fraction of its plotting area, and the position of its label, which the
// authors nudged away from the node where labels collided, so that chambers can
// lay the network out as the figure did.
//
// It writes two JSON files for chambers to embed, one object per line: the
// codes (field-code-codes.json) and the borrowings (field-code-borrowings.json).
// It touches no database (the data may move there later).
//
//	go run ./scripts/field-code-network/ --matches path/to/best_matches.csv \
//	    --figure path/to/figure-05-handedited-2.svg --out chambers/data
package main

import (
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	flag "github.com/spf13/pflag"
)

// The pruning of the network, as network-graphs.R set it.
const (
	minimumSections = 50   // a borrowing of fewer sections than this ...
	minimumShare    = 0.20 // ... and a smaller share of the borrower's sections is dropped
	topBorrowings   = 2    // a code keeps at most this many borrowings (ties kept)
)

// fieldCodes are the seven drafts of New York's Field Code, the centre of the
// network: the codes at distance 0.
var fieldCodes = []string{"NY1848", "NY1849", "NY1850", "NY1851", "NY1853", "NY1864", "NY1873"}

// renames relabel the codes of Dakota Territory by the state that later took
// them, as the figure does.
var renames = map[string]string{"DT1862": "ND1862", "DT1868": "ND1868", "DT1887": "SD1887"}

// The categories the figure colours by.
const (
	categoryField      = "Field code"
	categoryOriginal   = "Original borrower"
	categorySubsequent = "Subsequent borrower"
	categoryNone       = "Did not borrow"
)

// jurisdictions names the two-letter prefixes of the corpus's codes.
var jurisdictions = map[string]string{
	"AK": "Alaska", "AL": "Alabama", "AR": "Arkansas", "AZ": "Arizona", "BI": "British India",
	"CA": "California", "CO": "Colorado", "CT": "Connecticut", "DC": "District of Columbia",
	"DE": "Delaware", "DT": "Dakota Territory", "FL": "Florida", "FR": "France", "GA": "Georgia",
	"GB": "England", "HI": "Hawaii", "IA": "Iowa", "ID": "Idaho", "IL": "Illinois", "IN": "Indiana",
	"KS": "Kansas", "KY": "Kentucky", "LA": "Louisiana", "LC": "Lower Canada", "MA": "Massachusetts",
	"MD": "Maryland", "ME": "Maine", "MI": "Michigan", "MN": "Minnesota", "MO": "Missouri",
	"MS": "Mississippi", "MT": "Montana", "NC": "North Carolina", "ND": "North Dakota",
	"NE": "Nebraska", "NH": "New Hampshire", "NJ": "New Jersey", "NM": "New Mexico", "NV": "Nevada",
	"NY": "New York", "OH": "Ohio", "OK": "Oklahoma", "OR": "Oregon", "PA": "Pennsylvania",
	"RI": "Rhode Island", "SC": "South Carolina", "SD": "South Dakota", "TN": "Tennessee",
	"TX": "Texas", "UT": "Utah", "VA": "Virginia", "WA": "Washington", "WI": "Wisconsin",
	"WV": "West Virginia", "WY": "Wyoming",
}

// Code is one node: a code or statute of civil procedure in the corpus.
type Code struct {
	Code            string   `json:"code"`             // as the figure labels it, ND1862
	CorpusCode      string   `json:"corpus_code"`      // as the corpus names it, DT1862
	State           string   `json:"state"`            // the label's prefix, ND
	Jurisdiction    string   `json:"jurisdiction"`     // of the corpus code, Dakota Territory
	Year            int      `json:"year"`             // of enactment, from the code's name
	Sections        int      `json:"sections"`         // the sections the code was split into
	SectionsMatched int      `json:"sections_matched"` // of them, those matched to an earlier code
	Distance        *int     `json:"distance"`         // steps to a Field code along the borrowings; null when none
	Category        string   `json:"category"`         // Field code, Original borrower, Subsequent borrower, Did not borrow
	InFigure        bool     `json:"in_figure"`        // whether a borrowing survives the pruning, so the figure shows it
	FigureX         *float64 `json:"figure_x"`         // its place in the figure, as a fraction of the plot's width; null when absent
	FigureY         *float64 `json:"figure_y"`         // and of its height, downward
	FigureLabelX    *float64 `json:"figure_label_x"`   // the centre of its label in the figure, the same way; off the node where the authors nudged it
	FigureLabelY    *float64 `json:"figure_label_y"`
}

// figurePlace is a code's node and label in the figure, as fractions of the
// plotting area's width and height.
type figurePlace struct {
	x, y           float64
	labelX, labelY float64
}

// Borrowing is one edge: the sections of one code whose best match is in
// another, earlier code.
type Borrowing struct {
	Borrower         string  `json:"borrower"`
	BorrowedFrom     string  `json:"borrowed_from"`
	SectionsBorrowed int     `json:"sections_borrowed"`
	ShareOfSections  float64 `json:"share_of_sections"` // of the borrower's sections, matched or not
	MeanSimilarity   float64 `json:"mean_similarity"`   // mean Jaccard similarity of the matched sections
}

// match is one row of best_matches.csv that this program reads: a section of
// a code and the code of its best match, if any.
type match struct {
	borrower string
	matched  string // "" for a section with no match
	score    float64
}

func main() {
	var matchesPath, figurePath, outDir string
	flag.StringVar(&matchesPath, "matches", "", "the compendium's best_matches.csv")
	flag.StringVar(&figurePath, "figure", "", "the hand-edited SVG of figure 5, for the codes' positions (optional)")
	flag.StringVar(&outDir, "out", "chambers/data", "directory to write the JSON files to")
	flag.Parse()
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	if matchesPath == "" {
		slog.Error("--matches is required")
		os.Exit(2)
	}
	f, err := os.Open(matchesPath)
	if err != nil {
		slog.Error("opening matches", "error", err)
		os.Exit(1)
	}
	matches, err := readMatches(f)
	f.Close()
	if err != nil {
		slog.Error("reading matches", "error", err)
		os.Exit(1)
	}
	slog.Info("read section matches", "sections", len(matches))

	var positions map[string]figurePlace
	if figurePath != "" {
		g, err := os.Open(figurePath)
		if err != nil {
			slog.Error("opening figure", "error", err)
			os.Exit(1)
		}
		positions, err = figurePositions(g)
		g.Close()
		if err != nil {
			slog.Error("reading figure", "error", err)
			os.Exit(1)
		}
		slog.Info("read the figure's positions", "codes", len(positions))
	}

	codes, borrowings := network(matches, positions)
	placed, shown := 0, 0
	for _, c := range codes {
		if c.InFigure {
			shown++
		}
		if c.FigureX != nil {
			placed++
		}
	}
	slog.Info("derived the network", "codes", len(codes), "in_figure", shown, "positioned", placed, "borrowings", len(borrowings))
	if positions != nil && placed != shown {
		slog.Warn("some codes in the figure have no position, or some positions belong to no code", "in_figure", shown, "positioned", placed)
	}

	if err := writeJSON(filepath.Join(outDir, "field-code-codes.json"), codes); err != nil {
		slog.Error("writing codes", "error", err)
		os.Exit(1)
	}
	if err := writeJSON(filepath.Join(outDir, "field-code-borrowings.json"), borrowings); err != nil {
		slog.Error("writing borrowings", "error", err)
		os.Exit(1)
	}
	slog.Info("wrote the network", "dir", outDir)
}

// readMatches reads best_matches.csv: one row per section of every code, with
// the code of its best match or NA.
func readMatches(r io.Reader) ([]match, error) {
	cr := csv.NewReader(r)
	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("reading header: %w", err)
	}
	col := map[string]int{}
	for i, h := range header {
		col[h] = i
	}
	for _, h := range []string{"borrower_code", "match_code", "score"} {
		if _, ok := col[h]; !ok {
			return nil, fmt.Errorf("column %q missing from header", h)
		}
	}
	var out []match
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		m := match{borrower: rec[col["borrower_code"]]}
		if mc := rec[col["match_code"]]; mc != "NA" && mc != "" {
			m.matched = mc
			if m.score, err = strconv.ParseFloat(rec[col["score"]], 64); err != nil {
				return nil, fmt.Errorf("parsing score for %s: %w", rec[col["borrower_section"]], err)
			}
		}
		out = append(out, m)
	}
	return out, nil
}

// network reduces the section matches to the codes and borrowings of the
// figure, following network-graphs.R, and gives every code of the corpus its
// place in the figure when positions, keyed by the figure's labels, has one.
func network(matches []match, positions map[string]figurePlace) ([]Code, []Borrowing) {
	borrowings := selectBorrowings(summarize(matches))
	distances := distancesToFieldCodes(borrowings)

	sections := map[string]int{}
	matched := map[string]int{}
	for _, m := range matches {
		sections[m.borrower]++
		if m.matched != "" {
			matched[m.borrower]++
		}
	}
	// Every code with a section, and, should a borrowing name a code with
	// none, that code too.
	all := map[string]bool{}
	for code := range sections {
		all[code] = true
	}
	inFigure := map[string]bool{}
	for _, b := range borrowings {
		inFigure[b.Borrower] = true
		inFigure[b.BorrowedFrom] = true
		all[b.Borrower] = true
		all[b.BorrowedFrom] = true
	}

	codes := make([]Code, 0, len(all))
	for corpusCode := range all {
		label := corpusCode
		if r, ok := renames[corpusCode]; ok {
			label = r
		}
		c := Code{
			Code:            label,
			CorpusCode:      corpusCode,
			State:           state(label),
			Jurisdiction:    jurisdictions[state(corpusCode)],
			Year:            year(corpusCode),
			Sections:        sections[corpusCode],
			SectionsMatched: matched[corpusCode],
			Category:        categoryNone,
			InFigure:        inFigure[corpusCode],
		}
		if d, ok := distances[corpusCode]; ok {
			d := d
			c.Distance = &d
			c.Category = category(d)
		}
		if p, ok := positions[label]; ok {
			x, y, lx, ly := p.x, p.y, p.labelX, p.labelY
			c.FigureX, c.FigureY = &x, &y
			c.FigureLabelX, c.FigureLabelY = &lx, &ly
		}
		codes = append(codes, c)
	}
	sort.Slice(codes, func(i, j int) bool {
		if codes[i].Year != codes[j].Year {
			return codes[i].Year < codes[j].Year
		}
		return codes[i].Code < codes[j].Code
	})

	for i := range borrowings {
		if r, ok := renames[borrowings[i].Borrower]; ok {
			borrowings[i].Borrower = r
		}
		if r, ok := renames[borrowings[i].BorrowedFrom]; ok {
			borrowings[i].BorrowedFrom = r
		}
	}
	sort.Slice(borrowings, func(i, j int) bool {
		a, b := borrowings[i], borrowings[j]
		if a.Borrower != b.Borrower {
			return a.Borrower < b.Borrower
		}
		if a.SectionsBorrowed != b.SectionsBorrowed {
			return a.SectionsBorrowed > b.SectionsBorrowed
		}
		return a.BorrowedFrom < b.BorrowedFrom
	})
	return codes, borrowings
}

// candidate is a borrowing before pruning: the sections of one code whose best
// match is in another, the share they are of all the borrower's sections, and
// the mean similarity of the matches.
type candidate struct {
	borrower, from string
	sections       int
	share          float64
	meanScore      float64
}

// summarize sums the section matches into one candidate for each pair of a
// borrowing code and a code it borrowed from (summarize_borrowings in the
// article's R/section-matches.R). The share's denominator counts the
// borrower's unmatched sections too.
func summarize(matches []match) []candidate {
	type pair struct{ borrower, from string }
	count := map[pair]int{}
	score := map[pair]float64{}
	total := map[string]int{}
	for _, m := range matches {
		total[m.borrower]++
		if m.matched == "" {
			continue
		}
		p := pair{m.borrower, m.matched}
		count[p]++
		score[p] += m.score
	}
	out := make([]candidate, 0, len(count))
	for p, n := range count {
		out = append(out, candidate{
			borrower:  p.borrower,
			from:      p.from,
			sections:  n,
			share:     float64(n) / float64(total[p.borrower]),
			meanScore: score[p] / float64(n),
		})
	}
	return out
}

// selectBorrowings prunes the candidates to the figure's edges. A candidate
// must be at least minimumSections or minimumShare of the borrower's
// sections. Each borrower then keeps the candidates ranked in its top
// topBorrowings by sections, with dplyr's top_n semantics: a tie takes one
// rank, so two equal largest leave no room for a third, and a tie for second
// keeps both. Of those, the largest is the head (ties broken by the code's
// name, as the R sort left them); a second from the same state as the head is
// dropped unless that state is the borrower's own, since a state's own codes
// are its usual sources, while two codes of another state most likely reached
// it through one of them.
func selectBorrowings(cands []candidate) []Borrowing {
	byBorrower := map[string][]candidate{}
	for _, c := range cands {
		if c.sections >= minimumSections || c.share >= minimumShare {
			byBorrower[c.borrower] = append(byBorrower[c.borrower], c)
		}
	}
	var out []Borrowing
	for borrower, cs := range byBorrower {
		sort.Slice(cs, func(i, j int) bool {
			if cs[i].sections != cs[j].sections {
				return cs[i].sections > cs[j].sections
			}
			return cs[i].from < cs[j].from
		})
		head := cs[0]
		for _, c := range cs {
			rank := 1
			for _, o := range cs {
				if o.sections > c.sections {
					rank++
				}
			}
			if rank > topBorrowings {
				continue
			}
			if state(c.from) == state(head.from) && c.from != head.from && state(c.from) != state(borrower) {
				continue
			}
			out = append(out, Borrowing{
				Borrower:         c.borrower,
				BorrowedFrom:     c.from,
				SectionsBorrowed: c.sections,
				ShareOfSections:  round(c.share, 4),
				MeanSimilarity:   round(c.meanScore, 3),
			})
		}
	}
	return out
}

// distancesToFieldCodes finds, for every code in the network, the fewest
// borrowings that lead from it to a Field code, following each borrowing from
// the borrower to the code it borrowed from. A Field code is at distance 0; a
// code with no path is absent from the result.
func distancesToFieldCodes(borrowings []Borrowing) map[string]int {
	from := map[string][]string{}
	nodes := map[string]bool{}
	for _, b := range borrowings {
		from[b.Borrower] = append(from[b.Borrower], b.BorrowedFrom)
		nodes[b.Borrower] = true
		nodes[b.BorrowedFrom] = true
	}
	isField := map[string]bool{}
	for _, f := range fieldCodes {
		isField[f] = true
	}
	out := map[string]int{}
	for start := range nodes {
		seen := map[string]bool{start: true}
		frontier := []string{start}
		for d := 0; len(frontier) > 0; d++ {
			var next []string
			for _, n := range frontier {
				if isField[n] {
					out[start] = d
					next = nil
					goto done
				}
				for _, m := range from[n] {
					if !seen[m] {
						seen[m] = true
						next = append(next, m)
					}
				}
			}
			frontier = next
		}
	done:
	}
	return out
}

// category names a distance as the figure does: 0 is a Field code, 1 an
// original borrower, anything farther a subsequent borrower.
func category(distance int) string {
	switch {
	case distance == 0:
		return categoryField
	case distance == 1:
		return categoryOriginal
	default:
		return categorySubsequent
	}
}

// state is the two-letter prefix of a code's name, CA1851 to CA.
func state(code string) string {
	if len(code) < 2 {
		return code
	}
	return code[:2]
}

// year is the four digits of a code's name, CA1851 to 1851. A code named
// otherwise (CA1868short, LA1825french) takes the first four digits.
func year(code string) int {
	for i := 0; i+4 <= len(code); i++ {
		if y, err := strconv.Atoi(code[i : i+4]); err == nil {
			return y
		}
	}
	return 0
}

func round(x float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(x*p) / p
}

// The figure's SVG. ggraph drew each code as a circle with its label centred
// on it, and the authors then moved labels and nodes by hand in Inkscape to
// clear the overlaps, so the labels are no longer centred on the circles: a
// node's position is its circle's centre, and the circle belongs to the label
// nearest it. Hand edits are translate() transforms on the groups that hold
// the texts, and a few matrix() transforms on edges.

// affine is a 2-D affine transform, SVG's matrix(a b c d e f).
type affine struct{ a, b, c, d, e, f float64 }

var identity = affine{1, 0, 0, 1, 0, 0}

func (t affine) apply(x, y float64) (float64, float64) {
	return t.a*x + t.c*y + t.e, t.b*x + t.d*y + t.f
}

// then composes t with a transform applied inside it (a child's), so that a
// point is transformed by u first and then by t.
func (t affine) then(u affine) affine {
	return affine{
		a: t.a*u.a + t.c*u.b, b: t.b*u.a + t.d*u.b,
		c: t.a*u.c + t.c*u.d, d: t.b*u.c + t.d*u.d,
		e: t.a*u.e + t.c*u.f + t.e, f: t.b*u.e + t.d*u.f + t.f,
	}
}

var transformPattern = regexp.MustCompile(`(translate|matrix|scale)\(([^)]*)\)`)

// parseTransform reads an SVG transform attribute: translate, matrix and
// scale, composed left to right. Anything else is an error, since a position
// read through an unknown transform would be wrong.
func parseTransform(s string) (affine, error) {
	t := identity
	rest := strings.TrimSpace(s)
	for rest != "" {
		m := transformPattern.FindStringSubmatchIndex(rest)
		if m == nil || m[0] != 0 {
			return identity, fmt.Errorf("unsupported transform %q", s)
		}
		kind := rest[m[2]:m[3]]
		var args []float64
		for _, a := range strings.FieldsFunc(rest[m[4]:m[5]], func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' }) {
			v, err := strconv.ParseFloat(a, 64)
			if err != nil {
				return identity, fmt.Errorf("transform %q: %w", s, err)
			}
			args = append(args, v)
		}
		var u affine
		switch {
		case kind == "translate" && len(args) == 1:
			u = affine{1, 0, 0, 1, args[0], 0}
		case kind == "translate" && len(args) == 2:
			u = affine{1, 0, 0, 1, args[0], args[1]}
		case kind == "scale" && len(args) == 1:
			u = affine{args[0], 0, 0, args[0], 0, 0}
		case kind == "scale" && len(args) == 2:
			u = affine{args[0], 0, 0, args[1], 0, 0}
		case kind == "matrix" && len(args) == 6:
			u = affine{args[0], args[1], args[2], args[3], args[4], args[5]}
		default:
			return identity, fmt.Errorf("unsupported transform %q", s)
		}
		t = t.then(u)
		rest = strings.TrimSpace(rest[m[1]:])
	}
	return t, nil
}

var codeLabel = regexp.MustCompile(`^[A-Z]{2}\d{4}$`)

// figurePositions reads the hand-edited SVG of figure 5 and returns each
// code's node and label in it, keyed by the label the figure shows (ND1862,
// not DT1862), as fractions of the plotting area's width and height (y
// downward), taken from the clipPath rectangle ggplot drew the panel in. A
// node is the centre of its circle; the circles are matched to the labels
// nearest them, nearest pairs first, so that a label nudged toward a
// neighbour still finds its own circle. Circles below the plotting area are
// the legend's and are left out.
func figurePositions(r io.Reader) (map[string]figurePlace, error) {
	type point struct{ x, y float64 }
	type label struct {
		text string
		x, y float64 // the estimated centre of the text
	}
	var labels []label
	var circles []point
	var clip *[4]float64 // x, y, width, height of the panel
	var viewBox *[4]float64

	stack := []affine{identity}
	inClipPath := 0
	var textAttrs map[string]string
	var textTransform affine
	var textContent strings.Builder
	inText := false

	attr := func(se xml.StartElement, name string) string {
		for _, a := range se.Attr {
			if a.Name.Local == name {
				return a.Value
			}
		}
		return ""
	}
	num := func(s string) float64 {
		v, _ := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(s), "px"), 64)
		return v
	}

	dec := xml.NewDecoder(r)
	dec.Strict = false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parsing SVG: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			parent := stack[len(stack)-1]
			own := identity
			if tr := attr(t, "transform"); tr != "" {
				if own, err = parseTransform(tr); err != nil {
					return nil, err
				}
			}
			current := parent.then(own)
			stack = append(stack, current)
			switch t.Name.Local {
			case "svg":
				if vb := strings.Fields(attr(t, "viewBox")); len(vb) == 4 && viewBox == nil {
					viewBox = &[4]float64{num(vb[0]), num(vb[1]), num(vb[2]), num(vb[3])}
				}
			case "clipPath":
				inClipPath++
			case "rect":
				if inClipPath > 0 && clip == nil {
					clip = &[4]float64{num(attr(t, "x")), num(attr(t, "y")), num(attr(t, "width")), num(attr(t, "height"))}
				}
			case "circle":
				x, y := current.apply(num(attr(t, "cx")), num(attr(t, "cy")))
				circles = append(circles, point{x, y})
			case "text":
				inText = true
				textContent.Reset()
				textAttrs = map[string]string{"x": attr(t, "x"), "y": attr(t, "y"), "textLength": attr(t, "textLength")}
				textTransform = current
			}
		case xml.CharData:
			if inText {
				textContent.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "clipPath":
				inClipPath--
			case "text":
				inText = false
				text := strings.TrimSpace(textContent.String())
				if codeLabel.MatchString(text) {
					// svglite writes x as the left edge of the string and y as
					// its baseline; the centre is half the length along and
					// about a third of the font size up.
					x, y := textTransform.apply(num(textAttrs["x"])+num(textAttrs["textLength"])/2, num(textAttrs["y"])-5)
					labels = append(labels, label{text, x, y})
				}
			}
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	if clip == nil {
		if viewBox == nil {
			return nil, errors.New("the SVG has neither a clipPath rectangle nor a viewBox")
		}
		clip = viewBox
	}
	if len(labels) == 0 {
		return nil, errors.New("the SVG has no code labels")
	}

	// Keep the circles inside the panel (the legend's lie below it), then
	// assign them to labels, nearest pairs first.
	var inPanel []point
	for _, c := range circles {
		if c.x >= clip[0] && c.x <= clip[0]+clip[2] && c.y >= clip[1] && c.y <= clip[1]+clip[3] {
			inPanel = append(inPanel, c)
		}
	}
	type pair struct {
		label, circle int
		d             float64
	}
	var pairs []pair
	for i, l := range labels {
		for j, c := range inPanel {
			if d := math.Hypot(l.x-c.x, l.y-c.y); d <= 60 {
				pairs = append(pairs, pair{i, j, d})
			}
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].d < pairs[j].d })
	labelDone := make([]bool, len(labels))
	circleDone := make([]bool, len(inPanel))
	out := make(map[string]figurePlace, len(labels))
	fx := func(x float64) float64 { return round((x-clip[0])/clip[2], 4) }
	fy := func(y float64) float64 { return round((y-clip[1])/clip[3], 4) }
	for _, p := range pairs {
		if labelDone[p.label] || circleDone[p.circle] {
			continue
		}
		labelDone[p.label] = true
		circleDone[p.circle] = true
		c := inPanel[p.circle]
		l := labels[p.label]
		out[l.text] = figurePlace{x: fx(c.x), y: fy(c.y), labelX: fx(l.x), labelY: fy(l.y)}
	}
	for i, l := range labels {
		if !labelDone[i] {
			return nil, fmt.Errorf("no circle within 60px of the label %s", l.text)
		}
	}
	return out, nil
}

// writeJSON writes a slice as a JSON array with one element per line, so that
// a change to one code or borrowing is one line of a diff.
func writeJSON[T any](path string, items []T) error {
	var sb strings.Builder
	sb.WriteString("[\n")
	for i, item := range items {
		b, err := json.Marshal(item)
		if err != nil {
			return err
		}
		sb.Write(b)
		if i < len(items)-1 {
			sb.WriteString(",")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("]\n")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}
