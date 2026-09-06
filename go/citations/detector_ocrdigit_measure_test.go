package citations

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/lmullen/legal-modernism/go/sources"
	"github.com/stretchr/testify/require"
)

// This is a measurement harness, not an assertion. It quantifies what
// GenericOCRDigitDetector actually costs and buys by running it alongside
// GenericDetector over a sample of real MOML pages and classifying every
// citation it adds against legalhist.whitelist, which is what decides at link
// time whether a detection is a citation or noise.
//
// Counting uses the unique key of moml_citations.citations_unlinked --
// (treatise, page, volume, reporter_abbr, page) -- because that is what
// SaveCitation's ON CONFLICT DO NOTHING collapses on, so it is what would
// actually land in the table. Counting raw matches instead roughly doubles
// every number, since Detect's 25-character windows overlap.
//
// Skipped unless LAW_MEASURE is set, so neither CI nor a normal `go test ./...`
// pays for it. Reads the database through LAW_CLAUDE:
//
//	LAW_MEASURE=1 go test ./go/citations/ -run OCRDigitYield -v -timeout 30m
//
// LAW_SAMPLE_PCT sets the TABLESAMPLE percentage (default 0.25, ~26k pages).
func TestOCRDigitYield(t *testing.T) {
	if os.Getenv("LAW_MEASURE") == "" {
		t.Skip("LAW_MEASURE not set; skipping measurement harness")
	}
	dsn := os.Getenv("LAW_CLAUDE")
	require.NotEmpty(t, dsn, "LAW_MEASURE is set but LAW_CLAUDE is not")
	pct := 0.25
	if v := os.Getenv("LAW_SAMPLE_PCT"); v != "" {
		parsed, err := strconv.ParseFloat(v, 64)
		require.NoError(t, err)
		pct = parsed
	}

	ctx := context.Background()
	pool, err := pgxpool.Connect(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	// The real pipeline corrects OCR before detecting, so the harness must too.
	var subs []*sources.OCRSubstitution
	rows, err := pool.Query(ctx, `SELECT mistake, correction FROM legalhist.ocr_corrections`)
	require.NoError(t, err)
	for rows.Next() {
		var s sources.OCRSubstitution
		require.NoError(t, rows.Scan(&s.Mistake, &s.Correction))
		subs = append(subs, &s)
	}
	// Built once, as cite-detector-moml does: the replacer sorts its rules on
	// construction, so rebuilding it per page would repeat that work.
	ocrReplacer := sources.NewOCRReplacer(subs)
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
	t.Logf("loaded %d OCR corrections, %d whitelist entries", len(subs), len(whitelist))

	rows, err = pool.Query(ctx, fmt.Sprintf(
		`SELECT psmid, pageid, ocrtext FROM moml.page_ocrtext TABLESAMPLE SYSTEM (%f)`, pct))
	require.NoError(t, err)
	defer rows.Close()

	key := func(c *Citation) string {
		vol := "-1"
		if c.Volume != nil {
			vol = strconv.Itoa(*c.Volume)
		}
		return vol + "\x00" + c.ReporterAbbr + "\x00" + strconv.Itoa(c.Page)
	}

	var (
		pages                  int
		baseline               int
		added                  int
		linkable, junk, absent int
		addedByAbbr            = map[string]int{}
		absentByAbbr           = map[string]int{}
		exLinkable, exAbsent   []string
	)

	for rows.Next() {
		var psmid, pageid, text string
		require.NoError(t, rows.Scan(&psmid, &pageid, &text))
		pages++

		doc := sources.NewDoc(psmid+"/"+pageid, text)
		doc.CorrectOCR(ocrReplacer)

		// What the page yields today, keyed as the table would store it.
		existing := map[string]bool{}
		for _, c := range GenericDetector.Detect(doc) {
			existing[key(c)] = true
		}
		baseline += len(existing)

		// Only rows the new detector contributes that are not already there.
		fresh := map[string]*Citation{}
		for _, c := range GenericOCRDigitDetector.Detect(doc) {
			k := key(c)
			if existing[k] || fresh[k] != nil {
				continue
			}
			fresh[k] = c
		}

		for _, c := range fresh {
			added++
			addedByAbbr[c.ReporterAbbr]++
			entry, ok := whitelist[c.ReporterAbbr]
			switch {
			case ok && !entry.junk:
				linkable++
				if len(exLinkable) < 30 {
					exLinkable = append(exLinkable,
						fmt.Sprintf("%-24q -> %-18q => %s", c.Raw, c.ReporterAbbr, entry.standard))
				}
			case ok && entry.junk:
				junk++
			default:
				absent++
				absentByAbbr[c.ReporterAbbr]++
				if len(exAbsent) < 30 {
					exAbsent = append(exAbsent, fmt.Sprintf("%-24q -> %q", c.Raw, c.ReporterAbbr))
				}
			}
		}
	}
	require.NoError(t, rows.Err())

	t.Logf("pages sampled:                %d", pages)
	t.Logf("baseline rows (generic only): %d", baseline)
	t.Logf("rows added by OCR digit:      %d  (%.3f%% of baseline)",
		added, 100*float64(added)/float64(baseline))
	t.Logf("  whitelisted, would link:    %d  (%.1f%% of added)",
		linkable, 100*float64(linkable)/float64(added))
	t.Logf("  whitelisted as junk:        %d", junk)
	t.Logf("  not whitelisted (skipped):  %d", absent)

	t.Log("--- added and linkable ---")
	for _, e := range exLinkable {
		t.Log("  " + e)
	}
	t.Log("--- added but not whitelisted ---")
	for _, e := range exAbsent {
		t.Log("  " + e)
	}
	t.Log("--- most common added abbreviations ---")
	for _, e := range topN(addedByAbbr, 25) {
		t.Log("  " + e)
	}
}

func topN(counts map[string]int, n int) []string {
	type kv struct {
		k string
		v int
	}
	pairs := make([]kv, 0, len(counts))
	for k, v := range counts {
		pairs = append(pairs, kv{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].v != pairs[j].v {
			return pairs[i].v > pairs[j].v
		}
		return pairs[i].k < pairs[j].k
	})
	if len(pairs) > n {
		pairs = pairs[:n]
	}
	out := make([]string, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, fmt.Sprintf("%6d  %q", p.v, p.k))
	}
	return out
}
