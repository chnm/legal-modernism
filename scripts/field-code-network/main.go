// Command field-code-network derives the data behind chambers' Field Code
// network (/viz/field-code, issue #117) from the research compendium of
// Kellen Funk and Lincoln A. Mullen, "The Spine of American Law: Digital Text
// Analysis and U.S. Legal Practice," American Historical Review 123, no. 1
// (2018), https://doi.org/10.1093/ahr/123.1.132.
//
// The compendium's best_matches.csv gives, for every section of the 222 codes
// and statutes of civil procedure in the article's corpus, the section of an
// earlier code it was most likely borrowed from, if any. This program reduces
// those section matches to the network of the article's figure 5 the way
// scripts/network-graphs.R in the article's repository
// (https://github.com/lmullen/civil-procedure-codes) did: the matches are
// summed into borrowings from one code to another; a borrowing is kept when it
// is at least 50 sections or a fifth of the borrowing code's sections; each
// code keeps at most its two largest, and loses the second when it comes from
// the same state as the first and not from the borrower's own state. Each code
// in the network is then placed by its distance, along the borrowings, from
// one of the seven drafts of New York's Field Code: a Field code, an original
// borrower (one step), a subsequent borrower (two or more), or a code that did
// not borrow from one at all. The three codes of Dakota Territory are labelled,
// as in the figure, by the state that later took them.
//
// It writes two JSON files for chambers to embed, one object per line: the
// codes (field-code-codes.json) and the borrowings (field-code-borrowings.json).
// It touches no database (the data may move there later).
//
//	go run ./scripts/field-code-network/ --matches path/to/best_matches.csv --out chambers/data
package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
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

// Code is one node of the network: a code or statute of civil procedure.
type Code struct {
	Code            string `json:"code"`             // as the figure labels it, ND1862
	CorpusCode      string `json:"corpus_code"`      // as the corpus names it, DT1862
	State           string `json:"state"`            // the label's prefix, ND
	Jurisdiction    string `json:"jurisdiction"`     // of the corpus code, Dakota Territory
	Year            int    `json:"year"`             // of enactment, from the code's name
	Sections        int    `json:"sections"`         // the sections the code was split into
	SectionsMatched int    `json:"sections_matched"` // of them, those matched to an earlier code
	Distance        *int   `json:"distance"`         // steps to a Field code along the borrowings; null when none
	Category        string `json:"category"`         // Field code, Original borrower, Subsequent borrower, Did not borrow
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
	var matchesPath, outDir string
	flag.StringVar(&matchesPath, "matches", "", "the compendium's best_matches.csv")
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

	codes, borrowings := network(matches)
	slog.Info("derived the network", "codes", len(codes), "borrowings", len(borrowings))

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
// figure, following network-graphs.R.
func network(matches []match) ([]Code, []Borrowing) {
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

	inNetwork := map[string]bool{}
	for _, b := range borrowings {
		inNetwork[b.Borrower] = true
		inNetwork[b.BorrowedFrom] = true
	}
	codes := make([]Code, 0, len(inNetwork))
	for corpusCode := range inNetwork {
		label := corpusCode
		if r, ok := renames[corpusCode]; ok {
			label = r
		}
		c := Code{
			Code:            label,
			CorpusCode:      corpusCode,
			State:           label[:2],
			Jurisdiction:    jurisdictions[corpusCode[:2]],
			Year:            year(corpusCode),
			Sections:        sections[corpusCode],
			SectionsMatched: matched[corpusCode],
			Category:        categoryNone,
		}
		if d, ok := distances[corpusCode]; ok {
			d := d
			c.Distance = &d
			c.Category = category(d)
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
