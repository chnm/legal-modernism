package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMatrixLLMColumns(t *testing.T) {
	eds := matrixTestEditions()
	eds[0].HasCounts, eds[0].Cites, eds[0].Linked = true, 994, 243

	// A reading replaces the pipeline's counts on its own column only.
	applyLLMReadings(eds, map[string]llmReading{
		"ed2":   {Pages: 573, Cites: 2013, Linked: 1912},
		"other": {Pages: 1, Cites: 1, Linked: 1},
	})
	require.False(t, eds[0].LLM)
	require.Equal(t, "pipeline", eds[0].Method())
	require.Equal(t, 994, eds[0].Cites)
	require.True(t, eds[1].LLM)
	require.Equal(t, "LLM", eds[1].Method())
	require.Equal(t, 573, eds[1].LLMPages)
	require.Equal(t, 2013, eds[1].Cites)
	require.Equal(t, 1912, eds[1].Linked)
	require.True(t, eds[1].HasCounts)
	require.Equal(t, []string{"ed2"}, llmIDs(eds))
	require.Empty(t, llmIDs(matrixTestEditions()))
	require.NotNil(t, llmIDs(matrixTestEditions()), "an empty array, not NULL, for the query")

	// An LLM cell goes to the first page that cites the case; a pipeline
	// cell to the citations list.
	cases := []matrixCase{
		{CaseRef: CaseRef{Source: "er", ID: "100", Year: ptr(1670)}, Editions: 2, Cites: 3, First: 1, Last: 2,
			Positions: []int{1, 2}, Counts: []int{1, 2}, Pages: []*string{nil, ptr("19004567890/1230")}},
		// A reading's cell with no page falls back to the edition.
		{CaseRef: CaseRef{Source: "cap", ID: "7"}, Editions: 1, Cites: 1, First: 2, Last: 2,
			Positions: []int{2}, Counts: []int{1}, Pages: []*string{nil}},
	}
	rows := matrixRows(cases, eds, "editions")
	require.False(t, rows[0].Cells[0].LLM)
	require.Equal(t, "/citations?case=er%3A100&edition=ed1", rows[0].Cells[0].URL)
	require.True(t, rows[0].Cells[1].LLM)
	require.Equal(t, pageURL("19004567890", "1230"), rows[0].Cells[1].URL)
	require.Equal(t, "2 citations in the LLM reading: the first page", rows[0].Cells[1].Title())
	require.Equal(t, eds[1].URL(), rows[1].Cells[1].URL)
	require.Equal(t, "1 citation in the LLM reading: its page", rows[1].Cells[1].Title())

	// The page says which method the columns come from.
	d := WorkCasesPage{Editions: eds}
	require.Equal(t, 1, d.LLMEditions())
	require.Equal(t, "The LLM reading for 1 edition, the pipeline for 2", d.Methods())
	require.Equal(t, "The LLM reading for the 1 edition", WorkCasesPage{Editions: eds[1:2]}.Methods())
	require.Equal(t, "The pipeline (the detector and linker) for all 2 editions",
		WorkCasesPage{Editions: []MatrixEdition{eds[0], eds[2]}}.Methods())

	var buf bytes.Buffer
	require.NoError(t, parseTemplates()["work_cases.html"].ExecuteTemplate(&buf, "baseof", WorkCasesPage{
		Page: Page{Title: "x", Section: "works"}, WorkID: 1107, Title: "A treatise on pleading",
		Editions: eds, US: 1, UK: 2, Rows: rows, Total: 2, Sort: "editions",
	}))
	out := buf.String()
	require.Contains(t, out, `<span class="src llm" title="Claude's page-by-page reading: 573 pages read">LLM</span>`)
	require.Contains(t, out, `<span class="src pipe"`)
	require.Contains(t, out, `<span class="sw llm">LLM</span>`)
}

func TestMatrixSourceSQL(t *testing.T) {
	// With no LLM edition the reading's tables are not in the query.
	none := matrixSourceSQL(5, nil)
	require.Contains(t, none, "edition_case_citations")
	require.NotContains(t, none, "llm_links")
	some := matrixSourceSQL(3, []string{"ed2"})
	require.Contains(t, some, "llm_links")
	require.Contains(t, some, "ANY($3::text[])")
	require.Contains(t, some, "l.status <> 'no_match'")
}
