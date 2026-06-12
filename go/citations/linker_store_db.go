package citations

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v4/pgxpool"
)

// LinkerDBStore implements LinkerStore using PostgreSQL via pgx.
type LinkerDBStore struct {
	DB *pgxpool.Pool
}

// NewLinkerDBStore returns a new LinkerDBStore.
func NewLinkerDBStore(db *pgxpool.Pool) *LinkerDBStore {
	return &LinkerDBStore{DB: db}
}

func (s *LinkerDBStore) GetReporterWhitelist(ctx context.Context) (map[string]*WhitelistEntry, error) {
	query := `
	SELECT
		w.reporter_found,
		w.reporter_standard,
		r.reporter_cap,
		w.junk,
		COALESCE(r.jurisdiction LIKE 'uk%', false) AS uk,
		EXISTS (
			SELECT 1 FROM legalhist.reporters_diffvols d
			WHERE d.reporter_standard = w.reporter_standard
		) AS cap_different
	FROM legalhist.whitelist w
	LEFT JOIN legalhist.reporters r ON r.reporter_standard = w.reporter_standard
	`
	rows, err := s.DB.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying reporter whitelist: %w", err)
	}
	defer rows.Close()

	whitelist := make(map[string]*WhitelistEntry)
	for rows.Next() {
		var found string
		var e WhitelistEntry
		err := rows.Scan(&found, &e.ReporterStandard, &e.ReporterCAP, &e.Junk, &e.UK, &e.CAPDifferent)
		if err != nil {
			return nil, fmt.Errorf("scanning reporter whitelist row: %w", err)
		}
		whitelist[found] = &e
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating reporter whitelist: %w", err)
	}
	return whitelist, nil
}

func (s *LinkerDBStore) GetDiffVols(ctx context.Context) (map[string]map[int]*DiffVolEntry, error) {
	query := `
	SELECT reporter_standard, vol, cap_vol, cap_reporter
	FROM legalhist.reporters_diffvols
	WHERE reporter_standard IS NOT NULL
	  AND vol IS NOT NULL
	  AND cap_vol IS NOT NULL
	`
	rows, err := s.DB.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying diffvols: %w", err)
	}
	defer rows.Close()

	diffvols := make(map[string]map[int]*DiffVolEntry)
	for rows.Next() {
		var reporterStd string
		var vol, capVol int
		var capReporter string
		err := rows.Scan(&reporterStd, &vol, &capVol, &capReporter)
		if err != nil {
			return nil, fmt.Errorf("scanning diffvols row: %w", err)
		}
		if diffvols[reporterStd] == nil {
			diffvols[reporterStd] = make(map[int]*DiffVolEntry)
		}
		diffvols[reporterStd][vol] = &DiffVolEntry{
			CAPVol:      capVol,
			CAPReporter: capReporter,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating diffvols: %w", err)
	}
	return diffvols, nil
}

// StreamUnprocessedCitations runs a single anti-join over the whole
// citations_unlinked table, streaming every citation not yet in citation_links
// to fn in batches of at most batchSize.
//
// This replaces the old cursor-paginated fetch. That approach ran one
// LIMIT-bounded anti-join per batch; because the inner citation_links index
// scan had no lower bound, each of the ~12,500 batches re-scanned an
// ever-growing prefix of the 62M-row citation_links index to fast-forward to
// the cursor. The total work was quadratic in the table size and dominated the
// 13-hour runtime. One streaming pass scans each index once instead.
//
// The query holds a single connection (and a consistent snapshot) open for the
// duration of the stream, so the set delivered is exactly the citations that
// were unprocessed when the query began — concurrent inserts by the worker
// connections are invisible to it. Callers MUST apply backpressure inside fn;
// the whole table is read as fast as fn accepts batches.
func (s *LinkerDBStore) StreamUnprocessedCitations(ctx context.Context, batchSize int, fn func([]UnlinkedCitation) error) error {
	query := `
	SELECT cu.id, cu.moml_treatise, cu.moml_page, cu.raw, cu.volume, cu.reporter_abbr, cu.page
	FROM moml_citations.citations_unlinked cu
	WHERE NOT EXISTS (
		SELECT 1 FROM moml_citations.citation_links cl WHERE cl.citation_id = cu.id
	)
	`
	rows, err := s.DB.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("streaming unprocessed citations: %w", err)
	}
	defer rows.Close()

	batch := make([]UnlinkedCitation, 0, batchSize)
	for rows.Next() {
		var c UnlinkedCitation
		if err := rows.Scan(&c.ID, &c.MomlTreatise, &c.MomlPage, &c.Raw, &c.Volume, &c.ReporterAbbr, &c.Page); err != nil {
			return fmt.Errorf("scanning unlinked citation: %w", err)
		}
		batch = append(batch, c)
		if len(batch) >= batchSize {
			if err := fn(batch); err != nil {
				return err
			}
			batch = make([]UnlinkedCitation, 0, batchSize)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating unlinked citations: %w", err)
	}
	if len(batch) > 0 {
		if err := fn(batch); err != nil {
			return err
		}
	}
	return nil
}

// LoadCAPCitations loads cap.citations into an in-memory map of cite -> case ID.
func (s *LinkerDBStore) LoadCAPCitations(ctx context.Context) (map[string]int64, error) {
	query := `SELECT DISTINCT ON (cite) cite, "case" FROM cap.citations`
	rows, err := s.DB.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("loading CAP citations: %w", err)
	}
	defer rows.Close()

	m := make(map[string]int64)
	for rows.Next() {
		var cite string
		var caseID int64
		if err := rows.Scan(&cite, &caseID); err != nil {
			return nil, fmt.Errorf("scanning CAP citation: %w", err)
		}
		m[cite] = caseID
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating CAP citations: %w", err)
	}
	return m, nil
}

// LoadFreelawCites loads the FreeLaw parallel-citation crosswalk into an
// in-memory map of cite -> cap_case_id. The matview is keyed on the same
// "{volume} {reporter} {page}" cite string the linker builds, so the linker can
// probe it exactly like the cap.citations map. The matview already enforces one
// cap_case_id per cite, so no DISTINCT is needed here.
func (s *LinkerDBStore) LoadFreelawCites(ctx context.Context) (map[string]int64, error) {
	query := `SELECT cite, cap_case_id FROM freelaw.cite_to_cap`
	rows, err := s.DB.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("loading FreeLaw cites: %w", err)
	}
	defer rows.Close()

	m := make(map[string]int64)
	for rows.Next() {
		var cite string
		var caseID int64
		if err := rows.Scan(&cite, &caseID); err != nil {
			return nil, fmt.Errorf("scanning FreeLaw cite: %w", err)
		}
		m[cite] = caseID
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating FreeLaw cites: %w", err)
	}
	return m, nil
}

// LoadCodeReporterCitations loads code_reporter into an in-memory map of
// official_citation -> id.
func (s *LinkerDBStore) LoadCodeReporterCitations(ctx context.Context) (map[string]int64, error) {
	query := `SELECT official_citation, id FROM legalhist.code_reporter`
	rows, err := s.DB.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("loading code reporter citations: %w", err)
	}
	defer rows.Close()

	m := make(map[string]int64)
	for rows.Next() {
		var cite string
		var id int64
		if err := rows.Scan(&cite, &id); err != nil {
			return nil, fmt.Errorf("scanning code reporter citation: %w", err)
		}
		m[cite] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating code reporter citations: %w", err)
	}
	return m, nil
}

// LoadEnglishReportsCitations loads english_reports.cases into an in-memory map.
// Both er_cite and er_parallel_cite are mapped to the case ID.
func (s *LinkerDBStore) LoadEnglishReportsCitations(ctx context.Context) (map[string]string, error) {
	query := `SELECT id, er_cite, er_parallel_cite FROM english_reports.cases`
	rows, err := s.DB.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("loading English Reports citations: %w", err)
	}
	defer rows.Close()

	m := make(map[string]string)
	for rows.Next() {
		var id, erCite string
		var erParallel *string
		if err := rows.Scan(&id, &erCite, &erParallel); err != nil {
			return nil, fmt.Errorf("scanning English Reports citation: %w", err)
		}
		m[erCite] = id
		if erParallel != nil {
			m[*erParallel] = id
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating English Reports citations: %w", err)
	}
	return m, nil
}

// SaveLinkResults batch-inserts multiple link results in a single statement.
//
// Rather than build a VALUES list with up to batchSize*8 placeholders (which
// runs into Postgres's 65535-parameter limit at large batch sizes and forces
// the server to parse a huge statement on every batch), it passes one array per
// column and expands them server-side with unnest(). That is a fixed 8-parameter
// statement regardless of batch size, so it parses/plans cheaply and keeps the
// wire payload compact. citation_id is sent as text[] and cast to uuid in SQL to
// avoid relying on driver-side uuid-array encoding.
func (s *LinkerDBStore) SaveLinkResults(ctx context.Context, results []*LinkResult) error {
	if len(results) == 0 {
		return nil
	}

	ids := make([]string, len(results))
	statuses := make([]string, len(results))
	capIDs := make([]*int64, len(results))
	codeIDs := make([]*int64, len(results))
	erIDs := make([]*string, len(results))
	cleaned := make([]*string, len(results))
	normalized := make([]*string, len(results))
	linked := make([]*string, len(results))
	for i, r := range results {
		ids[i] = r.CitationID.String()
		statuses[i] = r.Status
		capIDs[i] = r.CAPCaseID
		codeIDs[i] = r.CodeReporterID
		erIDs[i] = r.ERCaseID
		cleaned[i] = r.CiteCleaned
		normalized[i] = r.CiteNormalized
		linked[i] = r.CiteLinked
	}

	query := `
	INSERT INTO moml_citations.citation_links
		(citation_id, status, cap_case_id, code_reporter_id, er_case_id, cite_cleaned, cite_normalized, cite_linked)
	SELECT u.citation_id::uuid, u.status, u.cap_case_id, u.code_reporter_id, u.er_case_id, u.cite_cleaned, u.cite_normalized, u.cite_linked
	FROM unnest($1::text[], $2::text[], $3::bigint[], $4::bigint[], $5::text[], $6::text[], $7::text[], $8::text[])
		AS u(citation_id, status, cap_case_id, code_reporter_id, er_case_id, cite_cleaned, cite_normalized, cite_linked)
	ON CONFLICT (citation_id) DO NOTHING`

	_, err := s.DB.Exec(ctx, query, ids, statuses, capIDs, codeIDs, erIDs, cleaned, normalized, linked)
	if err != nil {
		return fmt.Errorf("batch saving %d link results: %w", len(results), err)
	}
	return nil
}

func (s *LinkerDBStore) BatchSkipNonWhitelisted(ctx context.Context) (int64, error) {
	query := `
	INSERT INTO moml_citations.citation_links (citation_id, status)
	SELECT cu.id,
	       CASE
	         WHEN wl.junk = true THEN 'skipped_junk'
	         ELSE 'skipped_not_whitelisted'
	       END
	FROM moml_citations.citations_unlinked cu
	LEFT JOIN legalhist.whitelist wl ON cu.reporter_abbr = wl.reporter_found
	WHERE NOT EXISTS (
		SELECT 1 FROM moml_citations.citation_links cl WHERE cl.citation_id = cu.id
	)
	AND (wl.reporter_found IS NULL OR wl.junk = true)
	ON CONFLICT (citation_id) DO NOTHING
	`
	tag, err := s.DB.Exec(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("batch skipping non-whitelisted citations: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ResetUnlinked deletes every citation_links row that was not resolved to a case
// (status no_match, skipped_not_whitelisted, or skipped_junk) so the linker
// re-processes them on the next run; only linked_* rows are preserved. Deleting
// both skip statuses lets a re-run with --skip-unlisted re-derive them from the
// current whitelist, so a reporter later corrected from junk to legit is no
// longer stuck as skipped_junk. The delete runs as a single statement — one
// all-or-nothing transaction — and returns the number of rows deleted.
func (s *LinkerDBStore) ResetUnlinked(ctx context.Context) (int64, error) {
	query := `
	DELETE FROM moml_citations.citation_links
	WHERE status IN ($1, $2, $3)
	`
	tag, err := s.DB.Exec(ctx, query, StatusNoMatch, StatusSkippedNotWhitelisted, StatusSkippedJunk)
	if err != nil {
		return 0, fmt.Errorf("resetting unlinked citations: %w", err)
	}
	return tag.RowsAffected(), nil
}
