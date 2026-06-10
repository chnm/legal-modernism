package main

import (
	"bufio"
	"compress/bzip2"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/schollz/progressbar/v3"
)

var (
	// reClusterRecordStart matches the first physical line of an opinion-clusters
	// record: the integer id (column 0) immediately followed by the date_created
	// timestamp (column 1). Continuation lines produced by embedded newlines in
	// text fields never match this, so it recovers true record boundaries even
	// though the export's CSV quoting is unreliable.
	reClusterRecordStart = regexp.MustCompile(`^"?(\d+)"?,"?\d{4}-\d{2}-\d{2} \d{2}:\d{2}:`)
	// reCapJSONPath captures the CAP case id from a Harvard filepath_json_harvard
	// value, e.g. law.free.cap.mass.151/547.3507958.json -> 3507958.
	reCapJSONPath = regexp.MustCompile(`law\.free\.cap\.[^",\s]*\.(\d+)\.json`)
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
// This is used only for the citations export, whose columns are short, single
// line, and free of embedded quotes, so Postgres COPY parses it cleanly. The
// opinion-clusters export is NOT loaded this way: its large free-text fields
// have unreliable quoting that makes COPY mis-split rows ("extra data after last
// expected column"), so loadClusters scans that file directly instead.
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

// clusterCAPPair links a CourtListener opinion-cluster id to the CAP case id
// parsed from its Harvard filepath.
type clusterCAPPair struct {
	clusterID int64
	capID     int64
}

// extractClusterCAPPairs streams the bzip2-compressed opinion-clusters export
// and returns one (cluster_id, cap_id) pair per record that carries a Harvard
// filepath. It deliberately does not parse the CSV column structure: the
// export's quoting is unreliable (a stray quote in a free-text field prematurely
// ends quoting, so a standard CSV parser — including Postgres COPY — mis-splits
// the row). Instead it recovers records by their start-of-line "<id>,<timestamp>"
// shape and reads the CAP id from the unmistakable law.free.cap.….json field,
// independent of column position.
func extractClusterCAPPairs(path string, showProgress bool) ([]clusterCAPPair, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not open %s: %w", path, err)
	}
	defer file.Close()

	var src io.Reader = file
	if showProgress {
		info, err := file.Stat()
		if err != nil {
			return nil, fmt.Errorf("could not stat %s: %w", path, err)
		}
		bar := progressbar.NewOptions64(info.Size(),
			progressbar.OptionSetDescription("scanning opinion-clusters"),
			progressbar.OptionSetWriter(os.Stdout),
			progressbar.OptionShowBytes(true),
			progressbar.OptionSetPredictTime(true),
		)
		src = &progressReader{r: file, bar: bar}
	}
	reader := bufio.NewReader(bzip2.NewReader(src))

	var pairs []clusterCAPPair
	var pending clusterCAPPair
	var haveCluster, haveCap bool

	// emit finalizes the record being scanned, recording its pair when it had
	// both a cluster id and a Harvard CAP path.
	emit := func() {
		if haveCluster && haveCap {
			pairs = append(pairs, pending)
		}
		haveCluster, haveCap = false, false
	}

	for {
		line, rerr := reader.ReadString('\n')
		if len(line) > 0 {
			// A record begins only on a line starting with a digit; check that
			// cheaply before running the anchored timestamp regex.
			if line[0] >= '0' && line[0] <= '9' {
				if m := reClusterRecordStart.FindStringSubmatch(line); m != nil {
					emit() // close out the previous record
					if id, perr := strconv.ParseInt(m[1], 10, 64); perr == nil {
						pending = clusterCAPPair{clusterID: id}
						haveCluster = true
					}
				}
			}
			if haveCluster && !haveCap && strings.Contains(line, "law.free.cap") {
				if m := reCapJSONPath.FindStringSubmatch(line); m != nil {
					if capID, perr := strconv.ParseInt(m[1], 10, 64); perr == nil {
						pending.capID = capID
						haveCap = true
					}
				}
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return nil, fmt.Errorf("reading %s: %w", path, rerr)
		}
	}
	emit() // final record

	return pairs, nil
}

// loadClusters replaces freelaw.clusters_to_cap from the CourtListener
// opinion-clusters export. Every cluster with a Harvard filepath gets a row;
// cap_case_id is the parsed CAP case id when that case exists in cap.cases, and
// NULL otherwise. The number of Harvard-linked clusters whose CAP id is absent
// from cap.cases is logged.
func loadClusters(ctx context.Context, pool *pgxpool.Pool, path string, showProgress bool) error {
	pairs, err := extractClusterCAPPairs(path, showProgress)
	if err != nil {
		return err
	}
	slog.Info("extracted Harvard-linked clusters from opinion-clusters export", "count", len(pairs))

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("could not acquire connection: %w", err)
	}
	defer conn.Release()
	defer conn.Exec(context.Background(), "DROP TABLE IF EXISTS staging_clusters")

	if _, err := conn.Exec(ctx, "DROP TABLE IF EXISTS staging_clusters"); err != nil {
		return fmt.Errorf("could not drop existing staging table: %w", err)
	}
	if _, err := conn.Exec(ctx, "CREATE TEMP TABLE staging_clusters (cluster_id bigint, cap_id bigint)"); err != nil {
		return fmt.Errorf("could not create staging table: %w", err)
	}

	if _, err := conn.Conn().CopyFrom(ctx,
		pgx.Identifier{"staging_clusters"},
		[]string{"cluster_id", "cap_id"},
		pgx.CopyFromSlice(len(pairs), func(i int) ([]interface{}, error) {
			return []interface{}{pairs[i].clusterID, pairs[i].capID}, nil
		}),
	); err != nil {
		return fmt.Errorf("could not COPY into staging_clusters: %w", err)
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("could not begin transaction: %w", err)
	}
	defer tx.Rollback(context.Background())

	if _, err := tx.Exec(ctx, "TRUNCATE freelaw.clusters_to_cap"); err != nil {
		return fmt.Errorf("could not truncate freelaw.clusters_to_cap: %w", err)
	}

	insertSQL := `
	INSERT INTO freelaw.clusters_to_cap (cluster_id, cap_case_id)
	SELECT s.cluster_id, c.id
	FROM staging_clusters s
	LEFT JOIN cap.cases c ON c.id = s.cap_id
	ON CONFLICT (cluster_id) DO NOTHING;`
	tag, err := tx.Exec(ctx, insertSQL)
	if err != nil {
		return fmt.Errorf("could not insert into freelaw.clusters_to_cap: %w", err)
	}
	inserted := tag.RowsAffected()

	// Harvard-linked clusters whose CAP id is not in cap.cases were stored with
	// a NULL cap_case_id.
	var missing int64
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM staging_clusters s
		LEFT JOIN cap.cases c ON c.id = s.cap_id
		WHERE c.id IS NULL`).Scan(&missing); err != nil {
		return fmt.Errorf("could not count missing CAP cases: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("could not commit transaction: %w", err)
	}

	slog.Info("inserted clusters", "harvard_linked", inserted)
	if missing > 0 {
		slog.Warn("clusters with a Harvard CAP id absent from cap.cases (stored NULL)", "count", missing)
	}
	return nil
}

// quoteIdent double-quotes a SQL identifier, escaping embedded double quotes.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
