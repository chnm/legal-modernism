package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v4"
)

// Treatises are browsed from the work down: a work (moml.works) is a treatise
// across all its editions, and only the editions in the treatise view
// (moml.treatises) are listed and counted.

// WorkListItem is one work on the works list, with the totals of its treatise
// editions from work_citation_counts.
type WorkListItem struct {
	WorkID      int
	Title       string
	Author      *string
	Editions    int // treatise editions
	AllEditions int // every edition of the work in MOML
	Derivative  int
	FirstYear   *int
	LastYear    *int
	US          bool
	UK          bool
	Cites       int
	Linked      int
	Cases       int
}

func (w WorkListItem) URL() string { return workURL(w.WorkID) }

// Years is the span of the treatise editions' years.
func (w WorkListItem) Years() string { return yearSpan(w.FirstYear, w.LastYear) }

// Jurisdictions names the sides of the Atlantic the editions fall on.
func (w WorkListItem) Jurisdictions() string {
	switch {
	case w.US && w.UK:
		return "US, UK"
	case w.US:
		return "US"
	case w.UK:
		return "UK"
	}
	return ""
}

// LinkedPct is the share of the work's citations that linked to a case.
func (w WorkListItem) LinkedPct() string { return pct(w.Linked, w.Cites) }

// workSorts maps a sort key to its ORDER BY. Fixed literals, never user input.
var workSorts = map[string]string{
	"cites":    "c.cites DESC, w.title",
	"editions": "c.editions DESC, c.cites DESC, w.title",
	"cases":    "c.cases DESC, w.title",
	"earliest": "c.first_year NULLS LAST, w.title",
	"title":    "w.title, c.first_year",
}

func normalizeWorkSort(sort string) string {
	if _, ok := workSorts[sort]; ok {
		return sort
	}
	return "cites"
}

var workJurisdictions = map[string]string{
	"":     "true",
	"us":   "c.us",
	"uk":   "c.uk",
	"both": "(c.us AND c.uk)",
}

func normalizeWorkJurisdiction(jur string) string {
	if _, ok := workJurisdictions[jur]; ok {
		return jur
	}
	return ""
}

const worksPageSize = 100

// getWorks returns a page of works with a treatise edition, filtered by a
// title or author substring and a jurisdiction, with the total that matched.
func (s *server) getWorks(ctx context.Context, q, jur, sort string, limit, offset int) ([]WorkListItem, int, error) {
	slog.Debug("querying works", "q", q, "jurisdiction", jur, "sort", sort, "limit", limit, "offset", offset)
	query := `
	SELECT w.work_id, w.title, w.author,
	       c.editions, c.all_editions, c.derivative_editions, c.first_year, c.last_year, c.us, c.uk,
	       c.cites, c.linked, c.cases,
	       count(*) OVER() AS total
	FROM moml_citations.work_citation_counts c
	JOIN moml.works w ON w.work_id = c.work_id
	WHERE ($1 = '' OR w.title ILIKE '%' || $1 || '%' OR w.author ILIKE '%' || $1 || '%')
	  AND ` + workJurisdictions[normalizeWorkJurisdiction(jur)] + `
	ORDER BY ` + workSorts[normalizeWorkSort(sort)] + `
	LIMIT $2 OFFSET $3`
	total := 0
	items, err := collect(ctx, s.db, query, []any{q, limit, offset}, func(rows pgx.Rows) (WorkListItem, error) {
		var w WorkListItem
		err := rows.Scan(&w.WorkID, &w.Title, &w.Author, &w.Editions, &w.AllEditions, &w.Derivative,
			&w.FirstYear, &w.LastYear, &w.US, &w.UK, &w.Cites, &w.Linked, &w.Cases, &total)
		return w, err
	})
	if err != nil {
		return nil, 0, fmt.Errorf("works: %w", err)
	}
	slog.Debug("fetched works", "count", len(items), "total", total)
	return items, total, nil
}

// WorkEdition is one edition of a work on the work page. Jurisdiction is nil
// for an edition outside the treatise view; such editions are listed, muted,
// because a reader following a work wants to see what exists.
type WorkEdition struct {
	BiblioID         string
	Title            string
	Author           *string
	EditionStatement *string
	Imprint          *string
	Year             *int
	Volumes          int
	Pages            *int
	Jurisdiction     *string
	Derivative       bool
	Cites            int
	Linked           int
	Cases            int
	HasCounts        bool
}

func (e WorkEdition) URL() string       { return editionURL(e.BiblioID) }
func (e WorkEdition) InTreatises() bool { return e.Jurisdiction != nil }
func (e WorkEdition) LinkedPct() string { return pct(e.Linked, e.Cites) }
func (e WorkEdition) JurisdictionLabel() string {
	if e.Jurisdiction == nil {
		return "—"
	}
	return *e.Jurisdiction
}

// getWorkEditions lists every edition of a work, earliest first, with the
// treatise view's verdict on each (a per-id lookup of moml.treatises, which is
// fast) and, when withCounts, the counts from edition_citation_counts.
func (s *server) getWorkEditions(ctx context.Context, workID int, withCounts bool) ([]WorkEdition, error) {
	slog.Debug("querying work editions", "work_id", workID, "with_counts", withCounts)
	counts := "NULL::bigint, NULL::bigint, NULL::bigint"
	join := ""
	if withCounts {
		counts = "ec.cites, ec.linked, ec.cases"
		join = "LEFT JOIN moml_citations.edition_citation_counts ec ON ec.bibliographicid = e.bibliographicid"
	}
	query := `
	SELECT e.bibliographicid, v.title, e.author, v.edition_statement, v.imprint, v.year, v.volumes, v.pages,
	       (SELECT t.jurisdiction FROM moml.treatises t WHERE t.bibliographicid = e.bibliographicid),
	       e.derivative, ` + counts + `
	FROM moml.editions e
	JOIN LATERAL (
	  SELECT min(vv.year) AS year, count(*) AS volumes, sum(vv.total_pages)::bigint AS pages,
	         (array_agg(vv.display_title ORDER BY vv.current_volume, vv.psmid))[1] AS title,
	         (array_agg(vv.edition_statement ORDER BY vv.current_volume, vv.psmid))[1] AS edition_statement,
	         (array_agg(vv.imprint ORDER BY vv.current_volume, vv.psmid))[1] AS imprint
	  FROM moml.volumes vv WHERE vv.bibliographicid = e.bibliographicid
	) v ON true
	` + join + `
	WHERE e.work_id = $1
	ORDER BY v.year NULLS LAST, e.bibliographicid`
	items, err := collect(ctx, s.db, query, []any{workID}, func(rows pgx.Rows) (WorkEdition, error) {
		var e WorkEdition
		var cites, linked, cases *int
		err := rows.Scan(&e.BiblioID, &e.Title, &e.Author, &e.EditionStatement, &e.Imprint, &e.Year, &e.Volumes, &e.Pages,
			&e.Jurisdiction, &e.Derivative, &cites, &linked, &cases)
		if cites != nil {
			e.Cites, e.HasCounts = *cites, true
		}
		if linked != nil {
			e.Linked = *linked
		}
		if cases != nil {
			e.Cases = *cases
		}
		return e, err
	})
	if err != nil {
		return nil, fmt.Errorf("work editions: %w", err)
	}
	return items, nil
}

// TopCase is one row of a top-cases table: a case and how much the editions
// in question cite it.
type TopCase struct {
	CaseRef
	Editions     int  // citing editions, where the table spans several
	Cites        int  // citations behind the row
	PincitesOnly bool // every citation was a pin cite
	PagesURL     string
}

// TopCasesView is what the top-cases partial renders.
type TopCasesView struct {
	Rows          []TopCase
	ShowEditions  bool // the table spans several editions
	TotalEditions int  // "n of N" when the editions are a work's
	Empty         string
}

// getTopCasesForEditions ranks the cases cited by a set of editions by how
// many of them cite each and how often. The edition ids come from the caller
// (a work's treatise editions, or one edition), so the query is bound by the
// unique-index prefix on edition_case_citations.
func (s *server) getTopCasesForEditions(ctx context.Context, biblioIDs []string, excludePincite bool, limit int) ([]TopCase, error) {
	if len(biblioIDs) == 0 {
		return nil, nil
	}
	slog.Debug("querying top cases for editions", "editions", len(biblioIDs), "exclude_pincite", excludePincite, "limit", limit)
	query := `
	WITH a AS (
	  SELECT c.cap_case_id, c.er_case_id, c.code_reporter_id, c.stub_cite,
	         count(*) AS editions, sum(c.cite_count)::bigint AS cites, bool_and(c.pincite_only) AS pincite_only
	  FROM moml_citations.edition_case_citations c
	  WHERE c.bibliographicid = ANY($1::text[]) AND (NOT $2::bool OR NOT c.pincite_only)
	  GROUP BY 1, 2, 3, 4
	  ORDER BY editions DESC, cites DESC LIMIT $3
	)
	SELECT a.editions, a.cites, a.pincite_only, ` + caseMetaColumns("a") + `
	FROM a ` + caseMetaJoins("a") + `
	ORDER BY a.editions DESC, a.cites DESC`
	items, err := collect(ctx, s.db, query, []any{biblioIDs, excludePincite, limit}, func(rows pgx.Rows) (TopCase, error) {
		var t TopCase
		var source, id, name, cite *string
		var year *int
		err := rows.Scan(&t.Editions, &t.Cites, &t.PincitesOnly, &source, &id, &name, &year, &cite)
		if ref := caseRefFrom(source, id, name, year, cite); ref != nil {
			t.CaseRef = *ref
		}
		return t, err
	})
	if err != nil {
		return nil, fmt.Errorf("top cases: %w", err)
	}
	return items, nil
}

// WorkDetail is the work page.
type WorkDetail struct {
	Page
	WorkID         int
	Title          string
	Author         *string
	Counts         *WorkListItem
	Editions       []WorkEdition
	Outside        int // editions outside the treatise view
	TopCases       TopCasesView
	ExcludePincite bool
	Chart          []workChartRow
}

func (d WorkDetail) URL() string { return workURL(d.WorkID) }

// workChartRow is one edition on the work page's chart.
type workChartRow struct {
	Year  int    `json:"year"`
	Cites int    `json:"cites"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

func (s *server) handleWorks(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	jur := normalizeWorkJurisdiction(r.URL.Query().Get("jur"))
	sort := normalizeWorkSort(r.URL.Query().Get("sort"))
	page := parsePage(r.URL.Query().Get("page"))

	ctx, cancel := s.ctx(r, 60*time.Second)
	defer cancel()

	data := struct {
		Page
		Q     string
		Jur   string
		Sort  string
		Items []WorkListItem
		Nav   Pagination
	}{Page: Page{Title: "Treatises", Section: "works"}, Q: q, Jur: jur, Sort: sort}

	items, total, err := s.getWorks(ctx, q, jur, sort, worksPageSize, (page-1)*worksPageSize)
	if err := optional(err, &data.Page, "moml_citations.work_citation_counts"); err != nil {
		s.serverError(w, r, err, "listing works")
		return
	}
	data.Items = items
	data.Nav = paginate(r, page, worksPageSize, len(items), total)
	s.render(w, r, "works.html", http.StatusOK, data)
}

func (s *server) handleWork(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		s.notFound(w, r, "There is no work with that id.")
		return
	}
	excludePincite := r.URL.Query().Get("pincite") == "exclude"

	ctx, cancel := s.ctx(r, 60*time.Second)
	defer cancel()

	d := WorkDetail{WorkID: id, ExcludePincite: excludePincite}
	err = s.db.QueryRow(ctx, `SELECT title, author FROM moml.works WHERE work_id = $1`, id).Scan(&d.Title, &d.Author)
	if isNoRows(err) {
		s.notFound(w, r, "There is no work with that id.")
		return
	}
	if err != nil {
		s.serverError(w, r, err, "loading the work", "work_id", id)
		return
	}
	d.Page = Page{
		Title:   d.Title,
		Section: "works",
		Crumbs:  []Crumb{{Label: "Treatises", URL: "/works"}, {Label: truncate(d.Title, 60)}},
	}

	// The counts and the editions' counts read work_citation_counts and
	// edition_citation_counts; the editions list itself does not need them.
	counts, err := s.getWorkCounts(ctx, id)
	if err := optional(err, &d.Page, "moml_citations.work_citation_counts"); err != nil {
		s.serverError(w, r, err, "loading the work's counts", "work_id", id)
		return
	}
	d.Counts = counts

	editions, err := s.getWorkEditions(ctx, id, true)
	if isUnavailable(err) {
		d.Page.unavailable("moml_citations.edition_citation_counts")
		editions, err = s.getWorkEditions(ctx, id, false)
	}
	if err != nil {
		s.serverError(w, r, err, "listing the work's editions", "work_id", id)
		return
	}
	d.Editions = editions

	var treatiseIDs []string
	for _, e := range editions {
		if e.InTreatises() {
			treatiseIDs = append(treatiseIDs, e.BiblioID)
			if e.Year != nil && e.HasCounts {
				d.Chart = append(d.Chart, workChartRow{Year: *e.Year, Cites: e.Cites, Title: e.Title, URL: e.URL()})
			}
		} else {
			d.Outside++
		}
	}

	top, err := s.getTopCasesForEditions(ctx, treatiseIDs, excludePincite, 50)
	if err := optional(err, &d.Page, "moml_citations.edition_case_citations"); err != nil {
		s.serverError(w, r, err, "ranking the work's cases", "work_id", id)
		return
	}
	d.TopCases = TopCasesView{Rows: top, ShowEditions: true, TotalEditions: len(treatiseIDs),
		Empty: "No linked citations in this work's treatise editions."}

	s.render(w, r, "work.html", http.StatusOK, d)
}

// getWorkCounts reads a work's row of work_citation_counts; nil when the work
// has no treatise edition.
func (s *server) getWorkCounts(ctx context.Context, workID int) (*WorkListItem, error) {
	var c WorkListItem
	err := s.db.QueryRow(ctx, `
		SELECT c.work_id, w.title, w.author, c.editions, c.all_editions, c.derivative_editions,
		       c.first_year, c.last_year, c.us, c.uk, c.cites, c.linked, c.cases
		FROM moml_citations.work_citation_counts c
		JOIN moml.works w ON w.work_id = c.work_id
		WHERE c.work_id = $1`, workID).Scan(
		&c.WorkID, &c.Title, &c.Author, &c.Editions, &c.AllEditions, &c.Derivative,
		&c.FirstYear, &c.LastYear, &c.US, &c.UK, &c.Cites, &c.Linked, &c.Cases)
	if isNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("work counts: %w", err)
	}
	return &c, nil
}
