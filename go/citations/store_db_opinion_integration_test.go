package citations

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/lmullen/legal-modernism/go/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestOpinionDBStore builds the slice of opinion_citations that
// cite-detector-cap writes to: the table and its unique index, as migration
// 20260926130100_opinion-citations.sql creates them, minus the foreign keys
// onto cap.cases and cap.opinions, which the save never exercises. The DDL
// tracks the migration by hand, as newTestDBStore's does for moml_citations.
// Skipped unless LAW_TEST_DBSTR points at a throwaway database.
func newTestOpinionDBStore(t *testing.T) *DBStore {
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
			created_at timestamp without time zone NOT NULL
		)`,
		`CREATE UNIQUE INDEX citations_unlinked_uq
			ON opinion_citations.citations_unlinked
			USING btree (cap_opinion, COALESCE(volume, '-1'::integer), reporter_abbr, page, COALESCE(year, '-1'::integer))`,
	}
	for _, stmt := range setup {
		_, err := pool.Exec(ctx, stmt)
		require.NoError(t, err, stmt)
	}
	return NewOpinionDBStore(pool)
}

func testOpinionCitation(opinionID, caseID int64, vol *int, abbr string, pageNum int) *Citation {
	return &Citation{
		ID:           uuid.New(),
		Source:       sources.NewCAPOpinion(opinionID, caseID, "majority", ""),
		Raw:          abbr,
		Volume:       vol,
		ReporterAbbr: abbr,
		Page:         pageNum,
	}
}

func countOpinionCitations(t *testing.T, s *DBStore) int {
	t.Helper()
	var n int
	require.NoError(t, s.DB.QueryRow(context.Background(),
		`SELECT count(*) FROM opinion_citations.citations_unlinked`).Scan(&n))
	return n
}

// TestSaveOpinionCitationsIntegration covers the opinion store the way the
// MOML tests cover NewDBStore: the ids round-trip as bigint, a NULL volume is
// distinct from a volume, a repeated citation is one row, and a re-run adds
// nothing.
func TestSaveOpinionCitationsIntegration(t *testing.T) {
	s := newTestOpinionDBStore(t)
	ctx := context.Background()

	one := 1
	cites := []*Citation{
		testOpinionCitation(7, 3, &one, "U.S.", 10),
		testOpinionCitation(7, 3, nil, "U.S.", 10),  // NULL volume: a different citation
		testOpinionCitation(7, 3, &one, "U.S.", 10), // the same as the first: collapsed in the batch
		testOpinionCitation(8, 3, &one, "U.S.", 10), // another opinion of the same case
	}
	require.NoError(t, s.SaveCitations(ctx, cites))
	assert.Equal(t, 3, countOpinionCitations(t, s))

	// The citing case and opinion were written as the bigint ids the document
	// carries, not as the strings the interface hands over.
	var caseID, opinionID int64
	require.NoError(t, s.DB.QueryRow(ctx,
		`SELECT cap_case, cap_opinion FROM opinion_citations.citations_unlinked WHERE id = $1`, cites[3].ID).Scan(&caseID, &opinionID))
	assert.Equal(t, int64(3), caseID)
	assert.Equal(t, int64(8), opinionID)

	// Idempotent: the same page saved again, with fresh uuids, adds nothing.
	again := []*Citation{
		testOpinionCitation(7, 3, &one, "U.S.", 10),
		testOpinionCitation(8, 3, &one, "U.S.", 10),
	}
	require.NoError(t, s.SaveCitations(ctx, again))
	assert.Equal(t, 3, countOpinionCitations(t, s))

	// An empty batch is a no-op.
	require.NoError(t, s.SaveCitations(ctx, nil))
	assert.Equal(t, 3, countOpinionCitations(t, s))
}

func TestSaveOpinionCitationsIntegration_Year(t *testing.T) {
	s := newTestOpinionDBStore(t)
	ctx := context.Background()

	two, y1905, y1906 := 2, 1905, 1906
	cites := []*Citation{
		testOpinionCitation(7, 3, &two, "K. B.", 1),
		testOpinionCitation(7, 3, &two, "K. B.", 1),
		testOpinionCitation(7, 3, &two, "K. B.", 1),
	}
	cites[1].Year = &y1905
	cites[2].Year = &y1906
	require.NoError(t, s.SaveCitations(ctx, cites))
	assert.Equal(t, 3, countOpinionCitations(t, s), "the year is part of the key")

	var year *int
	require.NoError(t, s.DB.QueryRow(ctx,
		`SELECT year FROM opinion_citations.citations_unlinked WHERE id = $1`, cites[1].ID).Scan(&year))
	if assert.NotNil(t, year) {
		assert.Equal(t, 1905, *year)
	}
}

// TestSaveOpinionCitationsIntegration_RefusesTreatisePage pins that the
// opinion store cannot be handed a page: its ids are not numbers, so the
// insert fails at the cast instead of writing a row under a wrong key.
func TestSaveOpinionCitationsIntegration_RefusesTreatisePage(t *testing.T) {
	s := newTestOpinionDBStore(t)
	ctx := context.Background()

	one := 1
	page := testCitation("19003000100", "p12", &one, "U.S.", 10)
	err := s.SaveCitations(ctx, []*Citation{page})
	require.Error(t, err)
	assert.Equal(t, 0, countOpinionCitations(t, s))
}
