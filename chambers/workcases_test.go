package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func ptr[T any](v T) *T { return &v }

// matrixTestEditions is three columns: London 1824 (UK), Philadelphia 1831
// (US), and an undated London edition.
func matrixTestEditions() []MatrixEdition {
	uk, us := "UK", "US"
	return []MatrixEdition{
		{WorkEdition: WorkEdition{BiblioID: "ed1", Title: "A treatise on pleading", Year: ptr(1824), Jurisdiction: &uk,
			Place: ptr("London"), Imprint: ptr("London :  J. Butterworth, 1824."), Author: ptr("Stephen, Henry John")},
			Cited: 3, Entered: 3, Left: 1},
		{WorkEdition: WorkEdition{BiblioID: "ed2", Title: "A treatise on pleading", Year: ptr(1831), Jurisdiction: &us,
			Place: ptr("Philadelphia"), EditionStatement: ptr("2nd American ed., with large additions /  by Francis J. Troubat."),
			Derivative: true},
			Cited: 3, Entered: 1, Left: 2},
		{WorkEdition: WorkEdition{BiblioID: "ed3", Title: "A treatise on pleading", Jurisdiction: &uk},
			Cited: 1, Entered: 0, Left: 1},
	}
}

func TestStatementHead(t *testing.T) {
	tests := []struct {
		name      string
		statement *string
		want      string
	}{
		{"none", nil, ""},
		{"whole", ptr("5th ed."), "5th ed."},
		{"comma", ptr("2nd ed., with corrections and improvements."), "2nd ed."},
		{"slash", ptr("3rd ed. /  corr.  ... and otherwise improved."), "3rd ed."},
		{"comma before slash", ptr("2nd American ed., with large additions /  by Francis J. Troubat."), "2nd American ed."},
		{"semicolon", ptr("New ed. ; revised"), "New ed."},
		{"long head", ptr("3rd American from the 2nd London ed.,  ... / by Samuel Tyler."), "3rd American from the 2nd London ed."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, statementHead(tt.statement))
		})
	}
}

func TestMatrixEdition(t *testing.T) {
	eds := matrixTestEditions()

	require.Equal(t, "uk", eds[0].Class())
	require.Equal(t, "us", eds[1].Class())
	require.Equal(t, "1824", eds[0].YearLabel())
	require.Equal(t, "n.d.", eds[2].YearLabel())

	require.Equal(t, "1824, London", eds[0].Name())
	require.Equal(t, "1831, Philadelphia, 2nd American ed.", eds[1].Name())
	require.Equal(t, "n.d.", eds[2].Name())

	require.Equal(t, "", eds[0].Statement())
	require.Equal(t, "2nd American ed.", eds[1].Statement())
	long := MatrixEdition{WorkEdition: WorkEdition{EditionStatement: ptr("3rd American from the 2nd London ed.,  ... / by Samuel Tyler.")}}
	require.Equal(t, "3rd American from the 2nd…", long.Statement())

	// The tooltip is the record, a line a part, with the runs of spaces the
	// catalogue leaves collapsed.
	require.Equal(t, "A treatise on pleading\nLondon : J. Butterworth, 1824.\nStephen, Henry John", eds[0].Tooltip())
	require.Equal(t, "A treatise on pleading\n2nd American ed., with large additions / by Francis J. Troubat.\nDerivative edition", eds[1].Tooltip())
}

func TestMatrixSorts(t *testing.T) {
	require.Equal(t, "editions", normalizeMatrixSort(""))
	require.Equal(t, "editions", normalizeMatrixSort("cites; DROP TABLE"))
	for sort, order := range matrixSorts {
		require.Equal(t, sort, normalizeMatrixSort(sort))
		require.Contains(t, order, matrixCaseKey, "%s: ends on the case's key", sort)
	}
	require.Contains(t, matrixSorts["year"], caseYearExpr)
}

func TestMatrixPageSize(t *testing.T) {
	tests := []struct {
		editions int
		want     int
	}{
		{0, matrixMaxRows},
		{1, matrixMaxRows},
		{18, 5000},  // Stephen on Pleading, whose 3,219 cases are one page
		{50, 5000},  // the two limits meet at fifty editions
		{100, 2500}, // then the cells bind
		{119, 2100}, // Blackstone
		{1000000, 1},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, matrixPageSize(tt.editions), "%d editions", tt.editions)
	}
}

func TestMatrixArgs(t *testing.T) {
	ids, years := matrixArgs(matrixTestEditions())
	require.Equal(t, []string{"ed1", "ed2", "ed3"}, ids)
	require.Len(t, years, 3)
	require.Equal(t, int32(1824), *years[0])
	require.Equal(t, int32(1831), *years[1])
	require.Nil(t, years[2], "an undated edition has no year to span")
}

func TestMatrixColumns(t *testing.T) {
	uk, us := "UK", "US"
	editions := []WorkEdition{
		{BiblioID: "london1", Jurisdiction: &uk, Year: ptr(1824)},
		{BiblioID: "pamphlet", Year: ptr(1826)}, // outside the treatise view
		{BiblioID: "phila2", Jurisdiction: &us, Year: ptr(1831)},
		{BiblioID: "london3", Jurisdiction: &uk, Year: ptr(1835)},
		{BiblioID: "phila4", Jurisdiction: &us, Year: ptr(1841)},
		{BiblioID: "phila5", Jurisdiction: &us, Year: ptr(1845)},
	}
	tests := []struct {
		jur  string
		want []string
	}{
		{"", []string{"london1", "phila2", "london3", "phila4", "phila5"}},
		{"us", []string{"phila2", "phila4", "phila5"}},
		{"uk", []string{"london1", "london3"}},
		{"fr", []string{"london1", "phila2", "london3", "phila4", "phila5"}}, // unknown: every edition
	}
	for _, tt := range tests {
		t.Run("jur="+tt.jur, func(t *testing.T) {
			cols, nUS, nUK, outside := matrixColumns(editions, tt.jur)
			var got []string
			for _, c := range cols {
				got = append(got, c.BiblioID)
			}
			require.Equal(t, tt.want, got, "in the order given, which is by year")
			// The counts are of the work, not of the filter.
			require.Equal(t, 3, nUS)
			require.Equal(t, 2, nUK)
			require.Equal(t, 1, outside)
		})
	}

	require.Equal(t, "", normalizeMatrixJurisdiction(""))
	require.Equal(t, "us", normalizeMatrixJurisdiction("us"))
	require.Equal(t, "uk", normalizeMatrixJurisdiction("uk"))
	require.Equal(t, "", normalizeMatrixJurisdiction("US"))
	require.Equal(t, "", normalizeMatrixJurisdiction("both"))
}

func TestMatrixRows(t *testing.T) {
	eds := matrixTestEditions()
	name := "Hesketh v Lee"
	cases := []matrixCase{
		// Decided before every edition, cited by the first and third.
		{CaseRef: CaseRef{Source: "er", ID: "100", Name: &name, Year: ptr(1670)}, Editions: 2, Cites: 4, Span: ptr(7),
			First: 1, Last: 3, Positions: []int{1, 3}, Counts: []int{3, 1}},
		// Decided in 1828, between the first two editions; cited by the second.
		{CaseRef: CaseRef{Source: "cap", ID: "7", Year: ptr(1828)}, Editions: 1, Cites: 1,
			First: 2, Last: 2, Positions: []int{2}, Counts: []int{1}},
		// A stub with no year, cited by the first two. A position past the
		// last column is ignored rather than trusted.
		{CaseRef: CaseRef{Source: "stub", ID: "1 Wentw. 15"}, Editions: 2, Cites: 2,
			First: 1, Last: 2, Positions: []int{1, 2, 9}, Counts: []int{1, 1, 5}},
	}

	rows := matrixRows(cases, eds, "editions")
	require.Len(t, rows, 3)
	require.Equal(t, 7, *rows[0].Span)
	require.Nil(t, rows[1].Span)
	for _, r := range rows {
		require.Len(t, r.Cells, len(eds))
		require.Empty(t, r.Group, "the order by editions has no groups")
	}

	// The first case: filled, empty, filled; never before, since it is older
	// than every edition.
	require.Equal(t, 3, rows[0].Cells[0].Count)
	require.Equal(t, "3 citations", rows[0].Cells[0].Title())
	require.Equal(t, "/citations?case=er%3A100&edition=ed1", rows[0].Cells[0].URL)
	require.Equal(t, "uk", rows[0].Cells[0].Class)
	require.Zero(t, rows[0].Cells[1].Count)
	require.Empty(t, rows[0].Cells[1].URL)
	require.False(t, rows[0].Cells[1].Before)
	require.Equal(t, "us", rows[0].Cells[1].Class)
	require.Equal(t, 1, rows[0].Cells[2].Count)
	require.Equal(t, "1 citation", rows[0].Cells[2].Title())

	// The second: the 1824 edition is older than the 1828 case; the undated
	// edition is not known to be.
	require.True(t, rows[1].Cells[0].Before)
	require.Equal(t, 1, rows[1].Cells[1].Count)
	require.False(t, rows[1].Cells[1].Before)
	require.False(t, rows[1].Cells[2].Before)

	// The third: no year, so nothing is before it, and its key is escaped.
	require.False(t, rows[2].Cells[2].Before)
	require.Equal(t, "/citations?case=stub%3A1+Wentw.+15&edition=ed2", rows[2].Cells[1].URL)
	require.Zero(t, rows[2].Cells[2].Count)

	// A cited cell is never marked as before the case, whatever the years say.
	late := []matrixCase{{CaseRef: CaseRef{Source: "cap", ID: "8", Year: ptr(1900)}, Editions: 1, Cites: 1,
		First: 1, Last: 1, Positions: []int{1}, Counts: []int{1}}}
	cell := matrixRows(late, eds, "editions")[0].Cells[0]
	require.Equal(t, 1, cell.Count)
	require.False(t, cell.Before)
}

func TestMatrixGroups(t *testing.T) {
	eds := matrixTestEditions()
	at := func(first, last int) matrixCase {
		return matrixCase{CaseRef: CaseRef{Source: "er", ID: "1"}, First: first, Last: last}
	}
	groups := func(cases []matrixCase, sort string) []string {
		var out []string
		for _, r := range matrixRows(cases, eds, sort) {
			out = append(out, r.Group)
		}
		return out
	}

	// By first edition: a heading where the first edition changes.
	entered := []matrixCase{at(1, 3), at(1, 1), at(2, 2)}
	require.Equal(t, []string{"First cited in 1824, London: 3 cases", "", "First cited in 1831, Philadelphia, 2nd American ed.: 1 case"},
		groups(entered, "entered"))

	// By last edition.
	left := []matrixCase{at(1, 1), at(1, 2), at(2, 2), at(1, 3)}
	require.Equal(t, []string{"Last cited in 1824, London: 1 case", "Last cited in 1831, Philadelphia, 2nd American ed.: 2 cases", "", "Last cited in n.d.: 1 case"},
		groups(left, "left"))

	// The other orders have none, and a position outside the columns gives none.
	require.Equal(t, []string{"", "", ""}, groups(entered, "editions"))
	require.Equal(t, []string{"", "", ""}, groups(entered, "year"))
	require.Equal(t, []string{""}, groups([]matrixCase{at(7, 7)}, "entered"))
}

func TestPlural(t *testing.T) {
	require.Equal(t, "0 cases", plural(0, "case"))
	require.Equal(t, "1 case", plural(1, "case"))
	require.Equal(t, "1,480 cases", plural(1480, "case"))
}

func TestWorkCasesPage(t *testing.T) {
	d := WorkCasesPage{WorkID: 1107, Editions: matrixTestEditions(), US: 1, UK: 2, Cites: 300, Linked: 99}
	require.Equal(t, "/works/1107", d.WorkURL())
	require.Equal(t, 9, d.Columns())
	require.False(t, d.Paged())
	require.True(t, WorkCasesPage{Nav: Pagination{HasNext: true}}.Paged())
	require.Equal(t, "1824–1831", d.Years())
	require.Equal(t, "33%", d.LinkedPct())
	require.Equal(t, "—", WorkCasesPage{}.Years())
	require.Equal(t, "All 3 editions (2 UK, 1 US)", d.Shown())
	require.True(t, d.BothSides())
	require.False(t, WorkCasesPage{US: 4}.BothSides())

	us := WorkCasesPage{Jur: "us", Editions: matrixTestEditions()[1:2], US: 1, UK: 2}
	require.Equal(t, "The 1 US edition, of 3", us.Shown())

	// The links keep one setting while changing the other, and leave the
	// defaults out.
	require.Equal(t, "/works/1107/cases", d.Link("editions", ""))
	require.Equal(t, "/works/1107/cases?sort=entered", d.Link("entered", ""))
	require.Equal(t, "/works/1107/cases?sort=span", d.Link("span", ""))
	require.Equal(t, "/works/1107/cases?jur=uk", d.Link("editions", "uk"))
	require.Equal(t, "/works/1107/cases?jur=us&sort=left", d.Link("left", "us"))
	require.Equal(t, "/works/1107/cases", d.Link("nonsense", "fr"))
}
