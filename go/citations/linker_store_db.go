package citations

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
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
		COALESCE(r.type = 'statute', false) AS statute,
		COALESCE(r.cited_by_year_from, 0) AS cited_by_year_from,
		w.fallback_standard,
		f.reporter_cap AS fallback_cap,
		COALESCE(f.jurisdiction LIKE 'uk%', false) AS fallback_uk,
		EXISTS (
			SELECT 1 FROM legalhist.reporters_diffvols d
			WHERE d.reporter_standard = w.fallback_standard
		) AS fallback_cap_different,
		COALESCE(f.single_vol, false) AS fallback_single_vol,
		COALESCE(f.cited_by_year_from, 0) AS fallback_cited_by_year_from,
		COALESCE(r.jurisdiction, '') AS jurisdiction,
		COALESCE(f.jurisdiction, '') AS fallback_jurisdiction
	FROM legalhist.whitelist w
	LEFT JOIN legalhist.reporters r ON r.reporter_standard = w.reporter_standard
	LEFT JOIN legalhist.reporters f ON f.reporter_standard = w.fallback_standard
	`
	rows, err := s.DB.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying reporter whitelist: %w", err)
	}
	defer rows.Close()

	whitelist := make(map[string]*WhitelistEntry)
	for rows.Next() {
		var found string
		var e, fb WhitelistEntry
		err := rows.Scan(&found, &e.ReporterStandard, &e.ReporterCAP, &e.Junk, &e.UK, &e.CAPDifferent, &e.SingleVol, &e.Statute, &e.CitedByYearFrom,
			&fb.ReporterStandard, &fb.ReporterCAP, &fb.UK, &fb.CAPDifferent, &fb.SingleVol, &fb.CitedByYearFrom,
			&e.Jurisdiction, &fb.Jurisdiction)
		if err != nil {
			return nil, fmt.Errorf("scanning reporter whitelist row: %w", err)
		}
		if fb.ReporterStandard != nil {
			e.Fallback = &fb
		}
		whitelist[found] = &e
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating reporter whitelist: %w", err)
	}
	return whitelist, nil
}

// GetDiffVols loads the volume mapping for reporters whose volumes are numbered
// differently in CAP.
//
// A row with a NULL vol is a single-volume reporter mapped wholesale onto one
// CAP volume -- "Bail. Eq." is volume 8 of "S.C. Eq." -- and there are 18 of
// them, 17 for reporters flagged single_vol. Dropping those rows was wrong in
// both directions: the mapping was unavailable, so every probe carried the
// untranslated volume, while GetReporterWhitelist's cap_different is a bare
// EXISTS over the same table and still saw the row, which put the reporter on
// the diffvols path and made diffvolsMissing suppress range matching too
// (issue #292).
//
// They are loaded under volume 1, which is the form volumeForms already probes
// for a single-volume reporter beside the bare one, so the translated cite is
// reached without any special case in the cascade.
func (s *LinkerDBStore) GetDiffVols(ctx context.Context) (map[string]map[int]*DiffVolEntry, error) {
	query := `
	SELECT reporter_standard, COALESCE(vol, 1) AS vol, cap_vol, cap_reporter
	FROM legalhist.reporters_diffvols
	WHERE reporter_standard IS NOT NULL
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

// LoadCAPSharedCitations loads the cites LoadCAPCitations drops: each cite that
// belongs to more than one CAP case, with the distinct cases it belongs to and
// how many pages each occupies, longest first and then by ID. Measured
// 2026-10-20, these sit behind 108,357 CAP-ledger misses that were reported as
// us_page_absent, and 304,943 MOML misses (2026-10-23), nearly all of them a
// case and one or more one-page memorandum decisions beginning on the same page
// (issue #359). The page count is CAP's own, last_page - first_page + 1, taken
// as 0 when either page is missing or the range is inverted, so a case CAP
// knows nothing about the length of ranks below every case it does.
func (s *LinkerDBStore) LoadCAPSharedCitations(ctx context.Context) (map[string][]SharedCase, error) {
	query := `
	WITH shared AS (
		SELECT DISTINCT cite, "case"
		FROM cap.citations
		WHERE cite IN (
			SELECT cite FROM cap.citations GROUP BY cite HAVING count(DISTINCT "case") > 1
		)
	),
	sized AS (
		SELECT sh.cite, sh."case" AS case_id,
		       CASE WHEN c.last_page >= c.first_page THEN c.last_page - c.first_page + 1 ELSE 0 END AS pages
		FROM shared sh
		LEFT JOIN cap.cases c ON c.id = sh."case"
	)
	SELECT cite,
	       array_agg(case_id ORDER BY pages DESC, case_id) AS case_ids,
	       array_agg(pages ORDER BY pages DESC, case_id) AS pages
	FROM sized
	GROUP BY cite
	`
	rows, err := s.DB.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("loading shared CAP citations: %w", err)
	}
	defer rows.Close()

	m := make(map[string][]SharedCase)
	for rows.Next() {
		var cite string
		var caseIDs []int64
		var pages []int32
		if err := rows.Scan(&cite, &caseIDs, &pages); err != nil {
			return nil, fmt.Errorf("scanning shared CAP citation: %w", err)
		}
		if len(pages) != len(caseIDs) {
			return nil, fmt.Errorf("scanning shared CAP citation %q: %d cases but %d page counts", cite, len(caseIDs), len(pages))
		}
		cases := make([]SharedCase, len(caseIDs))
		for i := range caseIDs {
			cases[i] = SharedCase{ID: caseIDs[i], Pages: int(pages[i])}
		}
		m[cite] = cases
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating shared CAP citations: %w", err)
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
// map of reporter_standard -> []alt_abbr. The linker probes the CAP and FreeLaw
// maps with each alternate spelling (keyed by the canonical reporter_standard,
// like the diffvols mapping) after the standard/reporter_cap forms miss.
//
// An alternate that is itself some reporter's reporter_standard is left out. A
// standard spelling always trumps an alternative one: the spelling belongs to
// the reporter whose standard it is, so no other reporter may probe under it,
// or the probe asks CAP about a different reporter and links to whatever sits
// at that volume and page there — American Decisions under "A.D.", the Pacific
// Reporter under "P.R." (75 such rows, which had made 112,179 links; issue
// #289). The rows stay in the table, and the NOT EXISTS here is the one the
// seed migrations apply to new rows, so this is a load-time rule rather than a
// data change. A reporter whose only alternate collides ("Am. Dec.") is absent
// from the map, exactly like a reporter with no alternates at all.
//
// The ORDER BY makes the probe order — and therefore which alt wins when more
// than one would hit — deterministic across runs; COLLATE "C" keeps that order
// byte-identical across environments regardless of locale. alt_abbr is NOT
// NULL, so there is no null filter: a null would fail Scan loudly rather than
// be skipped.
func (s *LinkerDBStore) LoadReporterAltAbbrs(ctx context.Context) (map[string][]string, error) {
	query := `
	SELECT ra.reporter_standard, ra.alt_abbr
	FROM legalhist.reporters_abbreviations ra
	WHERE NOT EXISTS (
		SELECT 1 FROM legalhist.reporters r
		WHERE r.reporter_standard = ra.alt_abbr
	)
	ORDER BY ra.reporter_standard COLLATE "C", ra.alt_abbr COLLATE "C"
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
// trailing parenthetical year ("1 Code Rep. 91 (1848)") stripped.
// official_citation is trimmed for the same reason the segments are: four rows
// carry a trailing space ("2 Code Rep. 4 ", "3 Code Rep. 21 " three times), and
// an untrimmed key can never equal a probe string (issue #292). Commas are
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
		SELECT btrim(official_citation) AS cite, id FROM legalhist.code_reporter
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

// LoadStubCases loads the cite strings of legalhist.stub_cases. The table is
// small -- about 225K rows at the default threshold of five -- and only its
// key is needed: the counts that justify a stub are for people and later
// passes, not for the probe.
func (s *LinkerDBStore) LoadStubCases(ctx context.Context) (map[string]struct{}, error) {
	rows, err := s.DB.Query(ctx, `SELECT cite FROM legalhist.stub_cases`)
	if err != nil {
		return nil, fmt.Errorf("loading stub cases: %w", err)
	}
	defer rows.Close()

	m := make(map[string]struct{})
	for rows.Next() {
		var cite string
		if err := rows.Scan(&cite); err != nil {
			return nil, fmt.Errorf("scanning stub case: %w", err)
		}
		m[cite] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating stub cases: %w", err)
	}
	return m, nil
}

// LoadCAPCaseYears loads cap.cases.decision_year by case id.
func (s *LinkerDBStore) LoadCAPCaseYears(ctx context.Context) (map[int64]int, error) {
	return loadYears[int64](ctx, s.DB, "CAP case years",
		`SELECT id, decision_year FROM cap.cases WHERE decision_year IS NOT NULL`, 7_000_000)
}

// LoadCodeReporterYears loads legalhist.code_reporter.decision_year by id.
func (s *LinkerDBStore) LoadCodeReporterYears(ctx context.Context) (map[int64]int, error) {
	return loadYears[int64](ctx, s.DB, "code reporter years",
		`SELECT id, decision_year FROM legalhist.code_reporter WHERE decision_year IS NOT NULL`, 1_000)
}

// LoadERCaseYears loads the year of each English Reports case by id. Murrell's
// year is preferred; er_year stands in for the cases Murrell does not date.
func (s *LinkerDBStore) LoadERCaseYears(ctx context.Context) (map[string]int, error) {
	return loadYears[string](ctx, s.DB, "English Reports case years",
		`SELECT id, coalesce(murrell_year, er_year) FROM english_reports.cases
		WHERE coalesce(murrell_year, er_year) IS NOT NULL`, 125_000)
}

// loadYears runs a query that returns (id, year) rows and collects them into a
// map. The queries leave out rows with no year, so a missing key means the year
// is unknown.
func loadYears[K comparable](ctx context.Context, db *pgxpool.Pool, what, query string, sizeHint int) (map[K]int, error) {
	rows, err := db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", what, err)
	}
	defer rows.Close()

	m := make(map[K]int, sizeHint)
	for rows.Next() {
		var id K
		var year int
		if err := rows.Scan(&id, &year); err != nil {
			return nil, fmt.Errorf("scanning %s: %w", what, err)
		}
		m[id] = year
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating %s: %w", what, err)
	}
	return m, nil
}
