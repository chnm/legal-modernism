package citations

import (
	"bufio"
	_ "embed"
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/lmullen/legal-modernism/go/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The literal prefilter lets a single-volume detector skip the regex scan on a
// page that cannot contain its abbreviation. Skipping work is only ever correct
// if it changes nothing, and the failure it would cause -- a citation silently
// not detected -- is invisible downstream: a missing row in citations_unlinked
// looks exactly like a page that had no citations. So every test here compares
// the gated detector against the same detector with the gate removed, over the
// real abbreviations, real treatise text, generated text, and hand-written edge
// cases, and asserts the two agree exactly.
//
// The gate can only ever remove detections (it decides whether to run a scan,
// never what the scan returns), so equality between gated and ungated output is
// the whole correctness condition.

// ungated returns a copy of d with the prefilter disabled: the detector as it
// behaved before the gate existed. Everything else -- the compiled patterns,
// the abbreviation, the matching strategy -- is shared with d, so a comparison
// between the two isolates the gate and nothing else.
func ungated(d *Detector) *Detector {
	clone := *d
	clone.required = ""
	return &clone
}

// fingerprint identifies a detection by everything about it that is not
// randomly generated: where it was found, what was found, and what was parsed
// out of it. Citation.ID is a fresh UUID on every call and is deliberately left
// out.
func fingerprint(c *Citation) string {
	vol := "none"
	if c.Volume != nil {
		vol = fmt.Sprint(*c.Volume)
	}
	return fmt.Sprintf("[%d,%d) raw=%q abbr=%q vol=%s page=%d",
		c.Start, c.End, c.Raw, c.ReporterAbbr, vol, c.Page)
}

func fingerprints(cites []*Citation) []string {
	out := make([]string, 0, len(cites))
	for _, c := range cites {
		out = append(out, fingerprint(c))
	}
	return out
}

// requireSameDetections is the assertion every test in this file is built from.
func requireSameDetections(t *testing.T, d *Detector, doc sources.Document, where string) []*Citation {
	t.Helper()
	got := d.Detect(doc)
	want := ungated(d).Detect(doc)
	require.Equal(t, fingerprints(want), fingerprints(got),
		"prefilter changed the detections for abbreviation %q %s", d.Abbreviation, where)
	return got
}

//go:embed testdata/single-vol-abbrs.tsv
var singleVolAbbrsTSV string

type snapshotAbbr struct {
	reporter string
	abbr     string
}

var (
	snapshotOnce      sync.Once
	snapshotAbbrs     []snapshotAbbr
	snapshotDetectors []*Detector
)

// loadSnapshot reads the (reporter, spelling) pairs the detector is built from
// in production, snapshotted from the database. Compiling 1,054 detectors costs
// enough to be worth doing once for the whole package.
func loadSnapshot(t *testing.T) ([]snapshotAbbr, []*Detector) {
	t.Helper()
	snapshotOnce.Do(func() {
		scanner := bufio.NewScanner(strings.NewReader(singleVolAbbrsTSV))
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			reporter, abbr, found := strings.Cut(line, "\t")
			if !found {
				continue
			}
			snapshotAbbrs = append(snapshotAbbrs, snapshotAbbr{reporter: reporter, abbr: abbr})
			snapshotDetectors = append(snapshotDetectors, NewSingleVolDetector(reporter, abbr))
		}
	})
	require.NotEmpty(t, snapshotAbbrs, "the abbreviation snapshot must not be empty")
	return snapshotAbbrs, snapshotDetectors
}

// TestRequiredLiteral covers what the gate takes to be mandatory, including the
// abbreviation shapes the corpus actually contains: multi-word names, embedded
// parentheses and ampersands, periods used as separators, and the degenerate
// spellings a bad row in reporters_abbreviations could produce.
func TestRequiredLiteral(t *testing.T) {
	tests := []struct {
		name string
		abbr string
		want string
	}{
		{"single word", "Hob.", "Hob."},
		{"no punctuation", "Toth", "Toth"},
		{"two words, second longer", "Ch. Cas.", "Cas."},
		{"two words, first longer", "Bail Eq", "Bail"},
		{"three words, first is longest", "Sel. Cas. Ch.", "Sel."},
		{"parenthesized jurisdiction", "Bail Eq (SC)", "Bail"},
		{"parentheses in the longest run", "Speers Eq.(S.C.)", "Eq.(S.C.)"},
		{"ampersand as its own word", "M & M", "M"},
		{"ampersand attached", "Bro.& L.", "Bro.&"},
		{"periods as separators, no spaces", "Cas.t.Hard.", "Cas.t.Hard."},
		{"dotted initials with spaces", "B.& L.", "B.&"},
		{"court in parentheses is the longest run", "Vern (Eng)", "(Eng)"},
		{"long form", "Fost.Crown Law", "Fost.Crown"},
		{"tie takes the first", "Ab Cd", "Ab"},
		{"leading and trailing spaces", "  Hob.  ", "Hob."},
		{"repeated interior spaces", "Ch.   Cas.", "Cas."},
		{"tab separated", "Ch.\tCas.", "Cas."},
		{"newline separated", "Ch.\nCas.", "Cas."},
		{"non-breaking space", "Ch.\u00a0Cas.", "Cas."},
		{"multibyte characters", "Rép.", "Rép."},
		{"digits in the spelling", "F1ed.", "F1ed."},
		{"single character", "A", "A"},
		{"one period", ".", "."},
		{"empty", "", ""},
		{"only a space", " ", ""},
		{"only whitespace", " \t\n ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, requiredLiteral(tt.abbr))
		})
	}
}

// TestRequiredLiteral_IsLiteralInThePattern checks the structural claim the gate
// rests on: the literal it tests for appears in the compiled pattern as literal
// characters, for every abbreviation in production. If a future change to
// NewSingleVolDetector made part of an abbreviation optional -- another [\s.]*,
// a ? -- this fails rather than quietly dropping citations.
func TestRequiredLiteral_IsLiteralInThePattern(t *testing.T) {
	abbrs, detectors := loadSnapshot(t)
	for i, a := range abbrs {
		lit := requiredLiteral(a.abbr)
		require.NotEmpty(t, lit, "abbreviation %q has no mandatory literal", a.abbr)
		require.Equal(t, lit, detectors[i].required,
			"detector for %q gates on something other than its longest run", a.abbr)
		assert.Contains(t, detectors[i].regex.String(), regexp.QuoteMeta(lit),
			"the literal for %q is not in its pattern verbatim", a.abbr)
	}
}

// matchingVariants returns texts that a single-volume detector for abbr should
// match, built out of the abbreviation itself so that they are correct by
// construction for any spelling. They exercise the ways the pattern is flexible:
// the [\s.]* between words, the [.,]* before the page number, the \w* stem, and
// the surrounding text.
func matchingVariants(abbr string) []string {
	plain := abbr
	noSpaces := strings.ReplaceAll(abbr, " ", "")
	dotted := strings.ReplaceAll(abbr, " ", ".")
	broken := strings.ReplaceAll(abbr, " ", "\n\t\t\t") // how MOML OCR breaks a line
	doubled := strings.ReplaceAll(abbr, " ", "  ")

	return []string{
		plain + " 45",
		plain + " 1",
		plain + " 4567",
		plain + ", 45",
		plain + ". 45",
		plain + ".. 45",
		plain + ",. 45",
		plain + "\t45",
		plain + "\n45",
		plain + "   45",
		noSpaces + " 45",
		dotted + " 45",
		broken + " 45",
		doubled + " 45",
		"See " + plain + " 45 for the rule.",
		"see, e.g., " + plain + " 45; and compare.",
		"(" + plain + " 45)",
		"Résumé of the cases: " + plain + " 45.",
		plain + " 45\n" + plain + " 46\n" + plain + " 47",
		strings.Repeat("filler text ", 40) + plain + " 45" + strings.Repeat(" more text", 40),
	}
}

// nonMatchingVariants returns texts that should yield nothing, including the
// ones that make the gate interesting: the literal is present but the rest of
// the pattern is not.
func nonMatchingVariants(abbr string) []string {
	plain := abbr
	return []string{
		"",
		" ",
		plain,                  // the abbreviation with no page number at all
		plain + " ",            // trailing space, still no page
		plain + "45",           // no separator before the digits
		plain + " page",        // a word where the page number belongs
		plain + " -45",         // a sign is not a digit run the pattern accepts
		"45 " + plain,          // page number on the wrong side
		strings.ToLower(plain), // the pattern is case sensitive
		strings.ToLower(plain) + " 45",
		strings.ToUpper(plain) + " 45",
		"unrelated text with numbers 12 34 56 and no reporter",
		strings.Repeat("x", 500),
	}
}

// TestPrefilter_SnapshotAbbrs_MatchingText runs every abbreviation the detector
// is built from in production against text constructed to match it, and asserts
// the gate lets every one of those detections through. This is the test that
// would fail if the gate made the corpus miss citations.
func TestPrefilter_SnapshotAbbrs_MatchingText(t *testing.T) {
	abbrs, detectors := loadSnapshot(t)

	var checked, found int
	for i, a := range abbrs {
		d := detectors[i]
		for j, text := range matchingVariants(a.abbr) {
			doc := sources.NewDoc(fmt.Sprintf("match-%d-%d", i, j), text)
			got := requireSameDetections(t, d, doc, fmt.Sprintf("on %q", text))
			checked++
			if len(got) > 0 {
				found++
			}
		}
	}
	// Not every variant matches every spelling -- an abbreviation ending in a
	// digit runs into the page number, and one whose last word is a single
	// character behaves differently under [.,]* -- but the great majority do,
	// and a collapse here would mean the variants had stopped exercising the
	// detector at all.
	require.Greater(t, found, checked*3/4,
		"expected most constructed citations to be detected: %d of %d", found, checked)
	t.Logf("%d constructed texts checked, %d yielded a detection", checked, found)
}

// TestPrefilter_SnapshotAbbrs_NonMatchingText is the other half: text the
// detector should find nothing in, including text that contains the gate's
// literal without containing a citation. The gate must not invent a detection
// any more than it may hide one.
func TestPrefilter_SnapshotAbbrs_NonMatchingText(t *testing.T) {
	abbrs, detectors := loadSnapshot(t)
	for i, a := range abbrs {
		d := detectors[i]
		for j, text := range nonMatchingVariants(a.abbr) {
			doc := sources.NewDoc(fmt.Sprintf("nomatch-%d-%d", i, j), text)
			requireSameDetections(t, d, doc, fmt.Sprintf("on %q", text))
		}
	}
}

// TestPrefilter_LiteralPresentWithoutTheRest aims squarely at the case the gate
// cannot decide on its own: the mandatory literal is in the text, so the scan
// runs, and the pattern still has to reject it. Gated and ungated must agree.
func TestPrefilter_LiteralPresentWithoutTheRest(t *testing.T) {
	abbrs, detectors := loadSnapshot(t)
	for i, a := range abbrs {
		lit := requiredLiteral(a.abbr)
		texts := []string{
			lit,
			lit + " and nothing else",
			"a sentence containing " + lit + " but no page number",
			lit + " 45",                 // the literal alone may or may not be the whole abbreviation
			"12 " + lit + " nonsense",   // digits before, none after
			lit + lit + lit,             // the literal repeated
			strings.ToLower(lit) + " 4", // wrong case, right shape
		}
		for j, text := range texts {
			doc := sources.NewDoc(fmt.Sprintf("literal-%d-%d", i, j), text)
			requireSameDetections(t, detectors[i], doc, fmt.Sprintf("on %q", text))
		}
	}
}

// TestPrefilter_HandWrittenCases collects the specific shapes that worried me
// while writing the gate, each with the reasoning attached.
func TestPrefilter_HandWrittenCases(t *testing.T) {
	tests := []struct {
		name     string
		reporter string
		abbr     string
		text     string
	}{
		{"periods stand in for the space", "M & M", "M & M", "The rule in M.&.M. 45 is settled."},
		{"spaces removed entirely", "M & M", "M & M", "The rule in M&M 45 is settled."},
		{"line break inside the abbreviation", "Ch. Cas.", "Ch. Cas.", "cited in Ch.\n\t\t\tCas. 45 today"},
		{"parenthesized jurisdiction", "Bail Eq (SC)", "Bail Eq (SC)", "see Bail Eq (SC) 12"},
		{"parentheses collapsed", "Bail Eq (SC)", "Bail Eq (SC)", "see Bail.Eq.(SC) 12"},
		{"stem reaches the long form", "Toth", "Tothill", "cited in Tothill 234 and again"},
		{"stem on a long abbreviation", "Burrell", "Burrell", "cited in Burrells 234"},
		{"short abbreviation carries no stem", "Bur", "Bur", "cited in Burrell 234"},
		{"abbreviation at the very start", "Hob.", "Hob.", "Hob. 423 opens the page"},
		{"abbreviation at the very end", "Hob.", "Hob.", "the last thing on the page is Hob. 423"},
		{"two citations in a row", "Hob.", "Hob.", "Hob. 423, Hob. 424"},
		{"inside a longer citation", "Raym.", "Raym.", "compare 5 Ld. Raym. 45 here"},
		{"multibyte text before the match", "Missis.", "Missis.", "Résumé: 30 Missis. 673 and Missis. 12"},
		{"multibyte inside the abbreviation", "Rép.", "Rép.", "the French Rép. 45 reporter"},
		{"invalid UTF-8 in the page", "Hob.", "Hob.", "OCR junk \xff\xfe then Hob. 423"},
		{"NUL byte in the page", "Hob.", "Hob.", "junk \x00 then Hob. 423"},
		{"page number longer than the pattern allows", "Hob.", "Hob.", "Hob. 12345 is five digits"},
		{"word boundary blocks a mid-word match", "Bur", "Bur", "McBur 45 should not match"},
		{"the literal appears only lowercased", "Hob.", "Hob.", "hob. 423 is not a citation"},
		{"empty page", "Hob.", "Hob.", ""},
		{"whitespace-only page", "Hob.", "Hob.", " \n\t "},
		{"the abbreviation is the whole page", "Hob.", "Hob.", "Hob."},
		{"digits inside the abbreviation", "Fed.", "F1ed.", "see F1ed. 45 for the OCR case"},
		{"repeated across a long page", "Hob.", "Hob.", strings.Repeat("Hob. 423 filler filler ", 200)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewSingleVolDetector(tt.reporter, tt.abbr)
			doc := sources.NewDoc(tt.name, tt.text)
			requireSameDetections(t, d, doc, "in a hand-written case")
		})
	}
}

// TestPrefilter_DegenerateAbbreviations checks that a bad row in
// reporters_abbreviations -- an empty spelling, whitespace, punctuation alone --
// neither panics nor changes behavior. An abbreviation with no mandatory literal
// is simply not gated.
func TestPrefilter_DegenerateAbbreviations(t *testing.T) {
	texts := []string{"", " ", "45", " 45", ". 45", "Hob. 423", strings.Repeat("a 1 ", 100)}
	for _, abbr := range []string{"", " ", "\t", "\n", "  \t ", ".", ",", "&", "()", "1"} {
		t.Run(fmt.Sprintf("abbr=%q", abbr), func(t *testing.T) {
			d := NewSingleVolDetector("degenerate", abbr)
			if strings.TrimSpace(abbr) == "" {
				assert.Empty(t, d.required, "an all-whitespace abbreviation cannot be gated")
			}
			for _, text := range texts {
				requireSameDetections(t, d, sources.NewDoc("degenerate", text), fmt.Sprintf("on %q", text))
			}
		})
	}
}

// TestPrefilter_GenericDetectorsAreNotGated guards the boundary of the change.
// The generic detectors match abbreviations by character class, so no literal is
// mandatory and gating them would be wrong; they must carry no gate and must go
// on detecting.
func TestPrefilter_GenericDetectorsAreNotGated(t *testing.T) {
	for _, d := range []*Detector{GenericDetector, GenericOCRDigitDetector} {
		t.Run(d.Reporter, func(t *testing.T) {
			assert.Empty(t, d.required, "generic detectors must not be gated")
		})
	}

	text := `This doc has 43 Md. 295 as a citation, and 12 F1ed. 45 as an OCR case,
	plus 6 Watts & S. 314 and 1 C. R. (N. S.) 413 for good measure.`
	doc := sources.NewDoc("generic", text)
	assert.NotEmpty(t, GenericDetector.Detect(doc))
	assert.NotEmpty(t, GenericOCRDigitDetector.Detect(doc))
	requireSameDetections(t, GenericDetector, doc, "for the generic detector")
	requireSameDetections(t, GenericOCRDigitDetector, doc, "for the OCR digit detector")
}

// treatisePages chops the sample treatises into page-sized chunks, so the
// equivalence tests run over real OCR rather than text written to be matched.
// Chunks split on whitespace so a citation is not cut in half by the chunking
// itself, which would make the corpus quietly easier than it is.
func treatisePages(t *testing.T, chunk int) []string {
	t.Helper()
	entries, err := os.ReadDir("../../test-data/sample-treatises")
	require.NoError(t, err)

	var pages []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".txt") {
			continue
		}
		raw, err := os.ReadFile("../../test-data/sample-treatises/" + e.Name())
		require.NoError(t, err)
		text := string(raw)
		for start := 0; start < len(text); {
			end := start + chunk
			if end >= len(text) {
				pages = append(pages, text[start:])
				break
			}
			// Extend to the next whitespace so no citation straddles the cut.
			for end < len(text) && !strings.ContainsRune(" \n\t", rune(text[end])) {
				end++
			}
			pages = append(pages, text[start:end])
			start = end
		}
	}
	require.NotEmpty(t, pages)
	return pages
}

// TestPrefilter_RealTreatisePages is the closest offline analogue of the
// production run: every single-volume detector in the snapshot against real
// treatise pages, gated and ungated, compared detection by detection.
//
// It samples the pages by default so that `go test ./...` stays quick. Set
// LAW_PREFILTER_FULL to run every page of every sample treatise, which is the
// same comparison over roughly 3,000 pages.
func TestPrefilter_RealTreatisePages(t *testing.T) {
	_, detectors := loadSnapshot(t)
	pages := treatisePages(t, 2048)

	stride := 1
	if os.Getenv("LAW_PREFILTER_FULL") == "" {
		stride = len(pages) / 250
		if stride < 1 {
			stride = 1
		}
	}

	var scanned, detected int
	for i := 0; i < len(pages); i += stride {
		doc := sources.NewDoc(fmt.Sprintf("treatise-page-%d", i), pages[i])
		scanned++
		for _, d := range detectors {
			got := d.Detect(doc)
			want := ungated(d).Detect(doc)
			require.Equal(t, fingerprints(want), fingerprints(got),
				"page %d, abbreviation %q", i, d.Abbreviation)
			detected += len(got)
		}
	}
	require.Greater(t, detected, 0, "real treatise pages must yield some single-volume detections")
	t.Logf("%d of %d pages scanned by %d detectors, %d detections, all identical gated and ungated",
		scanned, len(pages), len(detectors), detected)
}

// TestPrefilter_GeneratedPages builds pages out of abbreviations, page numbers,
// punctuation and filler in combinations no real treatise is guaranteed to
// contain: abbreviations run together, citations with the separators shuffled,
// numbers everywhere. The generator is seeded so a failure is reproducible.
func TestPrefilter_GeneratedPages(t *testing.T) {
	abbrs, detectors := loadSnapshot(t)

	filler := []string{
		"the plaintiff", "in error", "see also", "supra", "id.", "ibid.", "cf.",
		"court held", "the statute", "\n\t\t\t", "Résumé", "\xff", "()", "&", ".", ",",
		"12", "345", "6789", "12345", "v.", "Ex parte", "Rex", "ed.", "4th ed.",
	}
	separators := []string{" ", "  ", ".", ", ", ",", "\n", "\t", "\n\t\t\t", ""}

	rng := rand.New(rand.NewSource(20260906))
	pages := make([]string, 0, 300)
	for i := 0; i < 300; i++ {
		var b strings.Builder
		for w := 0; w < 60; w++ {
			switch rng.Intn(3) {
			case 0:
				// A citation-shaped fragment from a real abbreviation.
				a := abbrs[rng.Intn(len(abbrs))].abbr
				b.WriteString(a)
				b.WriteString(separators[rng.Intn(len(separators))])
				b.WriteString(fmt.Sprint(rng.Intn(20000)))
			case 1:
				// A bare abbreviation, so the gate opens with nothing behind it.
				b.WriteString(abbrs[rng.Intn(len(abbrs))].abbr)
			default:
				b.WriteString(filler[rng.Intn(len(filler))])
			}
			b.WriteString(separators[rng.Intn(len(separators))])
		}
		pages = append(pages, b.String())
	}

	for i, page := range pages {
		doc := sources.NewDoc(fmt.Sprintf("generated-%d", i), page)
		for _, d := range detectors {
			got := d.Detect(doc)
			want := ungated(d).Detect(doc)
			require.Equal(t, fingerprints(want), fingerprints(got),
				"generated page %d, abbreviation %q, page text %q", i, d.Abbreviation, page)
		}
	}
}

// TestPrefilter_WholePipeline checks the gate where it actually sits: the full
// detector set for a page, followed by RemoveShadows, exactly as
// cite-detector-moml assembles it. A gate that dropped a long citation would
// also change which short ones count as shadows, so comparing the kept set is a
// stronger check than comparing one detector's output.
func TestPrefilter_WholePipeline(t *testing.T) {
	_, snapshot := loadSnapshot(t)

	detectors := append([]*Detector{GenericDetector, GenericOCRDigitDetector}, snapshot...)
	ungatedSet := make([]*Detector, 0, len(detectors))
	for _, d := range detectors {
		ungatedSet = append(ungatedSet, ungated(d))
	}

	pages := treatisePages(t, 2048)
	stride := len(pages) / 60
	if stride < 1 {
		stride = 1
	}

	run := func(set []*Detector, doc sources.Document) []string {
		var found []*Citation
		for _, d := range set {
			found = append(found, d.Detect(doc)...)
		}
		return fingerprints(RemoveShadows(found))
	}

	var kept int
	for i := 0; i < len(pages); i += stride {
		doc := sources.NewDoc(fmt.Sprintf("pipeline-page-%d", i), pages[i])
		got := run(detectors, doc)
		want := run(ungatedSet, doc)
		require.Equal(t, want, got, "page %d survived RemoveShadows differently", i)
		kept += len(got)
	}
	require.Greater(t, kept, 0)
	t.Logf("%d citations kept after RemoveShadows, identical gated and ungated", kept)
}

// TestPrefilter_OCRCorrectedText runs the comparison on text that has been
// through the OCR corrections, which is what the detector actually scans. A
// correction can create or destroy the gate's literal, so the gate has to be
// applied to the corrected text -- which it is, because Detect reads
// doc.Text().
func TestPrefilter_OCRCorrectedText(t *testing.T) {
	subs := []*sources.OCRSubstitution{
		{Mistake: "Cusl", Correction: "Cush"},
		{Mistake: "Wvis", Correction: "Wis"},
		{Mistake: "H0b.", Correction: "Hob."},
		{Mistake: "T0th", Correction: "Toth"},
	}
	replacer := sources.NewOCRReplacer(subs)

	texts := []string{
		"the case at H0b. 423 was decided",
		"see T0th 234 and T0thill 235",
		"Cusl 45 and Wvis 46 on one page",
		"nothing here needs correcting: Hob. 423",
	}
	detectors := []*Detector{
		NewSingleVolDetector("Hob.", "Hob."),
		NewSingleVolDetector("Toth", "Toth"),
		NewSingleVolDetector("Cush", "Cush"),
		NewSingleVolDetector("Wis", "Wis"),
	}

	for i, text := range texts {
		page := sources.NewTreatisePage(fmt.Sprint(i), "treatise", text)
		page.CorrectOCR(replacer)
		for _, d := range detectors {
			requireSameDetections(t, d, page, "after OCR correction")
		}
	}
}

// TestPrefilter_SkipsTheScanItCanSkip is the performance claim, asserted rather
// than described: on a page holding none of the abbreviations, essentially every
// detector must decline to scan. Without this a gate that silently stopped
// gating -- required left empty, say -- would pass every correctness test in
// this file while restoring the whole cost.
func TestPrefilter_SkipsTheScanItCanSkip(t *testing.T) {
	_, detectors := loadSnapshot(t)

	page := strings.Repeat("the plaintiff filed a demurrer and the court sustained it, 1876. ", 40)
	var wouldScan int
	for _, d := range detectors {
		if strings.Contains(page, d.required) {
			wouldScan++
		}
	}
	assert.Less(t, wouldScan, len(detectors)/10,
		"expected the gate to skip the great majority of detectors on ordinary prose, %d of %d would scan",
		wouldScan, len(detectors))

	// And on a page that does hold an abbreviation, that detector must scan.
	cited := page + " reported at Hob. 423."
	hob := NewSingleVolDetector("Hob.", "Hob.")
	require.True(t, strings.Contains(cited, hob.required))
	assert.NotEmpty(t, hob.Detect(sources.NewDoc("cited", cited)))
}

// FuzzPrefilterEquivalence lets the fuzzer look for an (abbreviation, page) pair
// where the gate and the bare regex disagree. The seeds are the shapes already
// known to be interesting; `go test -fuzz` explores from there.
func FuzzPrefilterEquivalence(f *testing.F) {
	seeds := []struct{ abbr, text string }{
		{"Hob.", "Hob. 423"},
		{"Hob.", "hob. 423"},
		{"Ch. Cas.", "Ch.Cas. 45"},
		{"Ch. Cas.", "Ch.\n\t\t\tCas. 45"},
		{"M & M", "M&M. 45"},
		{"Bail Eq (SC)", "Bail Eq (SC) 12"},
		{"Toth", "Tothill 234"},
		{"Bur", "McBur 45"},
		{"Cas.t.Hard.", "Cas.t.Hard. 45"},
		{"Rép.", "Rép. 45"},
		{"F1ed.", "12 F1ed. 45"},
		{"", "anything 45"},
		{" ", "anything 45"},
		{".", ". 45"},
		{"Hob.", ""},
		{"Hob.", "\xff\xfe Hob. 423"},
		{"Hob.", "Hob.\x0045"},
	}
	for _, s := range seeds {
		f.Add(s.abbr, s.text)
	}

	f.Fuzz(func(t *testing.T, abbr, text string) {
		// Keep the fuzzer on inputs the detector could plausibly be built from;
		// a megabyte-long abbreviation tests the regexp package, not the gate.
		if len(abbr) > 64 || len(text) > 4096 {
			t.Skip()
		}
		d := NewSingleVolDetector("fuzz", abbr)
		doc := sources.NewDoc("fuzz", text)
		got := fingerprints(d.Detect(doc))
		want := fingerprints(ungated(d).Detect(doc))
		if !assert.ObjectsAreEqual(want, got) {
			t.Fatalf("gate changed detections for abbr=%q text=%q:\n gated: %v\nungated: %v",
				abbr, text, got, want)
		}
	})
}

func BenchmarkSingleVolDetectors(b *testing.B) {
	entries, err := os.ReadDir("../../test-data/sample-treatises")
	if err != nil {
		b.Fatal(err)
	}
	var text string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".txt") {
			raw, err := os.ReadFile("../../test-data/sample-treatises/" + e.Name())
			if err != nil {
				b.Fatal(err)
			}
			text = string(raw)
			break
		}
	}
	if len(text) > 2048 {
		text = text[:2048]
	}
	doc := sources.NewDoc("bench", text)

	scanner := bufio.NewScanner(strings.NewReader(singleVolAbbrsTSV))
	var gatedSet, ungatedSet []*Detector
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		reporter, abbr, found := strings.Cut(line, "\t")
		if !found {
			continue
		}
		d := NewSingleVolDetector(reporter, abbr)
		gatedSet = append(gatedSet, d)
		ungatedSet = append(ungatedSet, ungated(d))
	}

	b.Run("gated", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for _, d := range gatedSet {
				d.Detect(doc)
			}
		}
	})
	b.Run("ungated", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for _, d := range ungatedSet {
				d.Detect(doc)
			}
		}
	})
}
