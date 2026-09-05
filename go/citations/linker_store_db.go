package citations

import (
	"context"
	"fmt"
	"strings"

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
		) AS cap_different,
		COALESCE(r.single_vol, false) AS single_vol,
		COALESCE(r.type = 'statute', false) AS statute
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
		err := rows.Scan(&found, &e.ReporterStandard, &e.ReporterCAP, &e.Junk, &e.UK, &e.CAPDifferent, &e.SingleVol, &e.Statute)
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
// A cite is included only if it resolves unambiguously to a single case, the
// same policy freelaw.cite_to_cap enforces with its HAVING clause: ~10% of
// distinct cite strings belong to more than one case (memorandum/table pages
// listing several decisions), and linking those to an arbitrary case would
// inject systematic errors, so they are dropped. min("case") is a formality —
// the HAVING guarantees a single distinct value.
func (s *LinkerDBStore) LoadCAPCitations(ctx context.Context) (map[string]int64, error) {
	query := `
	SELECT cite, min("case") AS case_id
	FROM cap.citations
	GROUP BY cite
	HAVING count(DISTINCT "case") = 1
	`
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

// LoadReporterAltAbbrs loads legalhist.reporters_abbreviations into an in-memory
// map of reporter_standard -> []alt_abbr. The linker probes the CAP, FreeLaw,
// and code-reporter maps with each alternate spelling (keyed by the canonical
// reporter_standard, like the diffvols mapping) after the standard/reporter_cap
// forms miss. The ORDER BY makes the probe order — and therefore which alt wins
// when more than one would hit — deterministic across runs; COLLATE "C" keeps
// that order byte-identical across environments regardless of locale.
func (s *LinkerDBStore) LoadReporterAltAbbrs(ctx context.Context) (map[string][]string, error) {
	query := `
	SELECT reporter_standard, alt_abbr
	FROM legalhist.reporters_abbreviations
	WHERE alt_abbr IS NOT NULL
	ORDER BY reporter_standard COLLATE "C", alt_abbr COLLATE "C"
	`
	rows, err := s.DB.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("loading reporter alt abbreviations: %w", err)
	}
	defer rows.Close()

	m := make(map[string][]string)
	for rows.Next() {
		var std, alt string
		if err := rows.Scan(&std, &alt); err != nil {
			return nil, fmt.Errorf("scanning reporter alt abbreviation: %w", err)
		}
		m[std] = append(m[std], alt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating reporter alt abbreviations: %w", err)
	}
	return m, nil
}

// LoadCodeReporterCitations loads code_reporter into an in-memory map of
// citation -> id, keyed by both official_citation and the individual
// parallel_citation entries. parallel_citation is a "; "-separated list
// ("4 Sandf. 21; 6 N.Y. Super. Ct. 21"), so it is split on semicolons, with a
// trailing parenthetical year ("1 Code Rep. 91 (1848)") stripped. Commas are
// NOT split on: reporter names legitimately contain them ("Cox, Manual
// Trade-Mark Cas. 51"), so the few comma-joined pairs inside one segment stay
// as harmless keys no probe string will ever match. As with the CAP map, a
// citation is included only if it resolves unambiguously to a single row —
// several Code Reports pages carry many short decisions, so the same cite can
// belong to more than one id.
func (s *LinkerDBStore) LoadCodeReporterCitations(ctx context.Context) (map[string]int64, error) {
	query := `
	SELECT cite, min(id) AS id
	FROM (
		SELECT official_citation AS cite, id FROM legalhist.code_reporter
		UNION ALL
		SELECT regexp_replace(trim(seg), '\s*\(\d{4}\)$', ''), id
		FROM legalhist.code_reporter,
		     unnest(string_to_array(parallel_citation, ';')) AS seg
		WHERE parallel_citation IS NOT NULL
	) t
	WHERE cite <> ''
	GROUP BY cite
	HAVING count(DISTINCT id) = 1
	`
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

// LoadEnglishReportsCitations loads english_reports.cases into an in-memory map
// keyed by both er_cite (the E.R. reprint cite, "84 E.R. 256") and
// er_parallel_cite (the nominate cite, "2 Keb 408"). The two keyspaces are
// disjoint — no string appears in both — so one map serves them without
// collision, and treatises overwhelmingly cite the nominate form.
//
// There is deliberately no HAVING clause here, which is where this diverges from
// LoadCAPCitations and LoadCodeReporterCitations: a cite belonging to more than
// one case is kept as a key and marked ambiguous instead of being dropped. The
// linking policy is the same — such a cite links to nothing, because assigning
// one of the cases arbitrarily is the bug in #256 — but keeping the key lets the
// cascade report uk_page_ambiguous rather than the much vaguer uk_page_absent,
// and keeps the volume in the tier index, where dropping it would understate how
// far a failed citation actually got.
//
// min(id) is a formality on the unambiguous branch: cases = 1 guarantees there
// is only one value to take.
func (s *LinkerDBStore) LoadEnglishReportsCitations(ctx context.Context) (map[string]ERCase, error) {
	query := `
	SELECT cite, min(id) AS id, count(DISTINCT id) AS cases
	FROM (
		SELECT er_cite AS cite, id FROM english_reports.cases
		UNION ALL
		SELECT er_parallel_cite, id FROM english_reports.cases
		WHERE er_parallel_cite IS NOT NULL
	) t
	WHERE cite <> ''
	GROUP BY cite
	`
	rows, err := s.DB.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("loading English Reports citations: %w", err)
	}
	defer rows.Close()

	m := make(map[string]ERCase)
	for rows.Next() {
		var cite, id string
		var cases int
		if err := rows.Scan(&cite, &id, &cases); err != nil {
			return nil, fmt.Errorf("scanning English Reports citation: %w", err)
		}
		if cases > 1 {
			m[cite] = ERCase{Ambiguous: true, Cases: cases}
			continue
		}
		m[cite] = ERCase{ID: id, Cases: 1}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating English Reports citations: %w", err)
	}
	return m, nil
}

// LoadCAPCaseSpans loads the first-page cites that the page-range index is built
// from, with each case's own page count.
//
// Only official and nominative cites are loaded. vendor and split cites are
// excluded because they are not "{volume} {reporter} {page}" at all — a vendor
// cite like "1996 WL 12345" parses as page 12345 and would wreck the volume it
// landed in. parallel cites are excluded on measurement rather than principle:
// they are safe (a reporter-volume key only ever receives pages in that
// reporter's own pagination) but add 1.28M cite strings for +1.5% of hits.
//
// Ambiguous cites are deliberately kept, unlike LoadCAPCitations; see the
// interface comment.
func (s *LinkerDBStore) LoadCAPCaseSpans(ctx context.Context) ([]CaseSpan[int64], error) {
	// The cite is parsed in Go rather than with regexp_match here: doing it
	// server-side over all 6.9M rows costs about nine minutes, against seconds for
	// the plain join.
	query := `
	SELECT c.cite, c."case", k.first_page, k.last_page
	FROM cap.citations c
	JOIN cap.cases k ON k.id = c."case"
	WHERE c.type IN ('official', 'nominative')
	`
	rows, err := s.DB.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("loading CAP case spans: %w", err)
	}
	defer rows.Close()

	spans := make([]CaseSpan[int64], 0, 7_000_000)
	for rows.Next() {
		var cite string
		var caseID int64
		var firstPage, lastPage *int
		if err := rows.Scan(&cite, &caseID, &firstPage, &lastPage); err != nil {
			return nil, fmt.Errorf("scanning CAP case span: %w", err)
		}
		spans = append(spans, CaseSpan[int64]{
			Cite:   cite,
			ID:     caseID,
			Length: pageLength(firstPage, lastPage),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating CAP case spans: %w", err)
	}
	return spans, nil
}

// pageLength converts a first/last page pair into the number of pages the case
// occupies, or 0 when either is NULL or the pair is inverted (167 rows in CAP
// have last_page < first_page). The +1 matters: CAP's ranges share their boundary
// page with the next case 72.8% of the time, so last-first+1 overshoots by one
// there and is exact under the other convention — taking the minimum with the
// distance to the next cite clamps it correctly either way.
func pageLength(firstPage, lastPage *int) int {
	if firstPage == nil || lastPage == nil || *lastPage < *firstPage {
		return 0
	}
	return *lastPage - *firstPage + 1
}

// LoadERCaseSpans loads the English Reports first-page cites, under both the E.R.
// keying and the nominate parallel keying. Each reporter-volume key receives only
// pages in its own pagination, so the two systems never mix.
func (s *LinkerDBStore) LoadERCaseSpans(ctx context.Context) ([]CaseSpan[string], error) {
	query := `SELECT id, er_cite, er_parallel_cite FROM english_reports.cases`
	rows, err := s.DB.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("loading English Reports case spans: %w", err)
	}
	defer rows.Close()

	spans := make([]CaseSpan[string], 0, 250_000)
	for rows.Next() {
		var id, erCite string
		var erParallel *string
		if err := rows.Scan(&id, &erCite, &erParallel); err != nil {
			return nil, fmt.Errorf("scanning English Reports case span: %w", err)
		}
		spans = append(spans, CaseSpan[string]{Cite: erCite, ID: id})
		if erParallel != nil {
			spans = append(spans, CaseSpan[string]{Cite: *erParallel, ID: id})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating English Reports case spans: %w", err)
	}
	return spans, nil
}

// SaveLinkResults batch-inserts multiple link results in a single statement.
//
// Rather than build a VALUES list with up to batchSize*9 placeholders (which
// runs into Postgres's 65535-parameter limit at large batch sizes and forces
// the server to parse a huge statement on every batch), it passes one array per
// column and expands them server-side with unnest(). That is a fixed 9-parameter
// statement regardless of batch size, so it parses/plans cheaply and keeps the
// wire payload compact. citation_id is sent as text[] and cast to uuid in SQL to
// avoid relying on driver-side uuid-array encoding.
//
// An empty MatchTier is sent as SQL NULL: the skip statuses reach no tier, and a
// NULL keeps them out of every tier aggregate instead of inventing a bucket for
// them.
func (s *LinkerDBStore) SaveLinkResults(ctx context.Context, results []*LinkResult) error {
	if len(results) == 0 {
		return nil
	}

	ids := make([]string, len(results))
	statuses := make([]string, len(results))
	tiers := make([]*string, len(results))
	capIDs := make([]*int64, len(results))
	codeIDs := make([]*int64, len(results))
	erIDs := make([]*string, len(results))
	cleaned := make([]*string, len(results))
	normalized := make([]*string, len(results))
	linked := make([]*string, len(results))
	for i, r := range results {
		ids[i] = r.CitationID.String()
		statuses[i] = r.Status
		if r.MatchTier != "" {
			tiers[i] = &r.MatchTier
		}
		capIDs[i] = r.CAPCaseID
		codeIDs[i] = r.CodeReporterID
		erIDs[i] = r.ERCaseID
		cleaned[i] = r.CiteCleaned
		normalized[i] = r.CiteNormalized
		linked[i] = r.CiteLinked
	}

	query := `
	INSERT INTO moml_citations.citation_links
		(citation_id, status, match_tier, cap_case_id, code_reporter_id, er_case_id, cite_cleaned, cite_normalized, cite_linked)
	SELECT u.citation_id::uuid, u.status, u.match_tier, u.cap_case_id, u.code_reporter_id, u.er_case_id, u.cite_cleaned, u.cite_normalized, u.cite_linked
	FROM unnest($1::text[], $2::text[], $3::text[], $4::bigint[], $5::bigint[], $6::text[], $7::text[], $8::text[], $9::text[])
		AS u(citation_id, status, match_tier, cap_case_id, code_reporter_id, er_case_id, cite_cleaned, cite_normalized, cite_linked)
	ON CONFLICT (citation_id) DO NOTHING`

	_, err := s.DB.Exec(ctx, query, ids, statuses, tiers, capIDs, codeIDs, erIDs, cleaned, normalized, linked)
	if err != nil {
		return fmt.Errorf("batch saving %d link results: %w", len(results), err)
	}
	return nil
}

// ResetUnlinked deletes every citation_links row that was not resolved to a case
// (every status in UnresolvedStatuses: no_match, skipped_not_whitelisted,
// skipped_junk, skipped_statute) so the linker re-processes them on the next
// run; only linked_* rows are preserved. Deleting the skip statuses too lets a
// re-run re-derive them from the current whitelist, so a reporter later
// corrected from junk to legit is no longer stuck as skipped_junk. The delete
// runs as a single statement — one all-or-nothing transaction — and returns the
// number of rows deleted.
func (s *LinkerDBStore) ResetUnlinked(ctx context.Context) (int64, error) {
	placeholders := make([]string, len(UnresolvedStatuses))
	args := make([]any, len(UnresolvedStatuses))
	for i, status := range UnresolvedStatuses {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = status
	}
	query := fmt.Sprintf(`
	DELETE FROM moml_citations.citation_links
	WHERE status IN (%s)
	`, strings.Join(placeholders, ", "))
	tag, err := s.DB.Exec(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("resetting unlinked citations: %w", err)
	}
	return tag.RowsAffected(), nil
}
