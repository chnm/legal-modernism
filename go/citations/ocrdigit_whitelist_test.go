package citations

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/lmullen/legal-modernism/go/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStripInteriorDigits(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no digits", "Fed.", "Fed."},
		{"interior digit", "F1ed.", "Fed."},
		{"interior digit mid-word", "Ma5ss.", "Mass."},
		{"interior digit in Ill.", "I1l.", "Il."},
		{"two interior digits", "M1a2ss.", "Mass."},
		{"adjacent interior digits", "Mi5s5s.", "Miss."},
		{"digit after space is a series designator", "Wn. (2d)", "Wn. (2d)"},
		{"digit after period is a series designator", "A.S.R.3d", "A.S.R.3d"},
		{"edition number survives", "Leach, 4th ed.", "Leach, 4th ed."},
		{"trailing digit survives", "Fed2", "Fed2"},
		{"leading digit survives", "2Fed", "2Fed"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, stripInteriorDigits(tt.in))
		})
	}
}

// TestOCRDigitWhitelistSuggestions compiles the candidate legalhist.whitelist
// rows that GenericOCRDigitDetector needs in order for its citations to link.
// The detector saves the spelling as scanned, so "102 F1ed. 785" stays "F1ed."
// and is skipped_not_whitelisted until the whitelist carries that spelling.
//
// A candidate is only proposed when removing the letter-flanked digits yields a
// spelling the whitelist already resolves to a reporter. That keeps the
// proposal mechanical -- it never invents a reporter, it only says "this looks
// like an existing entry with a digit scanned into it" -- and leaves the
// judgment call to a human. Spellings whose stripped form is unknown, or maps
// to junk, are reported separately and proposed for nothing.
//
// Skipped unless LAW_MEASURE is set, so neither CI nor a normal `go test ./...`
// pays for it. Reads the database through LAW_CLAUDE:
//
//	LAW_MEASURE=1 LAW_SAMPLE_PCT=10 LAW_SUGGEST_OUT=/path/suggestions.tsv \
//	  go test ./go/citations/ -run OCRDigitWhitelistSuggestions -v -timeout 60m
func TestOCRDigitWhitelistSuggestions(t *testing.T) {
	if os.Getenv("LAW_MEASURE") == "" {
		t.Skip("LAW_MEASURE not set; skipping suggestion generator")
	}
	dsn := os.Getenv("LAW_CLAUDE")
	require.NotEmpty(t, dsn, "LAW_MEASURE is set but LAW_CLAUDE is not")
	pct := 10.0
	if v := os.Getenv("LAW_SAMPLE_PCT"); v != "" {
		parsed, err := strconv.ParseFloat(v, 64)
		require.NoError(t, err)
		pct = parsed
	}

	ctx := context.Background()
	pool, err := pgxpool.Connect(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	var subs []*sources.OCRSubstitution
	rows, err := pool.Query(ctx, `SELECT mistake, correction FROM legalhist.ocr_corrections`)
	require.NoError(t, err)
	for rows.Next() {
		var s sources.OCRSubstitution
		require.NoError(t, rows.Scan(&s.Mistake, &s.Correction))
		subs = append(subs, &s)
	}
	rows.Close()

	type wl struct {
		standard string
		junk     bool
	}
	whitelist := make(map[string]wl)
	rows, err = pool.Query(ctx,
		`SELECT reporter_found, coalesce(reporter_standard, ''), junk FROM legalhist.whitelist`)
	require.NoError(t, err)
	for rows.Next() {
		var found, std string
		var junk bool
		require.NoError(t, rows.Scan(&found, &std, &junk))
		whitelist[found] = wl{standard: std, junk: junk}
	}
	rows.Close()

	// A proposal is stronger when the stripped spelling is itself a canonical
	// reporter than when it is only another whitelist variant, because in the
	// latter case the proposal inherits whatever looseness that entry already
	// has -- "E1q." -> "Eq." -> C.L.R. is a longer inference than
	// "M1o." -> "Mo." -> Mo. Flagging the difference points review at the rows
	// that actually need a decision.
	canonical := make(map[string]bool)
	rows, err = pool.Query(ctx, `SELECT reporter_standard FROM legalhist.reporters`)
	require.NoError(t, err)
	for rows.Next() {
		var std string
		require.NoError(t, rows.Scan(&std))
		canonical[std] = true
	}
	rows.Close()

	rows, err = pool.Query(ctx, fmt.Sprintf(
		`SELECT psmid, pageid, ocrtext FROM moml.page_ocrtext TABLESAMPLE SYSTEM (%f)`, pct))
	require.NoError(t, err)
	defer rows.Close()

	// Count each corrupted spelling by the citation rows it would produce, using
	// the table's unique key, so the counts mean "rows that would link" rather
	// than "regex matches".
	counts := map[string]int{}
	examples := map[string]string{}
	var pages int
	for rows.Next() {
		var psmid, pageid, text string
		require.NoError(t, rows.Scan(&psmid, &pageid, &text))
		pages++

		doc := sources.NewDoc(psmid+"/"+pageid, text)
		doc.CorrectOCR(subs)

		seen := map[string]bool{}
		for _, c := range GenericOCRDigitDetector.Detect(doc) {
			vol := "-1"
			if c.Volume != nil {
				vol = strconv.Itoa(*c.Volume)
			}
			k := vol + "\x00" + c.ReporterAbbr + "\x00" + strconv.Itoa(c.Page)
			if seen[k] {
				continue
			}
			seen[k] = true
			counts[c.ReporterAbbr]++
			if _, ok := examples[c.ReporterAbbr]; !ok {
				examples[c.ReporterAbbr] = c.Raw
			}
		}
	}
	require.NoError(t, rows.Err())

	type suggestion struct {
		found     string
		stripped  string
		standard  string
		count     int
		example   string
		canonical bool // stripped form is itself a reporter, not another variant
	}
	var proposed []suggestion
	var unresolved []suggestion
	var alreadyListed int
	var toJunk int

	for found, n := range counts {
		if _, ok := whitelist[found]; ok {
			alreadyListed++
			continue
		}
		stripped := stripInteriorDigits(found)
		entry, ok := whitelist[stripped]
		s := suggestion{found: found, stripped: stripped, count: n,
			example: examples[found], canonical: canonical[stripped]}
		switch {
		case ok && entry.junk:
			toJunk++
		case ok && entry.standard != "":
			s.standard = entry.standard
			proposed = append(proposed, s)
		default:
			unresolved = append(unresolved, s)
		}
	}

	byCount := func(s []suggestion) {
		sort.Slice(s, func(i, j int) bool {
			if s[i].count != s[j].count {
				return s[i].count > s[j].count
			}
			return s[i].found < s[j].found
		})
	}
	byCount(proposed)
	byCount(unresolved)

	var proposedRows, unresolvedRows int
	var canonicalSpellings, canonicalRows int
	for _, s := range proposed {
		proposedRows += s.count
		if s.canonical {
			canonicalSpellings++
			canonicalRows += s.count
		}
	}
	for _, s := range unresolved {
		unresolvedRows += s.count
	}

	t.Logf("pages sampled:                     %d (%.1f%% of corpus)", pages, pct)
	t.Logf("distinct corrupted spellings:      %d", len(counts))
	t.Logf("  already in whitelist:            %d", alreadyListed)
	t.Logf("  strip to a junk entry:           %d", toJunk)
	t.Logf("  PROPOSED (strip to a reporter):  %d spellings, %d citation rows", len(proposed), proposedRows)
	t.Logf("    of which strip to a canonical")
	t.Logf("    reporter (strongest):          %d spellings, %d citation rows", canonicalSpellings, canonicalRows)
	t.Logf("  unresolved (strip to nothing):   %d spellings, %d citation rows", len(unresolved), unresolvedRows)

	t.Log("--- top proposals ---")
	for i, s := range proposed {
		if i >= 40 {
			break
		}
		flag := "via-variant"
		if s.canonical {
			flag = "canonical  "
		}
		t.Logf("  %5d  %-20q -> %-18q  %-16s  %s  e.g. %q",
			s.count, s.found, s.stripped, s.standard, flag, s.example)
	}

	if out := os.Getenv("LAW_SUGGEST_OUT"); out != "" {
		var b strings.Builder
		fmt.Fprintf(&b, "# legalhist.whitelist candidates for OCR digit-corrupted reporter abbreviations\n")
		fmt.Fprintf(&b, "# sample: %d pages (%.1f%% of moml.page_ocrtext)\n", pages, pct)
		fmt.Fprintf(&b, "# reporter_standard is proposed by stripping letter-flanked digits and\n")
		fmt.Fprintf(&b, "# looking the result up in the current whitelist. REVIEW BEFORE APPLYING.\n")
		// The example is a raw OCR match and can contain newlines and tabs, which
		// would otherwise split one record across several lines. Quote every
		// string field so each record stays on exactly one line.
		fmt.Fprintf(&b, "# confidence is \"canonical\" when the stripped spelling is itself a reporter in\n")
		fmt.Fprintf(&b, "# legalhist.reporters, and \"via-variant\" when it is another whitelist variant\n")
		fmt.Fprintf(&b, "# that maps onward, in which case the proposal inherits that entry's looseness.\n")
		fmt.Fprintf(&b, "#\n# count\treporter_found\tstripped\treporter_standard\tconfidence\texample\n")
		for _, s := range proposed {
			conf := "via-variant"
			if s.canonical {
				conf = "canonical"
			}
			fmt.Fprintf(&b, "%d\t%q\t%q\t%q\t%s\t%q\n",
				s.count, s.found, s.stripped, s.standard, conf, s.example)
		}
		fmt.Fprintf(&b, "\n# UNRESOLVED: stripped form is not in the whitelist; no reporter proposed.\n")
		for _, s := range unresolved {
			fmt.Fprintf(&b, "#UNRESOLVED\t%d\t%q\t%q\t%q\n", s.count, s.found, s.stripped, s.example)
		}
		require.NoError(t, os.WriteFile(out, []byte(b.String()), 0o644))
		t.Logf("wrote %s", out)
	}
}
