package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// The editions dashboard lists every edition in the treatise view with the
// citations each of the two methods found in it: the pipeline (the regex
// detector and the linker, from edition_citation_counts) and Claude reading
// the edition page by page (moml_citations.llm_pages and llm_links, issue
// #439). The LLM tables are small, so they are aggregated live.

// EditionListItem is one edition on the dashboard.
type EditionListItem struct {
	BiblioID     string
	WorkID       *int
	Title        string
	Author       *string
	Year         *int
	Jurisdiction string
	Pages        int

	// The pipeline: citations detected, those linked to a case, and the cases.
	Cites  int
	Linked int
	Cases  int

	// The LLM reading: pages with a record, citations found, those linked to
	// a case, and the cases.
	LLMPages  int
	LLMCites  int
	LLMLinked int
	LLMCases  int
}

func (e EditionListItem) URL() string          { return editionURL(e.BiblioID) }
func (e EditionListItem) HasPipeline() bool    { return e.Cites > 0 }
func (e EditionListItem) HasLLM() bool         { return e.LLMPages > 0 }
func (e EditionListItem) LinkedPct() string    { return pct(e.Linked, e.Cites) }
func (e EditionListItem) LLMLinkedPct() string { return pct(e.LLMLinked, e.LLMCites) }

// LLMComplete reports whether every page of the edition has been read.
func (e EditionListItem) LLMComplete() bool { return e.LLMPages > 0 && e.LLMPages >= e.Pages }

// EditionListSummary counts the editions in the treatise view by which
// method has citations for them.
type EditionListSummary struct {
	Editions int
	Pipeline int
	LLM      int
	Both     int
	Neither  int
}

// editionListSorts maps a sort key to its ORDER BY. Fixed literals, never
// user input. Every order but "llm" still breaks ties the same way.
var editionListSorts = map[string]string{
	"llm":   "(p.pages_read IS NOT NULL) DESC, coalesce(l.cites, 0) DESC, c.cites DESC, c.title, c.bibliographicid",
	"cites": "c.cites DESC, c.title, c.bibliographicid",
	"year":  "c.year NULLS LAST, c.title, c.bibliographicid",
	"title": "c.title, c.year NULLS LAST, c.bibliographicid",
}

func normalizeEditionListSort(sort string) string {
	if _, ok := editionListSorts[sort]; ok {
		return sort
	}
	return "llm"
}

// editionListShows maps the "show" filter to a condition on the joined row.
var editionListShows = map[string]string{
	"":         "true",
	"llm":      "p.pages_read IS NOT NULL",
	"pipeline": "c.cites > 0",
	"neither":  "(c.cites = 0 AND p.pages_read IS NULL)",
}

func normalizeEditionListShow(show string) string {
	if _, ok := editionListShows[show]; ok {
		return show
	}
	return ""
}

var editionListJurisdictions = map[string]string{
	"":   "true",
	"us": "c.jurisdiction = 'US'",
	"uk": "c.jurisdiction = 'UK'",
}

func normalizeEditionListJurisdiction(jur string) string {
	if _, ok := editionListJurisdictions[jur]; ok {
		return jur
	}
	return ""
}

// editionListCTEs aggregates the LLM reading by edition. An edition is its
// volumes, so a record counts toward the edition its volume belongs to.
const editionListCTEs = `
WITH llm_p AS (
    SELECT v.bibliographicid, count(*) AS pages_read
      FROM moml_citations.llm_pages r
      JOIN moml.volumes v USING (psmid)
     GROUP BY v.bibliographicid
), llm_l AS (
    SELECT v.bibliographicid,
           count(*) AS cites,
           count(*) FILTER (WHERE l.status <> 'no_match') AS linked,
           count(DISTINCT coalesce('c' || l.cap_case_id, 'e' || l.er_case_id,
                                   'r' || l.code_reporter_id, 's' || l.stub_cite)) AS cases
      FROM moml_citations.llm_links l
      JOIN moml.volumes v USING (psmid)
     GROUP BY v.bibliographicid
)`

const editionListPageSize = 100

// getEditionList returns a page of treatise editions with both methods'
// counts, filtered by a title or author substring, a jurisdiction and which
// method has citations, with the total that matched.
func (s *server) getEditionList(ctx context.Context, q, jur, show, sort string, limit, offset int) ([]EditionListItem, int, error) {
	slog.Debug("querying editions", "q", q, "jurisdiction", jur, "show", show, "sort", sort, "limit", limit, "offset", offset)
	query := editionListCTEs + `
	SELECT c.bibliographicid, c.work_id, c.title, c.author, c.year, c.jurisdiction, c.pages,
	       c.cites, c.linked, c.cases,
	       coalesce(p.pages_read, 0), coalesce(l.cites, 0), coalesce(l.linked, 0), coalesce(l.cases, 0),
	       count(*) OVER() AS total
	  FROM moml_citations.edition_citation_counts c
	  LEFT JOIN llm_p p USING (bibliographicid)
	  LEFT JOIN llm_l l USING (bibliographicid)
	 WHERE ($1 = '' OR c.title ILIKE '%' || $1 || '%' OR c.author ILIKE '%' || $1 || '%')
	   AND ` + editionListJurisdictions[normalizeEditionListJurisdiction(jur)] + `
	   AND ` + editionListShows[normalizeEditionListShow(show)] + `
	 ORDER BY ` + editionListSorts[normalizeEditionListSort(sort)] + `
	 LIMIT $2 OFFSET $3`
	total := 0
	items, err := collect(ctx, s.db, query, []any{q, limit, offset}, func(rows pgx.Rows) (EditionListItem, error) {
		var e EditionListItem
		err := rows.Scan(&e.BiblioID, &e.WorkID, &e.Title, &e.Author, &e.Year, &e.Jurisdiction, &e.Pages,
			&e.Cites, &e.Linked, &e.Cases, &e.LLMPages, &e.LLMCites, &e.LLMLinked, &e.LLMCases, &total)
		return e, err
	})
	if err != nil {
		return nil, 0, fmt.Errorf("editions: %w", err)
	}
	slog.Debug("fetched editions", "count", len(items), "total", total)
	return items, total, nil
}

// getEditionListSummary counts every edition in the treatise view by which
// method has citations for it, whatever the page's filters.
func (s *server) getEditionListSummary(ctx context.Context) (EditionListSummary, error) {
	var sum EditionListSummary
	err := s.db.QueryRow(ctx, editionListCTEs+`
	SELECT count(*),
	       count(*) FILTER (WHERE c.cites > 0),
	       count(*) FILTER (WHERE p.pages_read IS NOT NULL),
	       count(*) FILTER (WHERE c.cites > 0 AND p.pages_read IS NOT NULL),
	       count(*) FILTER (WHERE c.cites = 0 AND p.pages_read IS NULL)
	  FROM moml_citations.edition_citation_counts c
	  LEFT JOIN llm_p p USING (bibliographicid)`).Scan(&sum.Editions, &sum.Pipeline, &sum.LLM, &sum.Both, &sum.Neither)
	if err != nil {
		return sum, fmt.Errorf("editions summary: %w", err)
	}
	return sum, nil
}

// editionListPage is the data of the editions dashboard.
type editionListPage struct {
	Page
	Q       string
	Jur     string
	Show    string
	Sort    string
	Summary *EditionListSummary
	Items   []EditionListItem
	Nav     Pagination
}

func (s *server) handleEditionList(w http.ResponseWriter, r *http.Request) {
	data := editionListPage{
		Page: Page{Title: "Editions: pipeline and LLM citations", Section: "works"},
		Q:    strings.TrimSpace(r.URL.Query().Get("q")),
		Jur:  normalizeEditionListJurisdiction(r.URL.Query().Get("jur")),
		Show: normalizeEditionListShow(r.URL.Query().Get("show")),
		Sort: normalizeEditionListSort(r.URL.Query().Get("sort")),
	}
	page := parsePage(r.URL.Query().Get("page"))

	ctx, cancel := s.ctx(r, 60*time.Second)
	defer cancel()

	// Both queries read the same view and tables, so one notice covers them.
	const reads = "moml_citations.edition_citation_counts or moml_citations.llm_links"
	sum, err := s.getEditionListSummary(ctx)
	if err := optional(err, &data.Page, reads); err != nil {
		s.serverError(w, r, err, "summarizing editions")
		return
	}
	if err == nil {
		data.Summary = &sum
		items, total, err := s.getEditionList(ctx, data.Q, data.Jur, data.Show, data.Sort, editionListPageSize, (page-1)*editionListPageSize)
		if err := optional(err, &data.Page, reads); err != nil {
			s.serverError(w, r, err, "listing editions")
			return
		}
		data.Items = items
		data.Nav = paginate(r, page, editionListPageSize, len(items), total)
	}
	s.render(w, r, "editions.html", http.StatusOK, data)
}
