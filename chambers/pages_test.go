package main

import (
	"testing"

	"github.com/lmullen/legal-modernism/go/sources"
	"github.com/stretchr/testify/require"
)

func TestLocateCitations(t *testing.T) {
	text := "See 2 Mass. 420 and 3 Mo. 391; also 2 Mass. 420 again, and 7 Ad. & El.\n\t540 there."
	cites := []PageCitation{
		{orig: "7 Ad. & El. 540"}, // whitespace differs from the text: relaxed match
		{orig: "2 Mass. 420"},     // occurs twice: first occurrence
		{orig: "9 Wend. 1"},       // absent
		{orig: "3 Mo. 391"},
		{orig: "2 Mass. 420"}, // second occurrence
	}
	got := locateCitations(text, cites)
	var order []string
	for _, c := range got {
		order = append(order, c.orig)
	}
	require.Equal(t, []string{"2 Mass. 420", "3 Mo. 391", "2 Mass. 420", "7 Ad. & El. 540", "9 Wend. 1"}, order)
	require.True(t, got[0].Found)
	require.True(t, got[3].Found, "a cite broken across a line is found with its whitespace relaxed")
	require.False(t, got[4].Found)
	require.Less(t, got[0].pos, got[2].pos, "duplicates claim successive occurrences")
	for i, c := range got {
		require.Equal(t, i+1, c.Index)
	}

	html := string(highlightSpans(text, got))
	require.Contains(t, html, `<mark id="cite-1">2 Mass. 420</mark>`)
	require.Contains(t, html, `<mark id="cite-3">2 Mass. 420</mark>`)
	require.Contains(t, html, "<mark id=\"cite-4\">7 Ad. &amp; El.\n\t540</mark>")
	require.NotContains(t, html, "cite-5", "an unfound citation gets no mark")
}

func TestHighlightSpansNested(t *testing.T) {
	text := "cited at 1 L. R. Ch. 100 <here>"
	cites := locateCitations(text, []PageCitation{{orig: "L. R. Ch. 100"}, {orig: "1 L. R. Ch. 100"}})
	html := string(highlightSpans(text, cites))
	require.Equal(t, 1, countOf(html, "<mark"), "a span inside a longer one is not nested")
	require.Contains(t, html, `<mark id="cite-1">1 L. R. Ch. 100</mark> &lt;here&gt;`)
}

func countOf(s, sub string) int {
	n := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			n++
		}
	}
	return n
}

func TestDetectorText(t *testing.T) {
	rep := sources.NewOCRReplacer([]*sources.OCRSubstitution{{Mistake: "(N. 8.)", Correction: "(N. S.)"}})
	got := detectorText(rep, "1 How. Pr. (N. 8.) 28; L. R. 5 Ch. 100")
	require.Equal(t, "1 How. Pr. (N. S.) 28; 5 L. R. Ch. 100", got)
	require.Equal(t, "as is", detectorText(nil, "as is"), "a nil replacer leaves the text alone")
}

func TestGroupLoCHeadings(t *testing.T) {
	rows := []locRow{
		{"a", "Law"}, {"z", "United States"}, {"v", "Cases."},
		{"a", "Contracts"}, {"x", "History"},
		{"a", ""},
		{"a", "Evidence (Law)"},
	}
	require.Equal(t, []string{
		"Law — United States — Cases",
		"Contracts — History",
		"Evidence (Law)",
	}, groupLoCHeadings(rows))
}
