package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFieldCodeData checks the embedded data of the Field Code network holds
// together: every borrowing joins two codes that exist, no code borrows from
// a later one, the categories and distances agree, and the seven drafts of
// the Field Code are the codes at distance 0.
func TestFieldCodeData(t *testing.T) {
	type code struct {
		Code            string `json:"code"`
		CorpusCode      string `json:"corpus_code"`
		State           string `json:"state"`
		Jurisdiction    string `json:"jurisdiction"`
		Year            int    `json:"year"`
		Sections        int    `json:"sections"`
		SectionsMatched int    `json:"sections_matched"`
		Distance        *int   `json:"distance"`
		Category        string `json:"category"`
	}
	type borrowing struct {
		Borrower         string  `json:"borrower"`
		BorrowedFrom     string  `json:"borrowed_from"`
		SectionsBorrowed int     `json:"sections_borrowed"`
		ShareOfSections  float64 `json:"share_of_sections"`
		MeanSimilarity   float64 `json:"mean_similarity"`
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
	}
	require.ElementsMatch(t, []string{"NY1848", "NY1849", "NY1850", "NY1851", "NY1853", "NY1864", "NY1873"}, fieldCodes)

	inNetwork := map[string]bool{}
	for _, e := range borrowings {
		borrower, ok := byCode[e.Borrower]
		require.True(t, ok, "borrower %s is not a code", e.Borrower)
		from, ok := byCode[e.BorrowedFrom]
		require.True(t, ok, "source %s is not a code", e.BorrowedFrom)
		require.NotEqual(t, e.Borrower, e.BorrowedFrom)
		require.GreaterOrEqual(t, borrower.Year, from.Year, "%s borrows from the later %s", e.Borrower, e.BorrowedFrom)
		require.Positive(t, e.SectionsBorrowed, "%s from %s", e.Borrower, e.BorrowedFrom)
		require.LessOrEqual(t, e.SectionsBorrowed, borrower.Sections)
		require.InDelta(t, float64(e.SectionsBorrowed)/float64(borrower.Sections), e.ShareOfSections, 0.0001)
		require.Greater(t, e.MeanSimilarity, 0.0)
		require.LessOrEqual(t, e.MeanSimilarity, 1.0)
		inNetwork[e.Borrower] = true
		inNetwork[e.BorrowedFrom] = true
	}
	for _, c := range codes {
		require.True(t, inNetwork[c.Code], "%s has no borrowing", c.Code)
	}
}
