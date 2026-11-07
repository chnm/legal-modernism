package main

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"time"
)

// Draft public visualizations (VISUALIZATIONS.md): charts meant for readers
// outside the project, developed here before they move to the website. Each
// has a page at /viz/<name> that holds the title, source and notes, and an API
// at /api/viz/<name> that serves a plain JSON array the chart module under
// static/viz/ draws. The handlers read nothing but the API's view, so the page
// renders without the database and the script shows the 503 when the view is
// not populated yet. A visualization whose data is not in the database yet
// serves it from the embedded files of data/ instead (data.go).

// vizPage is the data of a visualization page.
type vizPage struct {
	Page
}

// slugPattern is the shape of a cap.jurisdictions slug, which keys the
// jurisdictions API: lower-case letters, digits and hyphens.
var slugPattern = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)

// handleVizJurisdictions renders the citations-between-jurisdictions chart
// (issue #216). The page is a shell; the script fetches the two APIs below.
func (s *server) handleVizJurisdictions(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "viz_jurisdictions.html", http.StatusOK, vizPage{Page{
		Title: "Citations between jurisdictions", Section: "viz",
	}})
}

// vizJurisdiction is one row of the jurisdictions list: a jurisdiction that
// cites or is cited in opinion_citations.jurisdiction_citations, with its
// totals in each direction and the span of its citing cases' years.
type vizJurisdiction struct {
	Jurisdiction   string `json:"jurisdiction"` // cap.jurisdictions.slug
	Name           string `json:"name"`         // the abbreviation, N.Y.
	NameLong       string `json:"name_long"`    // New York
	Citations      int64  `json:"citations"`    // citations from its cases
	CitingCases    int64  `json:"citing_cases"` // its cases that cite
	CitedCitations int64  `json:"cited_citations"`
	MinYear        *int   `json:"min_year"` // of its citing cases; null when it only is cited
	MaxYear        *int   `json:"max_year"`
}

const vizJurisdictionsSQL = `
WITH citing AS (
    SELECT citing_jurisdiction AS slug,
           sum(citations) AS citations,
           sum(citing_cases) AS citing_cases,
           min(citing_year) AS min_year,
           max(citing_year) AS max_year
      FROM opinion_citations.jurisdiction_citations
     GROUP BY 1
), cited AS (
    SELECT cited_jurisdiction AS slug, sum(citations) AS citations
      FROM opinion_citations.jurisdiction_citations
     GROUP BY 1
)
SELECT j.slug, j.name, j.name_long,
       COALESCE(citing.citations, 0), COALESCE(citing.citing_cases, 0),
       COALESCE(cited.citations, 0),
       citing.min_year, citing.max_year
  FROM cap.jurisdictions j
  LEFT JOIN citing ON citing.slug = j.slug
  LEFT JOIN cited ON cited.slug = j.slug
 WHERE citing.slug IS NOT NULL OR cited.slug IS NOT NULL
 ORDER BY j.name`

func (s *server) getVizJurisdictions(ctx context.Context) ([]vizJurisdiction, error) {
	rows, err := s.db.Query(ctx, vizJurisdictionsSQL)
	if err != nil {
		return nil, fmt.Errorf("querying jurisdictions: %w", err)
	}
	defer rows.Close()
	out := []vizJurisdiction{}
	for rows.Next() {
		var j vizJurisdiction
		if err := rows.Scan(&j.Jurisdiction, &j.Name, &j.NameLong, &j.Citations, &j.CitingCases,
			&j.CitedCitations, &j.MinYear, &j.MaxYear); err != nil {
			return nil, fmt.Errorf("scanning jurisdiction: %w", err)
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// handleVizJurisdictionsAPI serves the list of jurisdictions, which fills the
// chart's menu and sets the year slider's range.
func (s *server) handleVizJurisdictionsAPI(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := s.ctx(r, 30*time.Second)
	defer cancel()
	data, err := s.getVizJurisdictions(ctx)
	if err != nil {
		apiError(w, err, "error querying jurisdictions", capCorpus)
		return
	}
	writeJSON(w, data)
}

// vizJurisdictionEdge is one row of opinion_citations.jurisdiction_citations:
// the citations from the cases of one jurisdiction decided in one year to the
// cases of another (or the same) jurisdiction, and the citing cases making
// them. A citation is one opinion citing one case, counted once however many
// strings cite it.
type vizJurisdictionEdge struct {
	CitingYear         int    `json:"citing_year"`
	CitingJurisdiction string `json:"citing_jurisdiction"`
	CitedJurisdiction  string `json:"cited_jurisdiction"`
	Citations          int64  `json:"citations"`
	CitingCases        int64  `json:"citing_cases"`
}

const vizJurisdictionEdgesSQL = `
SELECT citing_year, citing_jurisdiction, cited_jurisdiction, citations, citing_cases
  FROM opinion_citations.jurisdiction_citations
 WHERE citing_jurisdiction = $1 OR cited_jurisdiction = $1
 ORDER BY citing_year, citing_jurisdiction, cited_jurisdiction`

func (s *server) getVizJurisdictionEdges(ctx context.Context, slug string) ([]vizJurisdictionEdge, error) {
	rows, err := s.db.Query(ctx, vizJurisdictionEdgesSQL, slug)
	if err != nil {
		return nil, fmt.Errorf("querying jurisdiction edges: %w", err)
	}
	defer rows.Close()
	out := []vizJurisdictionEdge{}
	for rows.Next() {
		var e vizJurisdictionEdge
		if err := rows.Scan(&e.CitingYear, &e.CitingJurisdiction, &e.CitedJurisdiction, &e.Citations, &e.CitingCases); err != nil {
			return nil, fmt.Errorf("scanning jurisdiction edge: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// handleVizJurisdictionAPI serves every edge of one jurisdiction, citing and
// cited, year by year; the chart filters the years and picks the direction in
// the browser. An unknown slug is an empty array. The literal route
// /api/viz/jurisdictions/totals takes precedence over this pattern.
func (s *server) handleVizJurisdictionAPI(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if !slugPattern.MatchString(slug) {
		http.Error(w, "jurisdiction must be a cap.jurisdictions slug", http.StatusBadRequest)
		return
	}
	ctx, cancel := s.ctx(r, 30*time.Second)
	defer cancel()
	data, err := s.getVizJurisdictionEdges(ctx, slug)
	if err != nil {
		apiError(w, err, "error querying jurisdiction edges", capCorpus)
		return
	}
	writeJSON(w, data)
}

// vizJurisdictionTotal is one jurisdiction's citations in one year, in each
// direction, with its home citations left out: what it cites in other
// jurisdictions and what other jurisdictions cite in it. The share measure
// divides a bar by the other jurisdiction's total, so the browser fetches
// these once and takes the denominator for any years and direction itself.
type vizJurisdictionTotal struct {
	Year         int    `json:"year"`
	Jurisdiction string `json:"jurisdiction"`
	CitationsOut int64  `json:"citations_out"` // from its cases to other jurisdictions
	CitationsIn  int64  `json:"citations_in"`  // to its cases from other jurisdictions
}

const vizJurisdictionTotalsSQL = `
WITH outgoing AS (
    SELECT citing_year AS year, citing_jurisdiction AS slug, sum(citations) AS citations
      FROM opinion_citations.jurisdiction_citations
     WHERE citing_jurisdiction <> cited_jurisdiction
     GROUP BY 1, 2
), incoming AS (
    SELECT citing_year AS year, cited_jurisdiction AS slug, sum(citations) AS citations
      FROM opinion_citations.jurisdiction_citations
     WHERE citing_jurisdiction <> cited_jurisdiction
     GROUP BY 1, 2
)
SELECT COALESCE(o.year, i.year), COALESCE(o.slug, i.slug),
       COALESCE(o.citations, 0), COALESCE(i.citations, 0)
  FROM outgoing o
  FULL JOIN incoming i ON i.year = o.year AND i.slug = o.slug
 ORDER BY 1, 2`

func (s *server) getVizJurisdictionTotals(ctx context.Context) ([]vizJurisdictionTotal, error) {
	rows, err := s.db.Query(ctx, vizJurisdictionTotalsSQL)
	if err != nil {
		return nil, fmt.Errorf("querying jurisdiction totals: %w", err)
	}
	defer rows.Close()
	out := []vizJurisdictionTotal{}
	for rows.Next() {
		var t vizJurisdictionTotal
		if err := rows.Scan(&t.Year, &t.Jurisdiction, &t.CitationsOut, &t.CitationsIn); err != nil {
			return nil, fmt.Errorf("scanning jurisdiction total: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// handleVizJurisdictionTotalsAPI serves every jurisdiction's citations by
// year in each direction, the denominators of the share measure.
func (s *server) handleVizJurisdictionTotalsAPI(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := s.ctx(r, 30*time.Second)
	defer cancel()
	data, err := s.getVizJurisdictionTotals(ctx)
	if err != nil {
		apiError(w, err, "error querying jurisdiction totals", capCorpus)
		return
	}
	writeJSON(w, data)
}

// handleVizFieldCode renders the Field Code network (issue #117): figure 5 of
// Funk and Mullen, "The Spine of American Law" (AHR 2018), the codes of civil
// procedure of the nineteenth-century United States connected by the sections
// they borrowed from one another, as an interactive force layout. The page is
// a shell; the script fetches the two arrays embedded from data/, which
// scripts/field-code-network derives from the article's research compendium.
func (s *server) handleVizFieldCode(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "viz_field_code.html", http.StatusOK, vizPage{Page{
		Title: "The migration of the Field Code", Section: "viz",
	}})
}
