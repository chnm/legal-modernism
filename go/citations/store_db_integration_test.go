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

// newTestDBStore builds the slice of moml_citations that cite-detector-moml
// writes to, including citations_unlinked_uq -- the unique index that is the
// whole point of these tests, since SaveCitations relies on it and on its own
// in-batch deduplication to keep the table free of duplicate citations.
//
// Skipped unless LAW_TEST_DBSTR points at a throwaway database, exactly as the
// linker integration tests are. See newTestStore for the docker one-liner.
func newTestDBStore(t *testing.T) *DBStore {
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
			created_at timestamp without time zone NOT NULL,
			year integer
		)`,
		`CREATE UNIQUE INDEX citations_unlinked_uq
			ON moml_citations.citations_unlinked
			USING btree (moml_treatise, moml_page, COALESCE(volume, '-1'::integer), reporter_abbr, page, COALESCE(year, '-1'::integer))`,
	}
	for _, stmt := range setup {
		_, err := pool.Exec(ctx, stmt)
		require.NoError(t, err, stmt)
	}
	return NewDBStore(pool)
}

func testCitation(treatise, page string, vol *int, abbr string, pageNum int) *Citation {
	return &Citation{
		ID:           uuid.New(),
		Source:       sources.NewTreatisePage(page, treatise, ""),
		Raw:          abbr,
		Volume:       vol,
		ReporterAbbr: abbr,
		Page:         pageNum,
	}
}

func countCitations(t *testing.T, s *DBStore) int {
	t.Helper()
	var n int
	require.NoError(t, s.DB.QueryRow(context.Background(),
		`SELECT count(*) FROM moml_citations.citations_unlinked`).Scan(&n))
	return n
}

func TestSaveCitationsIntegration(t *testing.T) {
	s := newTestDBStore(t)
	ctx := context.Background()

	vol := 123
	cites := []*Citation{
		testCitation("t1", "p1", &vol, "Cal.", 185),
		testCitation("t1", "p1", nil, "Hob.", 423),
		testCitation("t1", "p1", nil, "Toth", 462),
	}
	require.NoError(t, s.SaveCitations(ctx, cites))
	assert.Equal(t, 3, countCitations(t, s))

	// A volume-less row and a volumed row for the same reporter and page are
	// different citations: COALESCE(volume, -1) keeps them apart.
	require.NoError(t, s.SaveCitations(ctx, []*Citation{
		testCitation("t1", "p1", &vol, "Hob.", 423),
	}))
	assert.Equal(t, 4, countCitations(t, s))
}

// TestSaveCitationsIntegration_DedupesWithinBatch covers the case the detectors
// really produce: two abbreviations that are prefixes of one another match the
// same span, so RemoveShadows keeps both (they are the same citation, not a
// shadow) and the batch carries duplicates on the unique index's key.
func TestSaveCitationsIntegration_DedupesWithinBatch(t *testing.T) {
	s := newTestDBStore(t)
	ctx := context.Background()

	// Distinct ids, identical unique-index keys.
	batch := []*Citation{
		testCitation("t1", "p1", nil, "Baldwin", 125),
		testCitation("t1", "p1", nil, "Baldwin", 125),
		testCitation("t1", "p1", nil, "Baldwin", 125),
	}
	require.NoError(t, s.SaveCitations(ctx, batch))
	assert.Equal(t, 1, countCitations(t, s), "the unique index key must appear once")
}

// TestSaveCitationsIntegration_IsIdempotent covers re-running the detector over
// a page it has already scanned, which is what makes an interrupted run safe to
// resubmit. The ids differ every time because Detect mints a fresh UUID, so the
// collision is on the unique index rather than on the primary key.
func TestSaveCitationsIntegration_IsIdempotent(t *testing.T) {
	s := newTestDBStore(t)
	ctx := context.Background()

	vol := 30
	first := []*Citation{
		testCitation("t1", "p1", &vol, "Missis.", 673),
		testCitation("t1", "p1", nil, "Hob.", 423),
	}
	require.NoError(t, s.SaveCitations(ctx, first))
	require.Equal(t, 2, countCitations(t, s))

	second := []*Citation{
		testCitation("t1", "p1", &vol, "Missis.", 673),
		testCitation("t1", "p1", nil, "Hob.", 423),
		testCitation("t1", "p1", nil, "Toth", 462), // new on the second pass
	}
	require.NoError(t, s.SaveCitations(ctx, second))
	assert.Equal(t, 3, countCitations(t, s))
}

func TestSaveCitationsIntegration_Empty(t *testing.T) {
	s := newTestDBStore(t)
	require.NoError(t, s.SaveCitations(context.Background(), nil))
	assert.Equal(t, 0, countCitations(t, s))
}

// TestSaveCitationIntegration checks that the single-citation entry point still
// works, since it is part of the Store interface.
func TestSaveCitationIntegration(t *testing.T) {
	s := newTestDBStore(t)
	ctx := context.Background()
	vol := 43
	require.NoError(t, s.SaveCitation(ctx, testCitation("t1", "p1", &vol, "Md.", 295)))
	assert.Equal(t, 1, countCitations(t, s))
}

// TestSaveCitationsIntegration_Year covers the year column: citations that
// differ only in their year are different rows, both inside a batch and against
// the table, and a citation without one stores NULL.
func TestSaveCitationsIntegration_Year(t *testing.T) {
	s := newTestDBStore(t)
	ctx := context.Background()

	vol := 2
	y1905, y1906 := 1905, 1906
	a := testCitation("t1", "p1", &vol, "K. B.", 1)
	a.Year = &y1905
	b := testCitation("t1", "p1", &vol, "K. B.", 1)
	b.Year = &y1906
	c := testCitation("t1", "p1", &vol, "K. B.", 1) // the year-less reading
	dup := testCitation("t1", "p1", &vol, "K. B.", 1)
	dup.Year = &y1905

	require.NoError(t, s.SaveCitations(ctx, []*Citation{a, b, c, dup}))
	assert.Equal(t, 3, countCitations(t, s))

	require.NoError(t, s.SaveCitations(ctx, []*Citation{dup}))
	assert.Equal(t, 3, countCitations(t, s), "a re-run must not duplicate a year-bearing row")

	var years []*int
	rows, err := s.DB.Query(ctx, `SELECT year FROM moml_citations.citations_unlinked ORDER BY year NULLS FIRST`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var y *int
		require.NoError(t, rows.Scan(&y))
		years = append(years, y)
	}
	require.NoError(t, rows.Err())
	require.Len(t, years, 3)
	assert.Nil(t, years[0])
	assert.Equal(t, 1905, *years[1])
	assert.Equal(t, 1906, *years[2])
}

func TestGetSingleVolReporterAbbrsIntegration(t *testing.T) {
	s := newTestDBStore(t)
	ctx := context.Background()

	// Build the legalhist slice the loader reads, as the linker's alt-abbr test
	// does: reporters with single_vol, and the abbreviations table with its FK
	// and distinct CHECK. Kept inside this test so the save tests don't depend
	// on a legalhist schema.
	setup := []string{
		`DROP SCHEMA IF EXISTS legalhist CASCADE`,
		`CREATE SCHEMA legalhist`,
		`CREATE TABLE legalhist.reporters (reporter_standard text PRIMARY KEY, single_vol boolean)`,
		`CREATE TABLE legalhist.reporters_abbreviations (
			reporter_standard text NOT NULL REFERENCES legalhist.reporters(reporter_standard),
			alt_abbr text NOT NULL,
			CONSTRAINT reporters_abbreviations_distinct_check CHECK (reporter_standard <> alt_abbr)
		)`,
		`INSERT INTO legalhist.reporters (reporter_standard, single_vol) VALUES
			('Calth', true),     -- single-volume; its alternate "Cal." is another reporter
			('Cal.', NULL),      -- California Reports: owns the spelling, not single-volume
			('Phil. Eq.', true), -- lists "Phil." as an alternate
			('Phil.', true),     -- owns the spelling and is single-volume itself
			('Hob.', true),      -- an ordinary single-volume reporter with a real alternate
			('U.S.', false)      -- not single-volume: contributes nothing`,
		`INSERT INTO legalhist.reporters_abbreviations (reporter_standard, alt_abbr) VALUES
			('Calth', 'Cal.'),
			('Calth', 'Calthrop'),
			('Phil. Eq.', 'Phil.'),
			('Hob.', 'Hobart'),
			('U.S.', 'US')`,
	}
	for _, stmt := range setup {
		_, err := s.DB.Exec(ctx, stmt)
		require.NoError(t, err, "setup: %s", stmt)
	}

	got, err := s.GetSingleVolReporterAbbrs(ctx)
	require.NoError(t, err)

	// A standard spelling trumps an alternative one (issue #289): "Cal." and
	// "Phil." are not built under Calth or Phil. Eq., but "Phil." survives under
	// its own reporter, and the non-colliding alternates are untouched. The
	// query has no ORDER BY, so order is not asserted.
	assert.ElementsMatch(t, []SingleVolReporter{
		{Standard: "Calth", Abbr: "Calth"},
		{Standard: "Calth", Abbr: "Calthrop"},
		{Standard: "Phil. Eq.", Abbr: "Phil. Eq."},
		{Standard: "Phil.", Abbr: "Phil."},
		{Standard: "Hob.", Abbr: "Hob."},
		{Standard: "Hob.", Abbr: "Hobart"},
	}, got)
}
