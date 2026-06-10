package main

import (
	"bufio"
	"compress/bzip2"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/schollz/progressbar/v3"
)

// progressReader wraps an io.Reader and advances a progress bar by the number
// of bytes read. It is nil-safe: a nil bar simply reads through.
type progressReader struct {
	r   io.Reader
	bar *progressbar.ProgressBar
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if p.bar != nil && n > 0 {
		_ = p.bar.Add(n)
	}
	return n, err
}

// loadCSVToStaging streams a bzip2-compressed CSV into a freshly created TEMP
// table named stagingName on the given connection, one text column per CSV
// header column. The connection must be reused for any subsequent work because
// the TEMP table is connection-scoped. It returns the number of data rows
// copied (excluding the header).
//
// The file was produced by Postgres COPY, so reading it back with Postgres COPY
// parses it losslessly — including the large multiline, quoted text fields in
// the opinion-clusters export that trip up streaming CSV parsers.
func loadCSVToStaging(ctx context.Context, conn *pgxpool.Conn, path, stagingName string, showProgress bool) (int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("could not open %s: %w", path, err)
	}
	defer file.Close()

	var src io.Reader = file
	if showProgress {
		info, err := file.Stat()
		if err != nil {
			return 0, fmt.Errorf("could not stat %s: %w", path, err)
		}
		bar := progressbar.NewOptions64(info.Size(),
			progressbar.OptionSetDescription("copying "+stagingName),
			progressbar.OptionSetWriter(os.Stdout),
			progressbar.OptionShowBytes(true),
			progressbar.OptionSetPredictTime(true),
		)
		src = &progressReader{r: file, bar: bar}
	}

	// Decompress, then buffer so we can peel off the header line and hand the
	// remainder (still buffered) straight to COPY.
	reader := bufio.NewReader(bzip2.NewReader(src))

	header, err := reader.ReadString('\n')
	if err != nil {
		return 0, fmt.Errorf("could not read CSV header from %s: %w", path, err)
	}
	cols := strings.Split(strings.TrimRight(header, "\r\n"), ",")
	if len(cols) == 0 {
		return 0, fmt.Errorf("no columns found in CSV header of %s", path)
	}

	// Build "col1" text, "col2" text, … from the header names.
	defs := make([]string, len(cols))
	for i, c := range cols {
		name := strings.TrimSpace(c)
		defs[i] = fmt.Sprintf(`%s text`, quoteIdent(name))
	}

	if _, err := conn.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", stagingName)); err != nil {
		return 0, fmt.Errorf("could not drop existing staging table %s: %w", stagingName, err)
	}
	createSQL := fmt.Sprintf("CREATE TEMP TABLE %s (%s)", stagingName, strings.Join(defs, ", "))
	if _, err := conn.Exec(ctx, createSQL); err != nil {
		return 0, fmt.Errorf("could not create staging table %s: %w", stagingName, err)
	}

	copySQL := fmt.Sprintf("COPY %s FROM STDIN WITH (FORMAT csv)", stagingName)
	tag, err := conn.Conn().PgConn().CopyFrom(ctx, reader, copySQL)
	if err != nil {
		return 0, fmt.Errorf("could not COPY into staging table %s: %w", stagingName, err)
	}
	staged := tag.RowsAffected()
	slog.Info("staged rows from file", "table", stagingName, "rows", staged, "columns", len(cols))
	return staged, nil
}

// loadCitations replaces freelaw.citations from the CourtListener citations
// export. The source `type` column is an integer 1–9 that is mapped to the
// lowercased CourtListener constant name; rows whose type is outside 1–9 are
// skipped. The generated `cite` column is computed by Postgres.
func loadCitations(ctx context.Context, pool *pgxpool.Pool, path string, showProgress bool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("could not acquire connection: %w", err)
	}
	defer conn.Release()
	defer conn.Exec(context.Background(), "DROP TABLE IF EXISTS staging_citations")

	staged, err := loadCSVToStaging(ctx, conn, path, "staging_citations", showProgress)
	if err != nil {
		return err
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("could not begin transaction: %w", err)
	}
	defer tx.Rollback(context.Background())

	if _, err := tx.Exec(ctx, "TRUNCATE freelaw.citations"); err != nil {
		return fmt.Errorf("could not truncate freelaw.citations: %w", err)
	}

	insertSQL := `
	INSERT INTO freelaw.citations
		(id, volume, reporter, page, type, cluster_id, date_created, date_modified)
	SELECT id::bigint, volume, reporter, page,
		CASE type
			WHEN '1' THEN 'federal'        WHEN '2' THEN 'state'
			WHEN '3' THEN 'state_regional' WHEN '4' THEN 'specialty'
			WHEN '5' THEN 'scotus_early'   WHEN '6' THEN 'lexis'
			WHEN '7' THEN 'west'           WHEN '8' THEN 'neutral'
			WHEN '9' THEN 'journal'
		END,
		cluster_id::bigint,
		NULLIF(date_created, '')::timestamptz,
		NULLIF(date_modified, '')::timestamptz
	FROM staging_citations
	WHERE type IN ('1','2','3','4','5','6','7','8','9');`
	tag, err := tx.Exec(ctx, insertSQL)
	if err != nil {
		return fmt.Errorf("could not insert into freelaw.citations: %w", err)
	}
	inserted := tag.RowsAffected()

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("could not commit transaction: %w", err)
	}

	slog.Info("inserted citations", "inserted", inserted, "skipped_bad_type", staged-inserted)
	return nil
}

// loadClusters replaces freelaw.clusters_to_cap from the CourtListener
// opinion-clusters export. A row is inserted for every cluster that has a
// Harvard filepath; cap_case_id is the CAP case id extracted from that path
// when the case exists in cap.cases, and NULL otherwise. The number of clusters
// whose extracted CAP id is absent from cap.cases is logged.
func loadClusters(ctx context.Context, pool *pgxpool.Pool, path string, showProgress bool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("could not acquire connection: %w", err)
	}
	defer conn.Release()
	defer conn.Exec(context.Background(), "DROP TABLE IF EXISTS staging_clusters")

	staged, err := loadCSVToStaging(ctx, conn, path, "staging_clusters", showProgress)
	if err != nil {
		return err
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("could not begin transaction: %w", err)
	}
	defer tx.Rollback(context.Background())

	if _, err := tx.Exec(ctx, "TRUNCATE freelaw.clusters_to_cap"); err != nil {
		return fmt.Errorf("could not truncate freelaw.clusters_to_cap: %w", err)
	}

	// ext: every cluster with a Harvard filepath, with the CAP id parsed from
	// the path (strip ".json", take the last "."-delimited token).
	const extCTE = `
	WITH ext AS (
		SELECT id::bigint AS cluster_id,
			split_part(regexp_replace(filepath_json_harvard, '\.json$', ''), '.', -1)::bigint AS cap_id
		FROM staging_clusters
		WHERE filepath_json_harvard <> ''
	)`

	insertSQL := extCTE + `
	INSERT INTO freelaw.clusters_to_cap (cluster_id, cap_case_id)
	SELECT ext.cluster_id, c.id
	FROM ext
	LEFT JOIN cap.cases c ON c.id = ext.cap_id
	ON CONFLICT (cluster_id) DO NOTHING;`
	tag, err := tx.Exec(ctx, insertSQL)
	if err != nil {
		return fmt.Errorf("could not insert into freelaw.clusters_to_cap: %w", err)
	}
	inserted := tag.RowsAffected()

	// Count Harvard-linked clusters whose extracted CAP id is not in cap.cases
	// (those rows were stored with a NULL cap_case_id).
	missingSQL := extCTE + `
	SELECT count(*), min(ext.cluster_id), min(ext.cap_id)
	FROM ext
	LEFT JOIN cap.cases c ON c.id = ext.cap_id
	WHERE c.id IS NULL;`
	var missing int64
	var sampleCluster, sampleCAP sql.NullInt64
	if err := tx.QueryRow(ctx, missingSQL).Scan(&missing, &sampleCluster, &sampleCAP); err != nil {
		return fmt.Errorf("could not count missing CAP cases: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("could not commit transaction: %w", err)
	}

	slog.Info("inserted clusters", "total_clusters", staged, "harvard_linked", inserted)
	if missing > 0 {
		slog.Warn("clusters with a Harvard CAP id absent from cap.cases (stored NULL)",
			"count", missing,
			"sample_cluster_id", sampleCluster.Int64,
			"sample_cap_id", sampleCAP.Int64)
	}
	return nil
}

// quoteIdent double-quotes a SQL identifier, escaping embedded double quotes.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
