package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFieldCodeData checks the embedded data of the Field Code network holds
// together: every borrowing joins two codes that exist, no code borrows from
// a later one, the categories and distances agree, the seven drafts of the
// Field Code are the codes at distance 0, every code has a drawn borrowing
// and its place in the figure, and the borrowings the figure does not draw
// each touch one of its codes.
func TestFieldCodeData(t *testing.T) {
	type code struct {
		Code            string   `json:"code"`
		CorpusCode      string   `json:"corpus_code"`
		State           string   `json:"state"`
		Jurisdiction    string   `json:"jurisdiction"`
		Slug            string   `json:"jurisdiction_slug"`
		Year            int      `json:"year"`
		Sections        int      `json:"sections"`
		SectionsMatched int      `json:"sections_matched"`
		Distance        *int     `json:"distance"`
		Category        string   `json:"category"`
		FigureX         *float64 `json:"figure_x"`
		FigureY         *float64 `json:"figure_y"`
		FigureLabelX    *float64 `json:"figure_label_x"`
		FigureLabelY    *float64 `json:"figure_label_y"`
		LayoutX         *float64 `json:"layout_x"`
		LayoutY         *float64 `json:"layout_y"`
		LayoutLabelX    *float64 `json:"layout_label_x"`
		LayoutLabelY    *float64 `json:"layout_label_y"`
	}
	type borrowing struct {
		Borrower         string  `json:"borrower"`
		BorrowedFrom     string  `json:"borrowed_from"`
		SectionsBorrowed int     `json:"sections_borrowed"`
		ShareOfSections  float64 `json:"share_of_sections"`
		MeanSimilarity   float64 `json:"mean_similarity"`
		InFigure         bool    `json:"in_figure"`
	}

	var codes []code
	b, err := dataFS.ReadFile("data/field-code-codes.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, &codes))
	var borrowings []borrowing
	b, err = dataFS.ReadFile("data/field-code-borrowings.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, &borrowings))

	require.NotEmpty(t, codes)
	require.NotEmpty(t, borrowings)

	byCode := map[string]code{}
	fieldCodes := []string{}
	for i, c := range codes {
		require.NotContains(t, byCode, c.Code, "duplicate code")
		byCode[c.Code] = c
		require.Equal(t, c.Code[:2], c.State, c.Code)
		require.NotEmpty(t, c.Jurisdiction, c.Code)
		require.NotEmpty(t, c.Slug, "%s has no cap.jurisdictions slug", c.Code)
		require.Positive(t, c.Sections, c.Code)
		require.LessOrEqual(t, c.SectionsMatched, c.Sections, c.Code)
		if i > 0 {
			require.LessOrEqual(t, codes[i-1].Year, c.Year, "codes are sorted by year")
		}
		switch c.Category {
		case "Field code":
			require.NotNil(t, c.Distance, c.Code)
			require.Equal(t, 0, *c.Distance, c.Code)
			fieldCodes = append(fieldCodes, c.Code)
		case "Original borrower":
			require.NotNil(t, c.Distance, c.Code)
			require.Equal(t, 1, *c.Distance, c.Code)
		case "Subsequent borrower":
			require.NotNil(t, c.Distance, c.Code)
			require.GreaterOrEqual(t, *c.Distance, 2, c.Code)
		case "Did not borrow":
			require.Nil(t, c.Distance, c.Code)
		default:
			t.Fatalf("%s: unknown category %q", c.Code, c.Category)
		}
		require.NotNil(t, c.FigureX, "%s has no position in the figure", c.Code)
		require.NotNil(t, c.FigureY, c.Code)
		require.GreaterOrEqual(t, *c.FigureX, 0.0, c.Code)
		require.LessOrEqual(t, *c.FigureX, 1.0, c.Code)
		require.GreaterOrEqual(t, *c.FigureY, 0.0, c.Code)
		require.LessOrEqual(t, *c.FigureY, 1.0, c.Code)
		require.NotNil(t, c.FigureLabelX, c.Code)
		require.NotNil(t, c.FigureLabelY, c.Code)
		require.InDelta(t, *c.FigureX, *c.FigureLabelX, 0.06, "%s: its label is near its node", c.Code)
		require.InDelta(t, *c.FigureY, *c.FigureLabelY, 0.06, c.Code)
		// The layout is the figure's position, adjusted: by a few pixels
		// for most codes, by up to a seventh of the panel for a few moved
		// out of a crowd.
		require.NotNil(t, c.LayoutX, c.Code)
		require.NotNil(t, c.LayoutLabelX, c.Code)
		require.InDelta(t, *c.FigureX, *c.LayoutX, 0.15, "%s: the layout stays near the figure", c.Code)
		require.InDelta(t, *c.FigureY, *c.LayoutY, 0.15, c.Code)
		require.InDelta(t, *c.LayoutX, *c.LayoutLabelX, 0.06, "%s: its label stays near its node", c.Code)
		require.InDelta(t, *c.LayoutY, *c.LayoutLabelY, 0.06, c.Code)
	}
	require.ElementsMatch(t, []string{"NY1848", "NY1849", "NY1850", "NY1851", "NY1853", "NY1864", "NY1873"}, fieldCodes)

	inNetwork := map[string]bool{}
	drawn := 0
	for _, e := range borrowings {
		borrower, borrowerKnown := byCode[e.Borrower]
		from, fromKnown := byCode[e.BorrowedFrom]
		require.True(t, borrowerKnown || fromKnown, "%s from %s touches no code of the figure", e.Borrower, e.BorrowedFrom)
		require.NotEqual(t, e.Borrower, e.BorrowedFrom)
		require.Positive(t, e.SectionsBorrowed, "%s from %s", e.Borrower, e.BorrowedFrom)
		require.Greater(t, e.MeanSimilarity, 0.0)
		require.LessOrEqual(t, e.MeanSimilarity, 1.0)
		if borrowerKnown && fromKnown {
			require.GreaterOrEqual(t, borrower.Year, from.Year, "%s borrows from the later %s", e.Borrower, e.BorrowedFrom)
		}
		if borrowerKnown {
			require.LessOrEqual(t, e.SectionsBorrowed, borrower.Sections)
			require.InDelta(t, float64(e.SectionsBorrowed)/float64(borrower.Sections), e.ShareOfSections, 0.0001)
		}
		if e.InFigure {
			require.True(t, borrowerKnown && fromKnown, "%s from %s is drawn but a code is missing", e.Borrower, e.BorrowedFrom)
			inNetwork[e.Borrower] = true
			inNetwork[e.BorrowedFrom] = true
			drawn++
		}
	}
	for _, c := range codes {
		require.True(t, inNetwork[c.Code], "%s has no drawn borrowing", c.Code)
	}
	require.Greater(t, len(borrowings), drawn, "the data lists borrowings beyond the figure's")
}
