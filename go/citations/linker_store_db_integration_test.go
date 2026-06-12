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

// These tests exercise the real SQL in LinkerDBStore against a live Postgres.
// They are skipped unless LAW_TEST_DBSTR points at a throwaway database the test
// is free to create/drop tables in (NOT a production DSN). CI has no database, so
// these are a no-op there; run them locally against a disposable container, e.g.:
//
//	docker run -d --name lm-linker-test -e POSTGRES_PASSWORD=test \
//	    -e POSTGRES_DB=lawtest -p 55432:5432 postgres:17
//	LAW_TEST_DBSTR='postgres://postgres:test@localhost:55432/lawtest' go test ./go/citations/ -run Integration -v
func newTestStore(t *testing.T) *LinkerDBStore {
	t.Helper()
	dsn := os.Getenv("LAW_TEST_DBSTR")
	if dsn == "" {
		t.Skip("LAW_TEST_DBSTR not set; skipping DB integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.Connect(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	// Build the minimal slice of the moml_citations schema the linker touches.
	// Drop first so each run starts clean.
	setup := []string{
		`DROP SCHEMA IF EXISTS moml_citations CASCADE`,
		`CREATE SCHEMA moml_citations`,
		`CREATE TABLE moml_citations.citations_unlinked (
			id uuid PRIMARY KEY,
			moml_treatise text NOT NULL,
			moml_page text NOT NULL,
			raw text NOT NULL,
			volume integer,
			reporter_abbr text NOT NULL,
			page integer NOT NULL,
			created_at timestamp without time zone NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE moml_citations.citation_links (
			citation_id uuid PRIMARY KEY,
			status text NOT NULL,
			cap_case_id bigint,
			code_reporter_id bigint,
			er_case_id text,
			cite_cleaned text,
			cite_normalized text,
			cite_linked text,
			created_at timestamp with time zone DEFAULT now() NOT NULL
		)`,
	}
	for _, stmt := range setup {
		_, err := pool.Exec(ctx, stmt)
		require.NoError(t, err, "setup: %s", stmt)
	}
	return &LinkerDBStore{DB: pool}
}

func seedUnlinked(t *testing.T, s *LinkerDBStore, id uuid.UUID, vol *int) {
	t.Helper()
	_, err := s.DB.Exec(context.Background(),
		`INSERT INTO moml_citations.citations_unlinked (id, moml_treatise, moml_page, raw, volume, reporter_abbr, page)
		 VALUES ($1, 'treatise', 'p1', 'raw cite', $2, 'U.S.', 10)`, id, vol)
	require.NoError(t, err)
}

func TestStreamUnprocessedCitationsIntegration(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// 10 unlinked citations; mark 3 of them already processed in citation_links.
	all := make([]uuid.UUID, 10)
	for i := range all {
		all[i] = uuid.New()
		v := i
		seedUnlinked(t, s, all[i], &v)
	}
	processed := map[uuid.UUID]bool{all[0]: true, all[4]: true, all[9]: true}
	for id := range processed {
		_, err := s.DB.Exec(ctx,
			`INSERT INTO moml_citations.citation_links (citation_id, status) VALUES ($1, 'no_match')`, id)
		require.NoError(t, err)
	}

	// Stream with a small batch size and collect everything delivered.
	var got []uuid.UUID
	var batchSizes []int
	err := s.StreamUnprocessedCitations(ctx, 3, func(batch []UnlinkedCitation) error {
		batchSizes = append(batchSizes, len(batch))
		for _, c := range batch {
			got = append(got, c.ID)
		}
		return nil
	})
	require.NoError(t, err)

	// Should deliver exactly the 7 unprocessed citations, none of the processed.
	want := make([]uuid.UUID, 0, 7)
	for _, id := range all {
		if !processed[id] {
			want = append(want, id)
		}
	}
	assert.Len(t, got, 7)
	for _, id := range got {
		assert.False(t, processed[id], "streamed an already-processed citation %s", id)
	}
	assert.ElementsMatch(t, want, got)

	// Batching: 7 rows at batch size 3 => batches of 3, 3, 1.
	assert.Equal(t, []int{3, 3, 1}, batchSizes)
}

func TestSaveLinkResultsIntegration(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	capID := int64(111)
	codeID := int64(999)
	erID := "er-7"
	cleaned := "5 U.S. 10"
	normalized := "5 U.S. 10"
	linked := "5 U.S. 10"

	idCAP := uuid.New()
	idCode := uuid.New()
	idER := uuid.New()
	idNoMatch := uuid.New()
	idSkipped := uuid.New()

	results := []*LinkResult{
		// A CAP link: cap_case_id set, the rest of the case IDs nil.
		{CitationID: idCAP, Status: StatusLinkedCAP, CAPCaseID: &capID,
			CiteCleaned: &cleaned, CiteNormalized: &normalized, CiteLinked: &linked},
		// A code-reporter link.
		{CitationID: idCode, Status: StatusLinkedCodeReporter, CodeReporterID: &codeID,
			CiteCleaned: &cleaned, CiteNormalized: &normalized, CiteLinked: &linked},
		// An English Reports link (text id).
		{CitationID: idER, Status: StatusLinkedEnglishReports, ERCaseID: &erID,
			CiteCleaned: &cleaned, CiteNormalized: &normalized, CiteLinked: &linked},
		// no_match: cite_cleaned/normalized set, everything else nil.
		{CitationID: idNoMatch, Status: StatusNoMatch,
			CiteCleaned: &cleaned, CiteNormalized: &normalized},
		// skipped: all nullable columns nil. Exercises NULL encoding in every array.
		{CitationID: idSkipped, Status: StatusSkippedNotWhitelisted},
	}

	require.NoError(t, s.SaveLinkResults(ctx, results))

	// Read each row back and verify the values (and the NULLs) round-tripped.
	type row struct {
		status                          string
		capCaseID, codeReporterID       *int64
		erCaseID, cleaned, norm, linked *string
	}
	read := func(id uuid.UUID) row {
		var r row
		err := s.DB.QueryRow(ctx,
			`SELECT status, cap_case_id, code_reporter_id, er_case_id, cite_cleaned, cite_normalized, cite_linked
			 FROM moml_citations.citation_links WHERE citation_id = $1`, id).
			Scan(&r.status, &r.capCaseID, &r.codeReporterID, &r.erCaseID, &r.cleaned, &r.norm, &r.linked)
		require.NoError(t, err)
		return r
	}

	cap := read(idCAP)
	assert.Equal(t, StatusLinkedCAP, cap.status)
	require.NotNil(t, cap.capCaseID)
	assert.Equal(t, capID, *cap.capCaseID)
	assert.Nil(t, cap.codeReporterID)
	assert.Nil(t, cap.erCaseID)
	require.NotNil(t, cap.linked)
	assert.Equal(t, linked, *cap.linked)

	code := read(idCode)
	assert.Equal(t, StatusLinkedCodeReporter, code.status)
	require.NotNil(t, code.codeReporterID)
	assert.Equal(t, codeID, *code.codeReporterID)
	assert.Nil(t, code.capCaseID)

	er := read(idER)
	assert.Equal(t, StatusLinkedEnglishReports, er.status)
	require.NotNil(t, er.erCaseID)
	assert.Equal(t, erID, *er.erCaseID)
	assert.Nil(t, er.capCaseID)

	nm := read(idNoMatch)
	assert.Equal(t, StatusNoMatch, nm.status)
	assert.Nil(t, nm.capCaseID)
	require.NotNil(t, nm.cleaned)
	assert.Equal(t, cleaned, *nm.cleaned)
	assert.Nil(t, nm.linked)

	sk := read(idSkipped)
	assert.Equal(t, StatusSkippedNotWhitelisted, sk.status)
	assert.Nil(t, sk.capCaseID)
	assert.Nil(t, sk.codeReporterID)
	assert.Nil(t, sk.erCaseID)
	assert.Nil(t, sk.cleaned)
	assert.Nil(t, sk.norm)
	assert.Nil(t, sk.linked)

	// ON CONFLICT DO NOTHING: re-saving the same citation_id with a different
	// status must not overwrite the existing row.
	conflicting := []*LinkResult{{CitationID: idCAP, Status: StatusNoMatch}}
	require.NoError(t, s.SaveLinkResults(ctx, conflicting))
	assert.Equal(t, StatusLinkedCAP, read(idCAP).status, "ON CONFLICT should have preserved the original row")

	// Empty input is a no-op, not an error.
	require.NoError(t, s.SaveLinkResults(ctx, nil))

	// Sanity: exactly the five rows we inserted exist.
	var n int
	require.NoError(t, s.DB.QueryRow(ctx, `SELECT count(*) FROM moml_citations.citation_links`).Scan(&n))
	assert.Equal(t, 5, n)
}
