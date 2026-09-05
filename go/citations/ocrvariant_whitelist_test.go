package citations

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/stretchr/testify/require"
)

// TestOCRVariantWhitelistSuggestions compiles candidate legalhist.whitelist rows
// for reporter spellings the OCR corrupted by confusing letters ("Vill." for
// "Will.", "Fcd." for "Fed.") or reading a letter as a digit ("I1l." for
// "Ill."). Every detector records the spelling as scanned, so such a citation
// sits at skipped_not_whitelisted until the whitelist carries it (issue #247).
//
// Unlike TestOCRDigitWhitelistSuggestions this does not sample pages: the
// spellings are already in moml_citations.citations_unlinked, so it reads the
// non-whitelisted ones that occur at least LAW_MIN_COUNT times, proposes
// readings by the rules in ocrvariant.go, keeps only readings that are already
// whitelisted spellings, and applies the ambiguity rule from PR #241 so the
// review file carries the verdict. Nothing is normalized: a reading must match
// a whitelist entry exactly.
//
// Skipped unless LAW_MEASURE is set, so neither CI nor a normal `go test ./...`
// pays for it. Reads the database through LAW_CLAUDE; the two aggregate
// queries scan the whole table and take a few minutes each:
//
//	LAW_MEASURE=1 LAW_MIN_COUNT=20 LAW_UNRESOLVED_MIN=100 \
//	  LAW_SUGGEST_OUT=db/whitelist-candidates-ocr-variants.tsv \
//	  go test ./go/citations/ -run OCRVariantWhitelistSuggestions -v -timeout 2h
func TestOCRVariantWhitelistSuggestions(t *testing.T) {
	if os.Getenv("LAW_MEASURE") == "" {
		t.Skip("LAW_MEASURE not set; skipping suggestion generator")
	}
	dsn := os.Getenv("LAW_CLAUDE")
	require.NotEmpty(t, dsn, "LAW_MEASURE is set but LAW_CLAUDE is not")
	minCount := 20
	if v := os.Getenv("LAW_MIN_COUNT"); v != "" {
		parsed, err := strconv.Atoi(v)
		require.NoError(t, err)
		minCount = parsed
	}
	threshold := 0.10
	if v := os.Getenv("LAW_ALT_THRESHOLD"); v != "" {
		parsed, err := strconv.ParseFloat(v, 64)
		require.NoError(t, err)
		threshold = parsed
	}
	// The unresolved tail is tens of thousands of spellings, most of them the
	// bare surnames the single-volume detectors' word stem produces, so the
	// review file lists only the ones frequent enough to be worth a look.
	unresolvedMin := 100
	if v := os.Getenv("LAW_UNRESOLVED_MIN"); v != "" {
		parsed, err := strconv.Atoi(v)
		require.NoError(t, err)
		unresolvedMin = parsed
	}

	ctx := context.Background()
	pool, err := pgxpool.Connect(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()
	// One connection throughout, so the statement timeout applies to the
	// aggregate queries below.
	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()
	_, err = conn.Exec(ctx, `SET statement_timeout = '3600s'`)
	require.NoError(t, err)

	lookup := &WhitelistLookup{
		Standard:  map[string]string{},
		Junk:      map[string]bool{},
		Canonical: map[string]bool{},
		Freq:      map[string]int{},
	}
	rows, err := conn.Query(ctx,
		`SELECT reporter_found, coalesce(reporter_standard, ''), junk FROM legalhist.whitelist`)
	require.NoError(t, err)
	for rows.Next() {
		var found, std string
		var junk bool
		require.NoError(t, rows.Scan(&found, &std, &junk))
		lookup.Standard[found] = std
		lookup.Junk[found] = junk
	}
	rows.Close()
	rows, err = conn.Query(ctx, `SELECT reporter_standard FROM legalhist.reporters`)
	require.NoError(t, err)
	for rows.Next() {
		var std string
		require.NoError(t, rows.Scan(&std))
		lookup.Canonical[std] = true
	}
	rows.Close()

	// Corpus frequency of every whitelisted spelling, the evidence the
	// ambiguity rule weighs competing readings by.
	t.Log("counting whitelisted spellings")
	started := time.Now()
	rows, err = conn.Query(ctx, `
		SELECT cu.reporter_abbr, count(*)
		FROM moml_citations.citations_unlinked cu
		JOIN legalhist.whitelist w ON w.reporter_found = cu.reporter_abbr
		GROUP BY cu.reporter_abbr`)
	require.NoError(t, err)
	for rows.Next() {
		var abbr string
		var n int
		require.NoError(t, rows.Scan(&abbr, &n))
		lookup.Freq[abbr] = n
	}
	rows.Close()
	t.Logf("counted %d whitelisted spellings in %s", len(lookup.Freq), time.Since(started).Round(time.Second))

	// The candidates: every spelling the whitelist does not know, with the
	// number of citation rows it accounts for and one raw example.
	type candidate struct {
		found   string
		count   int
		example string
	}
	t.Logf("collecting non-whitelisted spellings with at least %d rows", minCount)
	started = time.Now()
	rows, err = conn.Query(ctx, `
		SELECT cu.reporter_abbr, count(*) AS n, min(cu.raw)
		FROM moml_citations.citations_unlinked cu
		WHERE NOT EXISTS (
			SELECT 1 FROM legalhist.whitelist w WHERE w.reporter_found = cu.reporter_abbr
		)
		GROUP BY cu.reporter_abbr
		HAVING count(*) >= $1`, minCount)
	require.NoError(t, err)
	var candidates []candidate
	var candidateRows int
	for rows.Next() {
		var c candidate
		require.NoError(t, rows.Scan(&c.found, &c.count, &c.example))
		candidates = append(candidates, c)
		candidateRows += c.count
	}
	rows.Close()
	t.Logf("collected %d spellings, %d rows, in %s", len(candidates), candidateRows, time.Since(started).Round(time.Second))
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].count != candidates[j].count {
			return candidates[i].count > candidates[j].count
		}
		return candidates[i].found < candidates[j].found
	})

	type verdict struct {
		candidate
		Resolution
	}
	byKind := map[string][]verdict{}
	rowsByKind := map[string]int{}
	var canonicalSpellings, canonicalRows int
	for _, c := range candidates {
		r := resolve(c.found, lookup, threshold)
		byKind[r.Kind] = append(byKind[r.Kind], verdict{c, r})
		rowsByKind[r.Kind] += c.count
		if r.Kind == KindProposed && r.Best.Canonical && r.Best.Rule != RuleDigitLetter {
			canonicalSpellings++
			canonicalRows += c.count
		}
	}

	t.Logf("non-whitelisted spellings (>= %d rows): %d spellings, %d rows", minCount, len(candidates), candidateRows)
	t.Logf("  PROPOSED:    %6d spellings, %9d rows", len(byKind[KindProposed]), rowsByKind[KindProposed])
	t.Logf("    of which canonical and not digit_letter (seedable): %d spellings, %d rows", canonicalSpellings, canonicalRows)
	t.Logf("  AMBIGUOUS:   %6d spellings, %9d rows", len(byKind[KindAmbiguous]), rowsByKind[KindAmbiguous])
	t.Logf("  TOJUNK:      %6d spellings, %9d rows", len(byKind[KindToJunk]), rowsByKind[KindToJunk])
	t.Logf("  UNRESOLVED:  %6d spellings, %9d rows", len(byKind[KindUnresolved]), rowsByKind[KindUnresolved])
	t.Log("--- top proposals ---")
	for i, v := range byKind[KindProposed] {
		if i >= 40 {
			break
		}
		t.Logf("  %7d  %-20q -> %-18q  %-16s  %-12s %-11s e.g. %q",
			v.count, v.found, v.Best.Corrected, v.Best.Standard, v.Best.Rule, confidence(v.Best), v.example)
	}

	out := os.Getenv("LAW_SUGGEST_OUT")
	if out == "" {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# legalhist.whitelist candidates for OCR-corrupted reporter abbreviations (issue #247)\n")
	fmt.Fprintf(&b, "# source: moml_citations.citations_unlinked, spellings not in the whitelist with at least %d rows, %s\n",
		minCount, time.Now().Format("2006-01-02"))
	fmt.Fprintf(&b, "# %d such spellings, %d rows\n", len(candidates), candidateRows)
	fmt.Fprintf(&b, "# rules: digit (a letter-flanked digit removed), confusion (one OCR letter confusion from\n")
	fmt.Fprintf(&b, "# ocrConfusions in go/citations/ocrvariant.go), digit_letter (a letter-flanked digit read as\n")
	fmt.Fprintf(&b, "# some letter; reported for review only, never seeded). Every corrected spelling is an exact\n")
	fmt.Fprintf(&b, "# entry of the whitelist as it stood when this ran; nothing is normalized. REVIEW BEFORE APPLYING.\n")
	fmt.Fprintf(&b, "# confidence is \"canonical\" when the corrected spelling is itself a reporter in legalhist.reporters,\n")
	fmt.Fprintf(&b, "# and \"via-variant\" when it is another whitelist variant that maps onward, in which case the\n")
	fmt.Fprintf(&b, "# proposal inherits that entry's looseness. A spelling whose readings reach more than one reporter\n")
	fmt.Fprintf(&b, "# is AMBIGUOUS when the runner-up's spellings are at least %.0f%%%% as frequent in the corpus as the\n", threshold*100)
	fmt.Fprintf(&b, "# winner's; TOJUNK when every reading lands on a junk entry; UNRESOLVED when none is whitelisted.\n")
	fmt.Fprintf(&b, "#\n# count\treporter_found\tcorrected\treporter_standard\trule\tconfidence\texample\n")
	for _, v := range byKind[KindProposed] {
		fmt.Fprintf(&b, "%d\t%q\t%q\t%q\t%s\t%s\t%q\n",
			v.count, v.found, v.Best.Corrected, v.Best.Standard, v.Best.Rule, confidence(v.Best), v.example)
	}
	fmt.Fprintf(&b, "\n# AMBIGUOUS: readings reach more than one reporter and the runner-up is real competition.\n")
	fmt.Fprintf(&b, "#AMBIGUOUS\tcount\treporter_found\treadings\texample\n")
	for _, v := range byKind[KindAmbiguous] {
		var parts []string
		for _, c := range v.Candidates {
			if lookup.Junk[c.Corrected] {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s -> %s (%s, %d)", c.Corrected, c.Standard, c.Rule, c.Freq))
		}
		fmt.Fprintf(&b, "#AMBIGUOUS\t%d\t%q\t%q\t%q\n", v.count, v.found, strings.Join(parts, "; "), v.example)
	}
	fmt.Fprintf(&b, "\n# TOJUNK: every reading lands on a junk entry; not seeded, but a junk row would be consistent.\n")
	fmt.Fprintf(&b, "#TOJUNK\tcount\treporter_found\tcorrected\texample\n")
	for _, v := range byKind[KindToJunk] {
		fmt.Fprintf(&b, "#TOJUNK\t%d\t%q\t%q\t%q\n", v.count, v.found, v.Candidates[0].Corrected, v.example)
	}
	var omitted, omittedRows int
	for _, v := range byKind[KindUnresolved] {
		if v.count < unresolvedMin {
			omitted++
			omittedRows += v.count
		}
	}
	fmt.Fprintf(&b, "\n# UNRESOLVED: no reading is a whitelisted spelling. Only spellings with at least %d rows are\n", unresolvedMin)
	fmt.Fprintf(&b, "# listed; %d spellings (%d rows) below that are omitted.\n", omitted, omittedRows)
	fmt.Fprintf(&b, "#UNRESOLVED\tcount\treporter_found\texample\n")
	for _, v := range byKind[KindUnresolved] {
		if v.count < unresolvedMin {
			continue
		}
		fmt.Fprintf(&b, "#UNRESOLVED\t%d\t%q\t%q\n", v.count, v.found, v.example)
	}
	require.NoError(t, os.WriteFile(out, []byte(b.String()), 0o644))
	t.Logf("wrote %s", out)
}

// confidence labels a proposal the way the review file and the migration
// builder read it.
func confidence(c Candidate) string {
	if c.Canonical {
		return "canonical"
	}
	return "via-variant"
}
