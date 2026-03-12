package citations

import (
	"context"
	"fmt"

	"github.com/google/uuid"
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
	SELECT reporter_found, reporter_standard, reporter_cap, statute, uk, junk, cap_different
	FROM legalhist.reporters_citation_to_cap
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
		var capDiff *bool
		err := rows.Scan(&found, &e.ReporterStandard, &e.ReporterCAP, &e.Statute, &e.UK, &e.Junk, &capDiff)
		if err != nil {
			return nil, fmt.Errorf("scanning reporter whitelist row: %w", err)
		}
		if capDiff != nil {
			e.CAPDifferent = *capDiff
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

func (s *LinkerDBStore) CountUnprocessedCitations(ctx context.Context) (int64, error) {
	query := `
	SELECT count(*)
	FROM moml_citations.citations_unlinked cu
	WHERE NOT EXISTS (
		SELECT 1 FROM moml_citations.citation_links cl WHERE cl.citation_id = cu.id
	)
	`
	var count int64
	err := s.DB.QueryRow(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting unprocessed citations: %w", err)
	}
	return count, nil
}

func (s *LinkerDBStore) GetUnprocessedCitations(ctx context.Context, afterID uuid.UUID, limit int) ([]UnlinkedCitation, error) {
	query := `
	SELECT cu.id, cu.moml_treatise, cu.moml_page, cu.raw, cu.volume, cu.reporter_abbr, cu.page
	FROM moml_citations.citations_unlinked cu
	WHERE NOT EXISTS (
		SELECT 1 FROM moml_citations.citation_links cl WHERE cl.citation_id = cu.id
	)
	AND cu.id > $1
	ORDER BY cu.id
	LIMIT $2
	`
	rows, err := s.DB.Query(ctx, query, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("fetching unprocessed citations: %w", err)
	}
	defer rows.Close()

	var citations []UnlinkedCitation
	for rows.Next() {
		var c UnlinkedCitation
		err := rows.Scan(&c.ID, &c.MomlTreatise, &c.MomlPage, &c.Raw, &c.Volume, &c.ReporterAbbr, &c.Page)
		if err != nil {
			return nil, fmt.Errorf("scanning unlinked citation: %w", err)
		}
		citations = append(citations, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating unlinked citations: %w", err)
	}
	return citations, nil
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

// SaveLinkResults batch-inserts multiple link results in a single query.
func (s *LinkerDBStore) SaveLinkResults(ctx context.Context, results []*LinkResult) error {
	if len(results) == 0 {
		return nil
	}

	// Build a multi-row INSERT: VALUES ($1,$2,...,$8), ($9,$10,...), ...
	query := `
	INSERT INTO moml_citations.citation_links
		(citation_id, status, cap_case_id, code_reporter_id, er_case_id, cite_cleaned, cite_normalized, cite_linked)
	VALUES `

	args := make([]interface{}, 0, len(results)*8)
	for i, r := range results {
		if i > 0 {
			query += ", "
		}
		base := i * 8
		query += fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8)
		args = append(args, r.CitationID, r.Status, r.CAPCaseID, r.CodeReporterID, r.ERCaseID, r.CiteCleaned, r.CiteNormalized, r.CiteLinked)
	}
	query += " ON CONFLICT (citation_id) DO NOTHING"

	_, err := s.DB.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("batch saving %d link results: %w", len(results), err)
	}
	return nil
}

func (s *LinkerDBStore) BatchSkipNonWhitelisted(ctx context.Context) (int64, error) {
	query := `
	INSERT INTO moml_citations.citation_links (citation_id, status, cite_cleaned, cite_normalized)
	SELECT cu.id,
	       CASE
	         WHEN wl.statute = true THEN 'skipped_statute'
	         WHEN wl.junk = true THEN 'skipped_junk'
	         ELSE 'skipped_not_whitelisted'
	       END,
	       cu.reporter_abbr,
	       cu.reporter_abbr
	FROM moml_citations.citations_unlinked cu
	LEFT JOIN legalhist.reporters_citation_to_cap wl ON cu.reporter_abbr = wl.reporter_found
	WHERE NOT EXISTS (
		SELECT 1 FROM moml_citations.citation_links cl WHERE cl.citation_id = cu.id
	)
	AND (wl.reporter_found IS NULL OR wl.statute = true OR wl.junk = true)
	ON CONFLICT (citation_id) DO NOTHING
	`
	tag, err := s.DB.Exec(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("batch skipping non-whitelisted citations: %w", err)
	}
	return tag.RowsAffected(), nil
}
