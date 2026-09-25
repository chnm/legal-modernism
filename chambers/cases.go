package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v4"
)

// Cases are ranked by the treatise editions that cite them (case_edition_counts)
// and each one's page shows how it links out to editions: which cite it, how
// often, under what spellings, and through which tiers.

// CaseListItem is one case on the ranking.
type CaseListItem struct {
	CaseRef
	Editions       int
	EditionsDirect int // editions that reach the case other than only through pin cites
	Works          int
	Cites          int
}

// caseSorts maps a sort key to its ORDER BY over case_edition_counts. Fixed
// literals, never user input.
var caseSorts = map[string]string{
	"editions": "editions DESC, case_key",
	"direct":   "editions_not_pincite_only DESC, case_key",
	"works":    "works DESC, case_key",
	"cites":    "cites DESC, case_key",
}

func normalizeCaseSort(sort string, excludePincite bool) string {
	if _, ok := caseSorts[sort]; ok && sort != "direct" {
		return sort
	}
	if excludePincite {
		return "direct"
	}
	return "editions"
}

const casesPageSize = 100

const caseListColumns = `source, case_key, name, year, cite, editions, editions_not_pincite_only, works, cites`

func scanCaseListItem(rows pgx.Rows, total *int) (CaseListItem, error) {
	var c CaseListItem
	var key string
	dest := []any{&c.Source, &key, &c.Name, &c.Year, &c.Cite, &c.Editions, &c.EditionsDirect, &c.Works, &c.Cites}
	if total != nil {
		dest = append(dest, total)
	}
	err := rows.Scan(dest...)
	if _, id, ok := splitCaseKey(key); ok {
		c.ID = id
	}
	return c, err
}

// getTopCases returns a page of the ranking, optionally limited to one
// source. The total is not counted: a ranking's last page is not worth a scan
// of the whole view.
func (s *server) getTopCases(ctx context.Context, source, sort string, excludePincite bool, limit, offset int) ([]CaseListItem, error) {
	slog.Debug("querying top cases", "source", source, "sort", sort, "exclude_pincite", excludePincite, "limit", limit, "offset", offset)
	query := `
	SELECT ` + caseListColumns + `
	FROM moml_citations.case_edition_counts
	WHERE ($1 = '' OR source = $1) AND (NOT $2::bool OR editions_not_pincite_only > 0)
	ORDER BY ` + caseSorts[normalizeCaseSort(sort, excludePincite)] + `
	LIMIT $3 OFFSET $4`
	items, err := collect(ctx, s.db, query, []any{source, excludePincite, limit, offset}, func(rows pgx.Rows) (CaseListItem, error) {
		return scanCaseListItem(rows, nil)
	})
	if err != nil {
		return nil, fmt.Errorf("top cases: %w", err)
	}
	return items, nil
}

// reCiteLike matches what a citation string looks like: an optional bracketed
// year, an optional volume, a reporter, and a page. A query that matches is
// looked up as a cite first; if nothing has that cite, it is searched as a name.
var reCiteLike = regexp.MustCompile(`^(\[\d{4}\]\s*)?(\d+\s+)?[A-Za-z][A-Za-z.&'()\- ]*\s+\d+[a-z]?$`)

func looksLikeCite(q string) bool { return reCiteLike.MatchString(strings.TrimSpace(q)) }

// searchCases finds cases by an exact citation or by a name substring. An
// exact cite is looked up in every source's own citation columns (indexed) and
// in the ranking's denormalized cite, and the keys found are then fetched from
// the ranking by its unique key; a name goes through the trigram index on the
// ranking's name column. The keys are resolved first because an OR between a
// subquery and an equality made the planner scan the whole ranking.
func (s *server) searchCases(ctx context.Context, q, source string, limit, offset int) ([]CaseListItem, int, bool, error) {
	q = strings.TrimSpace(q)
	total := 0
	if looksLikeCite(q) {
		slog.Debug("searching cases by cite", "q", q)
		keys, err := collect(ctx, s.db, `
			SELECT 'cap:' || ci."case"::text FROM cap.citations ci WHERE ci.cite = $1
			UNION SELECT 'er:' || id FROM english_reports.cases WHERE er_cite = $1 OR er_parallel_cite = $1
			UNION SELECT 'code:' || id::text FROM legalhist.code_reporter WHERE official_citation = $1 OR parallel_citation = $1
			UNION SELECT 'stub:' || cite FROM legalhist.stub_cases WHERE cite = $1
			UNION SELECT case_key FROM moml_citations.case_edition_counts WHERE cite = $1`, []any{q},
			func(rows pgx.Rows) (string, error) {
				var k string
				return k, rows.Scan(&k)
			})
		if err != nil {
			return nil, 0, true, fmt.Errorf("searching cases by cite: %w", err)
		}
		if len(keys) > 0 {
			items, err := collect(ctx, s.db, `
				SELECT `+caseListColumns+`, count(*) OVER()
				FROM moml_citations.case_edition_counts
				WHERE case_key = ANY($1::text[]) AND ($2 = '' OR source = $2)
				ORDER BY editions DESC, case_key
				LIMIT $3 OFFSET $4`, []any{keys, source, limit, offset}, func(rows pgx.Rows) (CaseListItem, error) {
				return scanCaseListItem(rows, &total)
			})
			if err != nil {
				return nil, 0, true, fmt.Errorf("searching cases by cite: %w", err)
			}
			if len(items) > 0 {
				return items, total, true, nil
			}
		}
	}
	slog.Debug("searching cases by name", "q", q)
	query := `
	SELECT ` + caseListColumns + `, count(*) OVER()
	FROM moml_citations.case_edition_counts
	WHERE name ILIKE '%' || $1 || '%' AND ($2 = '' OR source = $2)
	ORDER BY editions DESC, case_key
	LIMIT $3 OFFSET $4`
	items, err := collect(ctx, s.db, query, []any{q, source, limit, offset}, func(rows pgx.Rows) (CaseListItem, error) {
		return scanCaseListItem(rows, &total)
	})
	if err != nil {
		return nil, 0, false, fmt.Errorf("searching cases by name: %w", err)
	}
	return items, total, false, nil
}

func (s *server) handleCases(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	source := r.URL.Query().Get("source")
	if !validCaseSource(source) {
		source = ""
	}
	excludePincite := r.URL.Query().Get("pincite") == "exclude"
	sort := normalizeCaseSort(r.URL.Query().Get("sort"), excludePincite)
	page := parsePage(r.URL.Query().Get("page"))

	ctx, cancel := s.ctx(r, 60*time.Second)
	defer cancel()

	data := struct {
		Page
		Q              string
		Source         string
		Sort           string
		ExcludePincite bool
		ByCite         bool
		Items          []CaseListItem
		Nav            Pagination
	}{Page: Page{Title: "Cases", Section: "cases"}, Q: q, Source: source, Sort: sort, ExcludePincite: excludePincite}

	var items []CaseListItem
	total := -1
	var err error
	if q != "" {
		items, total, data.ByCite, err = s.searchCases(ctx, q, source, casesPageSize, (page-1)*casesPageSize)
	} else {
		items, err = s.getTopCases(ctx, source, sort, excludePincite, casesPageSize, (page-1)*casesPageSize)
	}
	if err := optional(err, &data.Page, "moml_citations.case_edition_counts"); err != nil {
		s.serverError(w, r, err, "listing cases")
		return
	}
	data.Items = items
	data.Nav = paginate(r, page, casesPageSize, len(items), total)
	s.render(w, r, "cases.html", http.StatusOK, data)
}

// CaseDetail is the case page.
type CaseDetail struct {
	Page
	CaseRef
	FullName     *string
	Court        *string
	Jurisdiction *string
	Cites        []string // every citation string the source records
	ExternalURL  *string
	Reporter     *string // stub: the reporter standard of the cite
	VolYear      *int    // stub: the citation year that keys a year-cited reporter
	Pruned       bool    // stub: the stub is gone from legalhist.stub_cases but links remain
	Summary      *CaseSummary
	Decades      []decadeRow
	CitedAs      []CitedAs
	CitedAsTotal int
	NonBody      int // citations found on pages other than body pages
	Citing       []CitingEdition
	Nav          Pagination
	Sort         string
}

func (d *CaseDetail) ReporterURL() string {
	if d.Reporter == nil {
		return ""
	}
	return reporterURL(*d.Reporter)
}

func (d *CaseDetail) CitationsURL() string {
	return citationsURL(url.Values{"case": {d.Key()}})
}

// NonBodyPct is the share of the citations that came from index, table of
// contents, front-matter or back-matter pages, where tables of cases live.
func (d *CaseDetail) NonBodyPct() string { return pct(d.NonBody, d.CitedAsTotal) }

// CaseSummary is the case's row of case_edition_counts.
type CaseSummary struct {
	Editions       int
	EditionsDirect int
	Works          int
	Cites          int
	USEditions     int
	FirstCited     *int
	LastCited      *int
}

func (c CaseSummary) UKEditions() int { return c.Editions - c.USEditions }
func (c CaseSummary) Span() string    { return yearSpan(c.FirstCited, c.LastCited) }

// decadeRow is one bar of the case page's chart.
type decadeRow struct {
	Decade       int    `json:"decade"`
	Jurisdiction string `json:"jurisdiction"`
	Editions     int    `json:"editions"`
}

// CitedAs is one spelling under which the case was cited, with the tier the
// spelling linked through.
type CitedAs struct {
	Raw              string
	ReporterStandard *string
	Chip             Chip
	N                int
}

func (c CitedAs) ReporterURL() string {
	if c.ReporterStandard == nil {
		return ""
	}
	return reporterURL(*c.ReporterStandard)
}

// CitingEdition is one treatise edition that cites the case.
type CitingEdition struct {
	BiblioID     string
	Title        string
	Year         *int
	Jurisdiction string
	WorkID       int
	WorkTitle    string
	Cites        int
	PincitesOnly bool
	PagesURL     string
}

func (e CitingEdition) URL() string     { return editionURL(e.BiblioID) }
func (e CitingEdition) WorkURL() string { return workURL(e.WorkID) }

// getCaseMeta loads a case's record from its source. Returns false when the
// source has no such case.
func (s *server) getCaseMeta(ctx context.Context, d *CaseDetail) (bool, error) {
	var err error
	switch d.Source {
	case "cap":
		err = s.db.QueryRow(ctx, `
			SELECT cc.name_abbreviation, cc.name, cc.decision_year, ct.name, j.name_long, cc.frontend_url,
			       (SELECT array_agg(ci.cite ORDER BY (ci.type = 'official') DESC, ci.cite) FROM cap.citations ci WHERE ci."case" = cc.id)
			FROM cap.cases cc
			LEFT JOIN cap.courts ct ON ct.id = cc.court
			LEFT JOIN cap.jurisdictions j ON j.id = cc.jurisdiction
			WHERE cc.id = $1::bigint`, d.ID).Scan(
			&d.Name, &d.FullName, &d.Year, &d.Court, &d.Jurisdiction, &d.ExternalURL, &d.Cites)
	case "er":
		var parallel *string
		err = s.db.QueryRow(ctx, `
			SELECT COALESCE(murrell_title, er_name), er_name, COALESCE(murrell_year, er_year), court, er_url, er_cite, er_parallel_cite
			FROM english_reports.cases WHERE id = $1`, d.ID).Scan(
			&d.Name, &d.FullName, &d.Year, &d.Court, &d.ExternalURL, &d.Cite, &parallel)
		if err == nil {
			d.Cites = append(d.Cites, derefStr(d.Cite))
			if parallel != nil && *parallel != "" {
				d.Cites = append(d.Cites, *parallel)
			}
		}
	case "code":
		var parallel *string
		err = s.db.QueryRow(ctx, `
			SELECT name_abbreviation, name, decision_year, court_name, jurisdiction, official_citation, parallel_citation
			FROM legalhist.code_reporter WHERE id = $1::bigint`, d.ID).Scan(
			&d.Name, &d.FullName, &d.Year, &d.Court, &d.Jurisdiction, &d.Cite, &parallel)
		if err == nil {
			d.Cites = append(d.Cites, derefStr(d.Cite))
			if parallel != nil && strings.TrimSpace(*parallel) != "" {
				d.Cites = append(d.Cites, strings.TrimSpace(*parallel))
			}
		}
	case "stub":
		cite := d.ID
		d.Cite = &cite
		d.Cites = []string{cite}
		err = s.db.QueryRow(ctx, `
			SELECT st.reporter_standard, st.vol_year, sm.party_names, sm.year_decided, sm.jurisdiction
			FROM legalhist.stub_cases st
			LEFT JOIN legalhist.stub_case_metadata sm ON sm.cite = st.cite
			WHERE st.cite = $1`, d.ID).Scan(&d.Reporter, &d.VolYear, &d.Name, &d.Year, &d.Jurisdiction)
		if isNoRows(err) {
			// The stub was pruned by a later refresh; links to it remain until
			// the linker is rerun. Show the page if any do.
			var n int
			err = s.db.QueryRow(ctx, `SELECT count(*) FROM moml_citations.edition_case_citations WHERE stub_cite = $1`, d.ID).Scan(&n)
			if err == nil && n == 0 {
				return false, nil
			}
			d.Pruned = true
		}
	}
	if isNoRows(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("case record: %w", err)
	}
	return true, nil
}

// getCaseSummary reads the case's row of the ranking; nil when no treatise
// edition cites it.
func (s *server) getCaseSummary(ctx context.Context, key string) (*CaseSummary, error) {
	var c CaseSummary
	err := s.db.QueryRow(ctx, `
		SELECT editions, editions_not_pincite_only, works, cites, us_editions, first_cited, last_cited
		FROM moml_citations.case_edition_counts WHERE case_key = $1`, key).Scan(
		&c.Editions, &c.EditionsDirect, &c.Works, &c.Cites, &c.USEditions, &c.FirstCited, &c.LastCited)
	if isNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("case summary: %w", err)
	}
	return &c, nil
}

// citingSorts maps a sort key to its ORDER BY for the citing editions.
var citingSorts = map[string]string{
	"cites": "c.cite_count DESC, ec.year NULLS LAST, ec.title",
	"year":  "ec.year NULLS LAST, c.cite_count DESC, ec.title",
}

func normalizeCitingSort(sort string) string {
	if _, ok := citingSorts[sort]; ok {
		return sort
	}
	return "cites"
}

const citingPageSize = 100

// getCitingEditions lists the treatise editions that cite a case, a page at a
// time, with the total. The predicate is on the typed case column, so the
// partial index on it is used.
func (s *server) getCitingEditions(ctx context.Context, source, id, sort string, limit, offset int) ([]CitingEdition, int, error) {
	col, placeholder := caseIDColumn(source, 1)
	query := `
	SELECT c.bibliographicid, ec.title, ec.year, ec.jurisdiction, ec.work_id, w.title, c.cite_count, c.pincite_only,
	       count(*) OVER()
	FROM moml_citations.edition_case_citations c
	JOIN moml_citations.edition_citation_counts ec ON ec.bibliographicid = c.bibliographicid
	JOIN moml.works w ON w.work_id = ec.work_id
	WHERE c.` + col + ` = ` + placeholder + `
	ORDER BY ` + citingSorts[normalizeCitingSort(sort)] + `
	LIMIT $2 OFFSET $3`
	total := 0
	items, err := collect(ctx, s.db, query, []any{id, limit, offset}, func(rows pgx.Rows) (CitingEdition, error) {
		var e CitingEdition
		err := rows.Scan(&e.BiblioID, &e.Title, &e.Year, &e.Jurisdiction, &e.WorkID, &e.WorkTitle, &e.Cites, &e.PincitesOnly, &total)
		e.PagesURL = citationsURL(url.Values{"edition": {e.BiblioID}, "case": {source + ":" + id}})
		return e, err
	})
	if err != nil {
		return nil, 0, fmt.Errorf("citing editions: %w", err)
	}
	return items, total, nil
}

// getCaseDecades counts the citing treatise editions by decade and
// jurisdiction, for the chart.
func (s *server) getCaseDecades(ctx context.Context, source, id string) ([]decadeRow, error) {
	col, placeholder := caseIDColumn(source, 1)
	items, err := collect(ctx, s.db, `
		SELECT (ec.year / 10) * 10, ec.jurisdiction, count(*)
		FROM moml_citations.edition_case_citations c
		JOIN moml_citations.edition_citation_counts ec ON ec.bibliographicid = c.bibliographicid
		WHERE c.`+col+` = `+placeholder+` AND ec.year IS NOT NULL
		GROUP BY 1, 2 ORDER BY 1, 2`, []any{id},
		func(rows pgx.Rows) (decadeRow, error) {
			var d decadeRow
			return d, rows.Scan(&d.Decade, &d.Jurisdiction, &d.Editions)
		})
	if err != nil {
		return nil, fmt.Errorf("case decades: %w", err)
	}
	return items, nil
}

// getCitedAs groups the citations linked to a case by their raw spelling and
// tier, most frequent first, and counts how many came from pages other than
// body pages. Bound by the partial index on the case column; the most-cited
// case (44K links) takes under a second.
func (s *server) getCitedAs(ctx context.Context, source, id string, limit int) ([]CitedAs, int, int, error) {
	col, placeholder := caseIDColumn(source, 1)
	var total, nonBody int
	items, err := collect(ctx, s.db, `
		SELECT regexp_replace(cu.raw, '\s+', ' ', 'g'), wl.reporter_standard, cl.status, cl.match_tier, count(*),
		       sum(count(*)) OVER()::bigint,
		       sum(count(*) FILTER (WHERE mp.type IS DISTINCT FROM 'bodyPage')) OVER()::bigint
		FROM moml_citations.citation_links cl
		JOIN moml_citations.citations_unlinked cu ON cu.id = cl.citation_id
		LEFT JOIN legalhist.whitelist wl ON wl.reporter_found = cu.reporter_abbr
		LEFT JOIN moml.page mp ON mp.psmid = cu.moml_treatise AND mp.pageid = cu.moml_page
		WHERE cl.`+col+` = `+placeholder+`
		GROUP BY 1, 2, 3, 4
		ORDER BY count(*) DESC, 1
		LIMIT $2`, []any{id, limit},
		func(rows pgx.Rows) (CitedAs, error) {
			var c CitedAs
			var status, tier *string
			err := rows.Scan(&c.Raw, &c.ReporterStandard, &status, &tier, &c.N, &total, &nonBody)
			c.Chip = chipFor(status, tier)
			return c, err
		})
	if err != nil {
		return nil, 0, 0, fmt.Errorf("cited as: %w", err)
	}
	return items, total, nonBody, nil
}

func (s *server) handleCase(w http.ResponseWriter, r *http.Request) {
	source, id := r.PathValue("source"), r.PathValue("id")
	if !validCaseSource(source) || id == "" {
		s.notFound(w, r, "Cases are addressed as /cases/{cap|er|code|stub}/{id}.")
		return
	}
	sort := normalizeCitingSort(r.URL.Query().Get("sort"))
	page := parsePage(r.URL.Query().Get("page"))

	ctx, cancel := s.ctx(r, 60*time.Second)
	defer cancel()

	d := &CaseDetail{CaseRef: CaseRef{Source: source, ID: id}, Sort: sort}
	found, err := s.getCaseMeta(ctx, d)
	if err != nil {
		s.serverError(w, r, err, "loading the case", "source", source, "id", id)
		return
	}
	if !found {
		s.notFound(w, r, fmt.Sprintf("There is no %s case with the id %s.", d.SourceLabel(), id))
		return
	}
	d.Page = Page{
		Title:   d.Display(),
		Section: "cases",
		Crumbs:  []Crumb{{Label: "Cases", URL: "/cases"}, {Label: truncate(d.Display(), 60)}},
	}

	summary, err := s.getCaseSummary(ctx, d.Key())
	if err := optional(err, &d.Page, "moml_citations.case_edition_counts"); err != nil {
		s.serverError(w, r, err, "loading the case's summary", "source", source, "id", id)
		return
	}
	d.Summary = summary

	citing, total, err := s.getCitingEditions(ctx, source, id, sort, citingPageSize, (page-1)*citingPageSize)
	if err := optional(err, &d.Page, "moml_citations.edition_case_citations"); err != nil {
		s.serverError(w, r, err, "listing the case's citing editions", "source", source, "id", id)
		return
	}
	d.Citing = citing
	d.Nav = paginate(r, page, citingPageSize, len(citing), total)

	decades, err := s.getCaseDecades(ctx, source, id)
	if err := optional(err, &d.Page, "moml_citations.edition_citation_counts"); err != nil {
		s.serverError(w, r, err, "charting the case's citing editions", "source", source, "id", id)
		return
	}
	d.Decades = decades

	if d.CitedAs, d.CitedAsTotal, d.NonBody, err = s.getCitedAs(ctx, source, id, 30); err != nil {
		s.serverError(w, r, err, "grouping the case's spellings", "source", source, "id", id)
		return
	}

	s.render(w, r, "case.html", http.StatusOK, d)
}
