package citations

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestOpinionCorpusStore builds the slice of opinion_citations and cap the
// opinion corpus store touches: both ledger tables as migration
// 20260926130100_opinion-citations.sql creates them (minus the foreign keys,
// which the stream and save never exercise) and cap.cases with the one column
// the stream joins for. The DDL tracks the migration by hand, as newTestStore's
// does for moml_citations. Skipped unless LAW_TEST_DBSTR points at a throwaway
// database. The cap schema is dropped and rebuilt, as the other tests that need
// a slice of it do, so the order the tests run in does not matter.
func newTestOpinionCorpusStore(t *testing.T) *CorpusStore {
	t.Helper()
	dsn := os.Getenv("LAW_TEST_DBSTR")
	if dsn == "" {
		t.Skip("LAW_TEST_DBSTR not set; skipping DB integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.Connect(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	setup := []string{
		`DROP SCHEMA IF EXISTS opinion_citations CASCADE`,
		`CREATE SCHEMA opinion_citations`,
		`CREATE TABLE opinion_citations.citations_unlinked (
			id uuid PRIMARY KEY,
			cap_case bigint NOT NULL,
			cap_opinion bigint NOT NULL,
			raw text NOT NULL,
			volume integer,
			reporter_abbr text NOT NULL,
			page integer NOT NULL,
			year integer,
			created_at timestamp without time zone NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE opinion_citations.citation_links (
			citation_id uuid PRIMARY KEY,
			status text NOT NULL,
			match_tier text,
			cap_case_id bigint,
			code_reporter_id bigint,
			er_case_id text,
			stub_cite text,
			cite_cleaned text,
			cite_normalized text,
			cite_linked text,
			created_at timestamp with time zone DEFAULT now() NOT NULL
		)`,
		`DROP SCHEMA IF EXISTS cap CASCADE`,
		`CREATE SCHEMA cap`,
		`CREATE TABLE cap.cases (id bigint PRIMARY KEY, decision_year integer)`,
		`INSERT INTO cap.cases VALUES (3, 1899), (4, NULL)`,
	}
	for _, stmt := range setup {
		_, err := pool.Exec(ctx, stmt)
		require.NoError(t, err, "setup: %s", stmt)
	}
	return NewOpinionCorpusStore(pool)
}

func seedOpinionUnlinked(t *testing.T, s *CorpusStore, id uuid.UUID, caseID, opinionID int64) {
	t.Helper()
	_, err := s.DB.Exec(context.Background(),
		`INSERT INTO opinion_citations.citations_unlinked (id, cap_case, cap_opinion, raw, volume, reporter_abbr, page)
		 VALUES ($1, $2, $3, 'raw cite', 5, 'U.S.', 10)`, id, caseID, opinionID)
	require.NoError(t, err)
}

// TestStreamUnprocessedOpinionCitationsIntegration: the stream delivers only
// the citations not yet in opinion_citations.citation_links, in batches, each
// dated by its citing case's decision_year, or undated when the case has none
// or is missing.
func TestStreamUnprocessedOpinionCitationsIntegration(t *testing.T) {
	s := newTestOpinionCorpusStore(t)
	ctx := context.Background()

	dated, undated, orphan, done := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	seedOpinionUnlinked(t, s, dated, 3, 30)   // case 3 was decided in 1899
	seedOpinionUnlinked(t, s, undated, 4, 40) // case 4 has no decision year
	seedOpinionUnlinked(t, s, orphan, 5, 50)  // case 5 has no row at all
	seedOpinionUnlinked(t, s, done, 3, 31)    // already linked
	_, err := s.DB.Exec(ctx, `INSERT INTO opinion_citations.citation_links (citation_id, status) VALUES ($1, 'no_match')`, done)
	require.NoError(t, err)

	got := map[uuid.UUID]UnlinkedCitation{}
	var batchSizes []int
	err = s.StreamUnprocessedCitations(ctx, 2, func(batch []UnlinkedCitation) error {
		batchSizes = append(batchSizes, len(batch))
		for _, c := range batch {
			got[c.ID] = c
		}
		return nil
	})
	require.NoError(t, err)

	require.Len(t, got, 3)
	assert.NotContains(t, got, done, "a linked citation is not streamed again")
	assert.Equal(t, []int{2, 1}, batchSizes)

	if assert.NotNil(t, got[dated].SourceYear) {
		assert.Equal(t, 1899, *got[dated].SourceYear)
	}
	assert.Nil(t, got[undated].SourceYear, "a case without a year dates nothing")
	assert.Nil(t, got[orphan].SourceYear, "a citation whose case is missing dates nothing")
	assert.Equal(t, "U.S.", got[dated].ReporterAbbr)
	if assert.NotNil(t, got[dated].Volume) {
		assert.Equal(t, 5, *got[dated].Volume)
	}
	assert.Equal(t, 10, got[dated].Page)
}

// TestSaveOpinionLinkResultsIntegration: the ten columns land in
// opinion_citations.citation_links, and a citation that already has a row is
// left alone, so a resubmitted run is idempotent.
func TestSaveOpinionLinkResultsIntegration(t *testing.T) {
	s := newTestOpinionCorpusStore(t)
	ctx := context.Background()

	id := uuid.New()
	seedOpinionUnlinked(t, s, id, 3, 30)
	caseID := int64(111)
	cleaned, normalized, linked := "5 U.S. 10", "5 U.S. 10", "5 U.S. 10"
	results := []*LinkResult{{
		CitationID:     id,
		Status:         StatusLinkedCAP,
		MatchTier:      TierCAPDirect,
		CAPCaseID:      &caseID,
		CiteCleaned:    &cleaned,
		CiteNormalized: &normalized,
		CiteLinked:     &linked,
	}}
	require.NoError(t, s.SaveLinkResults(ctx, results))

	var status, tier string
	var gotCase *int64
	var gotCode *int64
	var gotER, gotStub, gotLinked *string
	require.NoError(t, s.DB.QueryRow(ctx,
		`SELECT status, match_tier, cap_case_id, code_reporter_id, er_case_id, stub_cite, cite_linked
		 FROM opinion_citations.citation_links WHERE citation_id = $1`, id).
		Scan(&status, &tier, &gotCase, &gotCode, &gotER, &gotStub, &gotLinked))
	assert.Equal(t, StatusLinkedCAP, status)
	assert.Equal(t, TierCAPDirect, tier)
	if assert.NotNil(t, gotCase) {
		assert.Equal(t, int64(111), *gotCase)
	}
	assert.Nil(t, gotCode)
	assert.Nil(t, gotER)
	assert.Nil(t, gotStub)
	if assert.NotNil(t, gotLinked) {
		assert.Equal(t, "5 U.S. 10", *gotLinked)
	}

	// Saving the same citation again, with a different verdict, changes nothing.
	results[0].Status = StatusNoMatch
	require.NoError(t, s.SaveLinkResults(ctx, results))
	var n int
	require.NoError(t, s.DB.QueryRow(ctx, `SELECT count(*) FROM opinion_citations.citation_links`).Scan(&n))
	assert.Equal(t, 1, n)
	require.NoError(t, s.DB.QueryRow(ctx, `SELECT status FROM opinion_citations.citation_links WHERE citation_id = $1`, id).Scan(&status))
	assert.Equal(t, StatusLinkedCAP, status)

	// After the save, the stream has nothing left to deliver.
	delivered := 0
	require.NoError(t, s.StreamUnprocessedCitations(ctx, 10, func(batch []UnlinkedCitation) error {
		delivered += len(batch)
		return nil
	}))
	assert.Equal(t, 0, delivered)
}
