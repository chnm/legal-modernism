package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// borrowed makes n section matches of borrower to from, each at score.
func borrowed(borrower, from string, n int, score float64) []match {
	out := make([]match, n)
	for i := range out {
		out[i] = match{borrower: borrower, matched: from, score: score}
	}
	return out
}

// unmatched makes n sections of code with no match.
func unmatched(code string, n int) []match {
	out := make([]match, n)
	for i := range out {
		out[i] = match{borrower: code}
	}
	return out
}

func concat(parts ...[]match) []match {
	var out []match
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// pairs reduces borrowings to their endpoints, for comparing with the
// expected edges.
func pairs(bs []Borrowing) [][2]string {
	out := make([][2]string, 0, len(bs))
	for _, b := range bs {
		out = append(out, [2]string{b.Borrower, b.BorrowedFrom})
	}
	return out
}

func TestSelectBorrowings(t *testing.T) {
	tests := []struct {
		name    string
		matches []match
		want    [][2]string
	}{
		{
			name: "a borrowing needs 50 sections or a fifth of the code",
			matches: concat(
				borrowed("AA1860", "BB1850", 10, 0.5), unmatched("AA1860", 90), // 10% of 100: dropped
				borrowed("CC1860", "BB1850", 10, 0.5), unmatched("CC1860", 30), // 25% of 40: kept
				borrowed("DD1860", "BB1850", 50, 0.5), unmatched("DD1860", 950), // 50 of 1000: kept
			),
			want: [][2]string{{"CC1860", "BB1850"}, {"DD1860", "BB1850"}},
		},
		{
			name: "two equal largest leave no room for a third (dplyr's top_n)",
			matches: concat(
				borrowed("WA1855", "IN1852", 162, 0.5),
				borrowed("WA1855", "OR1854", 162, 0.5),
				borrowed("WA1855", "CA1851", 58, 0.5),
			),
			want: [][2]string{{"WA1855", "IN1852"}, {"WA1855", "OR1854"}},
		},
		{
			name: "a tie for second keeps both",
			matches: concat(
				borrowed("AA1870", "BB1850", 100, 0.5),
				borrowed("AA1870", "CC1860", 50, 0.5),
				borrowed("AA1870", "DD1860", 50, 0.5),
			),
			want: [][2]string{{"AA1870", "BB1850"}, {"AA1870", "CC1860"}, {"AA1870", "DD1860"}},
		},
		{
			name: "a second source from the same state as the first is dropped",
			matches: concat(
				borrowed("NV1869", "CA1858", 300, 0.5),
				borrowed("NV1869", "CA1851", 100, 0.5),
			),
			want: [][2]string{{"NV1869", "CA1858"}},
		},
		{
			name: "unless that state is the borrower's own",
			matches: concat(
				borrowed("CA1872", "CA1868", 300, 0.5),
				borrowed("CA1872", "CA1851", 100, 0.5),
			),
			want: [][2]string{{"CA1872", "CA1868"}, {"CA1872", "CA1851"}},
		},
		{
			name: "a second source from another state is kept",
			matches: concat(
				borrowed("WA1873", "WA1855", 300, 0.5),
				borrowed("WA1873", "CA1872", 100, 0.5),
			),
			want: [][2]string{{"WA1873", "WA1855"}, {"WA1873", "CA1872"}},
		},
		{
			name: "the head of a tie is the first by name, and its state rules the second",
			matches: concat(
				borrowed("NV1869", "CA1858", 100, 0.5),
				borrowed("NV1869", "CA1851", 100, 0.5),
			),
			want: [][2]string{{"NV1869", "CA1851"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectBorrowings(summarize(tt.matches))
			assert.ElementsMatch(t, tt.want, pairs(got))
		})
	}
}

func TestSummarize(t *testing.T) {
	matches := concat(
		borrowed("CA1851", "NY1850", 3, 0.6),
		borrowed("CA1851", "LA1844", 1, 0.3),
		unmatched("CA1851", 4),
	)
	got := summarize(matches)
	require.Len(t, got, 2)
	byFrom := map[string]candidate{}
	for _, c := range got {
		byFrom[c.from] = c
	}
	// The share's denominator is every section of the borrower, matched or not.
	assert.Equal(t, 3, byFrom["NY1850"].sections)
	assert.InDelta(t, 3.0/8.0, byFrom["NY1850"].share, 1e-9)
	assert.InDelta(t, 0.6, byFrom["NY1850"].meanScore, 1e-9)
	assert.InDelta(t, 1.0/8.0, byFrom["LA1844"].share, 1e-9)
}

func TestDistancesAndCategories(t *testing.T) {
	bs := []Borrowing{
		{Borrower: "NY1850", BorrowedFrom: "NY1829"},
		{Borrower: "CA1851", BorrowedFrom: "NY1850"},
		{Borrower: "CA1858", BorrowedFrom: "CA1851"},
		{Borrower: "NV1861", BorrowedFrom: "CA1858"},
		{Borrower: "NV1861", BorrowedFrom: "NY1850"}, // a shortcut: NV1861 is one step, not three
		{Borrower: "MI1847", BorrowedFrom: "MI1839"},
	}
	got := distancesToFieldCodes(bs)
	want := map[string]int{"NY1850": 0, "CA1851": 1, "CA1858": 2, "NV1861": 1}
	assert.Equal(t, want, got)
	for _, absent := range []string{"NY1829", "MI1847", "MI1839"} {
		_, ok := got[absent]
		assert.False(t, ok, absent)
	}

	tests := []struct {
		distance int
		want     string
	}{
		{0, categoryField}, {1, categoryOriginal}, {2, categorySubsequent}, {6, categorySubsequent},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, category(tt.distance))
	}
}

func TestNetwork(t *testing.T) {
	matches := concat(
		borrowed("NY1850", "NY1829", 60, 0.4), unmatched("NY1850", 40),
		borrowed("CA1851", "NY1850", 80, 0.5), unmatched("CA1851", 20),
		borrowed("DT1862", "CA1851", 70, 0.5), unmatched("DT1862", 30),
		borrowed("MI1847", "MI1839", 55, 0.5), unmatched("MI1847", 45),
		borrowed("XX1900", "CA1851", 2, 0.5), unmatched("XX1900", 98), // below both thresholds: in the corpus, not the figure
		unmatched("PA1801", 120), // nothing matched at all: not the figure either
	)
	positions := map[string]figurePlace{
		"NY1850": {x: 0.5, y: 0.5, labelX: 0.5, labelY: 0.5},
		"ND1862": {x: 0.25, y: 0.75, labelX: 0.3, labelY: 0.75},
	}
	codes, borrowings := network(matches, positions)

	// The figure's codes, sorted by year, then by name; the Dakota code
	// relabelled by its state; the codes with no borrowing left out.
	var names []string
	for _, c := range codes {
		names = append(names, c.Code)
	}
	assert.Equal(t, []string{"NY1829", "MI1839", "MI1847", "NY1850", "CA1851", "ND1862"}, names)

	byCode := map[string]Code{}
	for _, c := range codes {
		byCode[c.Code] = c
	}
	nd := byCode["ND1862"]
	assert.Equal(t, "DT1862", nd.CorpusCode)
	assert.Equal(t, "ND", nd.State)
	assert.Equal(t, "Dakota Territory", nd.Jurisdiction)
	assert.Equal(t, 1862, nd.Year)
	assert.Equal(t, 100, nd.Sections)
	assert.Equal(t, 70, nd.SectionsMatched)
	require.NotNil(t, nd.Distance)
	assert.Equal(t, 2, *nd.Distance)
	assert.Equal(t, categorySubsequent, nd.Category)
	require.NotNil(t, nd.FigureX)
	assert.Equal(t, 0.25, *nd.FigureX)
	assert.Equal(t, 0.75, *nd.FigureY)
	require.NotNil(t, nd.FigureLabelX)
	assert.Equal(t, 0.3, *nd.FigureLabelX)
	assert.Equal(t, 0.75, *nd.FigureLabelY)

	ny := byCode["NY1850"]
	assert.Equal(t, "New York", ny.Jurisdiction)
	require.NotNil(t, ny.Distance)
	assert.Equal(t, 0, *ny.Distance)
	assert.Equal(t, categoryField, ny.Category)
	assert.Equal(t, 0.5, *ny.FigureX)

	// A source with no path to a Field code did not borrow, and has no distance.
	assert.Nil(t, byCode["NY1829"].Distance)
	assert.Equal(t, categoryNone, byCode["NY1829"].Category)
	assert.Nil(t, byCode["NY1829"].FigureX, "no position given for it")
	assert.Equal(t, categoryNone, byCode["MI1847"].Category)

	// The borrowings carry the new label too, sorted by borrower.
	assert.Equal(t, [][2]string{
		{"CA1851", "NY1850"}, {"MI1847", "MI1839"}, {"ND1862", "CA1851"}, {"NY1850", "NY1829"},
	}, pairs(borrowings))
	assert.InDelta(t, 0.7, borrowings[2].ShareOfSections, 1e-9)
	assert.InDelta(t, 0.5, borrowings[2].MeanSimilarity, 1e-9)
}

func TestYearAndState(t *testing.T) {
	tests := []struct {
		code  string
		year  int
		state string
	}{
		{"CA1851", 1851, "CA"},
		{"LA1825french", 1825, "LA"},
		{"CA1868short", 1868, "CA"},
		{"NY", 0, "NY"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.year, year(tt.code), tt.code)
		assert.Equal(t, tt.state, state(tt.code), tt.code)
	}
}

func TestParseTransform(t *testing.T) {
	tests := []struct {
		in   string
		x, y float64 // where (10, 20) lands
	}{
		{"", 10, 20},
		{"translate(5,-2)", 15, 18},
		{"translate(5)", 15, 20},
		{"scale(2)", 20, 40},
		{"matrix(1,0,0,1,3,4)", 13, 24},
		{"matrix(2,0,0,0.5,0,0)", 20, 10},
		{"translate(100,0) scale(2)", 120, 40}, // scaled first, then moved
	}
	for _, tt := range tests {
		tr, err := parseTransform(tt.in)
		require.NoError(t, err, tt.in)
		x, y := tr.apply(10, 20)
		assert.InDelta(t, tt.x, x, 1e-9, tt.in)
		assert.InDelta(t, tt.y, y, 1e-9, tt.in)
	}
	_, err := parseTransform("rotate(45)")
	assert.Error(t, err)
}

// A figure in miniature: a panel clipped to a 200 by 100 rectangle at (10,
// 50), two codes whose labels were nudged by hand (so the text is not centred
// on its circle), one label placed closer to the other code's circle than its
// own, a legend circle below the panel, and an edge drawn under a matrix
// transform, which must not break the parse.
const testSVG = `<?xml version="1.0"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 220 180">
  <defs><clipPath id="c"><rect x="10" y="50" width="200" height="100"/></clipPath></defs>
  <polyline points="60,100 160,100" transform="matrix(1,0,0,1,0,0)"/>
  <circle cx="60" cy="100" r="5"/>
  <circle cx="160" cy="100" r="5"/>
  <g transform="translate(12,0)">
    <text x="35" y="105" textLength="50px">NY1850</text>
  </g>
  <g transform="translate(-30,0)">
    <text x="135" y="105" textLength="50px">CA1851</text>
  </g>
  <text x="150" y="110" textLength="50px">not a code</text>
  <circle cx="60" cy="170" r="5"/>
  <text x="70" y="172">0</text>
</svg>`

func TestFigurePositions(t *testing.T) {
	got, err := figurePositions(strings.NewReader(testSVG))
	require.NoError(t, err)
	// NY1850's label centre is at (72, 100): its own circle at 60 is 12 away,
	// CA1851's at 160 is 88 away. CA1851's label centre is at (130, 100): its
	// own circle is 30 away and NY1850's 70. Each gets its own, as fractions
	// of the 200 by 100 panel at (10, 50), and keeps its label's own place.
	require.Len(t, got, 2)
	assert.Equal(t, figurePlace{x: 0.25, y: 0.5, labelX: 0.31, labelY: 0.5}, got["NY1850"])
	assert.Equal(t, figurePlace{x: 0.75, y: 0.5, labelX: 0.6, labelY: 0.5}, got["CA1851"])

	_, err = figurePositions(strings.NewReader(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><text x="1" y="1">NY1850</text></svg>`))
	assert.Error(t, err, "a label with no circle near it")
}
