package sources

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestPgxStore builds the slice of cap.cases and cap.opinions that
// StreamCAPOpinions reads, in a throwaway database. Skipped unless
// LAW_TEST_DBSTR is set, exactly as the go/citations integration tests are;
// see newTestStore there for the docker one-liner.
func newTestPgxStore(t *testing.T) *PgxStore {
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
		`DROP SCHEMA IF EXISTS cap CASCADE`,
		`CREATE SCHEMA cap`,
		`CREATE TABLE cap.cases (id bigint PRIMARY KEY, decision_year integer)`,
		`CREATE TABLE cap.opinions (id bigint PRIMARY KEY, "case" bigint NOT NULL REFERENCES cap.cases (id), type text NOT NULL, text text NOT NULL)`,
		`INSERT INTO cap.cases VALUES (1, 1919), (2, 1920), (3, 1921)`,
		`INSERT INTO cap.opinions VALUES
			(10, 1, 'majority', 'text of 10'),
			(11, 1, 'dissent', 'text of 11'),
			(20, 2, 'majority', 'text of 20'),
			(30, 3, 'majority', 'text of 30')`,
	}
	for _, stmt := range setup {
		_, err := pool.Exec(ctx, stmt)
		require.NoError(t, err, "setup: %s", stmt)
	}
	return NewPgxStore(pool)
}

// TestStreamCAPOpinionsIntegration pins the cutoff: --max-year is inclusive,
// every opinion of a qualifying case is delivered whatever its type, and the
// count agrees with the stream.
func TestStreamCAPOpinionsIntegration(t *testing.T) {
	s := newTestPgxStore(t)
	ctx := context.Background()

	var got []*CAPOpinion
	err := s.StreamCAPOpinions(ctx, 1920, func(o *CAPOpinion) error {
		got = append(got, o)
		return nil
	})
	require.NoError(t, err)

	ids := map[int64]*CAPOpinion{}
	for _, o := range got {
		ids[o.OpinionID] = o
	}
	assert.Len(t, ids, 3, "1919 and 1920 qualify, 1921 does not")
	assert.NotContains(t, ids, int64(30))
	if assert.Contains(t, ids, int64(11)) {
		assert.Equal(t, int64(1), ids[11].CaseID)
		assert.Equal(t, "dissent", ids[11].Type)
		assert.Equal(t, "text of 11", ids[11].Text())
	}

	n, err := s.CountCAPOpinions(ctx, 1920)
	require.NoError(t, err)
	assert.Equal(t, int64(3), n)

	n, err = s.CountCAPOpinions(ctx, 1919)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)
}
