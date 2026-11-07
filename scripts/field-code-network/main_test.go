package main

import (
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
		borrowed("XX1900", "CA1851", 2, 0.5), unmatched("XX1900", 98), // below both thresholds: not in the network
	)
	codes, borrowings := network(matches)

	// Sorted by year, then by name; the Dakota code relabelled by its state.
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

	ny := byCode["NY1850"]
	assert.Equal(t, "New York", ny.Jurisdiction)
	require.NotNil(t, ny.Distance)
	assert.Equal(t, 0, *ny.Distance)
	assert.Equal(t, categoryField, ny.Category)

	// A source with no path to a Field code did not borrow, and has no distance.
	assert.Nil(t, byCode["NY1829"].Distance)
	assert.Equal(t, categoryNone, byCode["NY1829"].Category)
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
